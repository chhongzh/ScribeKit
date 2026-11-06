package api

import (
	"github.com/gofiber/fiber/v3"

	"github.com/chhongzh/scribekit/internal/export"
)

const docxType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

func (s *server) exportArticle(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	format := c.Query("format", "md")
	if format != "md" && format != "docx" {
		return bad("format 只能是 md 或 docx")
	}
	art, err := s.Store.GetArticle(reqCtx(c), id)
	if err != nil {
		return err
	}
	doc := export.Article{Title: art.Title, Paragraphs: make([]export.Paragraph, len(art.Paragraphs))}
	for i, p := range art.Paragraphs {
		doc.Paragraphs[i] = export.Paragraph{Text: p.Text}
	}
	name := export.FileName(art.Title)

	var (
		body []byte
		ctyp string
		ext  string
	)
	switch format {
	case "docx":
		body, err = export.Docx(doc)
		if err != nil {
			return err
		}
		ctyp, ext = docxType, ".docx"
	default:
		body, ctyp, ext = export.Markdown(doc), "text/markdown; charset=utf-8", ".md"
	}
	c.Set(fiber.HeaderContentType, ctyp)
	c.Set(fiber.HeaderContentDisposition, export.ContentDisposition(name, ext))
	return c.Send(body)
}
