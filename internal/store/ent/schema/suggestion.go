package schema

import (
	"encoding/json"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Suggestion struct{ ent.Schema }

func (Suggestion) Fields() []ent.Field {
	return []ent.Field{
		field.Int("article_id"),
		field.String("request_id").Default(""),
		field.String("op"),
		field.JSON("target", json.RawMessage{}).Optional(),
		field.String("paragraph_kind").Default("normal"),
		field.Text("instruction").Default(""),
		field.Text("text").Default(""),
		field.JSON("halves", json.RawMessage{}).Optional(),
		field.Text("rationale").Default(""),
		field.String("status").Default("pending"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Suggestion) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("article", Article.Type).Ref("suggestions").Field("article_id").Unique().Required(),
	}
}
