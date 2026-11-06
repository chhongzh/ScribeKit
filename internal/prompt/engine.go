// Package prompt 集中管理发给模型的全部文本：系统提示、用户消息、工具描述和 JSON 修复提示。
// 模板是 Jinja2，用 gonja 配 embed FS 的 loader 渲染；eino 自带的 schema.Jinja2 对
// 单条消息做无 loader 渲染，include/import 被禁用，所以这里不复用它。
package prompt

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/cloudwego/eino/schema"
	"github.com/nikolalohinski/gonja"
	"github.com/nikolalohinski/gonja/config"
	"github.com/nikolalohinski/gonja/exec"
)

//go:embed all:templates
var templateFS embed.FS

type embedLoader struct{ fsys fs.FS }

func (l embedLoader) Get(name string) (io.Reader, error) {
	b, err := fs.ReadFile(l.fsys, path.Clean(name))
	if err != nil {
		return nil, err
	}
	return strings.NewReader(string(b)), nil
}

func (l embedLoader) Path(name string) (string, error) { return path.Clean(name), nil }

type engine struct {
	env       *gonja.Environment
	templates map[string]*exec.Template
}

// templateRoot 之下 _ 开头的目录是只被 include 的片段，不单独渲染。
func newEngine() (*engine, error) {
	sub, err := fs.Sub(templateFS, "templates")
	if err != nil {
		return nil, err
	}
	cfg := config.NewConfig()
	e := &engine{
		env:       gonja.NewEnvironment(cfg, embedLoader{sub}),
		templates: map[string]*exec.Template{},
	}
	err = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".jinja") || strings.HasPrefix(p, "_") {
			return err
		}
		tpl, err := e.env.FromFile(p)
		if err != nil {
			return fmt.Errorf("parse template %s: %w", p, err)
		}
		e.templates[p] = tpl
		return nil
	})
	return e, err
}

var eng = mustEngine()

func mustEngine() *engine {
	e, err := newEngine()
	if err != nil {
		panic(err)
	}
	return e
}

func (e *engine) render(name string, vars any) (result string, err error) {
	started := time.Now()
	defer func() {
		fields := []zap.Field{zap.String("template", name), zap.Duration("duration", time.Since(started)), zap.Int("output_bytes", len(result))}
		if err != nil {
			zap.L().Error("prompt.render.failed", append(fields, zap.Error(err))...)
		} else {
			zap.L().Info("prompt.render.completed", fields...)
		}
	}()
	tpl, ok := e.templates[name]
	if !ok {
		return "", fmt.Errorf("unknown template %s", name)
	}
	ctx, err := toContext(vars)
	if err != nil {
		return "", err
	}
	out, err := tpl.Execute(ctx)
	if err != nil {
		return "", fmt.Errorf("render %s: %w", name, err)
	}
	return strings.TrimSpace(out), nil
}

// 用 JSON 往返把强类型变量转成模板能遍历的 map，字段名取 json tag。
func toContext(vars any) (map[string]any, error) {
	if vars == nil {
		return map[string]any{}, nil
	}
	b, err := json.Marshal(vars)
	if err != nil {
		return nil, err
	}
	m := map[string]any{}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return normalizeNumbers(m).(map[string]any), nil
}

func (e *engine) messages(vars any, parts ...part) ([]*schema.Message, error) {
	msgs := make([]*schema.Message, 0, len(parts))
	for _, p := range parts {
		text, err := e.render(p.template, vars)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, &schema.Message{Role: p.role, Content: text})
	}
	return msgs, nil
}

type part struct {
	role     schema.RoleType
	template string
}

// encoding/json 把所有数字解成 float64，模板里会渲染成 "0.0"，这里还原整数值。
func normalizeNumbers(v any) any {
	switch x := v.(type) {
	case float64:
		if x == float64(int64(x)) {
			return int64(x)
		}
		return x
	case []any:
		for i := range x {
			x[i] = normalizeNumbers(x[i])
		}
		return x
	case map[string]any:
		for k := range x {
			x[k] = normalizeNumbers(x[k])
		}
		return x
	}
	return v
}
