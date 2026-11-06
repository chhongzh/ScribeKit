// Package export 把文章导出为 Markdown 或 Docx。分析结果不参与导出。
package export

import (
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

type Paragraph struct {
	Text string
}

type Article struct {
	Title      string
	Paragraphs []Paragraph
}

const untitled = "未命名"

func titleOf(a Article) string {
	if t := strings.TrimSpace(a.Title); t != "" {
		return t
	}
	return untitled
}

// Markdown 输出一级标题加段落；诗歌分行用行尾两个空格表示硬换行。
func Markdown(a Article) []byte {
	var b strings.Builder
	b.WriteString("# " + oneLine(titleOf(a)) + "\n")
	for _, p := range a.Paragraphs {
		text := strings.TrimSpace(normalizeNewlines(p.Text))
		if text == "" {
			continue
		}
		b.WriteString("\n" + strings.ReplaceAll(text, "\n", "  \n") + "\n")
	}
	return []byte(b.String())
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// FileName 把标题变成可作文件名的字符串。
func FileName(title string) string {
	name := strings.Map(func(r rune) rune {
		switch {
		case r < 0x20, strings.ContainsRune(`/\:*?"<>|`, r):
			return '_'
		}
		return r
	}, strings.TrimSpace(title))
	name = strings.Trim(name, ". ")
	if name == "" {
		return untitled
	}
	if utf8.RuneCountInString(name) > 80 {
		name = string([]rune(name)[:80])
	}
	return name
}

// ContentDisposition 同时给出 ASCII 兜底名和 RFC 5987 编码的 UTF-8 文件名。
func ContentDisposition(name, ext string) string {
	full := name + ext
	return fmt.Sprintf(`attachment; filename="article%s"; filename*=UTF-8''%s`, ext, rfc5987(full))
}

func rfc5987(s string) string {
	// url.PathEscape 会保留 !$&'()*+,;=:@ 等字符，RFC 5987 的 attr-char 不全包含它们，统一转义。
	var b strings.Builder
	for _, c := range []byte(s) {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9',
			strings.IndexByte("!#$&+-.^_`|~", c) >= 0:
			b.WriteByte(c)
		default:
			b.WriteString(url.QueryEscape(string([]byte{c})))
		}
	}
	return b.String()
}
