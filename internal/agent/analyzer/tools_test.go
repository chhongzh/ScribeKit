package analyzer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/chhongzh/scribekit/internal/agent"
	"github.com/chhongzh/scribekit/internal/llm"
	"github.com/chhongzh/scribekit/internal/logging"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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
			core, logs := observer.New(zap.InfoLevel)
			restore := zap.ReplaceGlobals(zap.New(core))
			defer restore()
			ctx := callbacks.InitCallbacks(context.Background(), nil, logging.EinoHandler())
			ctx = agent.WithUsageSink(ctx, func(*schema.TokenUsage) {})
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
			if logs.FilterMessage("ai.failed").Len() != 0 {
				t.Fatalf("normal tool completion was logged as a failure: %+v", logs.FilterMessage("ai.failed").All())
			}
		})
	}
}

type recoveringHalfModel struct {
	testing   *testing.T
	arguments string
	rounds    int
	broken    error
}

func (fake *recoveringHalfModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return fake, nil
}

func (fake *recoveringHalfModel) Generate(_ context.Context, messages []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	fake.rounds++
	if fake.broken != nil {
		return nil, fake.broken
	}
	if fake.rounds == 1 {
		return schema.AssistantMessage("", []schema.ToolCall{
			{ID: "valid", Type: "function", Function: schema.FunctionCall{Name: "submit_half", Arguments: `{"index":0,"desc":["action"]}`}},
			{ID: "invalid", Type: "function", Function: schema.FunctionCall{Name: "submit_half", Arguments: fake.arguments}},
		}), nil
	}
	if fake.rounds > 2 {
		return nil, errors.New("unexpected summary round")
	}
	found := false
	for _, message := range messages {
		if message.Role != schema.Tool || message.ToolCallID != "invalid" {
			continue
		}
		var feedback halfFeedback
		if err := json.Unmarshal([]byte(message.Content), &feedback); err != nil {
			fake.testing.Fatal(err)
		}
		if feedback.Error == "" || feedback.Done || !slices.Contains(feedback.Remaining, 1) {
			fake.testing.Fatalf("malformed arguments did not produce actionable feedback: %+v", feedback)
		}
		found = true
	}
	if !found {
		fake.testing.Fatal("tool feedback missing from retry context")
	}
	return schema.AssistantMessage("", []schema.ToolCall{{ID: "repair", Type: "function", Function: schema.FunctionCall{Name: "submit_half", Arguments: `{"index":1}`}}}), nil
}

func (fake *recoveringHalfModel) Stream(ctx context.Context, messages []*schema.Message, options ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	message, err := fake.Generate(ctx, messages, options...)
	if err != nil {
		return nil, err
	}
	var chunks []*schema.Message
	for index, call := range message.ToolCalls {
		arguments := call.Function.Arguments
		call.Index = &index
		call.Function.Arguments = arguments[:len(arguments)/2]
		chunks = append(chunks, schema.AssistantMessage("", []schema.ToolCall{call}))
		chunks = append(chunks, schema.AssistantMessage("", []schema.ToolCall{{Index: &index, Function: schema.FunctionCall{Arguments: arguments[len(arguments)/2:]}}}))
	}
	return schema.StreamReaderFromArray(chunks), nil
}

func TestHalfAgentRecoversMalformedArguments(t *testing.T) {
	for _, arguments := range []string{`{"index":1,"desc":[`, `{"index":"1"}`, "", `null`, `[]`} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%v", arguments, streaming), func(t *testing.T) {
				fake := &recoveringHalfModel{testing: t, arguments: arguments}
				service := &Service{sem: make(chan struct{}, 1)}
				ctx := context.Background()
				if streaming {
					ctx = agent.WithUsageSink(ctx, func(*schema.TokenUsage) {})
				}
				var partials int
				result, err := service.analyzeHalves(ctx, llm.Handle{Model: fake}, []*schema.Message{schema.UserMessage("分析半句")}, "推门，走了。", func(*rawParagraph) { partials++ })
				if err != nil {
					t.Fatal(err)
				}
				if fake.rounds != 2 || partials != 2 || len(result.Halves) != 2 || result.Halves[0].Desc[0] != "action" {
					t.Fatalf("successful submissions were lost or invalid ones published: rounds=%d partials=%d result=%+v", fake.rounds, partials, result)
				}
			})
		}
	}
}

func TestHalfAgentPreservesTransportFailure(t *testing.T) {
	wanted := errors.New("connection reset")
	fake := &recoveringHalfModel{testing: t, broken: wanted}
	service := &Service{sem: make(chan struct{}, 1)}
	result, err := service.analyzeHalves(context.Background(), llm.Handle{Model: fake}, []*schema.Message{schema.UserMessage("分析")}, "推门，走了。", nil)
	if !errors.Is(err, wanted) || result != nil || fake.rounds != 1 {
		t.Fatalf("transport failure must propagate: result=%+v rounds=%d err=%v", result, fake.rounds, err)
	}
}

func TestHalfArgumentsRespectCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := unmarshalHalfSubmission(ctx, `{"index":`); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was treated as repairable arguments: %v", err)
	}
}
