package textseg

import (
	"reflect"
	"strings"
	"testing"
)

func render(text string) (string, [][]int) {
	rs := []rune(text)
	seg := Segment(text)
	parts := make([]string, len(seg.Halves))
	for i, h := range seg.Halves {
		parts[i] = string(rs[h.Start:h.End])
	}
	var sents [][]int
	for _, s := range seg.Sentences {
		sents = append(sents, s.Halves)
	}
	return strings.Join(parts, "|"), sents
}

func TestSpecVectors(t *testing.T) {
	cases := []struct {
		in    string
		want  string
		sents [][]int
	}{
		{"我走了，他说：“你去吧。”然后", "我走了，|他说：|“你去吧。”|然后", [][]int{{0, 1, 2}, {3}}},
		{"春天、夏天，都好。", "春天、夏天，|都好。", [][]int{{0, 1}}},
		{"啊……他愣住了。", "啊……|他愣住了。", [][]int{{0}, {1}}},
		{"  空格开头，结尾无标点", "空格开头，|结尾无标点", [][]int{{0, 1}}},
		{"山那边\n是海，\n是海", "山那边|是海，|是海", [][]int{{0, 1, 2}}},
		{"他说。\n下一行", "他说。|下一行", [][]int{{0}, {1}}},
		{"", "", nil},
		{"   　", "", nil},
		{"真的吗？！好。", "真的吗？！|好。", [][]int{{0}, {1}}},
		{"他说，。", "他说，。", [][]int{{0}}},
	}
	for _, c := range cases {
		got, sents := render(c.in)
		if got != c.want {
			t.Errorf("%q: got %q want %q", c.in, got, c.want)
		}
		if !reflect.DeepEqual(sents, c.sents) {
			t.Errorf("%q: sentences got %v want %v", c.in, sents, c.sents)
		}
	}
}

func TestLeadingWhitespaceStart(t *testing.T) {
	seg := Segment("  空格开头，结尾无标点")
	if seg.Halves[0].Start != 2 {
		t.Fatalf("start = %d", seg.Halves[0].Start)
	}
}

func TestOffsetsAreCodePoints(t *testing.T) {
	seg := Segment("a😀b，c")
	if seg.Halves[0].End != 4 || seg.Halves[1].Start != 4 {
		t.Fatalf("%+v", seg.Halves)
	}
}

func TestEmptyIsNotNil(t *testing.T) {
	seg := Segment("")
	if seg.Halves == nil || seg.Sentences == nil {
		t.Fatal("nil slices")
	}
}
