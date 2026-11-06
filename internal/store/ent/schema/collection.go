package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type Collection struct{ ent.Schema }

func (Collection) Mixin() []ent.Mixin { return []ent.Mixin{timestamps{}} }

func (Collection) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").NotEmpty(),
		field.Text("outline").Default(""),
	}
}

func (Collection) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("articles", Article.Type).Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.To("materials", Material.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}
