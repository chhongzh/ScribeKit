package api

import (
	"bytes"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"github.com/chhongzh/scribekit/internal/store"
)

func (s *server) listMaterials(c fiber.Ctx) error {
	cid, err := optionalInt(c, "collectionId")
	if err != nil {
		return err
	}
	aid, err := optionalInt(c, "articleId")
	if err != nil {
		return err
	}
	out, err := s.Store.ListMaterials(reqCtx(c), store.MaterialFilter{CollectionID: cid, ArticleID: aid})
	if err != nil {
		return err
	}
	return c.JSON(out)
}

func (s *server) createMaterial(c fiber.Ctx) error {
	var in store.MaterialInput
	if err := parse(c, &in); err != nil {
		return err
	}
	out, err := s.Store.CreateMaterial(reqCtx(c), in)
	if err != nil {
		return err
	}
	return c.Status(http.StatusCreated).JSON(out)
}

func (s *server) deleteMaterial(c fiber.Ctx) error {
	id, err := idParam(c)
	if err != nil {
		return err
	}
	if err := s.Store.DeleteMaterial(reqCtx(c), id); err != nil {
		return err
	}
	return c.SendStatus(http.StatusNoContent)
}

func formInt(c fiber.Ctx, key string) (*int, error) {
	raw := strings.TrimSpace(c.FormValue(key))
	if raw == "" || raw == "null" {
		return nil, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil, bad("参数 %s 不是整数", key)
	}
	return &n, nil
}

func (s *server) uploadMaterial(c fiber.Ctx) error {
	fh, err := c.FormFile("file")
	if err != nil {
		return bad("缺少上传文件 file")
	}
	ext := strings.ToLower(filepath.Ext(fh.Filename))
	if ext != ".txt" && ext != ".md" {
		return bad("只支持 .txt 和 .md 文件")
	}
	f, err := fh.Open()
	if err != nil {
		return err
	}
	defer f.Close()
	raw, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	text, err := DecodeText(raw)
	if err != nil {
		return bad("无法识别文件编码，请转成 UTF-8 或 GB18030 后重试")
	}
	cid, err := formInt(c, "collectionId")
	if err != nil {
		return err
	}
	aid, err := formInt(c, "articleId")
	if err != nil {
		return err
	}
	out, err := s.Store.CreateMaterial(reqCtx(c), store.MaterialInput{
		Name: fh.Filename, Content: text, CollectionID: cid, ArticleID: aid,
	})
	if err != nil {
		return err
	}
	return c.Status(http.StatusCreated).JSON(out)
}

// DecodeText 识别 BOM、UTF-8 和 GB18030（含 GBK），统一换行为 \n。
func DecodeText(raw []byte) (string, error) {
	var text string
	switch {
	case bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}):
		text = string(raw[3:])
	case bytes.HasPrefix(raw, []byte{0xFF, 0xFE}), bytes.HasPrefix(raw, []byte{0xFE, 0xFF}):
		dec := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewDecoder()
		out, _, err := transform.Bytes(dec, raw)
		if err != nil {
			return "", err
		}
		text = string(out)
	case utf8.Valid(raw):
		text = string(raw)
	default:
		out, _, err := transform.Bytes(simplifiedchinese.GB18030.NewDecoder(), raw)
		if err != nil {
			return "", err
		}
		text = string(out)
	}
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n"), nil
}
