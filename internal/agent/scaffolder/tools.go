package scaffolder

import (
	"context"
	"fmt"
	"sort"
	"unicode"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/chhongzh/scribekit/internal/prompt"
	"github.com/chhongzh/scribekit/internal/store"
)

type noArgs struct{}

type paragraphView struct {
	ID   int    `json:"id"`
	Kind string `json:"kind"`
	Text string `json:"text"`
}

type articleView struct {
	ID         int             `json:"id,omitempty"`
	Title      string          `json:"title"`
	Synopsis   string          `json:"synopsis"`
	Paragraphs []paragraphView `json:"paragraphs"`
	Error      string          `json:"error,omitempty"`
}

type siblingView struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Synopsis string `json:"synopsis"`
	Opening  string `json:"opening"`
}

type listCollectionResult struct {
	Articles []siblingView `json:"articles"`
}

type byIDArgs struct {
	ID int `json:"id"`
}

type searchArgs struct {
	Query string `json:"query"`
}

type snippet struct {
	Source string `json:"source"`
	Text   string `json:"text"`
}

type searchResult struct {
	Snippets []snippet `json:"snippets"`
}

type outlineResult struct {
	Outline string `json:"outline"`
}

type guideArgs struct {
	ID string `json:"id"`
}

type guideEntry struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type guideResult struct {
	Guides []guideEntry `json:"guides,omitempty"`
	Title  string       `json:"title,omitempty"`
	Notes  string       `json:"notes,omitempty"`
	Error  string       `json:"error,omitempty"`
}

const (
	openingRunes   = 80
	maxSnippets    = 5
	snippetRunes   = 320
	articleTextCap = 200000
)

func (s *Service) tools(art store.Article, collectionOutline string) ([]tool.BaseTool, error) {
	var out []tool.BaseTool
	add := func(t tool.InvokableTool, err error) error {
		if err != nil {
			return err
		}
		out = append(out, t)
		return nil
	}

	if err := add(utils.InferTool(prompt.ToolReadArticle, prompt.ToolDescription(prompt.ToolReadArticle),
		func(ctx context.Context, _ noArgs) (articleView, error) {
			return s.articleView(ctx, art.ID)
		})); err != nil {
		return nil, err
	}

	if err := add(utils.InferTool(prompt.ToolListCollection, prompt.ToolDescription(prompt.ToolListCollection),
		func(ctx context.Context, _ noArgs) (listCollectionResult, error) {
			res := listCollectionResult{Articles: []siblingView{}}
			if art.CollectionID == nil {
				return res, nil
			}
			sibs, err := s.store.ListArticles(ctx, store.ArticleFilter{CollectionID: art.CollectionID})
			if err != nil {
				return res, err
			}
			for _, sb := range sibs {
				if sb.ID == art.ID {
					continue
				}
				full, err := s.store.GetArticle(ctx, sb.ID)
				if err != nil {
					return res, err
				}
				opening := ""
				for _, p := range full.Paragraphs {
					if p.Text != "" {
						opening = truncate(p.Text, openingRunes)
						break
					}
				}
				res.Articles = append(res.Articles, siblingView{ID: sb.ID, Title: sb.Title, Synopsis: full.Synopsis, Opening: opening})
			}
			return res, nil
		})); err != nil {
		return nil, err
	}

	if err := add(utils.InferTool(prompt.ToolReadArticleByID, prompt.ToolDescription(prompt.ToolReadArticleByID),
		func(ctx context.Context, a byIDArgs) (articleView, error) {
			if art.CollectionID == nil {
				return articleView{Paragraphs: []paragraphView{}, Error: "当前文章不属于任何文集，没有可读的同文集文章"}, nil
			}
			other, err := s.store.GetArticleMeta(ctx, a.ID)
			if err != nil || other.CollectionID == nil || *other.CollectionID != *art.CollectionID {
				return articleView{Paragraphs: []paragraphView{}, Error: fmt.Sprintf("文章 %d 不在当前文集里", a.ID)}, nil
			}
			return s.articleView(ctx, a.ID)
		})); err != nil {
		return nil, err
	}

	if err := add(utils.InferTool(prompt.ToolSearchMaterials, prompt.ToolDescription(prompt.ToolSearchMaterials),
		func(ctx context.Context, a searchArgs) (searchResult, error) {
			mats, err := s.store.MaterialsVisibleTo(ctx, art.ID)
			if err != nil {
				return searchResult{}, err
			}
			return searchResult{Snippets: searchMaterials(mats, a.Query, maxSnippets)}, nil
		})); err != nil {
		return nil, err
	}

	if err := add(utils.InferTool(prompt.ToolReadOutline, prompt.ToolDescription(prompt.ToolReadOutline),
		func(ctx context.Context, _ noArgs) (outlineResult, error) {
			return outlineResult{Outline: collectionOutline}, nil
		})); err != nil {
		return nil, err
	}

	if err := add(utils.InferTool(prompt.ToolReadGuide, prompt.ToolDescription(prompt.ToolReadGuide),
		func(ctx context.Context, a guideArgs) (guideResult, error) {
			if a.ID == "" {
				var res guideResult
				for _, sm := range s.guides.List() {
					res.Guides = append(res.Guides, guideEntry{ID: sm.ID, Title: sm.Title})
				}
				return res, nil
			}
			g, ok := s.guides.Get(a.ID)
			if !ok {
				return guideResult{Error: fmt.Sprintf("没有编号为 %s 的指引", a.ID)}, nil
			}
			return guideResult{Title: g.Title, Notes: g.ScaffolderSection()}, nil
		})); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) articleView(ctx context.Context, id int) (articleView, error) {
	a, err := s.store.GetArticle(ctx, id)
	if err != nil {
		return articleView{}, err
	}
	v := articleView{ID: a.ID, Title: a.Title, Synopsis: a.Synopsis, Paragraphs: make([]paragraphView, 0, len(a.Paragraphs))}
	budget := articleTextCap
	for _, p := range a.Paragraphs {
		n := len([]rune(p.Text))
		if n > budget {
			break
		}
		budget -= n
		v.Paragraphs = append(v.Paragraphs, paragraphView{ID: p.ID, Kind: p.Kind, Text: p.Text})
	}
	return v, nil
}

func truncate(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n])
}

func gramSet(s string, n int) map[string]struct{} {
	var rs []rune
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			rs = append(rs, unicode.ToLower(r))
		}
	}
	set := map[string]struct{}{}
	for i := 0; i+n <= len(rs); i++ {
		set[string(rs[i:i+n])] = struct{}{}
	}
	return set
}

func chunks(content string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	for _, line := range splitLines(content) {
		rs := []rune(line)
		if len(rs) == 0 {
			flush()
			continue
		}
		for len(rs) > snippetRunes {
			flush()
			out = append(out, string(rs[:snippetRunes]))
			rs = rs[snippetRunes:]
		}
		if len(cur)+len(rs) > snippetRunes {
			flush()
		}
		cur = append(cur, rs...)
	}
	flush()
	return out
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, r := range s {
		if r == '\n' {
			lines = append(lines, trimCR(s[start:i]))
			start = i + 1
		}
	}
	return append(lines, trimCR(s[start:]))
}

func trimCR(s string) string {
	if n := len(s); n > 0 && s[n-1] == '\r' {
		return s[:n-1]
	}
	return s
}

// searchMaterials 按查询词与片段的 bigram 重叠数排序；查询不足两个字时退化为单字重叠。
func searchMaterials(mats []store.Material, query string, limit int) []snippet {
	n := 2
	q := gramSet(query, n)
	if len(q) == 0 {
		n = 1
		q = gramSet(query, n)
	}
	type hit struct {
		snippet
		score int
	}
	var hits []hit
	for _, m := range mats {
		for _, c := range chunks(m.Content) {
			cs := gramSet(c, n)
			score := 0
			for g := range q {
				if _, ok := cs[g]; ok {
					score++
				}
			}
			if score > 0 {
				hits = append(hits, hit{snippet{Source: m.Name, Text: c}, score})
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	out := []snippet{}
	for _, h := range hits[:min(limit, len(hits))] {
		out = append(out, h.snippet)
	}
	return out
}
