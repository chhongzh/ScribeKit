package store

import (
	"context"

	"github.com/chhongzh/scribekit/internal/store/ent"
	"github.com/chhongzh/scribekit/internal/store/ent/paragraph"
)

// minGap 以下的相邻位置差视为精度耗尽，需要重新编号。
const minGap = 1e-9

type slot struct {
	id  int
	pos float64
}

func listSlots(ctx context.Context, tx *ent.Tx, articleID, excludeID int) ([]slot, error) {
	rows, err := tx.Paragraph.Query().Where(paragraph.ArticleID(articleID)).
		Order(ent.Asc(paragraph.FieldPosition), ent.Asc(paragraph.FieldID)).
		Select(paragraph.FieldID, paragraph.FieldPosition).All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]slot, 0, len(rows))
	for _, r := range rows {
		if r.ID != excludeID {
			out = append(out, slot{r.ID, r.Position})
		}
	}
	return out, nil
}

func renumber(ctx context.Context, tx *ent.Tx, articleID int) error {
	all, err := listSlots(ctx, tx, articleID, 0)
	if err != nil {
		return err
	}
	for i, sl := range all {
		if err := tx.Paragraph.UpdateOneID(sl.id).SetPosition(float64(i + 1)).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

// between 返回 after 之后的插入位置：after 缺省为末尾，Val 为 nil 为开头。
// excludeID 是正在移动的段落，计算邻居时要跳过自己。
func between(ctx context.Context, tx *ent.Tx, articleID, excludeID int, after Patch[int]) (float64, error) {
	for attempt := 0; attempt < 2; attempt++ {
		slots, err := listSlots(ctx, tx, articleID, excludeID)
		if err != nil {
			return 0, err
		}
		var lo, hi *slot
		switch {
		case !after.Set:
			if n := len(slots); n > 0 {
				lo = &slots[n-1]
			}
		case after.Val == nil:
			if len(slots) > 0 {
				hi = &slots[0]
			}
		default:
			idx := -1
			for i, sl := range slots {
				if sl.id == *after.Val {
					idx = i
					break
				}
			}
			if idx < 0 {
				return 0, invalid("afterId %d 不是本文章的段落", *after.Val)
			}
			lo = &slots[idx]
			if idx+1 < len(slots) {
				hi = &slots[idx+1]
			}
		}
		switch {
		case lo == nil && hi == nil:
			return 1, nil
		case hi == nil:
			return lo.pos + 1, nil
		case lo == nil:
			return hi.pos - 1, nil
		}
		if hi.pos-lo.pos >= minGap {
			return (lo.pos + hi.pos) / 2, nil
		}
		if err := renumber(ctx, tx, articleID); err != nil {
			return 0, err
		}
	}
	return 0, conflict("段落位置重新编号后仍无法插入")
}

func checkKind(kind string) (string, error) {
	if kind == "" {
		return "normal", nil
	}
	if !ValidParagraphKind(kind) {
		return "", invalid("未知段落类型 %q", kind)
	}
	return kind, nil
}

// InsertParagraph 在 after 之后插入新段落。
func (s *Store) InsertParagraph(ctx context.Context, articleID int, after Patch[int], kind, text string) (Paragraph, error) {
	kind, err := checkKind(kind)
	if err != nil {
		return Paragraph{}, err
	}
	var out Paragraph
	err = s.tx(ctx, func(tx *ent.Tx) error {
		if _, err := tx.Article.Get(ctx, articleID); err != nil {
			return notFoundErr(err, "文章", articleID)
		}
		pos, err := between(ctx, tx, articleID, 0, after)
		if err != nil {
			return err
		}
		p, err := tx.Paragraph.Create().SetArticleID(articleID).SetPosition(pos).SetKind(kind).SetText(text).Save(ctx)
		if err != nil {
			return err
		}
		out = toParagraph(p)
		return touchArticle(ctx, tx, articleID)
	})
	return out, err
}

func (s *Store) GetParagraph(ctx context.Context, id int) (Paragraph, error) {
	p, err := s.client.Paragraph.Get(ctx, id)
	if err != nil {
		return Paragraph{}, notFoundErr(err, "段落", id)
	}
	return toParagraph(p), nil
}

func (s *Store) UpdateParagraph(ctx context.Context, id int, patch ParagraphPatch) (Paragraph, error) {
	var out Paragraph
	err := s.tx(ctx, func(tx *ent.Tx) error {
		u := tx.Paragraph.UpdateOneID(id)
		if patch.Text != nil {
			u.SetText(*patch.Text)
		}
		if patch.Kind != nil {
			kind, err := checkKind(*patch.Kind)
			if err != nil {
				return err
			}
			u.SetKind(kind)
		}
		p, err := u.Save(ctx)
		if err != nil {
			return notFoundErr(err, "段落", id)
		}
		out = toParagraph(p)
		return touchArticle(ctx, tx, p.ArticleID)
	})
	return out, err
}

// MoveParagraph 把段落移到 after 之后；after 的 Val 为 nil 表示移到开头。
func (s *Store) MoveParagraph(ctx context.Context, id int, afterID *int) (Paragraph, error) {
	var out Paragraph
	err := s.tx(ctx, func(tx *ent.Tx) error {
		p, err := tx.Paragraph.Get(ctx, id)
		if err != nil {
			return notFoundErr(err, "段落", id)
		}
		if afterID != nil && *afterID == id {
			out = toParagraph(p)
			return nil
		}
		pos, err := between(ctx, tx, p.ArticleID, id, Patch[int]{Set: true, Val: afterID})
		if err != nil {
			return err
		}
		p, err = tx.Paragraph.UpdateOneID(id).SetPosition(pos).Save(ctx)
		if err != nil {
			return err
		}
		out = toParagraph(p)
		return touchArticle(ctx, tx, p.ArticleID)
	})
	return out, err
}

func (s *Store) DeleteParagraph(ctx context.Context, id int) error {
	return s.tx(ctx, func(tx *ent.Tx) error {
		p, err := tx.Paragraph.Get(ctx, id)
		if err != nil {
			return notFoundErr(err, "段落", id)
		}
		if err := tx.Paragraph.DeleteOneID(id).Exec(ctx); err != nil {
			return err
		}
		return touchArticle(ctx, tx, p.ArticleID)
	})
}

// SplitParagraph 在码点 offset 处把段落拆成两段，后一段类型为 normal。
func (s *Store) SplitParagraph(ctx context.Context, id, offset int) ([2]Paragraph, error) {
	var out [2]Paragraph
	err := s.tx(ctx, func(tx *ent.Tx) error {
		p, err := tx.Paragraph.Get(ctx, id)
		if err != nil {
			return notFoundErr(err, "段落", id)
		}
		rs := []rune(p.Text)
		if offset < 0 || offset > len(rs) {
			return invalid("offset %d 超出段落长度 %d", offset, len(rs))
		}
		pos, err := between(ctx, tx, p.ArticleID, 0, Patch[int]{Set: true, Val: &id})
		if err != nil {
			return err
		}
		second, err := tx.Paragraph.Create().SetArticleID(p.ArticleID).SetPosition(pos).
			SetKind("normal").SetText(string(rs[offset:])).Save(ctx)
		if err != nil {
			return err
		}
		first, err := tx.Paragraph.UpdateOneID(id).SetText(string(rs[:offset])).Save(ctx)
		if err != nil {
			return err
		}
		out = [2]Paragraph{toParagraph(first), toParagraph(second)}
		return touchArticle(ctx, tx, p.ArticleID)
	})
	return out, err
}

// MergeNext 把下一段并入本段，保留本段的类型。
func (s *Store) MergeNext(ctx context.Context, id int) (Paragraph, error) {
	var out Paragraph
	err := s.tx(ctx, func(tx *ent.Tx) error {
		p, err := tx.Paragraph.Get(ctx, id)
		if err != nil {
			return notFoundErr(err, "段落", id)
		}
		slots, err := listSlots(ctx, tx, p.ArticleID, 0)
		if err != nil {
			return err
		}
		nextID := 0
		for i, sl := range slots {
			if sl.id == id && i+1 < len(slots) {
				nextID = slots[i+1].id
			}
		}
		if nextID == 0 {
			return invalid("段落 %d 是最后一段，没有可合并的下一段", id)
		}
		next, err := tx.Paragraph.Get(ctx, nextID)
		if err != nil {
			return err
		}
		if err := tx.Paragraph.DeleteOneID(nextID).Exec(ctx); err != nil {
			return err
		}
		merged, err := tx.Paragraph.UpdateOneID(id).SetText(p.Text + next.Text).Save(ctx)
		if err != nil {
			return err
		}
		out = toParagraph(merged)
		return touchArticle(ctx, tx, p.ArticleID)
	})
	return out, err
}

// replaceRange 把码点区间 [start,end) 换成 text。
func replaceRange(orig string, start, end int, text string) (string, error) {
	rs := []rune(orig)
	if start < 0 || end < start || end > len(rs) {
		return "", conflict("区间 [%d,%d) 超出段落当前长度 %d，段落可能已被修改", start, end, len(rs))
	}
	return string(rs[:start]) + text + string(rs[end:]), nil
}
