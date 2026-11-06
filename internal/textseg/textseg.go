// Package textseg 把段落文本切成句子与半句，偏移量单位为 Unicode 码点下标。
// 段内换行结束当前半句。行为必须与前端 web/src/core/textseg.ts 一致，规则见 docs/SPEC.md §3。
package textseg

import "unicode"

type Half struct {
	Index    int `json:"index"`
	Start    int `json:"start"`
	End      int `json:"end"`
	Sentence int `json:"sentence"`
}

type Sentence struct {
	Index  int   `json:"index"`
	Start  int   `json:"start"`
	End    int   `json:"end"`
	Halves []int `json:"halves"`
}

type Segmentation struct {
	Sentences []Sentence `json:"sentences"`
	Halves    []Half     `json:"halves"`
}

func isSentenceEnd(r rune) bool {
	switch r {
	case '。', '！', '？', '!', '?', '…':
		return true
	}
	return false
}

func isHalfDelim(r rune) bool {
	switch r {
	case '，', '；', '：', ',', ';', ':':
		return true
	}
	return false
}

func isCloser(r rune) bool {
	switch r {
	case '”', '’', '」', '』', '）', ')', '》', '】', '"', '\'':
		return true
	}
	return false
}

func isBlank(r rune) bool {
	return r == ' ' || r == '　' || r == '\t' || r == '\n' || r == '\r'
}

func hasContent(rs []rune) bool {
	for _, r := range rs {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
}

type piece struct {
	start, end int
	sentEnd    bool
}

// Segment 按 §3 切分。空文本返回空切片而非 nil，方便直接序列化。
func Segment(text string) Segmentation {
	return SegmentRunes([]rune(text))
}

func SegmentRunes(rs []rune) Segmentation {
	var pieces []piece
	flush := func(start, end int, sentEnd bool) {
		if start >= end {
			return
		}
		body := rs[start:end]
		if hasContent(body) {
			pieces = append(pieces, piece{start, end, sentEnd})
			return
		}
		allBlank := true
		for _, r := range body {
			if !isBlank(r) {
				allBlank = false
				break
			}
		}
		if allBlank {
			return
		}
		if n := len(pieces); n > 0 {
			pieces[n-1].end = end
			pieces[n-1].sentEnd = pieces[n-1].sentEnd || sentEnd
			return
		}
		pieces = append(pieces, piece{start, end, sentEnd})
	}

	n := len(rs)
	i := 0
	for i < n {
		for i < n && isBlank(rs[i]) {
			i++
		}
		start := i
		for i < n && !isSentenceEnd(rs[i]) && !isHalfDelim(rs[i]) && rs[i] != '\n' {
			i++
		}
		if i >= n {
			flush(start, n, false)
			break
		}
		if rs[i] == '\n' {
			// 换行只结束半句，不结束句子；换行符本身不属于任何片段。
			flush(start, i, false)
			i++
			continue
		}
		sentEnd := isSentenceEnd(rs[i])
		for i < n && (isSentenceEnd(rs[i]) || isHalfDelim(rs[i])) {
			if isSentenceEnd(rs[i]) {
				sentEnd = true
			}
			i++
		}
		for i < n && isCloser(rs[i]) {
			i++
		}
		flush(start, i, sentEnd)
	}

	seg := Segmentation{Sentences: []Sentence{}, Halves: []Half{}}
	cur := -1
	for idx, p := range pieces {
		if cur < 0 {
			seg.Sentences = append(seg.Sentences, Sentence{Index: len(seg.Sentences), Start: p.start})
			cur = len(seg.Sentences) - 1
		}
		s := &seg.Sentences[cur]
		s.Halves = append(s.Halves, idx)
		s.End = p.end
		seg.Halves = append(seg.Halves, Half{Index: idx, Start: p.start, End: p.end, Sentence: cur})
		if p.sentEnd {
			cur = -1
		}
	}
	return seg
}
