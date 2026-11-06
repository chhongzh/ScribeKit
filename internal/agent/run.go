// Package agent 封装 eino ADK 的 ChatModelAgent 运行流程，供 Analyzer 与 Scaffolder 共用。
package agent

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type Spec struct {
	Name        string
	Description string
	Model       model.ToolCallingChatModel
	Tools       []tool.BaseTool
	// MaxIterations 为 0 时使用 ADK 默认值。
	MaxIterations int
}

// Run 把 msgs（含 system）交给一个 ChatModelAgent 跑完，返回最后一条不带工具调用的助手文本。
// onText 非空时以流式方式运行，并把助手文本增量回调出来；
// 夹带工具调用的那一轮文本不会进入返回值，但已经回调出去的增量无法撤回。
func Run(ctx context.Context, spec Spec, msgs []*schema.Message, onText func(string)) (string, error) {
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
	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: ag, EnableStreaming: onText != nil})

	iter := runner.Run(ctx, msgs)
	final := ""
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if ev.Err != nil {
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
		text, hasToolCalls, err := readAssistant(mv, onText)
		if err != nil {
			if cerr := ctx.Err(); cerr != nil {
				return "", cerr
			}
			return "", err
		}
		if !hasToolCalls {
			final = text
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return final, nil
}

func readAssistant(mv *adk.MessageVariant, onText func(string)) (text string, hasToolCalls bool, err error) {
	if !mv.IsStreaming {
		if mv.Message == nil {
			return "", false, nil
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
		if chunk.Content != "" {
			buf = append(buf, chunk.Content...)
			if onText != nil {
				onText(chunk.Content)
			}
		}
	}
}
