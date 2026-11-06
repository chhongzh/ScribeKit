package logging

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestStreamLogsUsageAndFailure(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	ctx := context.WithValue(context.Background(), loggerKey{}, zap.New(core).With(zap.String("request_id", "test")))
	handler := EinoHandler()
	ctx = handler.OnStart(ctx, &callbacks.RunInfo{Name: "model"}, &model.CallbackInput{})
	reader, writer := schema.Pipe[callbacks.CallbackOutput](2)
	handler.OnEndWithStreamOutput(ctx, nil, reader)
	writer.Send(&model.CallbackOutput{Message: schema.AssistantMessage("private output", nil), TokenUsage: &model.TokenUsage{TotalTokens: 7}}, nil)
	writer.Send(nil, errors.New("stream broken"))
	writer.Close()
	deadline := time.Now().Add(time.Second)
	for logs.FilterMessage("ai.failed").Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if logs.FilterMessage("ai.failed").Len() != 1 {
		t.Fatal("stream failure was not logged")
	}
	if logs.FilterMessage("eino.model.usage").All()[0].ContextMap()["total_tokens"] != int64(7) {
		t.Fatal("token usage missing")
	}
	for _, entry := range logs.All() {
		if entry.ContextMap()["request_id"] != "test" {
			t.Fatal("request correlation lost")
		}
		for _, field := range entry.Context {
			if field.String == "private output" {
				t.Fatal("model content leaked")
			}
		}
	}
}

func TestStartClassifiesCancellationAndTimeout(t *testing.T) {
	for _, test := range []struct {
		err     error
		message string
	}{
		{context.Canceled, "ai.canceled"},
		{context.DeadlineExceeded, "ai.timeout"},
		{nil, "ai.completed"},
	} {
		core, logs := observer.New(zap.InfoLevel)
		ctx := context.WithValue(context.Background(), loggerKey{}, zap.New(core))
		_, finish := Start(ctx, "test")
		finish(test.err)
		if logs.FilterMessage(test.message).Len() != 1 {
			t.Fatalf("missing %s", test.message)
		}
	}
}

func TestCompletionSignalOnlyClassifiesMatchingErrors(t *testing.T) {
	signal := errors.New("tools complete")
	for _, test := range []struct {
		name    string
		err     error
		message string
	}{
		{"direct", signal, "ai.completed"},
		{"wrapped", fmt.Errorf("node failed: %w", signal), "ai.completed"},
		{"same text", errors.New("tools complete"), "ai.failed"},
		{"real failure", errors.New("unexpected EOF"), "ai.failed"},
		{"canceled", context.Canceled, "ai.canceled"},
		{"timeout", context.DeadlineExceeded, "ai.timeout"},
	} {
		t.Run(test.name, func(t *testing.T) {
			core, logs := observer.New(zap.InfoLevel)
			ctx := context.WithValue(context.Background(), loggerKey{}, zap.New(core))
			ctx = WithCompletionSignal(ctx, signal)
			handler := EinoHandler()
			ctx = handler.OnStart(ctx, &callbacks.RunInfo{Name: "AfterToolCalls"}, nil)
			handler.OnError(ctx, nil, test.err)
			if logs.FilterMessage(test.message).Len() != 1 {
				t.Fatalf("expected %s, got %+v", test.message, logs.All())
			}
		})
	}
}

func TestNestedSpansReplaceFieldsAndLinkParent(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	ctx := context.WithValue(context.Background(), loggerKey{}, zap.New(core))
	parent, finishParent := Start(ctx, "parent")
	_, finishChild := Start(parent, "child")
	finishChild(nil)
	finishParent(nil)
	entries := logs.All()
	parentID := entries[0].ContextMap()["span_id"]
	if entries[1].ContextMap()["parent_span_id"] != parentID {
		t.Fatal("child span does not reference parent")
	}
	for _, entry := range entries {
		keys := map[string]bool{}
		for _, field := range entry.Context {
			if keys[field.Key] {
				t.Fatalf("duplicate field %s", field.Key)
			}
			keys[field.Key] = true
		}
	}
}
