// Package guide 加载内嵌的写作指引语料，并按文体和小节取出给 Scaffolder / Analyzer 的条目。
package guide

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed all:data
var dataFS embed.FS

type Source struct {
	Title string `yaml:"title" json:"title"`
	URL   string `yaml:"url" json:"url"`
}

type Guide struct {
	ID      string   `yaml:"id" json:"id"`
	Title   string   `yaml:"title" json:"title"`
	Genres  []string `yaml:"genres" json:"genres"`
	Tags    []string `yaml:"tags" json:"tags"`
	Sources []Source `yaml:"sources" json:"sources"`
	Body    string   `yaml:"-" json:"body"`
}

type Summary struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Genres []string `json:"genres"`
	Tags   []string `json:"tags"`
}

const (
	sectionScaffolder = "用于 Scaffolder"
	sectionAnalyzer   = "用于 Analyzer"
)

type Library struct {
	guides []Guide
	byID   map[string]*Guide
}

// Embedded 加载编译进二进制的语料。
func Embedded() (*Library, error) {
	sub, err := fs.Sub(dataFS, "data")
	if err != nil {
		return nil, err
	}
	return Load(sub)
}

// Load 读取目录下全部带 front matter 的 .md；没有 id 的文件（如 README）被忽略。
func Load(fsys fs.FS) (*Library, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	lib := &Library{}
	seen := map[string]bool{}
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ".md" {
			continue
		}
		raw, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, err
		}
		g, ok, err := parse(string(raw))
		if err != nil {
			return nil, fmt.Errorf("guide %s: %w", e.Name(), err)
		}
		if !ok {
			continue
		}
		if seen[g.ID] {
			return nil, fmt.Errorf("guide %s: duplicate id %q", e.Name(), g.ID)
		}
		seen[g.ID] = true
		lib.guides = append(lib.guides, g)
	}
	sort.Slice(lib.guides, func(i, j int) bool { return lib.guides[i].ID < lib.guides[j].ID })
	lib.byID = make(map[string]*Guide, len(lib.guides))
	for i := range lib.guides {
		lib.byID[lib.guides[i].ID] = &lib.guides[i]
	}
	return lib, nil
}

func parse(raw string) (Guide, bool, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.TrimPrefix(raw, "\ufeff")
	rest, ok := strings.CutPrefix(raw, "---\n")
	if !ok {
		return Guide{}, false, nil
	}
	front, body, ok := strings.Cut(rest, "\n---")
	if !ok {
		return Guide{}, false, fmt.Errorf("front matter 没有结束的 ---")
	}
	var g Guide
	if err := yaml.Unmarshal([]byte(front), &g); err != nil {
		return Guide{}, false, err
	}
	if g.ID == "" {
		return Guide{}, false, nil
	}
	g.Body = strings.TrimSpace(strings.TrimPrefix(body, "\n"))
	return g, true, nil
}

func (l *Library) List() []Summary {
	out := make([]Summary, len(l.guides))
	for i, g := range l.guides {
		out[i] = Summary{ID: g.ID, Title: g.Title, Genres: nonNil(g.Genres), Tags: nonNil(g.Tags)}
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (l *Library) Get(id string) (Guide, bool) {
	g, ok := l.byID[id]
	if !ok {
		return Guide{}, false
	}
	out := *g
	out.Genres, out.Tags = nonNil(out.Genres), nonNil(out.Tags)
	if out.Sources == nil {
		out.Sources = []Source{}
	}
	return out, true
}

// ForGenre 返回适用于该文体的指引；genres 为空的指引适用于所有文体。
func (l *Library) ForGenre(genre string) []Guide {
	var out []Guide
	for _, g := range l.guides {
		if len(g.Genres) == 0 || slices.Contains(g.Genres, genre) {
			out = append(out, g)
		}
	}
	return out
}

// ScaffolderNotes 返回该文体全部指引的“用于 Scaffolder”小节，每条指引一个元素。
func (l *Library) ScaffolderNotes(genre string) []string { return l.notes(genre, sectionScaffolder) }

// AnalyzerNotes 返回该文体全部指引的“用于 Analyzer”小节。
func (l *Library) AnalyzerNotes(genre string) []string { return l.notes(genre, sectionAnalyzer) }

func (l *Library) notes(genre, section string) []string {
	var out []string
	for _, g := range l.ForGenre(genre) {
		if s := g.Section(section); s != "" {
			out = append(out, fmt.Sprintf("【%s】\n%s", g.Title, s))
		}
	}
	return out
}

func (g Guide) ScaffolderSection() string { return g.Section(sectionScaffolder) }

// ScaffolderNote 取单条指引的 Scaffolder 小节，供 read_guide 工具使用。
func (l *Library) ScaffolderNote(id string) (string, bool) {
	g, ok := l.byID[id]
	if !ok {
		return "", false
	}
	return g.Section(sectionScaffolder), true
}

// Section 返回以 “## <name>” 为标题的二级小节正文，不含标题行；标题允许带后缀说明。
func (g Guide) Section(name string) string {
	var buf []string
	in := false
	for _, line := range strings.Split(g.Body, "\n") {
		if rest, ok := strings.CutPrefix(line, "## "); ok {
			if in {
				break
			}
			in = strings.HasPrefix(strings.TrimSpace(rest), name)
			continue
		}
		if in {
			buf = append(buf, line)
		}
	}
	return strings.TrimSpace(strings.Join(buf, "\n"))
}
