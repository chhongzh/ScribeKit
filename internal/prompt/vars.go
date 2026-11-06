package prompt

// NumberedHalf 是带编号的半句，编号与 textseg 的 halves 下标一致。
type NumberedHalf struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
}

type AnalyzerParagraphVars struct {
	ArticleTitle  string         `json:"article_title"`
	Synopsis      string         `json:"synopsis"`
	Genre         string         `json:"genre"`
	Subgenre      string         `json:"subgenre"`
	ParagraphKind string         `json:"paragraph_kind"`
	Halves        []NumberedHalf `json:"halves"`
	GuideNotes    []string       `json:"guide_notes"`
}

type ArticleParagraph struct {
	ID   int    `json:"id"`
	Kind string `json:"kind"`
	Text string `json:"text"`
}

type AnalyzerArticleVars struct {
	ArticleTitle string             `json:"article_title"`
	Synopsis     string             `json:"synopsis"`
	Genre        string             `json:"genre"`
	Subgenre     string             `json:"subgenre"`
	Paragraphs   []ArticleParagraph `json:"paragraphs"`
	GuideNotes   []string           `json:"guide_notes"`
}

type AnalyzerRepairVars struct {
	Error string `json:"error"`
}

// ScaffolderVars 里的文本都是用户原文或结构化事实，提示词措辞全部在模板里。
type ScaffolderVars struct {
	Mode           string   `json:"mode"`
	Downgraded     bool     `json:"downgraded"`
	Force          bool     `json:"force"`
	TargetKind     string   `json:"target_kind"`
	ParagraphKind  string   `json:"paragraph_kind"`
	NewParagraph   bool     `json:"new_paragraph"`
	NewKind        string   `json:"new_kind"`
	Title          string   `json:"title"`
	Synopsis       string   `json:"synopsis"`
	Genre          string   `json:"genre"`
	Subgenre       string   `json:"subgenre"`
	CollectionName string   `json:"collection_name"`
	Outline        string   `json:"outline"`
	Instruction    string   `json:"instruction"`
	HasCursor      bool     `json:"has_cursor"`
	Before         string   `json:"before"`
	After          string   `json:"after"`
	ParagraphText  string   `json:"paragraph_text"`
	Selection      string   `json:"selection"`
	PrevParagraph  string   `json:"prev_paragraph"`
	NextParagraph  string   `json:"next_paragraph"`
	GuideNotes     []string `json:"guide_notes"`
}
