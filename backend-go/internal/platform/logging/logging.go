package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
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

// Init 装配日志管道。
//
// 通道策略（fix-log-stdout-rotation D1/D2）：
//   - 文件日志启用：只写轮转文件。历史实现无条件扇出 stdout，而 dev 栈把
//     stdout 重定向到 backend-go/nohup.out（无轮转），同一份日志写两遍、其中
//     一份无界增长（2026-11-24 实测 756MB / 285 万行）。
//   - LOG_CONSOLE=true：显式恢复 stdout 通道，供需要 tail stdout 的临时调试。
//   - 文件未启用：维持历史行为（纯 stdout），脚本与单测依赖此语义。
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

	consoleHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel,
	})

	var handlers []slog.Handler
	fileEnabled := fileCfg.Enabled && fileCfg.Path != ""

	// console 在前保持历史输出顺序；文件启用且未显式要求 console 时跳过。
	if !fileEnabled || consoleEnabled() {
		handlers = append(handlers, consoleHandler)
	}

	if fileEnabled {
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
	} else {
		mu.Lock()
		fileWriter = nil
		mu.Unlock()
	}

	slog.SetDefault(slog.New(&fanoutHandler{handlers: handlers}))
}

// FileWriter 返回当前的轮转文件 writer；文件日志未启用时为 nil。
// 供非 slog 通道（Gin 的访问/恢复日志）复用同一个 lumberjack 实例，
// 避免为它们另起一套轮转或在无轮转的 stdout 上继续堆积。
func FileWriter() io.Writer {
	mu.RLock()
	defer mu.RUnlock()
	return fileWriter
}

// consoleEnabled 解析 LOG_CONSOLE 开关。命名刻意用 LOG_ 前缀并在
// docs/reference/configuration.md 登记为本包唯一的进程级环境变量例外。
func consoleEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("LOG_CONSOLE"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
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
