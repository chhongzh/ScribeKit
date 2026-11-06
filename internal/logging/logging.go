package logging

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type loggerKey struct{}
type spanKey struct{}

type contextLogger struct {
	base   *zap.Logger
	fields []zap.Field
}

func With(ctx context.Context, fields ...zap.Field) context.Context {
	state := contextLogger{base: zap.L()}
	switch current := ctx.Value(loggerKey{}).(type) {
	case contextLogger:
		state.base = current.base
		state.fields = append(state.fields, current.fields...)
	case *zap.Logger:
		state.base = current
	}
	for _, field := range fields {
		replaced := false
		for index := range state.fields {
			if state.fields[index].Key == field.Key {
				state.fields[index] = field
				replaced = true
				break
			}
		}
		if !replaced {
			state.fields = append(state.fields, field)
		}
	}
	return context.WithValue(ctx, loggerKey{}, state)
}

func From(ctx context.Context) *zap.Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*zap.Logger); ok {
		return logger
	}
	if state, ok := ctx.Value(loggerKey{}).(contextLogger); ok {
		return state.base.With(state.fields...)
	}
	return zap.L()
}

func Start(ctx context.Context, operation string, fields ...zap.Field) (context.Context, func(error)) {
	spanID := uuid.NewString()
	parentID, _ := ctx.Value(spanKey{}).(string)
	ctx = With(ctx, append(fields, zap.String("operation", operation), zap.String("span_id", spanID), zap.String("parent_span_id", parentID))...)
	ctx = context.WithValue(ctx, spanKey{}, spanID)
	logger := From(ctx)
	started := time.Now()
	logger.Info("ai.started")
	return ctx, func(err error) {
		fields := []zap.Field{zap.Duration("duration", time.Since(started))}
		switch {
		case errors.Is(err, context.Canceled):
			logger.Info("ai.canceled", fields...)
		case errors.Is(err, context.DeadlineExceeded):
			logger.Warn("ai.timeout", append(fields, zap.Error(err))...)
		case err != nil:
			logger.Error("ai.failed", append(fields, zap.Error(err))...)
		default:
			logger.Info("ai.completed", fields...)
		}
	}
}
