package api

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
)

const buildHint = "ScribeKit 后端已启动，但前端还没有构建。请先执行：cd web && bun install && bun run build，然后重新编译并启动。\n"

// NewApp 组装 Fiber 应用：/api 路由在前，静态资源和 SPA 回退在后。
func NewApp(d Deps, static fs.FS) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: ErrorHandler, BodyLimit: BodyLimit})
	app.Use(recover.New())
	Register(app, d)
	app.Use(staticHandler(static))
	return app
}

// staticHandler 服务前端构建产物：存在的文件原样返回；带扩展名但不存在的路径 404；
// 其余路径回退到 index.html（前端路由）。dist 里没有 index.html 时回一行构建提示。
func staticHandler(fsys fs.FS) fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead {
			return fiber.ErrMethodNotAllowed
		}
		name := strings.TrimPrefix(path.Clean("/"+c.Path()), "/")
		if name == "" {
			name = "index.html"
		}
		if body, err := readFile(fsys, name); err == nil {
			return send(c, name, body)
		}
		if ext := path.Ext(name); ext != "" && ext != ".html" {
			return fiber.ErrNotFound
		}
		body, err := readFile(fsys, "index.html")
		if err != nil {
			c.Set(fiber.HeaderContentType, "text/plain; charset=utf-8")
			return c.Status(http.StatusServiceUnavailable).SendString(buildHint)
		}
		return send(c, "index.html", body)
	}
}

func readFile(fsys fs.FS, name string) ([]byte, error) {
	info, err := fs.Stat(fsys, name)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fs.ErrNotExist
	}
	return fs.ReadFile(fsys, name)
}

func send(c fiber.Ctx, name string, body []byte) error {
	ctype := mime.TypeByExtension(path.Ext(name))
	if ctype == "" {
		ctype = http.DetectContentType(body)
	}
	c.Set(fiber.HeaderContentType, ctype)
	if strings.HasPrefix(name, "assets/") {
		c.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
	} else {
		c.Set(fiber.HeaderCacheControl, "no-cache")
	}
	return c.Send(body)
}
