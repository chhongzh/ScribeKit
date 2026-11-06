package store

import "slices"

var (
	Genres         = []string{"narrative", "argumentative", "prose", "poetry", "essay", "expository", "practical", "other"}
	ParagraphKinds = []string{"opening", "closing", "callback", "lyric", "transition", "normal"}
	SuggestionOps  = []string{"replace", "insert_paragraph", "advice"}
	statuses       = []string{"streaming", "pending", "applied", "rejected"}
)

func ValidGenre(s string) bool         { return slices.Contains(Genres, s) }
func ValidParagraphKind(s string) bool { return slices.Contains(ParagraphKinds, s) }
