package store

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/chhongzh/scribekit/internal/store/ent"
	"github.com/chhongzh/scribekit/internal/store/ent/article"
	"github.com/chhongzh/scribekit/internal/store/ent/paragraph"
)

func toArticle(a *ent.Article) Article {
	return Article{
		ID: a.ID, CollectionID: a.CollectionID, Title: a.Title, Synopsis: a.Synopsis,
		Genre: a.Genre, Subgenre: a.Subgenre, CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
}

func toParagraph(p *ent.Paragraph) Paragraph {
	return Paragraph{
		ID: p.ID, ArticleID: p.ArticleID, Position: p.Position, Kind: p.Kind, Text: p.Text,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

// CountChars 统计非空白字符数，作为展示用的字数。
func CountChars(text string) int {
	n := 0
	for _, r := range text {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	return n
}

func (s *Store) charCounts(ctx context.Context, articleIDs []int) (map[int]int, error) {
	counts := make(map[int]int, len(articleIDs))
	if len(articleIDs) == 0 {
		return counts, nil
	}
	rows, err := s.client.Paragraph.Query().
		Where(paragraph.ArticleIDIn(articleIDs...)).
		Select(paragraph.FieldArticleID, paragraph.FieldText).All(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range rows {
		counts[p.ArticleID] += CountChars(p.Text)
	}
	return counts, nil
}

func (s *Store) ListArticles(ctx context.Context, f ArticleFilter) ([]ArticleSummary, error) {
	q := s.client.Article.Query().Order(ent.Desc(article.FieldUpdatedAt), ent.Desc(article.FieldID))
	switch {
	case f.LooseOnly:
		q.Where(article.CollectionIDIsNil())
	case f.CollectionID != nil:
		q.Where(article.CollectionID(*f.CollectionID))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int, len(rows))
	for i, a := range rows {
		ids[i] = a.ID
	}
	counts, err := s.charCounts(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]ArticleSummary, len(rows))
	for i, a := range rows {
		out[i] = ArticleSummary{
			ID: a.ID, CollectionID: a.CollectionID, Title: a.Title, Genre: a.Genre,
			Subgenre: a.Subgenre, UpdatedAt: a.UpdatedAt, CharCount: counts[a.ID],
		}
	}
	return out, nil
}

func (s *Store) CreateArticle(ctx context.Context, in ArticleInput) (Article, error) {
	genre := in.Genre
	if genre == "" {
		genre = "other"
	}
	if !ValidGenre(genre) {
		return Article{}, invalid("未知文体 %q", genre)
	}
	c := s.client.Article.Create().SetTitle(in.Title).SetSynopsis(in.Synopsis).
		SetGenre(genre).SetSubgenre(strings.TrimSpace(in.Subgenre))
	if in.CollectionID != nil {
		col, err := s.client.Collection.Get(ctx, *in.CollectionID)
		if err != nil {
			return Article{}, notFoundErr(err, "文集", *in.CollectionID)
		}
		c.SetCollectionID(col.ID)
	}
	a, err := c.Save(ctx)
	if err != nil {
		return Article{}, err
	}
	return toArticle(a), nil
}

func (s *Store) GetArticleMeta(ctx context.Context, id int) (Article, error) {
	a, err := s.client.Article.Get(ctx, id)
	if err != nil {
		return Article{}, notFoundErr(err, "文章", id)
	}
	return toArticle(a), nil
}

func (s *Store) GetArticle(ctx context.Context, id int) (ArticleDetail, error) {
	a, err := s.GetArticleMeta(ctx, id)
	if err != nil {
		return ArticleDetail{}, err
	}
	ps, err := s.ListParagraphs(ctx, id)
	if err != nil {
		return ArticleDetail{}, err
	}
	return ArticleDetail{Article: a, Paragraphs: ps}, nil
}

func (s *Store) ListParagraphs(ctx context.Context, articleID int) ([]Paragraph, error) {
	rows, err := s.client.Paragraph.Query().Where(paragraph.ArticleID(articleID)).
		Order(ent.Asc(paragraph.FieldPosition), ent.Asc(paragraph.FieldID)).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Paragraph, len(rows))
	for i, p := range rows {
		out[i] = toParagraph(p)
	}
	return out, nil
}

func (s *Store) UpdateArticle(ctx context.Context, id int, p ArticlePatch) (Article, error) {
	u := s.client.Article.UpdateOneID(id)
	if p.Title != nil {
		u.SetTitle(*p.Title)
	}
	if p.Synopsis != nil {
		u.SetSynopsis(*p.Synopsis)
	}
	if p.Genre != nil {
		if !ValidGenre(*p.Genre) {
			return Article{}, invalid("未知文体 %q", *p.Genre)
		}
		u.SetGenre(*p.Genre)
	}
	if p.Subgenre != nil {
		u.SetSubgenre(strings.TrimSpace(*p.Subgenre))
	}
	if p.CollectionID.Set {
		if p.CollectionID.Val == nil {
			u.ClearCollectionID()
		} else {
			if _, err := s.client.Collection.Get(ctx, *p.CollectionID.Val); err != nil {
				return Article{}, notFoundErr(err, "文集", *p.CollectionID.Val)
			}
			u.SetCollectionID(*p.CollectionID.Val)
		}
	}
	a, err := u.Save(ctx)
	if err != nil {
		return Article{}, notFoundErr(err, "文章", id)
	}
	return toArticle(a), nil
}

func (s *Store) DeleteArticle(ctx context.Context, id int) error {
	return notFoundErr(s.client.Article.DeleteOneID(id).Exec(ctx), "文章", id)
}

// touchArticle 在段落变动后刷新文章的 updatedAt，文章列表按它排序。
func touchArticle(ctx context.Context, tx *ent.Tx, id int) error {
	return tx.Article.UpdateOneID(id).SetUpdatedAt(time.Now()).Exec(ctx)
}
