// Package agent 封装 eino ADK 的 ChatModelAgent 运行流程，供 Analyzer 与 Scaffolder 共用。
package agent

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/chhongzh/scribekit/internal/logging"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

type Spec struct {
	Name        string
	Description string
	Model       model.ToolCallingChatModel
	Tools       []tool.BaseTool
	// MaxIterations 为 0 时使用 ADK 默认值。
	MaxIterations  int
	StopAfterTools func() bool
}

type usageSinkKey struct{}
type activitySinkKey struct{}

func WithUsageSink(ctx context.Context, sink func(*schema.TokenUsage)) context.Context {
	return context.WithValue(ctx, usageSinkKey{}, sink)
}

func usageSink(ctx context.Context) func(*schema.TokenUsage) {
	sink, _ := ctx.Value(usageSinkKey{}).(func(*schema.TokenUsage))
	return sink
}

func WithActivitySink(ctx context.Context, sink func(int)) context.Context {
	return context.WithValue(ctx, activitySinkKey{}, sink)
}

var errToolsComplete = errors.New("agent tools complete")

// Run 把 msgs（含 system）交给一个 ChatModelAgent 跑完，返回最后一条不带工具调用的助手文本。
// onText 非空时以流式方式运行，并把助手文本增量回调出来；
// 夹带工具调用的那一轮文本不会进入返回值，但已经回调出去的增量无法撤回。
func Run(ctx context.Context, spec Spec, msgs []*schema.Message, onText func(string)) (result string, err error) {
	if sink, ok := ctx.Value(activitySinkKey{}).(func(int)); ok {
		sink(1)
		defer sink(-1)
	}
	streaming := onText != nil || usageSink(ctx) != nil
	ctx, finish := logging.Start(ctx, "agent.run", zap.String("agent", spec.Name), zap.Int("message_count", len(msgs)), zap.Int("tool_count", len(spec.Tools)), zap.Int("max_iterations", spec.MaxIterations), zap.Bool("streaming", streaming))
	defer func() { finish(err) }()
	cfg := &adk.ChatModelAgentConfig{
		Name:          spec.Name,
		Description:   spec.Description,
		Model:         spec.Model,
		MaxIterations: spec.MaxIterations,
	}
	if len(spec.Tools) > 0 {
		cfg.ToolsConfig = adk.ToolsConfig{ToolsNodeConfig: compose.ToolsNodeConfig{Tools: spec.Tools}}
	}
	ag, err := adk.NewChatModelAgent(ctx, cfg)
	if err != nil {
		return "", fmt.Errorf("create agent %s: %w", spec.Name, err)
	}
	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: ag, EnableStreaming: streaming})

	iter := runner.Run(ctx, msgs, adk.WithAfterToolCallsHook(func(context.Context) error {
		if spec.StopAfterTools != nil && spec.StopAfterTools() {
			return errToolsComplete
		}
		return nil
	}))
	final := ""
	rounds := 0
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if ev.Err != nil {
			if errors.Is(ev.Err, errToolsComplete) {
				return final, nil
			}
			if cerr := ctx.Err(); cerr != nil {
				return "", cerr
			}
			return "", ev.Err
		}
		if ev.Output == nil || ev.Output.MessageOutput == nil {
			continue
		}
		mv := ev.Output.MessageOutput
		if mv.Role == schema.Tool {
			continue
		}
		text, hasToolCalls, err := readAssistant(mv, onText, usageSink(ctx))
		if err != nil {
			if cerr := ctx.Err(); cerr != nil {
				return "", cerr
			}
			return "", err
		}
		if !hasToolCalls {
			final = text
		}
		rounds++
		logging.From(ctx).Info("agent.round.completed", zap.Int("round", rounds), zap.Bool("has_tool_calls", hasToolCalls), zap.Int("output_bytes", len(text)))
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	logging.From(ctx).Info("agent.output", zap.Int("rounds", rounds), zap.Int("output_bytes", len(final)))
	return final, nil
}

func readAssistant(mv *adk.MessageVariant, onText func(string), onUsage func(*schema.TokenUsage)) (text string, hasToolCalls bool, err error) {
	var usage *schema.TokenUsage
	defer func() {
		if onUsage != nil && usage == nil {
			onUsage(nil)
		}
	}()
	reportUsage := func(reported *schema.TokenUsage) {
		if reported == nil {
			return
		}
		previous := schema.TokenUsage{}
		if usage != nil {
			previous = *usage
		}
		current := schema.TokenUsage{
			PromptTokens:     max(previous.PromptTokens, reported.PromptTokens),
			CompletionTokens: max(previous.CompletionTokens, reported.CompletionTokens),
			TotalTokens:      max(previous.TotalTokens, reported.TotalTokens),
		}
		current.TotalTokens = max(current.TotalTokens, current.PromptTokens+current.CompletionTokens)
		delta := schema.TokenUsage{
			PromptTokens:     current.PromptTokens - previous.PromptTokens,
			CompletionTokens: current.CompletionTokens - previous.CompletionTokens,
			TotalTokens:      current.TotalTokens - previous.TotalTokens,
		}
		usage = &current
		if onUsage != nil && (delta.PromptTokens != 0 || delta.CompletionTokens != 0 || delta.TotalTokens != 0) {
			onUsage(&delta)
		}
	}
	if !mv.IsStreaming {
		if mv.Message == nil {
			return "", false, nil
		}
		if mv.Message.ResponseMeta != nil {
			reportUsage(mv.Message.ResponseMeta.Usage)
		}
		if onText != nil && len(mv.Message.ToolCalls) == 0 && mv.Message.Content != "" {
			onText(mv.Message.Content)
		}
		return mv.Message.Content, len(mv.Message.ToolCalls) > 0, nil
	}
	defer mv.MessageStream.Close()
	var buf []byte
	for {
		chunk, err := mv.MessageStream.Recv()
		if errors.Is(err, io.EOF) {
			return string(buf), hasToolCalls, nil
		}
		if err != nil {
			return "", false, err
		}
		if len(chunk.ToolCalls) > 0 {
			hasToolCalls = true
		}
		if chunk.ResponseMeta != nil {
			reportUsage(chunk.ResponseMeta.Usage)
		}
		if chunk.Content != "" {
			buf = append(buf, chunk.Content...)
			if onText != nil {
				onText(chunk.Content)
			}
		}
	}
}
