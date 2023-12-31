package logger

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime/debug"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type ctxKey struct{}

var once sync.Once

var logger *zap.Logger

// Get initializes a zap.Logger instance if it has not been initialized
// already and returns the same instance for subsequent calls.
func Get(logPath, logLevel string) *zap.Logger {
	once.Do(func() {
		stdout := zapcore.AddSync(os.Stdout)

		file := zapcore.AddSync(&lumberjack.Logger{
			Filename:   logPath,
			MaxSize:    3,    // log size 3MB
			MaxBackups: 30,   // Keeps last 30 log files
			Compress:   true, // Compress old logs
		})

		level := zap.InfoLevel
		levelEnv := logLevel
		if levelEnv != "" {
			levelFromEnv, err := zapcore.ParseLevel(levelEnv)
			if err != nil {
				log.Println(
					fmt.Errorf("invalid level, defaulting to INFO: %w", err),
				)
			}

			level = levelFromEnv
		}

		logLevel := zap.NewAtomicLevelAt(level)

		productionCfg := zap.NewProductionEncoderConfig()
		productionCfg.TimeKey = "timestamp"
		productionCfg.EncodeTime = zapcore.ISO8601TimeEncoder

		developmentCfg := zap.NewDevelopmentEncoderConfig()
		developmentCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder

		consoleEncoder := zapcore.NewConsoleEncoder(productionCfg)
		fileEncoder := zapcore.NewJSONEncoder(productionCfg)

		var gitRevision string

		buildInfo, ok := debug.ReadBuildInfo()
		if ok {
			for _, v := range buildInfo.Settings {
				if v.Key == "vcs.revision" {
					gitRevision = v.Value
					break
				}
			}
		}

		var core zapcore.Core

		// In development env write only to console
		// In non-dev env write only to file
		if os.Getenv("APP_ENV") == "dev" {
			core = zapcore.NewTee(
				zapcore.NewCore(consoleEncoder, stdout, logLevel),
			)
		} else {
			core = zapcore.NewTee(
				zapcore.NewCore(fileEncoder, file, logLevel).
					With(
						[]zapcore.Field{
							zap.String("git_revision", gitRevision),
							zap.String("go_version", buildInfo.GoVersion),
						},
					),
			)
		}

		logger = zap.New(core, zap.AddStacktrace(zap.ErrorLevel))
	})

	return logger
}

// FromCtx returns the Logger associated with the ctx. If no logger
// is associated, the default logger is returned, unless it is nil
// in which case a disabled logger is returned.
func FromCtx(ctx context.Context) *zap.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*zap.Logger); ok {
		return l
	} else if l := logger; l != nil {
		return l
	}

	return zap.NewNop()
}

// WithCtx returns a copy of ctx with the Logger attached.
func WithCtx(ctx context.Context, l *zap.Logger) context.Context {
	if lp, ok := ctx.Value(ctxKey{}).(*zap.Logger); ok {
		if lp == l {
			// Do not store same logger.
			return ctx
		}
	}

	return context.WithValue(ctx, ctxKey{}, l)
}

func GetContextLogger(ctx context.Context) (context.Context, *zap.Logger) {
	log := FromCtx(ctx)
	context := WithCtx(ctx, log)
	return context, log
}

func LogUserId(ctx context.Context) zapcore.Field {
	return zap.Any("user_id", ctx.Value("userId"))
}
