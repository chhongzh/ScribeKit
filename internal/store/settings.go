package store

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chhongzh/scribekit/internal/store/ent"
	"github.com/chhongzh/scribekit/internal/store/ent/analysiscache"
	"github.com/chhongzh/scribekit/internal/store/ent/setting"
)

func DefaultSettings() Settings {
	return Settings{
		Provider:        "claude",
		AnalyzerModel:   "claude-haiku-5-5",
		ScaffolderModel: "claude-sonnet-5-5",
		AutoAnalyze:     true,
		Theme:           json.RawMessage("null"),
	}
}

func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	rows, err := s.client.Setting.Query().All(ctx)
	if err != nil {
		return Settings{}, err
	}
	base, err := json.Marshal(DefaultSettings())
	if err != nil {
		return Settings{}, err
	}
	merged := map[string]json.RawMessage{}
	if err := json.Unmarshal(base, &merged); err != nil {
		return Settings{}, err
	}
	for _, r := range rows {
		merged[r.Key] = json.RawMessage(r.Value)
	}
	b, err := json.Marshal(merged)
	if err != nil {
		return Settings{}, err
	}
	var out Settings
	if err := json.Unmarshal(b, &out); err != nil {
		return Settings{}, fmt.Errorf("decode settings: %w", err)
	}
	if len(out.Theme) == 0 {
		out.Theme = json.RawMessage("null")
	}
	return out, nil
}

// SaveSettings 整体保存设置；in.APIKey 为空表示沿用已保存的密钥。
func (s *Store) SaveSettings(ctx context.Context, in Settings) (Settings, error) {
	if in.Provider != "claude" && in.Provider != "openai" {
		return Settings{}, invalid("provider 只能是 claude 或 openai")
	}
	if len(in.Theme) == 0 {
		in.Theme = json.RawMessage("null")
	}
	if !json.Valid(in.Theme) {
		return Settings{}, invalid("theme 不是合法 JSON")
	}
	if in.APIKey == "" {
		cur, err := s.GetSettings(ctx)
		if err != nil {
			return Settings{}, err
		}
		in.APIKey = cur.APIKey
	}
	b, err := json.Marshal(in)
	if err != nil {
		return Settings{}, err
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(b, &fields); err != nil {
		return Settings{}, err
	}
	err = s.tx(ctx, func(tx *ent.Tx) error {
		for k, v := range fields {
			n, err := tx.Setting.Update().Where(setting.Key(k)).SetValue(string(v)).Save(ctx)
			if err != nil {
				return err
			}
			if n == 0 {
				if err := tx.Setting.Create().SetKey(k).SetValue(string(v)).Exec(ctx); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return Settings{}, err
	}
	return in, nil
}

// CachedAnalysis 返回 (段落, 文本哈希, 模型) 命中的分析结果 JSON。
func (s *Store) CachedAnalysis(ctx context.Context, paragraphID int, textHash, model string) (json.RawMessage, bool, error) {
	row, err := s.client.AnalysisCache.Query().Where(
		analysiscache.ParagraphID(paragraphID),
		analysiscache.TextHash(textHash),
		analysiscache.Model(model),
	).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return row.Result, true, nil
}

// PutAnalysis 写入缓存，并清掉该段落其他哈希的旧结果。
func (s *Store) PutAnalysis(ctx context.Context, paragraphID int, textHash, model string, result json.RawMessage) error {
	return s.tx(ctx, func(tx *ent.Tx) error {
		if _, err := tx.AnalysisCache.Delete().Where(
			analysiscache.ParagraphID(paragraphID),
			analysiscache.Or(analysiscache.TextHashNEQ(textHash), analysiscache.Model(model)),
		).Exec(ctx); err != nil {
			return err
		}
		return tx.AnalysisCache.Create().SetParagraphID(paragraphID).SetTextHash(textHash).
			SetModel(model).SetResult(result).Exec(ctx)
	})
}
