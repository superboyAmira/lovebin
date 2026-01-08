package logger

import (
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	globalLogger *zap.Logger
	once         sync.Once
)

// Config holds logger configuration
type Config struct {
	Level string
}

// Logger interface for dependency injection
type Logger interface {
	Info(msg string, fields ...zap.Field)
	Error(msg string, fields ...zap.Field)
	Warn(msg string, fields ...zap.Field)
	Debug(msg string, fields ...zap.Field)
	Fatal(msg string, fields ...zap.Field)
	Sync() error
	With(fields ...zap.Field) *zap.Logger
}

type loggerImpl struct {
	logger *zap.Logger
}

func (l *loggerImpl) Info(msg string, fields ...zap.Field) {
	l.logger.Info(msg, fields...)
}

func (l *loggerImpl) Error(msg string, fields ...zap.Field) {
	l.logger.Error(msg, fields...)
}

func (l *loggerImpl) Warn(msg string, fields ...zap.Field) {
	l.logger.Warn(msg, fields...)
}

func (l *loggerImpl) Debug(msg string, fields ...zap.Field) {
	l.logger.Debug(msg, fields...)
}

func (l *loggerImpl) Fatal(msg string, fields ...zap.Field) {
	l.logger.Fatal(msg, fields...)
}

func (l *loggerImpl) Sync() error {
	return l.logger.Sync()
}

func (l *loggerImpl) With(fields ...zap.Field) *zap.Logger {
	return l.logger.With(fields...)
}

// Init initializes the logger module
func Init(cfg Config) (Logger, error) {
	level := cfg.Level
	if level == "" {
		level = getDefaultLevel()
	}

	var err error
	var logger *zap.Logger

	once.Do(func() {
		logger, err = newLogger(level)
		if err == nil {
			globalLogger = logger
		}
	})

	if err != nil {
		return nil, err
	}

	return &loggerImpl{logger: globalLogger}, nil
}

// Get returns the global logger instance
func Get() Logger {
	if globalLogger == nil {
		level := getDefaultLevel()
		logger, _ := Init(Config{Level: level})
		return logger
	}
	return &loggerImpl{logger: globalLogger}
}

func newLogger(level string) (*zap.Logger, error) {
	var zapLevel zapcore.Level
	if err := zapLevel.UnmarshalText([]byte(level)); err != nil {
		zapLevel = zapcore.InfoLevel
	}

	config := zap.NewProductionConfig()
	config.Level = zap.NewAtomicLevelAt(zapLevel)
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.EncoderConfig.MessageKey = "message"
	config.EncoderConfig.LevelKey = "level"
	config.EncoderConfig.CallerKey = "caller"
	config.EncoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	logger, err := config.Build(zap.AddCaller(), zap.AddCallerSkip(1))
	if err != nil {
		return nil, err
	}

	return logger, nil
}

func getDefaultLevel() string {
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		return level
	}
	return "info"
}
