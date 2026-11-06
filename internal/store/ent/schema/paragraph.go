package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Paragraph struct{ ent.Schema }

func (Paragraph) Mixin() []ent.Mixin { return []ent.Mixin{timestamps{}} }

func (Paragraph) Fields() []ent.Field {
	return []ent.Field{
		field.Int("article_id"),
		field.Float("position"),
		field.String("kind").Default("normal"),
		field.Text("text").Default(""),
	}
}

func (Paragraph) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("article", Article.Type).Ref("paragraphs").Field("article_id").Unique().Required(),
		edge.To("analyses", AnalysisCache.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (Paragraph) Indexes() []ent.Index {
	return []ent.Index{index.Fields("article_id", "position")}
}
