package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	mu         sync.RWMutex
	fileWriter io.Writer
)

type FileConfig struct {
	Enabled    bool
	Path       string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
}

func Init(level string, fileCfg FileConfig) {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	var handlers []slog.Handler

	consoleHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel,
	})
	handlers = append(handlers, consoleHandler)

	if fileCfg.Enabled && fileCfg.Path != "" {
		lj := &lumberjack.Logger{
			Filename:   fileCfg.Path,
			MaxSize:    fileCfg.MaxSizeMB,
			MaxBackups: fileCfg.MaxBackups,
			MaxAge:     fileCfg.MaxAgeDays,
			Compress:   fileCfg.Compress,
		}
		mu.Lock()
		fileWriter = lj
		mu.Unlock()

		fileHandler := slog.NewTextHandler(lj, &slog.HandlerOptions{
			Level: slogLevel,
		})
		handlers = append(handlers, fileHandler)
	}

	handler := &fanoutHandler{handlers: handlers}
	slog.SetDefault(slog.New(handler))
}

func Close() {
	mu.Lock()
	defer mu.Unlock()
	if fileWriter != nil {
		if closer, ok := fileWriter.(interface{ Close() }); ok {
			closer.Close()
		}
		fileWriter = nil
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))
}

func ConfigureStdlib() {
}

func SetWriters(info io.Writer, err io.Writer) {
	handlers := []slog.Handler{
		slog.NewTextHandler(info, &slog.HandlerOptions{Level: slog.LevelDebug}),
	}
	_ = err
	slog.SetDefault(slog.New(&fanoutHandler{handlers: handlers}))
}

func ResetWriters() {
	slog.SetDefault(slog.New(&fanoutHandler{handlers: []slog.Handler{
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}),
	}}))
}

func Infof(format string, args ...any) {
	slog.Info(fmt.Sprintf(format, args...))
}

func Infoln(args ...any) {
	slog.Info(fmt.Sprint(args...))
}

func Warnf(format string, args ...any) {
	slog.Warn(fmt.Sprintf(format, args...))
}

func Warnln(args ...any) {
	slog.Warn(fmt.Sprint(args...))
}

func Errorf(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
}

func Errorln(args ...any) {
	slog.Error(fmt.Sprint(args...))
}

func Fatalf(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}

type fanoutHandler struct {
	handlers []slog.Handler
}

func (h *fanoutHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, hh := range h.handlers {
		if hh.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *fanoutHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, hh := range h.handlers {
		if err := hh.Handle(ctx, r.Clone()); err != nil {
			return err
		}
	}
	return nil
}

func (h *fanoutHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, hh := range h.handlers {
		newHandlers[i] = hh.WithAttrs(attrs)
	}
	return &fanoutHandler{handlers: newHandlers}
}

func (h *fanoutHandler) WithGroup(name string) slog.Handler {
	newHandlers := make([]slog.Handler, len(h.handlers))
	for i, hh := range h.handlers {
		newHandlers[i] = hh.WithGroup(name)
	}
	return &fanoutHandler{handlers: newHandlers}
}
