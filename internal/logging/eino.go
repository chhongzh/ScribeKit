package logging

import (
	"context"
	"errors"
	"io"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"go.uber.org/zap"
)

type callbackKey struct{}

func EinoHandler() callbacks.Handler {
	return callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
			var fields []zap.Field
			if info != nil {
				fields = append(fields, zap.String("component", string(info.Component)), zap.String("component_name", info.Name), zap.String("component_type", info.Type))
			}
			if in := model.ConvCallbackInput(input); in != nil {
				fields = append(fields, zap.Int("message_count", len(in.Messages)), zap.Int("tool_count", len(in.Tools)))
				if in.Config != nil {
					fields = append(fields, zap.String("model", in.Config.Model))
				}
			}
			ctx, finish := Start(ctx, "eino.component", fields...)
			return context.WithValue(ctx, callbackKey{}, finish)
		}).
		OnStartWithStreamInputFn(func(ctx context.Context, info *callbacks.RunInfo, input *schema.StreamReader[callbacks.CallbackInput]) context.Context {
			input.Close()
			var fields []zap.Field
			if info != nil {
				fields = append(fields, zap.String("component", string(info.Component)), zap.String("component_name", info.Name), zap.String("component_type", info.Type))
			}
			ctx, finish := Start(ctx, "eino.component", append(fields, zap.Bool("stream_input", true))...)
			return context.WithValue(ctx, callbackKey{}, finish)
		}).
		OnEndFn(func(ctx context.Context, _ *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
			logOutput(ctx, output)
			finishCallback(ctx, nil)
			return ctx
		}).
		OnErrorFn(func(ctx context.Context, _ *callbacks.RunInfo, err error) context.Context {
			finishCallback(ctx, err)
			return ctx
		}).
		OnEndWithStreamOutputFn(func(ctx context.Context, _ *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) context.Context {
			go func() {
				defer output.Close()
				chunks, bytes := 0, 0
				for {
					chunk, err := output.Recv()
					if err != nil {
						if errors.Is(err, io.EOF) {
							err = nil
						}
						From(ctx).Info("eino.stream.summary", zap.Int("chunks", chunks), zap.Int("output_bytes", bytes))
						finishCallback(ctx, err)
						return
					}
					chunks++
					if out := model.ConvCallbackOutput(chunk); out != nil && out.Message != nil {
						bytes += len(out.Message.Content)
					}
					logUsage(ctx, chunk)
				}
			}()
			return ctx
		}).Build()
}

func finishCallback(ctx context.Context, err error) {
	if finish, ok := ctx.Value(callbackKey{}).(func(error)); ok {
		finish(err)
	}
}

func logUsage(ctx context.Context, output callbacks.CallbackOutput) {
	if out := model.ConvCallbackOutput(output); out != nil && out.TokenUsage != nil {
		From(ctx).Info("eino.model.usage", zap.Int("prompt_tokens", out.TokenUsage.PromptTokens), zap.Int("completion_tokens", out.TokenUsage.CompletionTokens), zap.Int("total_tokens", out.TokenUsage.TotalTokens))
	}
}

func logOutput(ctx context.Context, output callbacks.CallbackOutput) {
	logUsage(ctx, output)
	if out := model.ConvCallbackOutput(output); out != nil && out.Message != nil {
		From(ctx).Info("eino.model.output", zap.Int("output_bytes", len(out.Message.Content)), zap.Int("tool_calls", len(out.Message.ToolCalls)))
	}
	if out := tool.ConvCallbackOutput(output); out != nil {
		From(ctx).Info("eino.tool.output", zap.Int("output_bytes", len(out.Response)))
	}
}
