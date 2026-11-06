package prompt

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, name string, msgs []*schema.Message, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString("=== " + string(m.Role) + " ===\n" + m.Content + "\n")
	}
	got := b.String()
	path := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (go test -update)", path)
	}
	if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
		t.Errorf("%s differs from golden (go test -update to rewrite)", name)
	}
}

func TestAnalyzer(t *testing.T) {
	base := AnalyzerParagraphVars{
		ArticleTitle: "秋天的菜市场", Synopsis: "写外婆买菜的清晨",
		Genre: "narrative", Subgenre: "写人记叙文", ParagraphKind: "normal",
		Halves: []NumberedHalf{{0, "外婆提着竹篮，"}, {1, "走进雾里。"}},
	}
	msgs, err := AnalyzerParagraph(base)
	golden(t, "analyzer_paragraph_narrative", msgs, err)

	arg := base
	arg.Genre, arg.Subgenre, arg.ParagraphKind = "argumentative", "", "opening"
	arg.GuideNotes = []string{"- 论点句通常带有判断词。"}
	msgs, err = AnalyzerParagraph(arg)
	golden(t, "analyzer_paragraph_argumentative", msgs, err)

	msgs, err = AnalyzerArticle(AnalyzerArticleVars{
		ArticleTitle: "论坚持", Genre: "argumentative",
		Paragraphs: []ArticleParagraph{{1, "opening", "坚持是一种能力。"}},
		GuideNotes: []string{"- 注意引议联结。"},
	})
	golden(t, "analyzer_article", msgs, err)

	msgs, err = AnalyzerRepair(AnalyzerRepairVars{Error: "invalid character"})
	golden(t, "analyzer_repair", msgs, err)
}

func TestScaffolder(t *testing.T) {
	base := ScaffolderVars{
		Title: "秋天的菜市场", Synopsis: "写外婆买菜的清晨", Genre: "narrative",
		ParagraphKind: "normal", TargetKind: "normal",
		HasCursor: true, Before: "外婆提着竹篮，", ParagraphText: "外婆提着竹篮，",
	}
	cases := map[string]func(v *ScaffolderVars){
		"complete": func(v *ScaffolderVars) { v.Mode = "complete" },
		"generate_new_lyric": func(v *ScaffolderVars) {
			v.Mode, v.NewParagraph, v.NewKind, v.TargetKind = "generate", true, "lyric", "lyric"
			v.Instruction, v.CollectionName, v.Outline = "补一段环境描写", "外婆系列", "语言朴素"
			v.PrevParagraph, v.NextParagraph = "前一段", "后一段"
			v.GuideNotes = []string{"- 写动作时用具体动词。"}
		},
		"polish": func(v *ScaffolderVars) { v.Mode, v.Selection, v.HasCursor = "polish", "走进雾里", false },
		"detail": func(v *ScaffolderVars) { v.Mode, v.Selection, v.HasCursor = "detail", "走进雾里", false },
		"advise": func(v *ScaffolderVars) { v.Mode = "advise" },
		"downgraded_transition": func(v *ScaffolderVars) {
			v.Mode, v.Downgraded, v.TargetKind = "advise", true, "transition"
		},
		"forced_callback": func(v *ScaffolderVars) { v.Mode, v.Force, v.TargetKind = "generate", true, "callback" },
		"opening":         func(v *ScaffolderVars) { v.Mode, v.TargetKind = "generate", "opening" },
		"closing":         func(v *ScaffolderVars) { v.Mode, v.TargetKind = "generate", "closing" },
		"lyric":           func(v *ScaffolderVars) { v.Mode, v.TargetKind = "generate", "lyric" },
	}
	for name, mod := range cases {
		t.Run(name, func(t *testing.T) {
			v := base
			mod(&v)
			msgs, err := Scaffolder(v)
			golden(t, "scaffolder_"+name, msgs, err)
		})
	}
}

func TestToolDescriptions(t *testing.T) {
	for _, n := range toolNames {
		if ToolDescription(n) == "" {
			t.Errorf("empty description for %s", n)
		}
	}
}
