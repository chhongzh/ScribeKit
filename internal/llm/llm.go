// Package llm 按设置构建 eino ChatModel，并在设置变更后惰性重建。
package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"

	"github.com/chhongzh/scribekit/internal/apperr"
	"github.com/chhongzh/scribekit/internal/logging"
	"github.com/chhongzh/scribekit/internal/prompt"
	"github.com/chhongzh/scribekit/internal/store"
	"go.uber.org/zap"
)

type Role int

const (
	Analyzer Role = iota
	Scaffolder
)

// ErrNotConfigured 在没有 apiKey 或模型名时返回，前端据此引导用户去设置页。
var ErrNotConfigured = apperr.New(apperr.ModelNotConfigured, "尚未配置模型：请在设置页填写 API Key 和模型名")

const maxTokens = 4096

type SettingsSource interface {
	GetSettings(ctx context.Context) (store.Settings, error)
}

// Factory 用设置和模型名构建模型，测试里替换成 fake。
type Factory func(ctx context.Context, s store.Settings, modelName string) (model.ToolCallingChatModel, error)

type Handle struct {
	Model model.ToolCallingChatModel
	Name  string
}

type Manager struct {
	src     SettingsSource
	factory Factory

	mu    sync.Mutex
	cache map[Role]cached
}

type cached struct {
	fingerprint string
	handle      Handle
}

func NewManager(src SettingsSource) *Manager {
	return NewManagerWithFactory(src, Build)
}

func NewManagerWithFactory(src SettingsSource, f Factory) *Manager {
	return &Manager{src: src, factory: f, cache: map[Role]cached{}}
}

func modelFor(s store.Settings, role Role) string {
	if role == Analyzer {
		return strings.TrimSpace(s.AnalyzerModel)
	}
	return strings.TrimSpace(s.ScaffolderModel)
}

// For 返回角色对应的模型；设置里的 provider、地址、密钥或模型名变化后自动重建。
func (m *Manager) For(ctx context.Context, role Role) (handle Handle, err error) {
	ctx, finish := logging.Start(ctx, "model.resolve", zap.Int("model_role", int(role)))
	defer func() { finish(err) }()
	s, err := m.src.GetSettings(ctx)
	if err != nil {
		return Handle{}, err
	}
	name := modelFor(s, role)
	if strings.TrimSpace(s.APIKey) == "" || name == "" {
		return Handle{}, ErrNotConfigured
	}
	fp := strings.Join([]string{s.Provider, s.BaseURL, s.APIKey, name}, "\x00")

	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.cache[role]; ok && c.fingerprint == fp {
		logging.From(ctx).Info("model.cache.hit", zap.String("model", name), zap.String("provider", s.Provider))
		return c.handle, nil
	}
	logging.From(ctx).Info("model.cache.miss", zap.String("model", name), zap.String("provider", s.Provider))
	cm, err := m.factory(ctx, s, name)
	if err != nil {
		return Handle{}, fmt.Errorf("构建模型失败: %w", err)
	}
	h := Handle{Model: cm, Name: name}
	m.cache[role] = cached{fingerprint: fp, handle: h}
	return h, nil
}

func Build(ctx context.Context, s store.Settings, modelName string) (cm model.ToolCallingChatModel, err error) {
	ctx, finish := logging.Start(ctx, "model.build", zap.String("provider", s.Provider), zap.String("model", modelName))
	defer func() { finish(err) }()
	baseURL := strings.TrimSpace(s.BaseURL)
	switch s.Provider {
	case "claude":
		cfg := &claude.Config{APIKey: s.APIKey, Model: modelName, MaxTokens: maxTokens}
		if baseURL != "" {
			cfg.BaseURL = &baseURL
		}
		return claude.NewChatModel(ctx, cfg)
	case "openai":
		return openai.NewChatModel(ctx, &openai.ChatModelConfig{APIKey: s.APIKey, BaseURL: baseURL, Model: modelName})
	default:
		return nil, fmt.Errorf("未知 provider %q", s.Provider)
	}
}

// Test 对分析和生成两个模型各发一个极短请求，返回给设置页的结果。
func (m *Manager) Test(ctx context.Context) (result string, err error) {
	ctx, finish := logging.Start(ctx, "model.test")
	defer func() { finish(err) }()
	probe, err := prompt.Probe()
	if err != nil {
		return "", err
	}
	var parts []string
	seen := map[string]bool{}
	for _, role := range []Role{Analyzer, Scaffolder} {
		h, err := m.For(ctx, role)
		if err != nil {
			return "", err
		}
		if seen[h.Name] {
			logging.From(ctx).Info("model.probe.skipped", zap.String("model", h.Name))
			continue
		}
		seen[h.Name] = true
		logging.From(ctx).Info("model.probe.started", zap.String("model", h.Name))
		msg, err := h.Model.Generate(ctx, probe, model.WithMaxTokens(16))
		if err != nil {
			return "", fmt.Errorf("模型 %s 请求失败: %w", h.Name, err)
		}
		if msg == nil {
			return "", fmt.Errorf("模型 %s 没有返回内容", h.Name)
		}
		parts = append(parts, h.Name)
		logging.From(ctx).Info("model.probe.completed", zap.String("model", h.Name), zap.Int("output_bytes", len(msg.Content)))
	}
	return "连接正常：" + strings.Join(parts, "、"), nil
}
