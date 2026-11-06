package analyzer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/chhongzh/scribekit/internal/store"
	"github.com/chhongzh/scribekit/internal/textseg"
)

var (
	descKinds     = []string{"appearance", "speech", "action", "psychology", "expression", "environment", "narration", "argument"}
	rhetoricKinds = []string{"metaphor", "simile", "personification", "parallelism", "hyperbole", "rhetorical_question", "repetition", "contrast", "synesthesia", "quotation"}
	relationKinds = []string{"sequence", "cause", "result", "contrast", "concession", "progression", "parallel", "condition", "purpose", "explanation", "supplement", "summary"}
	markKinds     = []string{"theme", "keyword", "pronoun", "callback", "repetition", "claim", "evidence", "reasoning"}
	severities    = []string{"hint", "info", "warning"}
)

// TextHash 是 sha256(text) 十六进制的前 16 位，前端据此丢弃过期结果。
func TextHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:16]
}

var errBadOutput = errors.New("模型输出不是合法的 JSON")

// extractJSON 去掉代码块围栏和前后的闲话，取出最外层的 JSON 对象。
func extractJSON(s string) (string, error) {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return "", errBadOutput
	}
	return s[start : end+1], nil
}

type rawRef struct {
	Half int    `json:"half"`
	Text string `json:"text"`
}

type rawParagraph struct {
	Halves []struct {
		Index    int      `json:"index"`
		Desc     []string `json:"desc"`
		Rhetoric []string `json:"rhetoric"`
	} `json:"halves"`
	Relations []struct {
		From int    `json:"from"`
		To   int    `json:"to"`
		Kind string `json:"kind"`
		Note string `json:"note"`
	} `json:"relations"`
	Marks []struct {
		Half int     `json:"half"`
		Text string  `json:"text"`
		Nth  int     `json:"nth"`
		Kind string  `json:"kind"`
		Note string  `json:"note"`
		Ref  *rawRef `json:"ref"`
	} `json:"marks"`
	Diagnostics []struct {
		Half     int    `json:"half"`
		Text     string `json:"text"`
		Severity string `json:"severity"`
		Message  string `json:"message"`
	} `json:"diagnostics"`
}

func parseParagraph(output string) (*rawParagraph, error) {
	js, err := extractJSON(output)
	if err != nil {
		return nil, err
	}
	var raw rawParagraph
	if err := json.Unmarshal([]byte(js), &raw); err != nil {
		return nil, errors.Join(errBadOutput, err)
	}
	return &raw, nil
}

// filterEnum 保留合法枚举值并去重，永远返回非 nil 切片。
func filterEnum(in, allowed []string) []string {
	out := []string{}
	for _, v := range in {
		if slices.Contains(allowed, v) && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// locate 在 hay 中找第 nth（从 1 起）次出现的 needle，返回码点区间。
func locate(hay []rune, needle string, nth int) (start, end int, ok bool) {
	n := []rune(needle)
	if len(n) == 0 {
		return 0, 0, false
	}
	if nth < 1 {
		nth = 1
	}
	seen := 0
	for i := 0; i+len(n) <= len(hay); i++ {
		if slices.Equal(hay[i:i+len(n)], n) {
			seen++
			if seen == nth {
				return i, i + len(n), true
			}
		}
	}
	return 0, 0, false
}

// spanIn 先在指定半句里找子串，找不到再退回整段第一处；模型数错半句编号时仍能对上。
func spanIn(rs []rune, seg textseg.Segmentation, half int, text string, nth int) (int, int, bool) {
	if half >= 0 && half < len(seg.Halves) {
		h := seg.Halves[half]
		if s, e, ok := locate(rs[h.Start:h.End], text, nth); ok {
			return h.Start + s, h.Start + e, true
		}
	}
	return locate(rs, text, 1)
}

// buildParagraph 校验模型输出：枚举外的值和对不上原文的项直接丢弃，偏移量一律由后端按码点计算。
func buildParagraph(paragraphID int, text, model string, raw *rawParagraph) ParagraphAnalysis {
	rs := []rune(text)
	seg := textseg.SegmentRunes(rs)
	out := ParagraphAnalysis{
		ParagraphID:  paragraphID,
		TextHash:     TextHash(text),
		Segmentation: seg,
		Halves:       make([]HalfInfo, len(seg.Halves)),
		Relations:    []Relation{},
		Marks:        []Mark{},
		Diagnostics:  []Diagnostic{},
		Model:        model,
	}
	for i := range out.Halves {
		out.Halves[i] = HalfInfo{Index: i, Desc: []string{}, Rhetoric: []string{}}
	}
	if raw == nil {
		return out
	}
	inRange := func(i int) bool { return i >= 0 && i < len(seg.Halves) }

	for _, h := range raw.Halves {
		if inRange(h.Index) {
			out.Halves[h.Index].Desc = filterEnum(h.Desc, descKinds)
			out.Halves[h.Index].Rhetoric = filterEnum(h.Rhetoric, rhetoricKinds)
		}
	}
	for _, r := range raw.Relations {
		if !inRange(r.From) || !inRange(r.To) || r.From == r.To || !slices.Contains(relationKinds, r.Kind) {
			continue
		}
		out.Relations = append(out.Relations, Relation{From: r.From, To: r.To, Kind: r.Kind, Note: strings.TrimSpace(r.Note)})
	}
	for _, m := range raw.Marks {
		if !slices.Contains(markKinds, m.Kind) {
			continue
		}
		s, e, ok := spanIn(rs, seg, m.Half, m.Text, m.Nth)
		if !ok {
			continue
		}
		mark := Mark{Start: s, End: e, Kind: m.Kind, Note: strings.TrimSpace(m.Note)}
		if m.Ref != nil {
			if rs0, re0, ok := spanIn(rs, seg, m.Ref.Half, m.Ref.Text, 1); ok {
				mark.Ref = &Span{Start: rs0, End: re0}
			}
		}
		out.Marks = append(out.Marks, mark)
	}
	for _, d := range raw.Diagnostics {
		msg := strings.TrimSpace(d.Message)
		if msg == "" || !slices.Contains(severities, d.Severity) || !inRange(d.Half) {
			continue
		}
		h := seg.Halves[d.Half]
		start, end := h.Start, h.End
		if strings.TrimSpace(d.Text) != "" {
			if s, e, ok := locate(rs[h.Start:h.End], d.Text, 1); ok {
				start, end = h.Start+s, h.Start+e
			}
		}
		out.Diagnostics = append(out.Diagnostics, Diagnostic{Start: start, End: end, Severity: d.Severity, Message: msg})
	}
	return out
}

type rawArticle struct {
	ThemeWords []string `json:"themeWords"`
	Paragraphs []struct {
		ParagraphID  int    `json:"paragraphId"`
		DetectedKind string `json:"detectedKind"`
		Role         string `json:"role"`
		Note         string `json:"note"`
	} `json:"paragraphs"`
	Structure struct {
		Name string   `json:"name"`
		Fit  string   `json:"fit"`
		Gaps []string `json:"gaps"`
	} `json:"structure"`
	Notes []string `json:"notes"`
}

func parseArticle(output string) (*rawArticle, error) {
	js, err := extractJSON(output)
	if err != nil {
		return nil, err
	}
	var raw rawArticle
	if err := json.Unmarshal([]byte(js), &raw); err != nil {
		return nil, errors.Join(errBadOutput, err)
	}
	return &raw, nil
}

const maxThemeHits = 300

// buildArticle 的 themeHits 由后端在正文里直接查找点题词得到，不依赖模型报告位置。
func buildArticle(articleID int, paragraphs []store.Paragraph, raw *rawArticle) ArticleAnalysis {
	out := ArticleAnalysis{
		ArticleID:  articleID,
		ThemeWords: []string{},
		ThemeHits:  []ThemeHit{},
		Paragraphs: []ParagraphRole{},
		Structure:  Structure{Gaps: []string{}},
		Notes:      []string{},
	}
	if raw == nil {
		return out
	}
	known := make(map[int][]rune, len(paragraphs))
	for _, p := range paragraphs {
		known[p.ID] = []rune(p.Text)
	}
	for _, w := range raw.ThemeWords {
		w = strings.TrimSpace(w)
		if w == "" || slices.Contains(out.ThemeWords, w) {
			continue
		}
		found := false
		for _, p := range paragraphs {
			if strings.Contains(p.Text, w) {
				found = true
				break
			}
		}
		if found {
			out.ThemeWords = append(out.ThemeWords, w)
		}
	}
	for _, p := range paragraphs {
		rs := known[p.ID]
		for _, w := range out.ThemeWords {
			for nth := 1; len(out.ThemeHits) < maxThemeHits; nth++ {
				s, e, ok := locate(rs, w, nth)
				if !ok {
					break
				}
				out.ThemeHits = append(out.ThemeHits, ThemeHit{ParagraphID: p.ID, Start: s, End: e})
			}
		}
	}
	for _, p := range raw.Paragraphs {
		if _, ok := known[p.ParagraphID]; !ok || !store.ValidParagraphKind(p.DetectedKind) {
			continue
		}
		out.Paragraphs = append(out.Paragraphs, ParagraphRole{
			ParagraphID: p.ParagraphID, DetectedKind: p.DetectedKind,
			Role: strings.TrimSpace(p.Role), Note: strings.TrimSpace(p.Note),
		})
	}
	out.Structure.Name = strings.TrimSpace(raw.Structure.Name)
	out.Structure.Fit = strings.TrimSpace(raw.Structure.Fit)
	out.Structure.Gaps = cleanStrings(raw.Structure.Gaps)
	out.Notes = cleanStrings(raw.Notes)
	return out
}

func cleanStrings(in []string) []string {
	out := []string{}
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}
