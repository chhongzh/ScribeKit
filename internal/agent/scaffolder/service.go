// Package scaffolder 是唯一产生写作建议的 Agent：生成、补全、润色、补细节和只读建议都经过这里。
package scaffolder

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/chhongzh/scribekit/internal/agent"
	"github.com/chhongzh/scribekit/internal/apperr"
	"github.com/chhongzh/scribekit/internal/guide"
	"github.com/chhongzh/scribekit/internal/llm"
	"github.com/chhongzh/scribekit/internal/logging"
	"github.com/chhongzh/scribekit/internal/prompt"
	"github.com/chhongzh/scribekit/internal/store"
	"github.com/chhongzh/scribekit/internal/textseg"
	"go.uber.org/zap"
)

const (
	ModeComplete = "complete"
	ModeGenerate = "generate"
	ModePolish   = "polish"
	ModeDetail   = "detail"
	ModeAdvise   = "advise"

	// ForceMinChars 是对转折段、Call Back 段强制生成时要求的用户正文字数。
	ForceMinChars = 300

	neighborRunes = 400
)

var modes = []string{ModeComplete, ModeGenerate, ModePolish, ModeDetail, ModeAdvise}

type Cursor struct {
	ParagraphID int `json:"paragraphId"`
	Offset      int `json:"offset"`
}

type Range struct {
	ParagraphID int `json:"paragraphId"`
	Start       int `json:"start"`
	End         int `json:"end"`
}

type Request struct {
	ArticleID        int     `json:"articleId"`
	Cursor           *Cursor `json:"cursor"`
	Selection        *Range  `json:"selection"`
	Instruction      string  `json:"instruction"`
	CommandRange     *Range  `json:"commandRange"`
	Mode             string  `json:"mode"`
	NewParagraphKind *string `json:"newParagraphKind"`
	Force            bool    `json:"force"`
}

type Events struct {
	Started func(suggestionID int)
	Delta   func(suggestionID int, text string)
}

type Service struct {
	store  *store.Store
	models *llm.Manager
	guides *guide.Library
}

func New(st *store.Store, models *llm.Manager, guides *guide.Library) *Service {
	return &Service{store: st, models: models, guides: guides}
}

// plan 是前置检查和上下文解析的结果，不含任何模型调用。
type plan struct {
	mode       string
	op         string
	target     json.RawMessage
	targetKind string
	vars       prompt.ScaffolderVars
}

func needsInput(format string, args ...any) error {
	return apperr.New(apperr.NeedsUserInput, format, args...)
}

func badRequest(format string, args ...any) error {
	return apperr.New(apperr.BadRequest, format, args...)
}

func runeSlice(s string, start, end int) string {
	rs := []rune(s)
	return string(rs[start:end])
}

func validRange(r *Range, length int) bool {
	return r.Start >= 0 && r.Start <= r.End && r.End <= length
}

// stripCommand 把 [ ... ] 命令自身从段落里去掉，并把光标位置换算到去掉之后的文本上。
func stripCommand(text string, cmd *Range, cursor int) (string, int) {
	if cmd == nil {
		return text, cursor
	}
	rs := []rune(text)
	n := cmd.End - cmd.Start
	switch {
	case cursor >= cmd.End:
		cursor -= n
	case cursor > cmd.Start:
		cursor = cmd.Start
	}
	return string(rs[:cmd.Start]) + string(rs[cmd.End:]), cursor
}

func totalChars(paras []store.Paragraph) int {
	n := 0
	for _, p := range paras {
		n += store.CountChars(p.Text)
	}
	return n
}

func (s *Service) plan(ctx context.Context, req Request) (plan, store.ArticleDetail, error) {
	var zero plan
	if !slices.Contains(modes, req.Mode) {
		return zero, store.ArticleDetail{}, badRequest("未知 mode %q", req.Mode)
	}
	art, err := s.store.GetArticle(ctx, req.ArticleID)
	if err != nil {
		return zero, art, err
	}
	if strings.TrimSpace(art.Title) == "" || strings.TrimSpace(art.Synopsis) == "" {
		return zero, art, needsInput("请先写好文章的标题和梗概：用户先动笔，AI 才有东西可以生成")
	}
	if req.NewParagraphKind != nil && !store.ValidParagraphKind(*req.NewParagraphKind) {
		return zero, art, badRequest("未知段落类型 %q", *req.NewParagraphKind)
	}

	byID := make(map[int]int, len(art.Paragraphs))
	for i, p := range art.Paragraphs {
		byID[p.ID] = i
	}
	lookup := func(id int) (int, error) {
		i, ok := byID[id]
		if !ok {
			return 0, badRequest("段落 %d 不属于文章 %d", id, art.ID)
		}
		return i, nil
	}

	p := plan{mode: req.Mode, op: "replace"}
	newParagraph := req.NewParagraphKind != nil && req.Mode == ModeGenerate

	// anchor 是上下文和落点所在的段落下标，-1 表示文章里还没有段落。
	anchor := -1
	var cursorOffset int
	hasCursor := false
	var sel *Range

	switch req.Mode {
	case ModeComplete:
		if req.Cursor == nil {
			return zero, art, needsInput("光标处没有可续写的文字")
		}
	case ModePolish, ModeDetail:
		if req.Selection == nil {
			return zero, art, needsInput("请先选中要处理的文字")
		}
	}
	if req.Mode == ModePolish || req.Mode == ModeDetail {
		sel = req.Selection
		i, err := lookup(sel.ParagraphID)
		if err != nil {
			return zero, art, err
		}
		anchor = i
	} else {
		switch {
		case req.Cursor != nil:
			i, err := lookup(req.Cursor.ParagraphID)
			if err != nil {
				return zero, art, err
			}
			anchor, cursorOffset, hasCursor = i, req.Cursor.Offset, true
		case req.Selection != nil:
			i, err := lookup(req.Selection.ParagraphID)
			if err != nil {
				return zero, art, err
			}
			anchor, sel = i, req.Selection
		case req.CommandRange != nil:
			i, err := lookup(req.CommandRange.ParagraphID)
			if err != nil {
				return zero, art, err
			}
			anchor = i
		case newParagraph && len(art.Paragraphs) > 0:
			anchor = len(art.Paragraphs) - 1
		}
	}

	var orig string
	if anchor >= 0 {
		orig = art.Paragraphs[anchor].Text
	}
	length := len([]rune(orig))
	if hasCursor && (cursorOffset < 0 || cursorOffset > length) {
		return zero, art, badRequest("光标位置 %d 超出段落长度 %d", cursorOffset, length)
	}
	if sel != nil && !validRange(sel, length) {
		return zero, art, badRequest("选区 [%d,%d) 超出段落长度 %d", sel.Start, sel.End, length)
	}
	var cmd *Range
	if req.CommandRange != nil && anchor >= 0 && req.CommandRange.ParagraphID == art.Paragraphs[anchor].ID {
		if !validRange(req.CommandRange, length) {
			return zero, art, badRequest("命令区间 [%d,%d) 超出段落长度 %d", req.CommandRange.Start, req.CommandRange.End, length)
		}
		cmd = req.CommandRange
	}
	stripped, strippedCursor := stripCommand(orig, cmd, cursorOffset)

	p.targetKind = "normal"
	switch {
	case newParagraph:
		p.targetKind = *req.NewParagraphKind
	case anchor >= 0:
		p.targetKind = art.Paragraphs[anchor].Kind
	}

	switch req.Mode {
	case ModeComplete:
		before := []rune(stripped)[:strippedCursor]
		if strings.TrimSpace(string(before)) == "" {
			return zero, art, needsInput("光标前没有用户文字可续写")
		}
	case ModePolish, ModeDetail:
		if strings.TrimSpace(runeSlice(orig, sel.Start, sel.End)) == "" {
			return zero, art, needsInput("选区里没有文字")
		}
	case ModeGenerate:
		if !newParagraph && !hasCursor && sel == nil && cmd == nil {
			return zero, art, badRequest("生成需要光标、选区、命令位置或新段落类型")
		}
	}

	downgraded, force := false, false
	if p.mode != ModeAdvise && (p.targetKind == "transition" || p.targetKind == "callback") {
		switch {
		case !req.Force:
			p.mode, downgraded = ModeAdvise, true
		case totalChars(art.Paragraphs) < ForceMinChars:
			return zero, art, apperr.New(apperr.UserOwnedParagraph,
				"转折段和 Call Back 段由用户亲自写；用户正文不足 %d 字时不能强制生成", ForceMinChars)
		default:
			force = true
		}
	}

	switch {
	case p.mode == ModeAdvise:
		p.op = "advice"
	case newParagraph:
		p.op = "insert_paragraph"
		var after *int
		if anchor >= 0 {
			id := art.Paragraphs[anchor].ID
			after = &id
		}
		p.target, err = json.Marshal(store.InsertTarget{AfterParagraphID: after})
	default:
		t := store.ReplaceTarget{ParagraphID: art.Paragraphs[anchor].ID}
		switch {
		case p.mode == ModePolish || p.mode == ModeDetail || (!hasCursor && sel != nil):
			t.Start, t.End = sel.Start, sel.End
		case cmd != nil:
			t.Start, t.End = cmd.Start, cmd.End
		default:
			t.Start, t.End = cursorOffset, cursorOffset
		}
		p.target, err = json.Marshal(t)
	}
	if err != nil {
		return zero, art, err
	}

	v := prompt.ScaffolderVars{
		Mode: p.mode, Downgraded: downgraded, Force: force,
		TargetKind: p.targetKind, ParagraphKind: p.targetKind,
		NewParagraph: p.op == "insert_paragraph", Title: art.Title, Synopsis: art.Synopsis,
		Genre: art.Genre, Subgenre: art.Subgenre, Instruction: strings.TrimSpace(req.Instruction),
		ParagraphText: stripped, GuideNotes: s.guides.ScaffolderNotes(art.Genre),
	}
	if req.NewParagraphKind != nil {
		v.NewKind = *req.NewParagraphKind
	}
	if anchor >= 0 {
		v.ParagraphKind = art.Paragraphs[anchor].Kind
	}
	if hasCursor && !v.NewParagraph && (p.mode == ModeComplete || p.mode == ModeGenerate) {
		rs := []rune(stripped)
		v.HasCursor = true
		v.Before, v.After = string(rs[:strippedCursor]), string(rs[strippedCursor:])
	}
	if sel != nil {
		v.Selection = runeSlice(orig, sel.Start, sel.End)
	}
	if anchor > 0 {
		v.PrevParagraph = tailRunes(art.Paragraphs[anchor-1].Text, neighborRunes)
	}
	if anchor >= 0 && anchor+1 < len(art.Paragraphs) {
		v.NextParagraph = truncate(art.Paragraphs[anchor+1].Text, neighborRunes)
	}
	p.vars = v
	return p, art, nil
}

func tailRunes(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[len(rs)-n:])
}

// Run 完成一次生成请求并把结果作为 pending 建议入库。取消或失败时不留下任何半成品。
func (s *Service) Run(ctx context.Context, requestID string, req Request, ev Events) (result store.Suggestion, err error) {
	ctx, finish := logging.Start(ctx, "scaffolder.run", zap.String("request_id", requestID), zap.Int("article_id", req.ArticleID), zap.String("requested_mode", req.Mode))
	defer func() { finish(err) }()
	h, err := s.models.For(ctx, llm.Scaffolder)
	if err != nil {
		return store.Suggestion{}, err
	}
	pl, art, err := s.plan(ctx, req)
	if err != nil {
		return store.Suggestion{}, err
	}
	logging.From(ctx).Info("scaffolder.plan.ready", zap.String("mode", pl.mode), zap.String("op", pl.op), zap.String("paragraph_kind", pl.targetKind), zap.Bool("downgraded", pl.mode != req.Mode))
	var outline, collectionName string
	if art.CollectionID != nil {
		c, err := s.store.GetCollection(ctx, *art.CollectionID)
		if err != nil {
			return store.Suggestion{}, err
		}
		outline, collectionName = c.Outline, c.Name
	}
	pl.vars.Outline, pl.vars.CollectionName = outline, collectionName

	msgs, err := prompt.Scaffolder(pl.vars)
	if err != nil {
		return store.Suggestion{}, err
	}
	tools, err := s.tools(art.Article, outline)
	if err != nil {
		return store.Suggestion{}, err
	}
	logging.From(ctx).Info("scaffolder.tools.ready", zap.Int("tool_count", len(tools)))

	paragraphKind := pl.targetKind
	if pl.op == "insert_paragraph" && req.NewParagraphKind != nil {
		paragraphKind = *req.NewParagraphKind
	}
	sg, err := s.store.CreateSuggestion(ctx, store.SuggestionInput{
		ArticleID: art.ID, RequestID: requestID, Op: pl.op, Target: pl.target,
		ParagraphKind: paragraphKind, Instruction: strings.TrimSpace(req.Instruction),
	})
	if err != nil {
		return store.Suggestion{}, err
	}
	discard := func() {
		if derr := s.store.DeleteSuggestion(context.WithoutCancel(ctx), sg.ID); derr != nil {
			logging.From(ctx).Warn("scaffolder.suggestion.discard.failed", zap.Int("suggestion_id", sg.ID), zap.Error(derr))
		} else {
			logging.From(ctx).Info("scaffolder.suggestion.discarded", zap.Int("suggestion_id", sg.ID))
		}
	}
	ctx = logging.With(ctx, zap.Int("suggestion_id", sg.ID))
	logging.From(ctx).Info("scaffolder.suggestion.created")
	if ev.Started != nil {
		ev.Started(sg.ID)
	}

	var onText func(string)
	if ev.Delta != nil {
		emit := func(t string) { ev.Delta(sg.ID, t) }
		if pl.op == "advice" {
			onText = emit
		} else {
			f := &deltaFilter{emit: emit}
			defer f.flush()
			onText = f.write
		}
	}
	final, err := agent.Run(ctx, agent.Spec{
		Name: "scaffolder", Description: "生成写作建议", Model: h.Model, Tools: tools, MaxIterations: 12,
	}, msgs, onText)
	if err != nil {
		discard()
		return store.Suggestion{}, err
	}

	text, rationale := "", ""
	if pl.op == "advice" {
		rationale = strings.TrimSpace(final)
	} else {
		text, rationale = splitRationale(final)
		if pl.mode == ModeComplete {
			text = firstSentence(text)
		}
	}
	if strings.TrimSpace(text) == "" && strings.TrimSpace(rationale) == "" {
		discard()
		return store.Suggestion{}, apperr.New(apperr.Internal, "模型没有返回内容，请重试")
	}
	if pl.op != "advice" && strings.TrimSpace(text) == "" {
		discard()
		return store.Suggestion{}, apperr.New(apperr.Internal, "模型只返回了说明，没有可插入的正文，请重试")
	}

	done, err := s.store.FinishSuggestion(context.WithoutCancel(ctx), sg.ID, text, rationale, halvesOf(text))
	if err != nil {
		discard()
		return store.Suggestion{}, err
	}
	logging.From(ctx).Info("scaffolder.suggestion.finished", zap.Int("output_bytes", len(text)), zap.Int("rationale_bytes", len(rationale)))
	return done, nil
}

// firstSentence 保证“补全”模式只续写当前句。
func firstSentence(text string) string {
	rs := []rune(text)
	seg := textseg.SegmentRunes(rs)
	if len(seg.Sentences) <= 1 {
		return text
	}
	return string(rs[:seg.Sentences[0].End])
}

func halvesOf(text string) []store.SuggestionHalf {
	rs := []rune(text)
	seg := textseg.SegmentRunes(rs)
	out := make([]store.SuggestionHalf, len(seg.Halves))
	for i, h := range seg.Halves {
		out[i] = store.SuggestionHalf{Index: i, Start: h.Start, End: h.End, Text: string(rs[h.Start:h.End]), State: "pending"}
	}
	return out
}
