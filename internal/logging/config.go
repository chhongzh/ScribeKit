package logging

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewConfig(jsonLog bool) zap.Config {
	cfg := zap.NewProductionConfig()
	cfg.Sampling = nil
	if !jsonLog {
		cfg.Encoding = "console"
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		cfg.EncoderConfig.EncodeDuration = zapcore.StringDurationEncoder
	}
	return cfg
}
