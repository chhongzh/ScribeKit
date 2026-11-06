package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Material struct{ ent.Schema }

func (Material) Fields() []ent.Field {
	return []ent.Field{
		field.Int("collection_id").Optional().Nillable(),
		field.Int("article_id").Optional().Nillable(),
		field.String("name"),
		field.Text("content"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Material) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("collection", Collection.Type).Ref("materials").Field("collection_id").Unique(),
		edge.From("article", Article.Type).Ref("materials").Field("article_id").Unique(),
	}
}
