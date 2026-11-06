package analyzer

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/chhongzh/scribekit/internal/agent"
	"github.com/chhongzh/scribekit/internal/apperr"
	"github.com/chhongzh/scribekit/internal/llm"
	"github.com/chhongzh/scribekit/internal/prompt"
	"github.com/chhongzh/scribekit/internal/textseg"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/schema"
)

type halfRelation struct {
	From int    `json:"from"`
	Kind string `json:"kind"`
	Note string `json:"note,omitempty"`
}

type halfMark struct {
	Text string  `json:"text"`
	Kind string  `json:"kind"`
	Nth  int     `json:"nth,omitempty"`
	Note string  `json:"note,omitempty"`
	Ref  *rawRef `json:"ref,omitempty"`
}

type halfDiagnostic struct {
	Text     string `json:"text,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type halfSubmission struct {
	Index       *int             `json:"index" jsonschema:"required,description=半句编号，从 0 开始"`
	Desc        []string         `json:"desc,omitempty"`
	Rhetoric    []string         `json:"rhetoric,omitempty"`
	Relations   []halfRelation   `json:"relations,omitempty"`
	Marks       []halfMark       `json:"marks,omitempty"`
	Diagnostics []halfDiagnostic `json:"diagnostics,omitempty"`
}

type halfFeedback struct {
	Error     string `json:"error,omitempty"`
	Remaining []int  `json:"remaining"`
	Done      bool   `json:"done"`
}

type halfCollector struct {
	mu     sync.Mutex
	halves [][]rune
	items  map[int]halfSubmission
}

func newHalfCollector(text string) *halfCollector {
	runes := []rune(text)
	seg := textseg.SegmentRunes(runes)
	collector := &halfCollector{items: make(map[int]halfSubmission)}
	for _, half := range seg.Halves {
		collector.halves = append(collector.halves, runes[half.Start:half.End])
	}
	return collector
}

func (collector *halfCollector) feedback(message string) halfFeedback {
	remaining := []int{}
	for index := range collector.halves {
		if _, ok := collector.items[index]; !ok {
			remaining = append(remaining, index)
		}
	}
	return halfFeedback{Error: message, Remaining: remaining, Done: len(remaining) == 0}
}

func (collector *halfCollector) submit(ctx context.Context, input halfSubmission) (halfFeedback, error) {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return halfFeedback{}, err
	}
	if input.Index == nil || *input.Index < 0 || *input.Index >= len(collector.halves) {
		return collector.feedback("index 必须是给定的半句编号"), nil
	}
	index := *input.Index
	for _, group := range []struct{ values, allowed []string }{{input.Desc, descKinds}, {input.Rhetoric, rhetoricKinds}} {
		for _, value := range group.values {
			if !slices.Contains(group.allowed, value) {
				return collector.feedback(fmt.Sprintf("未知标签 %q，请使用枚举值", value)), nil
			}
		}
	}
	for _, relation := range input.Relations {
		if relation.From < 0 || relation.From >= index || !slices.Contains(relationKinds, relation.Kind) {
			return collector.feedback("relations.from 必须是前面的半句，kind 必须是合法关系枚举"), nil
		}
	}
	for _, mark := range input.Marks {
		_, _, found := locate(collector.halves[index], mark.Text, mark.Nth)
		if !found || !slices.Contains(markKinds, mark.Kind) {
			return collector.feedback("marks.text/nth 必须能定位到本半句原文，kind 必须是合法标记枚举"), nil
		}
		if mark.Ref != nil {
			if mark.Ref.Half < 0 || mark.Ref.Half >= len(collector.halves) {
				return collector.feedback("ref.half 超出范围"), nil
			}
			if _, _, found := locate(collector.halves[mark.Ref.Half], mark.Ref.Text, 1); !found {
				return collector.feedback("ref.text 必须逐字出现在 ref.half 的原文中"), nil
			}
		}
	}
	for _, diagnostic := range input.Diagnostics {
		if !slices.Contains(severities, diagnostic.Severity) || strings.TrimSpace(diagnostic.Message) == "" {
			return collector.feedback("diagnostics 需要合法 severity 和非空 message"), nil
		}
		if diagnostic.Text != "" {
			if _, _, found := locate(collector.halves[index], diagnostic.Text, 1); !found {
				return collector.feedback("diagnostics.text 必须逐字出现在本半句中，或省略以指整半句"), nil
			}
		}
	}
	collector.items[index] = input
	return collector.feedback(""), nil
}

func (collector *halfCollector) result() (*rawParagraph, error) {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	if pending := collector.feedback("").Remaining; len(pending) > 0 {
		return nil, apperr.New(apperr.Internal, "模型未完成半句分析，缺少编号 %v，请重试", pending)
	}
	return collector.snapshot(), nil
}

func (s *Service) analyzeHalves(ctx context.Context, handle llm.Handle, messages []*schema.Message, text string, onHalf func(*rawParagraph)) (*rawParagraph, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	release, err := s.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	collector := newHalfCollector(text)
	var publishMu sync.Mutex
	description, err := prompt.AnalyzerToolDescription()
	if err != nil {
		return nil, err
	}
	submit, err := utils.InferTool("submit_half", description, func(ctx context.Context, input halfSubmission) (halfFeedback, error) {
		publishMu.Lock()
		defer publishMu.Unlock()
		feedback, err := collector.submit(ctx, input)
		if err == nil && feedback.Error == "" && onHalf != nil {
			if partial, partialErr := collector.resultPartial(); partialErr == nil {
				onHalf(partial)
			}
		}
		return feedback, err
	})
	if err != nil {
		return nil, err
	}
	_, err = agent.Run(ctx, agent.Spec{
		Name: "analyzer", Description: "逐半句的只读文本分析", Model: handle.Model,
		Tools: []tool.BaseTool{submit}, MaxIterations: len(collector.halves)*2 + 3,
		StopAfterTools: func() bool {
			collector.mu.Lock()
			defer collector.mu.Unlock()
			return collector.feedback("").Done
		},
	}, messages, nil)
	if err != nil {
		return nil, err
	}
	return collector.result()
}

func (collector *halfCollector) resultPartial() (*rawParagraph, error) {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	return collector.snapshot(), nil
}

func (collector *halfCollector) snapshot() *rawParagraph {
	result := &rawParagraph{}
	for index := range collector.halves {
		input, ok := collector.items[index]
		if !ok {
			continue
		}
		result.Halves = append(result.Halves, HalfInfo{Index: index, Desc: input.Desc, Rhetoric: input.Rhetoric})
		for _, relation := range input.Relations {
			result.Relations = append(result.Relations, Relation{From: relation.From, To: index, Kind: relation.Kind, Note: relation.Note})
		}
		for _, mark := range input.Marks {
			result.Marks = append(result.Marks, rawMark{Half: index, Text: mark.Text, Nth: mark.Nth, Kind: mark.Kind, Note: mark.Note, Ref: mark.Ref})
		}
		for _, diagnostic := range input.Diagnostics {
			result.Diagnostics = append(result.Diagnostics, rawDiagnostic{Half: index, Text: diagnostic.Text, Severity: diagnostic.Severity, Message: diagnostic.Message})
		}
	}
	return result
}
