package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestReadAssistantReportsStreamUsageDeltasWithoutDoubleCounting(t *testing.T) {
	reader, writer := schema.Pipe[*schema.Message](3)
	writer.Send(&schema.Message{Content: "first", ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7}}}, nil)
	writer.Send(&schema.Message{Content: "second", ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}}}, nil)
	writer.Send(&schema.Message{ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}}}, nil)
	writer.Close()
	var reports []*schema.TokenUsage
	text, hasTools, err := readAssistant(&adk.MessageVariant{IsStreaming: true, MessageStream: reader}, nil, func(usage *schema.TokenUsage) { reports = append(reports, usage) })
	if err != nil || hasTools || text != "firstsecond" {
		t.Fatalf("unexpected assistant result: text=%q tools=%v err=%v", text, hasTools, err)
	}
	if len(reports) != 2 || reports[0] == nil || reports[1] == nil {
		t.Fatalf("expected two usage deltas, got %v", reports)
	}
	usage := &schema.TokenUsage{
		PromptTokens:     reports[0].PromptTokens + reports[1].PromptTokens,
		CompletionTokens: reports[0].CompletionTokens + reports[1].CompletionTokens,
		TotalTokens:      reports[0].TotalTokens + reports[1].TotalTokens,
	}
	if usage.PromptTokens != 5 || usage.CompletionTokens != 3 || usage.TotalTokens != 8 {
		t.Fatalf("cumulative stream usage was counted more than once: %+v", usage)
	}
}

type usageStreamModel struct {
	reader *schema.StreamReader[*schema.Message]
}

func (fake *usageStreamModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return fake, nil
}

func (fake *usageStreamModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("usage reporting must use streaming")
}

func (fake *usageStreamModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return fake.reader, nil
}

func TestRunReportsUsageBeforeStreamCompletesWithoutTextCallback(t *testing.T) {
	reader, writer := schema.Pipe[*schema.Message](2)
	closeStream := sync.OnceFunc(writer.Close)
	defer closeStream()
	reports := make(chan schema.TokenUsage, 8)
	ctx := WithUsageSink(context.Background(), func(usage *schema.TokenUsage) {
		if usage != nil {
			reports <- *usage
		}
	})
	done := make(chan error, 1)
	go func() {
		_, err := Run(ctx, Spec{Name: "usage-test", Model: &usageStreamModel{reader: reader}}, []*schema.Message{schema.UserMessage("test")}, nil)
		done <- err
	}()
	writer.Send(&schema.Message{Role: schema.Assistant, Content: "first", ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 5, CompletionTokens: 2}}}, nil)
	select {
	case usage := <-reports:
		if usage.PromptTokens != 5 || usage.CompletionTokens != 2 || usage.TotalTokens != 7 {
			t.Fatalf("unexpected first stream usage: %+v", usage)
		}
	case err := <-done:
		t.Fatalf("agent ended before stream usage: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("usage was withheld until the stream ended")
	}
	select {
	case err := <-done:
		t.Fatalf("agent must still be reading the stream: %v", err)
	default:
	}
	writer.Send(&schema.Message{Role: schema.Assistant, ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{CompletionTokens: 4}}}, nil)
	select {
	case usage := <-reports:
		if usage.PromptTokens != 0 || usage.CompletionTokens != 2 || usage.TotalTokens != 2 {
			t.Fatalf("split input/output usage was not merged: %+v", usage)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second usage delta was not reported while streaming")
	}
	closeStream()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("agent did not finish after stream closed")
	}
}

func TestReadAssistantPreservesUsageOnStreamError(t *testing.T) {
	reader, writer := schema.Pipe[*schema.Message](2)
	writer.Send(&schema.Message{ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 5}}}, nil)
	wanted := errors.New("stream interrupted")
	writer.Send(nil, wanted)
	writer.Close()
	var reports []*schema.TokenUsage
	_, _, err := readAssistant(&adk.MessageVariant{IsStreaming: true, MessageStream: reader}, nil, func(usage *schema.TokenUsage) { reports = append(reports, usage) })
	if !errors.Is(err, wanted) || len(reports) != 1 || reports[0] == nil || reports[0].TotalTokens != 5 {
		t.Fatalf("partial usage was lost or counted as missing: reports=%v err=%v", reports, err)
	}
}

func TestReadAssistantDoesNotCountZeroUsageAsMissing(t *testing.T) {
	called := false
	_, _, err := readAssistant(&adk.MessageVariant{Message: &schema.Message{ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{}}}}, nil, func(*schema.TokenUsage) { called = true })
	if err != nil || called {
		t.Fatalf("zero usage should not add tokens or missing reports: called=%v err=%v", called, err)
	}
}

func TestReadAssistantPreservesMissingUsage(t *testing.T) {
	reader, writer := schema.Pipe[*schema.Message](1)
	writer.Send(&schema.Message{Content: "answer"}, nil)
	writer.Close()
	called := false
	_, _, err := readAssistant(&adk.MessageVariant{IsStreaming: true, MessageStream: reader}, nil, func(usage *schema.TokenUsage) {
		called = true
		if usage != nil {
			t.Fatalf("provider supplied no usage, got %+v", usage)
		}
	})
	if err != nil || !called {
		t.Fatalf("missing usage did not reach the sink: called=%v err=%v", called, err)
	}
}
