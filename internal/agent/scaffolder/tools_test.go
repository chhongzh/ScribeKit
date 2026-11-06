package scaffolder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/chhongzh/scribekit/internal/agent"
	"github.com/chhongzh/scribekit/internal/guide"
	"github.com/chhongzh/scribekit/internal/prompt"
	"github.com/chhongzh/scribekit/internal/store"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

func registeredScaffolderTools(t *testing.T) []tool.BaseTool {
	t.Helper()
	ctx := context.Background()
	storage, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { storage.Close() })
	collection, err := storage.CreateCollection(ctx, store.CollectionInput{Name: "文集", Outline: "使用第一人称"})
	if err != nil {
		t.Fatal(err)
	}
	article, err := storage.CreateArticle(ctx, store.ArticleInput{Title: "窗前", Synopsis: "清晨推开窗", Genre: "narrative", CollectionID: &collection.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.InsertParagraph(ctx, article.ID, store.Patch[int]{}, "normal", "我推开窗。"); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.CreateArticle(ctx, store.ArticleInput{Title: "门外", Synopsis: "走向门外", Genre: "narrative", CollectionID: &collection.ID}); err != nil {
		t.Fatal(err)
	}
	guides, err := guide.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	tools, err := New(storage, nil, guides).tools(article, collection.Outline)
	if err != nil {
		t.Fatal(err)
	}
	return tools
}

func TestRegisteredNoArgumentToolsAcceptEmptyArguments(t *testing.T) {
	ctx := context.Background()
	registered := map[string]tool.InvokableTool{}
	for _, candidate := range registeredScaffolderTools(t) {
		info, err := candidate.Info(ctx)
		if err != nil {
			t.Fatal(err)
		}
		registered[info.Name] = candidate.(tool.InvokableTool)
	}
	for _, name := range []string{prompt.ToolReadArticle, prompt.ToolListCollection, prompt.ToolReadOutline} {
		t.Run(name, func(t *testing.T) {
			candidate := registered[name]
			if candidate == nil {
				t.Fatalf("tool %s was not registered", name)
			}
			var baseline string
			for _, arguments := range []string{"", " \t\r\n ", "{}"} {
				result, err := candidate.InvokableRun(ctx, arguments)
				if err != nil {
					t.Fatalf("arguments %q: %v", arguments, err)
				}
				if !json.Valid([]byte(result)) {
					t.Fatalf("tool did not return JSON: %q", result)
				}
				if baseline == "" {
					baseline = result
				} else if result != baseline {
					t.Fatalf("empty arguments changed the tool result: %q != %q", result, baseline)
				}
			}
			for _, arguments := range []string{"{", "[]", "{} {}"} {
				if _, err := candidate.InvokableRun(ctx, arguments); err == nil {
					t.Fatalf("malformed arguments %q were accepted", arguments)
				}
			}
		})
	}
}

type emptyReadArticleModel struct {
	rounds        int
	sawToolResult bool
}

func (fake *emptyReadArticleModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	for _, candidate := range tools {
		if candidate.Name == prompt.ToolReadArticle {
			return fake, nil
		}
	}
	return nil, errors.New("read_article was not bound to the model")
}

func (fake *emptyReadArticleModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("usage sink must preserve streaming")
}

func (fake *emptyReadArticleModel) Stream(_ context.Context, messages []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	fake.rounds++
	if fake.rounds == 1 {
		index := 0
		return schema.StreamReaderFromArray([]*schema.Message{{
			Role:         schema.Assistant,
			ToolCalls:    []schema.ToolCall{{Index: &index, ID: "read-current", Type: "function", Function: schema.FunctionCall{Name: prompt.ToolReadArticle}}},
			ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 5, CompletionTokens: 1, TotalTokens: 6}},
		}}), nil
	}
	if fake.rounds != 2 {
		return nil, fmt.Errorf("unexpected model round %d", fake.rounds)
	}
	for _, message := range messages {
		if message.Role != schema.Tool || message.ToolCallID != "read-current" {
			continue
		}
		var article articleView
		if err := json.Unmarshal([]byte(message.Content), &article); err != nil {
			return nil, fmt.Errorf("read_article result: %w", err)
		}
		fake.sawToolResult = article.Title == "窗前" && article.Synopsis == "清晨推开窗" && len(article.Paragraphs) == 1 && article.Paragraphs[0].Text == "我推开窗。"
	}
	if !fake.sawToolResult {
		return nil, errors.New("next model round did not receive the current article")
	}
	return schema.StreamReaderFromArray([]*schema.Message{
		{Role: schema.Assistant, Content: "先补充窗外的声音，"},
		{Role: schema.Assistant, Content: "再写人物的反应。", ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 7, CompletionTokens: 2, TotalTokens: 9}}},
	}), nil
}

func TestStreamingAgentContinuesAfterReadArticleWithMissingArguments(t *testing.T) {
	fake := &emptyReadArticleModel{}
	var used schema.TokenUsage
	missingUsage := 0
	ctx := agent.WithUsageSink(context.Background(), func(usage *schema.TokenUsage) {
		if usage == nil {
			missingUsage++
			return
		}
		used.PromptTokens += usage.PromptTokens
		used.CompletionTokens += usage.CompletionTokens
		used.TotalTokens += usage.TotalTokens
	})
	result, err := agent.Run(ctx, agent.Spec{Name: "scaffolder", Model: fake, Tools: registeredScaffolderTools(t), MaxIterations: 3}, []*schema.Message{schema.UserMessage("给本段建议")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != "先补充窗外的声音，再写人物的反应。" || fake.rounds != 2 || !fake.sawToolResult {
		t.Fatalf("tool call did not complete into advice: result=%q rounds=%d toolResult=%v", result, fake.rounds, fake.sawToolResult)
	}
	if used.PromptTokens != 12 || used.CompletionTokens != 3 || used.TotalTokens != 15 || missingUsage != 0 {
		t.Fatalf("stream usage was lost or duplicated: usage=%+v missing=%d", used, missingUsage)
	}
}
