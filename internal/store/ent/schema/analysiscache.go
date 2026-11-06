package schema

import (
	"encoding/json"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AnalysisCache struct{ ent.Schema }

func (AnalysisCache) Fields() []ent.Field {
	return []ent.Field{
		field.Int("paragraph_id"),
		field.String("text_hash"),
		field.String("model"),
		field.JSON("result", json.RawMessage{}),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (AnalysisCache) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("paragraph", Paragraph.Type).Ref("analyses").Field("paragraph_id").Unique().Required(),
	}
}

func (AnalysisCache) Indexes() []ent.Index {
	return []ent.Index{index.Fields("paragraph_id", "text_hash", "model").Unique()}
}
