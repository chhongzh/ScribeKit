package store

import (
	"context"
	"strings"

	"github.com/chhongzh/scribekit/internal/store/ent"
	"github.com/chhongzh/scribekit/internal/store/ent/material"
	"github.com/chhongzh/scribekit/internal/store/ent/predicate"
)

func toMaterial(m *ent.Material) Material {
	return Material{
		ID: m.ID, CollectionID: m.CollectionID, ArticleID: m.ArticleID,
		Name: m.Name, Content: m.Content, CreatedAt: m.CreatedAt,
	}
}

func toMaterials(rows []*ent.Material) []Material {
	out := make([]Material, len(rows))
	for i, m := range rows {
		out[i] = toMaterial(m)
	}
	return out
}

// ListMaterials 按给定的作用域精确过滤；两个条件都缺省时返回全部素材。
func (s *Store) ListMaterials(ctx context.Context, f MaterialFilter) ([]Material, error) {
	q := s.client.Material.Query().Order(ent.Desc(material.FieldCreatedAt), ent.Desc(material.FieldID))
	if f.CollectionID != nil {
		q.Where(material.CollectionID(*f.CollectionID))
	}
	if f.ArticleID != nil {
		q.Where(material.ArticleID(*f.ArticleID))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	return toMaterials(rows), nil
}

// MaterialsVisibleTo 返回对某篇文章可见的素材：全局、所属文集、文章自己的。
func (s *Store) MaterialsVisibleTo(ctx context.Context, articleID int) ([]Material, error) {
	a, err := s.client.Article.Get(ctx, articleID)
	if err != nil {
		return nil, notFoundErr(err, "文章", articleID)
	}
	preds := []predicate.Material{
		material.And(material.CollectionIDIsNil(), material.ArticleIDIsNil()),
		material.ArticleID(articleID),
	}
	if a.CollectionID != nil {
		preds = append(preds, material.And(material.CollectionID(*a.CollectionID), material.ArticleIDIsNil()))
	}
	rows, err := s.client.Material.Query().Where(material.Or(preds...)).
		Order(ent.Asc(material.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	return toMaterials(rows), nil
}

func (s *Store) CreateMaterial(ctx context.Context, in MaterialInput) (Material, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Material{}, invalid("素材名称不能为空")
	}
	if strings.TrimSpace(in.Content) == "" {
		return Material{}, invalid("素材内容不能为空")
	}
	c := s.client.Material.Create().SetName(name).SetContent(in.Content)
	if in.CollectionID != nil {
		if _, err := s.client.Collection.Get(ctx, *in.CollectionID); err != nil {
			return Material{}, notFoundErr(err, "文集", *in.CollectionID)
		}
		c.SetCollectionID(*in.CollectionID)
	}
	if in.ArticleID != nil {
		if _, err := s.client.Article.Get(ctx, *in.ArticleID); err != nil {
			return Material{}, notFoundErr(err, "文章", *in.ArticleID)
		}
		c.SetArticleID(*in.ArticleID)
	}
	m, err := c.Save(ctx)
	if err != nil {
		return Material{}, err
	}
	return toMaterial(m), nil
}

func (s *Store) DeleteMaterial(ctx context.Context, id int) error {
	return notFoundErr(s.client.Material.DeleteOneID(id).Exec(ctx), "素材", id)
}
