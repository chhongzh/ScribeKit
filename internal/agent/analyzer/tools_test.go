package analyzer

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/chhongzh/scribekit/internal/agent"
	"github.com/chhongzh/scribekit/internal/llm"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestHalfCollectorValidatesAndRepairs(t *testing.T) {
	collector := newHalfCollector("𠀀推门，他走了。")
	first, second := 0, 1
	for _, input := range []halfSubmission{
		{},
		{Index: &first, Desc: []string{"unknown"}},
		{Index: &first, Marks: []halfMark{{Text: "不存在", Kind: "keyword"}}},
		{Index: &second, Relations: []halfRelation{{From: 1, Kind: "sequence"}}},
		{Index: &first, Marks: []halfMark{{Text: "推门", Kind: "keyword", Ref: &rawRef{Half: 4, Text: "他"}}}},
		{Index: &first, Diagnostics: []halfDiagnostic{{Text: "不存在", Severity: "hint", Message: "事实"}}},
	} {
		feedback, err := collector.submit(context.Background(), input)
		if err != nil || feedback.Error == "" || feedback.Done {
			t.Fatalf("invalid submission accepted: %+v, %v", feedback, err)
		}
	}
	if _, err := collector.result(); err == nil {
		t.Fatal("incomplete analysis must not be returned or cached")
	}
	feedback, err := collector.submit(context.Background(), halfSubmission{
		Index: &second, Desc: []string{"action"},
		Relations: []halfRelation{{From: 0, Kind: "sequence"}},
		Marks:     []halfMark{{Text: "他", Kind: "pronoun", Ref: &rawRef{Half: 0, Text: "𠀀"}}},
	})
	if err != nil || feedback.Error != "" || len(feedback.Remaining) != 1 || feedback.Remaining[0] != 0 {
		t.Fatalf("wrong feedback: %+v, %v", feedback, err)
	}
	collector.submit(context.Background(), halfSubmission{Index: &first, Desc: []string{"narration"}})
	collector.submit(context.Background(), halfSubmission{Index: &first, Desc: []string{"action"}})
	raw, err := collector.result()
	if err != nil {
		t.Fatal(err)
	}
	result := buildParagraph(7, "𠀀推门，他走了。", "fake", raw)
	if len(result.Halves) != 2 || result.Halves[0].Desc[0] != "action" || len(result.Relations) != 1 {
		t.Fatalf("incorrect merged analysis: %+v", result)
	}
	if len(result.Marks) != 1 || result.Marks[0].Start != 4 || result.Marks[0].Ref.End != 1 {
		t.Fatalf("incorrect Unicode offsets: %+v", result.Marks)
	}
}

func TestHalfCollectorConcurrentCalls(t *testing.T) {
	collector := newHalfCollector(strings.Repeat("半句，", 30))
	var workers sync.WaitGroup
	for index := range collector.halves {
		workers.Go(func() {
			if _, err := collector.submit(context.Background(), halfSubmission{Index: &index}); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	if _, err := collector.result(); err != nil {
		t.Fatal(err)
	}
}

type halfModel struct {
	testing *testing.T
	rounds  int
	repair  bool
	streams int
}

func (fake *halfModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	if len(tools) != 1 || tools[0].Name != "submit_half" {
		fake.testing.Fatalf("expected submit_half tool, got %+v", tools)
	}
	return fake, nil
}

func (fake *halfModel) Generate(_ context.Context, messages []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	fake.rounds++
	if fake.rounds > 2 {
		return nil, fmt.Errorf("unexpected model summary round")
	}
	if fake.repair && fake.rounds == 1 {
		return schema.AssistantMessage("", []schema.ToolCall{{ID: "bad", Type: "function", Function: schema.FunctionCall{Name: "submit_half", Arguments: `{"index":0,"desc":["invalid"]}`}}}), nil
	}
	if fake.repair {
		found := false
		for _, message := range messages {
			if message.Role == schema.Tool && strings.Contains(message.Content, "remaining") && strings.Contains(message.Content, "error") {
				found = true
			}
		}
		if !found {
			fake.testing.Fatal("agent must receive tool feedback before retry")
		}
	}
	return schema.AssistantMessage("", []schema.ToolCall{
		{ID: "first", Type: "function", Function: schema.FunctionCall{Name: "submit_half", Arguments: `{"index":0,"desc":["action"]}`}},
		{ID: "second", Type: "function", Function: schema.FunctionCall{Name: "submit_half", Arguments: `{"index":1}`}},
	}), nil
}

func (fake *halfModel) Stream(ctx context.Context, messages []*schema.Message, options ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	fake.streams++
	message, err := fake.Generate(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	for index := range message.ToolCalls {
		message.ToolCalls[index].Index = &index
	}
	return schema.StreamReaderFromArray([]*schema.Message{message}), nil
}

func TestHalfAgentExitsAfterToolsAndRepairsLocally(t *testing.T) {
	for _, repair := range []bool{false, true} {
		t.Run(fmt.Sprint(repair), func(t *testing.T) {
			fake := &halfModel{testing: t, repair: repair}
			service := &Service{sem: make(chan struct{}, 1)}
			ctx := agent.WithUsageSink(context.Background(), func(*schema.TokenUsage) {})
			result, err := service.analyzeHalves(ctx, llm.Handle{Model: fake, Name: "fake"}, []*schema.Message{schema.UserMessage("分析半句")}, "推门，走了。", nil)
			if err != nil {
				t.Fatal(err)
			}
			wantedRounds := 1
			if repair {
				wantedRounds = 2
			}
			if len(result.Halves) != 2 || fake.rounds != wantedRounds || fake.streams != wantedRounds {
				t.Fatalf("unexpected rounds or coverage: %d, %+v", fake.rounds, result)
			}
		})
	}
}
