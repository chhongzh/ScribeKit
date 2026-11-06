package store

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/chhongzh/scribekit/internal/store/ent"
	"github.com/chhongzh/scribekit/internal/store/ent/suggestion"
)

func toSuggestion(r *ent.Suggestion) (Suggestion, error) {
	out := Suggestion{
		ID: r.ID, ArticleID: r.ArticleID, RequestID: r.RequestID, Op: r.Op,
		ParagraphKind: r.ParagraphKind, Instruction: r.Instruction, Text: r.Text,
		Rationale: r.Rationale, Status: r.Status, CreatedAt: r.CreatedAt,
		Target: json.RawMessage("null"), Halves: []SuggestionHalf{},
	}
	if len(r.Target) > 0 {
		out.Target = r.Target
	}
	if len(r.Halves) > 0 {
		if err := json.Unmarshal(r.Halves, &out.Halves); err != nil {
			return Suggestion{}, err
		}
	}
	return out, nil
}

// CreateSuggestion 在生成开始时落一条 streaming 状态的记录，让前端先拿到 suggestionId。
func (s *Store) CreateSuggestion(ctx context.Context, in SuggestionInput) (Suggestion, error) {
	if !slices.Contains(SuggestionOps, in.Op) {
		return Suggestion{}, invalid("未知建议类型 %q", in.Op)
	}
	c := s.client.Suggestion.Create().SetArticleID(in.ArticleID).SetRequestID(in.RequestID).
		SetOp(in.Op).SetParagraphKind(in.ParagraphKind).SetInstruction(in.Instruction).
		SetStatus("streaming")
	if len(in.Target) > 0 {
		c.SetTarget(in.Target)
	}
	r, err := c.Save(ctx)
	if err != nil {
		return Suggestion{}, err
	}
	return toSuggestion(r)
}

// FinishSuggestion 写入最终文本并转为 pending。
func (s *Store) FinishSuggestion(ctx context.Context, id int, text, rationale string, halves []SuggestionHalf) (Suggestion, error) {
	if halves == nil {
		halves = []SuggestionHalf{}
	}
	hb, err := json.Marshal(halves)
	if err != nil {
		return Suggestion{}, err
	}
	r, err := s.client.Suggestion.UpdateOneID(id).SetText(text).SetRationale(rationale).
		SetHalves(hb).SetStatus("pending").Save(ctx)
	if err != nil {
		return Suggestion{}, notFoundErr(err, "建议", id)
	}
	return toSuggestion(r)
}

func (s *Store) DeleteSuggestion(ctx context.Context, id int) error {
	return s.client.Suggestion.DeleteOneID(id).Exec(ctx)
}

func (s *Store) GetSuggestion(ctx context.Context, id int) (Suggestion, error) {
	r, err := s.client.Suggestion.Get(ctx, id)
	if err != nil {
		return Suggestion{}, notFoundErr(err, "建议", id)
	}
	return toSuggestion(r)
}

// ListSuggestions 的 status 为空时返回除 streaming 以外的全部建议。
func (s *Store) ListSuggestions(ctx context.Context, articleID int, status string) ([]Suggestion, error) {
	q := s.client.Suggestion.Query().Where(suggestion.ArticleID(articleID)).
		Order(ent.Asc(suggestion.FieldCreatedAt), ent.Asc(suggestion.FieldID))
	if status != "" {
		if !slices.Contains(statuses, status) {
			return nil, invalid("未知状态 %q", status)
		}
		q.Where(suggestion.StatusEQ(status))
	} else {
		q.Where(suggestion.StatusNEQ("streaming"))
	}
	rows, err := q.All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Suggestion, len(rows))
	for i, r := range rows {
		if out[i], err = toSuggestion(r); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) PatchSuggestion(ctx context.Context, id int, p SuggestionPatch) (Suggestion, error) {
	u := s.client.Suggestion.UpdateOneID(id)
	if p.Halves != nil {
		hb, err := json.Marshal(*p.Halves)
		if err != nil {
			return Suggestion{}, err
		}
		u.SetHalves(hb)
	}
	if p.Status != nil {
		if !slices.Contains([]string{"pending", "applied", "rejected"}, *p.Status) {
			return Suggestion{}, invalid("状态只能是 pending、applied 或 rejected")
		}
		u.SetStatus(*p.Status)
	}
	r, err := u.Save(ctx)
	if err != nil {
		return Suggestion{}, notFoundErr(err, "建议", id)
	}
	return toSuggestion(r)
}

func (s *Store) RejectSuggestion(ctx context.Context, id int) (Suggestion, error) {
	status := "rejected"
	return s.PatchSuggestion(ctx, id, SuggestionPatch{Status: &status})
}

// ApplySuggestion 按建议类型把用户评审后的最终文本写入文章，返回受影响的段落。
func (s *Store) ApplySuggestion(ctx context.Context, id int, text string) ([]Paragraph, error) {
	var out []Paragraph
	err := s.tx(ctx, func(tx *ent.Tx) error {
		sg, err := tx.Suggestion.Get(ctx, id)
		if err != nil {
			return notFoundErr(err, "建议", id)
		}
		if sg.Status != "pending" {
			return conflict("建议 %d 的状态是 %s，只有 pending 的建议可以应用", id, sg.Status)
		}
		var t SuggestionTarget
		if len(sg.Target) > 0 {
			if err := json.Unmarshal(sg.Target, &t); err != nil {
				return err
			}
		}
		switch sg.Op {
		case "replace":
			p, err := tx.Paragraph.Get(ctx, t.ParagraphID)
			if err != nil {
				return notFoundErr(err, "段落", t.ParagraphID)
			}
			merged, err := replaceRange(p.Text, t.Start, t.End, text)
			if err != nil {
				return err
			}
			p, err = tx.Paragraph.UpdateOneID(p.ID).SetText(merged).Save(ctx)
			if err != nil {
				return err
			}
			out = []Paragraph{toParagraph(p)}
		case "insert_paragraph":
			after := Patch[int]{Set: true, Val: t.AfterParagraphID}
			pos, err := between(ctx, tx, sg.ArticleID, 0, after)
			if err != nil {
				return err
			}
			p, err := tx.Paragraph.Create().SetArticleID(sg.ArticleID).SetPosition(pos).
				SetKind(sg.ParagraphKind).SetText(text).Save(ctx)
			if err != nil {
				return err
			}
			out = []Paragraph{toParagraph(p)}
		default:
			return invalid("%s 类型的建议是只读的，不能应用", sg.Op)
		}
		if err := tx.Suggestion.UpdateOneID(id).SetStatus("applied").Exec(ctx); err != nil {
			return err
		}
		return touchArticle(ctx, tx, sg.ArticleID)
	})
	return out, err
}
