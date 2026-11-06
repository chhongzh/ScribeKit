package analyzer

import "github.com/chhongzh/scribekit/internal/textseg"

type HalfInfo struct {
	Index    int      `json:"index"`
	Desc     []string `json:"desc"`
	Rhetoric []string `json:"rhetoric"`
}

type Relation struct {
	From int    `json:"from"`
	To   int    `json:"to"`
	Kind string `json:"kind"`
	Note string `json:"note,omitempty"`
}

type Span struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type Mark struct {
	Start int    `json:"start"`
	End   int    `json:"end"`
	Kind  string `json:"kind"`
	Note  string `json:"note,omitempty"`
	Ref   *Span  `json:"ref,omitempty"`
}

type Diagnostic struct {
	Start    int    `json:"start"`
	End      int    `json:"end"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type ParagraphAnalysis struct {
	ParagraphID  int                  `json:"paragraphId"`
	TextHash     string               `json:"textHash"`
	Segmentation textseg.Segmentation `json:"segmentation"`
	Halves       []HalfInfo           `json:"halves"`
	Relations    []Relation           `json:"relations"`
	Marks        []Mark               `json:"marks"`
	Diagnostics  []Diagnostic         `json:"diagnostics"`
	Model        string               `json:"model"`
}

type ThemeHit struct {
	ParagraphID int `json:"paragraphId"`
	Start       int `json:"start"`
	End         int `json:"end"`
}

type ParagraphRole struct {
	ParagraphID  int    `json:"paragraphId"`
	DetectedKind string `json:"detectedKind"`
	Role         string `json:"role"`
	Note         string `json:"note,omitempty"`
}

type Structure struct {
	Name string   `json:"name"`
	Fit  string   `json:"fit"`
	Gaps []string `json:"gaps"`
}

type ArticleAnalysis struct {
	ArticleID  int             `json:"articleId"`
	ThemeWords []string        `json:"themeWords"`
	ThemeHits  []ThemeHit      `json:"themeHits"`
	Paragraphs []ParagraphRole `json:"paragraphs"`
	Structure  Structure       `json:"structure"`
	Notes      []string        `json:"notes"`
}
