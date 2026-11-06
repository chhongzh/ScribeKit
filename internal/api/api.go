// Package api 注册 /api 下的 REST 路由、统一错误格式和静态资源服务。
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/chhongzh/scribekit/internal/apperr"
	"github.com/chhongzh/scribekit/internal/guide"
	"github.com/chhongzh/scribekit/internal/llm"
	"github.com/chhongzh/scribekit/internal/store"
)

// 素材上传是整文件读进内存，限制在 16 MiB 以内足够纯文本使用。
const BodyLimit = 16 << 20

type Deps struct {
	Store  *store.Store
	Models *llm.Manager
	Guides *guide.Library
	// WS 是 /api/ws 的握手处理器。
	WS fiber.Handler
}

type errorBody struct {
	Error *apperr.Error `json:"error"`
}

func status(code string) int {
	switch code {
	case apperr.NotFound:
		return http.StatusNotFound
	case apperr.Conflict:
		return http.StatusConflict
	case apperr.Internal:
		return http.StatusInternalServerError
	default:
		return http.StatusBadRequest
	}
}

// ErrorHandler 把所有错误统一成 {error:{code,message}}。
func ErrorHandler(c fiber.Ctx, err error) error {
	var ae *apperr.Error
	var fe *fiber.Error
	switch {
	case errors.As(err, &ae):
		return c.Status(status(ae.Code)).JSON(errorBody{ae})
	case errors.As(err, &fe):
		code := apperr.BadRequest
		if fe.Code == http.StatusNotFound {
			code = apperr.NotFound
		}
		return c.Status(fe.Code).JSON(errorBody{&apperr.Error{Code: code, Message: fe.Message}})
	}
	slog.Error("request failed", "method", c.Method(), "path", c.Path(), "err", err)
	return c.Status(http.StatusInternalServerError).JSON(errorBody{apperr.From(err)})
}

type server struct {
	Deps
}

// Register 挂载 /api 路由；调用方负责之后挂静态资源兜底。
func Register(app *fiber.App, d Deps) {
	s := &server{d}
	api := app.Group("/api")
	if d.WS != nil {
		api.Get("/ws", d.WS)
	}

	api.Get("/collections", s.listCollections)
	api.Post("/collections", s.createCollection)
	api.Get("/collections/:id", s.getCollection)
	api.Patch("/collections/:id", s.patchCollection)
	api.Delete("/collections/:id", s.deleteCollection)

	api.Get("/articles", s.listArticles)
	api.Post("/articles", s.createArticle)
	api.Get("/articles/:id", s.getArticle)
	api.Patch("/articles/:id", s.patchArticle)
	api.Delete("/articles/:id", s.deleteArticle)
	api.Get("/articles/:id/export", s.exportArticle)
	api.Get("/articles/:id/suggestions", s.listSuggestions)
	api.Post("/articles/:id/paragraphs", s.createParagraph)

	api.Patch("/paragraphs/:id", s.patchParagraph)
	api.Post("/paragraphs/:id/move", s.moveParagraph)
	api.Delete("/paragraphs/:id", s.deleteParagraph)
	api.Post("/paragraphs/:id/split", s.splitParagraph)
	api.Post("/paragraphs/:id/merge-next", s.mergeNext)

	api.Get("/materials", s.listMaterials)
	api.Post("/materials", s.createMaterial)
	api.Post("/materials/upload", s.uploadMaterial)
	api.Delete("/materials/:id", s.deleteMaterial)

	api.Patch("/suggestions/:id", s.patchSuggestion)
	api.Post("/suggestions/:id/apply", s.applySuggestion)
	api.Post("/suggestions/:id/reject", s.rejectSuggestion)

	api.Get("/guides", s.listGuides)
	api.Get("/guides/:id", s.getGuide)

	api.Get("/settings", s.getSettings)
	api.Put("/settings", s.putSettings)
	api.Post("/settings/test", s.testSettings)

	api.All("/*", func(c fiber.Ctx) error {
		return apperr.New(apperr.NotFound, "接口 %s %s 不存在", c.Method(), c.Path())
	})
}

func bad(format string, args ...any) error { return apperr.New(apperr.BadRequest, format, args...) }

func idParam(c fiber.Ctx) (int, error) {
	id, err := strconv.Atoi(c.Params("id"))
	if err != nil || id <= 0 {
		return 0, bad("id %q 不合法", c.Params("id"))
	}
	return id, nil
}

func optionalInt(c fiber.Ctx, key string) (*int, error) {
	raw := c.Query(key)
	if raw == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil, bad("参数 %s 不是整数", key)
	}
	return &n, nil
}

func parse(c fiber.Ctx, dst any) error {
	if err := json.Unmarshal(c.Body(), dst); err != nil {
		return bad("请求体不是合法的 JSON: %v", err)
	}
	return nil
}

func reqCtx(c fiber.Ctx) context.Context { return c.Context() }

// ---- 文集 ----

func (s *server) listCollections(c fiber.Ctx) error {
	out, err := s.Store.ListCollections(reqCtx(c))
	if err != nil {
		return err
	}
	return c.JSON(out)
}

func (s *server) createCollection(c fiber.Ctx) error {
	var in store.CollectionInput
	if err := parse(c, &in); err != nil {
		return err
	}
	out, err := s.Store.CreateCollection(reqCtx(c), in)
	if err != nil {
		return err
	}
	return c.Status(http.StatusCreated).JSON(out)
}

func (s *server) getCollection(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	out, err := s.Store.GetCollection(reqCtx(c), id)
	if err != nil {
		return err
	}
	return c.JSON(out)
}

func (s *server) patchCollection(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	var p store.CollectionPatch
	if err := parse(c, &p); err != nil {
		return err
	}
	out, err := s.Store.UpdateCollection(reqCtx(c), id, p)
	if err != nil {
		return err
	}
	return c.JSON(out)
}

func (s *server) deleteCollection(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	if err := s.Store.DeleteCollection(reqCtx(c), id); err != nil {
		return err
	}
	return c.SendStatus(http.StatusNoContent)
}

// ---- 文章 ----

func (s *server) listArticles(c fiber.Ctx) error {
	cid, err := optionalInt(c, "collectionId")
	if err != nil {
		return err
	}
	out, err := s.Store.ListArticles(reqCtx(c), store.ArticleFilter{CollectionID: cid, LooseOnly: c.Query("loose") == "1"})
	if err != nil {
		return err
	}
	return c.JSON(out)
}

func (s *server) createArticle(c fiber.Ctx) error {
	var in store.ArticleInput
	if err := parse(c, &in); err != nil {
		return err
	}
	out, err := s.Store.CreateArticle(reqCtx(c), in)
	if err != nil {
		return err
	}
	return c.Status(http.StatusCreated).JSON(out)
}

func (s *server) getArticle(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	out, err := s.Store.GetArticle(reqCtx(c), id)
	if err != nil {
		return err
	}
	return c.JSON(out)
}

func (s *server) patchArticle(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	var p store.ArticlePatch
	if err := parse(c, &p); err != nil {
		return err
	}
	out, err := s.Store.UpdateArticle(reqCtx(c), id, p)
	if err != nil {
		return err
	}
	return c.JSON(out)
}

func (s *server) deleteArticle(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	if err := s.Store.DeleteArticle(reqCtx(c), id); err != nil {
		return err
	}
	return c.SendStatus(http.StatusNoContent)
}

// ---- 段落 ----

func (s *server) createParagraph(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	var in struct {
		AfterID store.Patch[int] `json:"afterId"`
		Kind    string           `json:"kind"`
		Text    string           `json:"text"`
	}
	if err := parse(c, &in); err != nil {
		return err
	}
	out, err := s.Store.InsertParagraph(reqCtx(c), id, in.AfterID, in.Kind, in.Text)
	if err != nil {
		return err
	}
	return c.Status(http.StatusCreated).JSON(out)
}

func (s *server) patchParagraph(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	var p store.ParagraphPatch
	if err := parse(c, &p); err != nil {
		return err
	}
	out, err := s.Store.UpdateParagraph(reqCtx(c), id, p)
	if err != nil {
		return err
	}
	return c.JSON(out)
}

func (s *server) moveParagraph(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	var in struct {
		AfterID *int `json:"afterId"`
	}
	if err := parse(c, &in); err != nil {
		return err
	}
	out, err := s.Store.MoveParagraph(reqCtx(c), id, in.AfterID)
	if err != nil {
		return err
	}
	return c.JSON(out)
}

func (s *server) deleteParagraph(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	if err := s.Store.DeleteParagraph(reqCtx(c), id); err != nil {
		return err
	}
	return c.SendStatus(http.StatusNoContent)
}

func (s *server) splitParagraph(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	var in struct {
		Offset *int `json:"offset"`
	}
	if err := parse(c, &in); err != nil {
		return err
	}
	if in.Offset == nil {
		return bad("缺少 offset")
	}
	out, err := s.Store.SplitParagraph(reqCtx(c), id, *in.Offset)
	if err != nil {
		return err
	}
	return c.JSON(out)
}

func (s *server) mergeNext(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	out, err := s.Store.MergeNext(reqCtx(c), id)
	if err != nil {
		return err
	}
	return c.JSON(out)
}

// ---- 建议 ----

func (s *server) listSuggestions(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	if _, err := s.Store.GetArticleMeta(reqCtx(c), id); err != nil {
		return err
	}
	out, err := s.Store.ListSuggestions(reqCtx(c), id, c.Query("status"))
	if err != nil {
		return err
	}
	return c.JSON(out)
}

func (s *server) patchSuggestion(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	var p store.SuggestionPatch
	if err := parse(c, &p); err != nil {
		return err
	}
	out, err := s.Store.PatchSuggestion(reqCtx(c), id, p)
	if err != nil {
		return err
	}
	return c.JSON(out)
}

func (s *server) applySuggestion(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	var in struct {
		Text string `json:"text"`
	}
	if err := parse(c, &in); err != nil {
		return err
	}
	out, err := s.Store.ApplySuggestion(reqCtx(c), id, in.Text)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"paragraphs": out})
}

func (s *server) rejectSuggestion(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	out, err := s.Store.RejectSuggestion(reqCtx(c), id)
	if err != nil {
		return err
	}
	return c.JSON(out)
}

// ---- 指引 ----

func (s *server) listGuides(c fiber.Ctx) error { return c.JSON(s.Guides.List()) }

func (s *server) getGuide(c fiber.Ctx) error {
	g, ok := s.Guides.Get(c.Params("id"))
	if !ok {
		return apperr.New(apperr.NotFound, "指引 %s 不存在", c.Params("id"))
	}
	return c.JSON(g)
}

// ---- 设置 ----

type settingsView struct {
	Provider        string          `json:"provider"`
	BaseURL         string          `json:"baseUrl"`
	APIKeySet       bool            `json:"apiKeySet"`
	AnalyzerModel   string          `json:"analyzerModel"`
	ScaffolderModel string          `json:"scaffolderModel"`
	AutoAnalyze     bool            `json:"autoAnalyze"`
	Theme           json.RawMessage `json:"theme"`
}

func viewOf(s store.Settings) settingsView {
	return settingsView{
		Provider: s.Provider, BaseURL: s.BaseURL, APIKeySet: s.APIKey != "",
		AnalyzerModel: s.AnalyzerModel, ScaffolderModel: s.ScaffolderModel,
		AutoAnalyze: s.AutoAnalyze, Theme: s.Theme,
	}
}

func (s *server) getSettings(c fiber.Ctx) error {
	cur, err := s.Store.GetSettings(reqCtx(c))
	if err != nil {
		return err
	}
	return c.JSON(viewOf(cur))
}

func (s *server) putSettings(c fiber.Ctx) error {
	cur, err := s.Store.GetSettings(reqCtx(c))
	if err != nil {
		return err
	}
	// 在现有值上叠加请求体，缺省字段保持原值；apiKey 为空串表示不修改。
	next := cur
	if err := parse(c, &next); err != nil {
		return err
	}
	saved, err := s.Store.SaveSettings(reqCtx(c), next)
	if err != nil {
		return err
	}
	return c.JSON(viewOf(saved))
}

func (s *server) testSettings(c fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(reqCtx(c), 30*time.Second)
	defer cancel()
	msg, err := s.Models.Test(ctx)
	if err != nil {
		return c.JSON(fiber.Map{"ok": false, "message": err.Error()})
	}
	return c.JSON(fiber.Map{"ok": true, "message": msg})
}
