package pkglog

import (
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewLogger(opts ...Option) (*zap.Logger, error) {
	options := newOptions(opts...)

	var encoderConfig zapcore.EncoderConfig
	switch options.encoding {
	case "json":
		encoderConfig = zap.NewProductionEncoderConfig()
	default:
		encoderConfig = zap.NewDevelopmentEncoderConfig()
	}

	cfg := zap.Config{
		Level:            zap.NewAtomicLevelAt(zap.InfoLevel),
		Encoding:         options.encoding,
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
		EncoderConfig:    encoderConfig,
	}

	if options.level != "" {
		level, err := zap.ParseAtomicLevel(options.level)
		if err != nil {
			return nil, fmt.Errorf("failed to parse log level: %w", err)
		}
		cfg.Level = level
	}

	if options.encoding != "" {
		cfg.Encoding = options.encoding
	}

	cfg.OutputPaths = []string{"stdout"}
	cfg.ErrorOutputPaths = []string{"stderr"}
	cfg.EncoderConfig = zap.NewProductionEncoderConfig()

	return cfg.Build()
}
