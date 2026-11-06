// Package analyzer 是只读的分析 Agent：它只产出描写类型、逻辑关系、标记和诊断，从不产出可插入的文本。
package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"

	"github.com/chhongzh/scribekit/internal/agent"
	"github.com/chhongzh/scribekit/internal/apperr"
	"github.com/chhongzh/scribekit/internal/llm"
	"github.com/chhongzh/scribekit/internal/logging"
	"github.com/chhongzh/scribekit/internal/prompt"
	"github.com/chhongzh/scribekit/internal/store"
	"github.com/chhongzh/scribekit/internal/textseg"
	"go.uber.org/zap"
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
func ask[T any](ctx context.Context, s *Service, name string, h llm.Handle, msgs []*schema.Message, parse func(string) (T, error)) (result T, err error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	ctx, finish := logging.Start(ctx, "analyzer.ask", zap.String("agent", name), zap.String("model", h.Name))
	defer func() { finish(err) }()
	var zero T
	waitStarted := time.Now()
	release, err := s.acquire(ctx)
	if err != nil {
		return zero, err
	}
	defer release()
	logging.From(ctx).Info("analyzer.slot.acquired", zap.Duration("wait_duration", time.Since(waitStarted)))

	spec := agent.Spec{Name: name, Description: "只读文本分析", Model: h.Model}
	out, err := agent.Run(ctx, spec, msgs, nil)
	if err != nil {
		return zero, err
	}
	v, perr := parse(out)
	if perr == nil {
		return v, nil
	}
	logging.From(ctx).Warn("analyzer.parse.retry", zap.Error(perr), zap.Int("attempt", 1))
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
		logging.From(ctx).Warn("analyzer.parse.failed", zap.Error(perr), zap.Int("attempt", 2))
		return zero, apperr.New(apperr.Internal, "模型两次都没有返回可解析的 JSON，请稍后重试")
	}
	logging.From(ctx).Info("analyzer.parse.repaired", zap.Int("attempt", 2))
	return v, nil
}

func slicesClone(in []*schema.Message) []*schema.Message {
	return append([]*schema.Message(nil), in...)
}

// Paragraphs 分析给定段落，每完成一个就调用 emit（可能并发调用）。命中缓存的段落不占用模型名额。
func (s *Service) Paragraphs(ctx context.Context, articleID int, ids []int, emit func(ParagraphAnalysis)) (err error) {
	ctx, finish := logging.Start(ctx, "analyzer.paragraphs", zap.Int("article_id", articleID), zap.Int("paragraph_count", len(ids)))
	defer func() { finish(err) }()
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
			res, err := s.paragraph(ctx, art, id, h, emit)
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

func (s *Service) paragraph(ctx context.Context, art store.Article, id int, h llm.Handle, emit func(ParagraphAnalysis)) (result ParagraphAnalysis, err error) {
	ctx, finish := logging.Start(ctx, "analyzer.paragraph", zap.Int("paragraph_id", id))
	defer func() { finish(err) }()
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
			logging.From(ctx).Info("analyzer.cache.hit")
			return res, nil
		}
		logging.From(ctx).Warn("analyzer.cache.invalid")
	}
	logging.From(ctx).Info("analyzer.cache.miss")
	res, err := s.analyzeText(ctx, art, p.Kind, p.Text, id, h, emit)
	if err != nil {
		return ParagraphAnalysis{}, err
	}
	if b, err := json.Marshal(res); err == nil {
		if err := s.store.PutAnalysis(ctx, id, hash, h.Name, b); err != nil {
			logging.From(ctx).Warn("analyzer.cache.write.failed", zap.Error(err))
		} else {
			logging.From(ctx).Info("analyzer.cache.written")
		}
	} else {
		logging.From(ctx).Warn("analyzer.cache.encode.failed", zap.Error(err))
	}
	return res, nil
}

// Text 分析一段尚未落库的文本，结果的 paragraphId 为 0，不写缓存。
func (s *Service) Text(ctx context.Context, articleID int, paragraphKind, text string) (result ParagraphAnalysis, err error) {
	ctx, finish := logging.Start(ctx, "analyzer.text", zap.Int("article_id", articleID), zap.String("paragraph_kind", paragraphKind), zap.Int("input_bytes", len(text)))
	defer func() { finish(err) }()
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
	return s.analyzeText(ctx, art, paragraphKind, text, 0, h, nil)
}

func (s *Service) analyzeText(ctx context.Context, art store.Article, kind, text string, paragraphID int, h llm.Handle, onPartial func(ParagraphAnalysis)) (ParagraphAnalysis, error) {
	rs := []rune(text)
	seg := textseg.SegmentRunes(rs)
	if len(seg.Halves) == 0 {
		logging.From(ctx).Info("analyzer.empty_text.skipped", zap.Int("paragraph_id", paragraphID))
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
	raw, err := s.analyzeHalves(ctx, h, msgs, text, func(partial *rawParagraph) {
		if onPartial != nil {
			result := buildParagraph(paragraphID, text, h.Name, partial)
			result.Partial = true
			onPartial(result)
		}
	})
	if err != nil {
		return ParagraphAnalysis{}, err
	}
	return buildParagraph(paragraphID, text, h.Name, raw), nil
}

// Article 做文章级的结构分析。
func (s *Service) Article(ctx context.Context, articleID int) (result ArticleAnalysis, err error) {
	ctx, finish := logging.Start(ctx, "analyzer.article", zap.Int("article_id", articleID))
	defer func() { finish(err) }()
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
		logging.From(ctx).Info("analyzer.empty_article.skipped")
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
