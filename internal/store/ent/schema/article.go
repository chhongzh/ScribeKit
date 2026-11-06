package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Article struct{ ent.Schema }

func (Article) Mixin() []ent.Mixin { return []ent.Mixin{timestamps{}} }

func (Article) Fields() []ent.Field {
	return []ent.Field{
		field.Int("collection_id").Optional().Nillable(),
		field.String("title").Default(""),
		field.Text("synopsis").Default(""),
		field.String("genre").Default("other"),
		field.String("subgenre").Default(""),
	}
}

func (Article) Edges() []ent.Edge {
	cascade := entsql.OnDelete(entsql.Cascade)
	return []ent.Edge{
		edge.From("collection", Collection.Type).Ref("articles").Field("collection_id").Unique(),
		edge.To("paragraphs", Paragraph.Type).Annotations(cascade),
		edge.To("materials", Material.Type).Annotations(cascade),
		edge.To("suggestions", Suggestion.Type).Annotations(cascade),
	}
}
