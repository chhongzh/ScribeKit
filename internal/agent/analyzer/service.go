// Package analyzer 是只读的分析 Agent：它只产出描写类型、逻辑关系、标记和诊断，从不产出可插入的文本。
package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/cloudwego/eino/schema"

	"github.com/chhongzh/scribekit/internal/agent"
	"github.com/chhongzh/scribekit/internal/apperr"
	"github.com/chhongzh/scribekit/internal/llm"
	"github.com/chhongzh/scribekit/internal/prompt"
	"github.com/chhongzh/scribekit/internal/store"
	"github.com/chhongzh/scribekit/internal/textseg"
)

// MaxConcurrent 是全局同时在途的模型请求上限。
const MaxConcurrent = 3

type Guides interface {
	AnalyzerNotes(genre string) []string
}

type Service struct {
	store  *store.Store
	models *llm.Manager
	guides Guides
	sem    chan struct{}
}

func New(st *store.Store, models *llm.Manager, guides Guides) *Service {
	return &Service{store: st, models: models, guides: guides, sem: make(chan struct{}, MaxConcurrent)}
}

func (s *Service) acquire(ctx context.Context) (release func(), err error) {
	select {
	case s.sem <- struct{}{}:
		return func() { <-s.sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ask 占用一个并发名额跑一次模型调用；输出解析失败时带着错误说明重试一次。
func ask[T any](ctx context.Context, s *Service, name string, h llm.Handle, msgs []*schema.Message, parse func(string) (T, error)) (T, error) {
	var zero T
	release, err := s.acquire(ctx)
	if err != nil {
		return zero, err
	}
	defer release()

	spec := agent.Spec{Name: name, Description: "只读文本分析", Model: h.Model}
	out, err := agent.Run(ctx, spec, msgs, nil)
	if err != nil {
		return zero, err
	}
	v, perr := parse(out)
	if perr == nil {
		return v, nil
	}
	repair, err := prompt.AnalyzerRepair(prompt.AnalyzerRepairVars{Error: perr.Error()})
	if err != nil {
		return zero, err
	}
	retry := append(append(slicesClone(msgs), schema.AssistantMessage(out, nil)), repair...)
	out, err = agent.Run(ctx, spec, retry, nil)
	if err != nil {
		return zero, err
	}
	v, perr = parse(out)
	if perr != nil {
		slog.Warn("analyzer output unusable after retry", "agent", name, "err", perr)
		return zero, apperr.New(apperr.Internal, "模型两次都没有返回可解析的 JSON，请稍后重试")
	}
	return v, nil
}

func slicesClone(in []*schema.Message) []*schema.Message {
	return append([]*schema.Message(nil), in...)
}

// Paragraphs 分析给定段落，每完成一个就调用 emit（可能并发调用）。命中缓存的段落不占用模型名额。
func (s *Service) Paragraphs(ctx context.Context, articleID int, ids []int, emit func(ParagraphAnalysis)) error {
	h, err := s.models.For(ctx, llm.Analyzer)
	if err != nil {
		return err
	}
	art, err := s.store.GetArticleMeta(ctx, articleID)
	if err != nil {
		return err
	}
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		firstEr error
	)
	for _, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := s.paragraph(ctx, art, id, h)
			if err != nil {
				mu.Lock()
				if firstEr == nil {
					firstEr = err
				}
				mu.Unlock()
				return
			}
			emit(res)
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	return firstEr
}

func (s *Service) paragraph(ctx context.Context, art store.Article, id int, h llm.Handle) (ParagraphAnalysis, error) {
	p, err := s.store.GetParagraph(ctx, id)
	if err != nil {
		return ParagraphAnalysis{}, err
	}
	if p.ArticleID != art.ID {
		return ParagraphAnalysis{}, apperr.New(apperr.BadRequest, "段落 %d 不属于文章 %d", id, art.ID)
	}
	hash := TextHash(p.Text)
	if cached, ok, err := s.store.CachedAnalysis(ctx, id, hash, h.Name); err != nil {
		return ParagraphAnalysis{}, err
	} else if ok {
		var res ParagraphAnalysis
		if err := json.Unmarshal(cached, &res); err == nil {
			return res, nil
		}
		slog.Warn("discarding unreadable analysis cache", "paragraph", id)
	}
	res, err := s.analyzeText(ctx, art, p.Kind, p.Text, id, h)
	if err != nil {
		return ParagraphAnalysis{}, err
	}
	if b, err := json.Marshal(res); err == nil {
		if err := s.store.PutAnalysis(ctx, id, hash, h.Name, b); err != nil {
			slog.Warn("cache analysis failed", "paragraph", id, "err", err)
		}
	}
	return res, nil
}

// Text 分析一段尚未落库的文本，结果的 paragraphId 为 0，不写缓存。
func (s *Service) Text(ctx context.Context, articleID int, paragraphKind, text string) (ParagraphAnalysis, error) {
	h, err := s.models.For(ctx, llm.Analyzer)
	if err != nil {
		return ParagraphAnalysis{}, err
	}
	art, err := s.store.GetArticleMeta(ctx, articleID)
	if err != nil {
		return ParagraphAnalysis{}, err
	}
	if paragraphKind == "" {
		paragraphKind = "normal"
	}
	if !store.ValidParagraphKind(paragraphKind) {
		return ParagraphAnalysis{}, apperr.New(apperr.BadRequest, "未知段落类型 %q", paragraphKind)
	}
	return s.analyzeText(ctx, art, paragraphKind, text, 0, h)
}

func (s *Service) analyzeText(ctx context.Context, art store.Article, kind, text string, paragraphID int, h llm.Handle) (ParagraphAnalysis, error) {
	rs := []rune(text)
	seg := textseg.SegmentRunes(rs)
	if len(seg.Halves) == 0 {
		return buildParagraph(paragraphID, text, h.Name, nil), nil
	}
	halves := make([]prompt.NumberedHalf, len(seg.Halves))
	for i, hf := range seg.Halves {
		halves[i] = prompt.NumberedHalf{Index: i, Text: string(rs[hf.Start:hf.End])}
	}
	msgs, err := prompt.AnalyzerParagraph(prompt.AnalyzerParagraphVars{
		ArticleTitle: art.Title, Synopsis: art.Synopsis, Genre: art.Genre, Subgenre: art.Subgenre,
		ParagraphKind: kind, Halves: halves, GuideNotes: s.guides.AnalyzerNotes(art.Genre),
	})
	if err != nil {
		return ParagraphAnalysis{}, err
	}
	raw, err := ask(ctx, s, "analyzer", h, msgs, parseParagraph)
	if err != nil {
		return ParagraphAnalysis{}, err
	}
	return buildParagraph(paragraphID, text, h.Name, raw), nil
}

// Article 做文章级的结构分析。
func (s *Service) Article(ctx context.Context, articleID int) (ArticleAnalysis, error) {
	h, err := s.models.For(ctx, llm.Analyzer)
	if err != nil {
		return ArticleAnalysis{}, err
	}
	art, err := s.store.GetArticle(ctx, articleID)
	if err != nil {
		return ArticleAnalysis{}, err
	}
	paras := make([]prompt.ArticleParagraph, 0, len(art.Paragraphs))
	for _, p := range art.Paragraphs {
		paras = append(paras, prompt.ArticleParagraph{ID: p.ID, Kind: p.Kind, Text: p.Text})
	}
	if len(paras) == 0 {
		return buildArticle(articleID, nil, nil), nil
	}
	msgs, err := prompt.AnalyzerArticle(prompt.AnalyzerArticleVars{
		ArticleTitle: art.Title, Synopsis: art.Synopsis, Genre: art.Genre, Subgenre: art.Subgenre,
		Paragraphs: paras, GuideNotes: s.guides.AnalyzerNotes(art.Genre),
	})
	if err != nil {
		return ArticleAnalysis{}, err
	}
	raw, err := ask(ctx, s, "article-analyzer", h, msgs, parseArticle)
	if err != nil {
		return ArticleAnalysis{}, fmt.Errorf("analyze article %d: %w", articleID, err)
	}
	return buildArticle(articleID, art.Paragraphs, raw), nil
}
