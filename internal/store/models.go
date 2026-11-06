package store

import (
	"bytes"
	"encoding/json"
	"time"
)

// Patch 区分“字段缺省”和“显式 null”，PATCH 里 collectionId: null 表示移出文集。
type Patch[T any] struct {
	Set bool
	Val *T
}

func (p *Patch[T]) UnmarshalJSON(b []byte) error {
	p.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		p.Val = nil
		return nil
	}
	p.Val = new(T)
	return json.Unmarshal(b, p.Val)
}

type Collection struct {
	ID           int       `json:"id"`
	Name         string    `json:"name"`
	Outline      string    `json:"outline"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	ArticleCount int       `json:"articleCount"`
}

type CollectionDetail struct {
	Collection
	Articles []ArticleSummary `json:"articles"`
}

type CollectionInput struct {
	Name    string `json:"name"`
	Outline string `json:"outline"`
}

type CollectionPatch struct {
	Name    *string `json:"name"`
	Outline *string `json:"outline"`
}

type ArticleSummary struct {
	ID           int       `json:"id"`
	CollectionID *int      `json:"collectionId"`
	Title        string    `json:"title"`
	Genre        string    `json:"genre"`
	Subgenre     string    `json:"subgenre"`
	UpdatedAt    time.Time `json:"updatedAt"`
	CharCount    int       `json:"charCount"`
}

type Article struct {
	ID           int       `json:"id"`
	CollectionID *int      `json:"collectionId"`
	Title        string    `json:"title"`
	Synopsis     string    `json:"synopsis"`
	Genre        string    `json:"genre"`
	Subgenre     string    `json:"subgenre"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type ArticleDetail struct {
	Article
	Paragraphs []Paragraph `json:"paragraphs"`
}

type ArticleInput struct {
	Title        string `json:"title"`
	Synopsis     string `json:"synopsis"`
	Genre        string `json:"genre"`
	Subgenre     string `json:"subgenre"`
	CollectionID *int   `json:"collectionId"`
}

type ArticlePatch struct {
	Title        *string    `json:"title"`
	Synopsis     *string    `json:"synopsis"`
	Genre        *string    `json:"genre"`
	Subgenre     *string    `json:"subgenre"`
	CollectionID Patch[int] `json:"collectionId"`
}

type ArticleFilter struct {
	CollectionID *int
	LooseOnly    bool
}

type Paragraph struct {
	ID        int       `json:"id"`
	ArticleID int       `json:"articleId"`
	Position  float64   `json:"position"`
	Kind      string    `json:"kind"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type ParagraphPatch struct {
	Text *string `json:"text"`
	Kind *string `json:"kind"`
}

type Material struct {
	ID           int       `json:"id"`
	CollectionID *int      `json:"collectionId"`
	ArticleID    *int      `json:"articleId"`
	Name         string    `json:"name"`
	Content      string    `json:"content"`
	CreatedAt    time.Time `json:"createdAt"`
}

type MaterialInput struct {
	Name         string `json:"name"`
	Content      string `json:"content"`
	CollectionID *int   `json:"collectionId"`
	ArticleID    *int   `json:"articleId"`
}

type MaterialFilter struct {
	CollectionID *int
	ArticleID    *int
}

type SuggestionHalf struct {
	Index  int     `json:"index"`
	Start  int     `json:"start"`
	End    int     `json:"end"`
	Text   string  `json:"text"`
	State  string  `json:"state"`
	Edited *string `json:"edited,omitempty"`
}

type Suggestion struct {
	ID            int              `json:"id"`
	ArticleID     int              `json:"articleId"`
	RequestID     string           `json:"requestId"`
	Op            string           `json:"op"`
	Target        json.RawMessage  `json:"target"`
	ParagraphKind string           `json:"paragraphKind"`
	Instruction   string           `json:"instruction"`
	Text          string           `json:"text"`
	Halves        []SuggestionHalf `json:"halves"`
	Rationale     string           `json:"rationale"`
	Status        string           `json:"status"`
	CreatedAt     time.Time        `json:"createdAt"`
}

type SuggestionInput struct {
	ArticleID     int
	RequestID     string
	Op            string
	Target        json.RawMessage
	ParagraphKind string
	Instruction   string
}

type SuggestionPatch struct {
	Halves *[]SuggestionHalf `json:"halves"`
	Status *string           `json:"status"`
}

// SuggestionTarget 是 Suggestion.Target 的两种形状，replace 用前三个字段，insert_paragraph 用最后一个。
type SuggestionTarget struct {
	ParagraphID      int  `json:"paragraphId"`
	Start            int  `json:"start"`
	End              int  `json:"end"`
	AfterParagraphID *int `json:"afterParagraphId"`
}

type Settings struct {
	Provider        string          `json:"provider"`
	BaseURL         string          `json:"baseUrl"`
	APIKey          string          `json:"apiKey"`
	AnalyzerModel   string          `json:"analyzerModel"`
	ScaffolderModel string          `json:"scaffolderModel"`
	AutoAnalyze     bool            `json:"autoAnalyze"`
	Theme           json.RawMessage `json:"theme"`
}

type ReplaceTarget struct {
	ParagraphID int `json:"paragraphId"`
	Start       int `json:"start"`
	End         int `json:"end"`
}

type InsertTarget struct {
	AfterParagraphID *int `json:"afterParagraphId"`
}
