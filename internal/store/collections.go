package store

import (
	"context"
	"strings"

	"github.com/chhongzh/scribekit/internal/store/ent"
	"github.com/chhongzh/scribekit/internal/store/ent/article"
	"github.com/chhongzh/scribekit/internal/store/ent/collection"
)

func toCollection(c *ent.Collection, articleCount int) Collection {
	return Collection{
		ID: c.ID, Name: c.Name, Outline: c.Outline,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, ArticleCount: articleCount,
	}
}

func (s *Store) articleCounts(ctx context.Context) (map[int]int, error) {
	var rows []struct {
		CollectionID int `json:"collection_id"`
		Count        int `json:"count"`
	}
	err := s.client.Article.Query().
		Where(article.CollectionIDNotNil()).
		GroupBy(article.FieldCollectionID).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	m := make(map[int]int, len(rows))
	for _, r := range rows {
		m[r.CollectionID] = r.Count
	}
	return m, nil
}

func (s *Store) ListCollections(ctx context.Context) ([]Collection, error) {
	rows, err := s.client.Collection.Query().
		Order(ent.Desc(collection.FieldUpdatedAt), ent.Desc(collection.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := s.articleCounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Collection, len(rows))
	for i, c := range rows {
		out[i] = toCollection(c, counts[c.ID])
	}
	return out, nil
}

func (s *Store) CreateCollection(ctx context.Context, in CollectionInput) (Collection, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return Collection{}, invalid("文集名称不能为空")
	}
	c, err := s.client.Collection.Create().SetName(name).SetOutline(in.Outline).Save(ctx)
	if err != nil {
		return Collection{}, err
	}
	return toCollection(c, 0), nil
}

func (s *Store) GetCollection(ctx context.Context, id int) (CollectionDetail, error) {
	c, err := s.client.Collection.Get(ctx, id)
	if err != nil {
		return CollectionDetail{}, notFoundErr(err, "文集", id)
	}
	arts, err := s.ListArticles(ctx, ArticleFilter{CollectionID: &id})
	if err != nil {
		return CollectionDetail{}, err
	}
	return CollectionDetail{Collection: toCollection(c, len(arts)), Articles: arts}, nil
}

func (s *Store) UpdateCollection(ctx context.Context, id int, p CollectionPatch) (Collection, error) {
	u := s.client.Collection.UpdateOneID(id)
	if p.Name != nil {
		name := strings.TrimSpace(*p.Name)
		if name == "" {
			return Collection{}, invalid("文集名称不能为空")
		}
		u.SetName(name)
	}
	if p.Outline != nil {
		u.SetOutline(*p.Outline)
	}
	c, err := u.Save(ctx)
	if err != nil {
		return Collection{}, notFoundErr(err, "文集", id)
	}
	n, err := s.client.Article.Query().Where(article.CollectionID(id)).Count(ctx)
	if err != nil {
		return Collection{}, err
	}
	return toCollection(c, n), nil
}

// DeleteCollection 删除文集；文章由外键 SET NULL 变为散篇，文集素材随文集级联删除。
func (s *Store) DeleteCollection(ctx context.Context, id int) error {
	return notFoundErr(s.client.Collection.DeleteOneID(id).Exec(ctx), "文集", id)
}
