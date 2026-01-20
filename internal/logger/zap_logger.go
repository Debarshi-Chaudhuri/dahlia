package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type ZapLogger struct {
	logger *zap.Logger
}

type zapLogger = ZapLogger

func NewZapLogger() (Logger, error) {
	config := zap.NewProductionConfig()
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.OutputPaths = []string{"stdout"}

	log, err := config.Build()
	if err != nil {
		return nil, err
	}

	return &zapLogger{logger: log}, nil
}

func NewZapLoggerForDev() (Logger, error) {
	log, err := zap.NewDevelopment()
	if err != nil {
		return nil, err
	}
	return &zapLogger{logger: log}, nil
}

func (l *zapLogger) Debug(msg string, fields ...Field) {
	l.logger.Debug(msg, l.convertFields(fields)...)
}

func (l *zapLogger) Info(msg string, fields ...Field) {
	l.logger.Info(msg, l.convertFields(fields)...)
}

func (l *zapLogger) Warn(msg string, fields ...Field) {
	l.logger.Warn(msg, l.convertFields(fields)...)
}

func (l *zapLogger) Error(msg string, fields ...Field) {
	l.logger.Error(msg, l.convertFields(fields)...)
}

func (l *zapLogger) Fatal(msg string, fields ...Field) {
	l.logger.Fatal(msg, l.convertFields(fields)...)
}

func (l *zapLogger) With(fields ...Field) Logger {
	return &zapLogger{
		logger: l.logger.With(l.convertFields(fields)...),
	}
}

func (l *zapLogger) WithContext(ctx context.Context) Logger {
	if reqID, ok := ctx.Value("request_id").(string); ok {
		return &zapLogger{
			logger: l.logger.With(zap.String("request_id", reqID)),
		}
	}
	return l
}

func (l *zapLogger) convertFields(fields []Field) []zap.Field {
	zapFields := make([]zap.Field, len(fields))
	for i, f := range fields {
		zapFields[i] = zap.Any(f.Key, f.Value)
	}
	return zapFields
}

func (l *zapLogger) Sync() error {
	return l.logger.Sync()
}

func (l *zapLogger) Logger() *zap.Logger {
	return l.logger
}
