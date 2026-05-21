# Slog Logging Phase 1: 换引擎 + 文件滚动

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将 logging 包底层从 stdlib log 替换为 slog，添加 lumberjack 文件滚动，保持所有调用方 API 不变。

**Architecture:** 重写 `internal/platform/logging/logging.go`，初始化 slog 全局 logger，FanoutHandler 同时写 stdout 和 lumberjack 文件。调用方 83 个文件的 `logging.Infof/Warnf/Errorf` 签名不变。新增 `LogConfig` 到 config.yaml。移除旧的 routeWriter hack。

**Tech Stack:** `log/slog` (stdlib), `gopkg.in/natefinch/lumberjack.v2`

---

### Task 1: 安装 lumberjack 依赖

**Files:**
- Modify: `backend-go/go.mod`
- Modify: `backend-go/go.sum`

**Step 1: 安装依赖**

```bash
cd backend-go && go get gopkg.in/natefinch/lumberjack.v2
```

**Step 2: 验证**

```bash
cd backend-go && go mod tidy
```

**Step 3: Commit**

```bash
git add backend-go/go.mod backend-go/go.sum
git commit -m "chore: add lumberjack dependency for log file rotation"
```

---

### Task 2: 添加 LogConfig 配置

**Files:**
- Modify: `backend-go/internal/platform/config/config.go`
- Modify: `backend-go/configs/config.yaml`

**Step 1: 在 config.go 添加 LogConfig 结构体**

在 `Config` struct 中加入 `Log LogConfig` 字段，并添加以下类型：

```go
type LogConfig struct {
	Level string    `mapstructure:"level"`
	File  LogFileConfig `mapstructure:"file"`
}

type LogFileConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	Path       string `mapstructure:"path"`
	MaxSizeMB  int    `mapstructure:"max_size_mb"`
	MaxBackups int    `mapstructure:"max_backups"`
	MaxAgeDays int    `mapstructure:"max_age_days"`
	Compress   bool   `mapstructure:"compress"`
}
```

在 `LoadConfig` 函数的 defaults 区块中添加：

```go
viper.SetDefault("log.level", "debug")
viper.SetDefault("log.file.enabled", true)
viper.SetDefault("log.file.path", "logs/app.log")
viper.SetDefault("log.file.max_size_mb", 50)
viper.SetDefault("log.file.max_backups", 30)
viper.SetDefault("log.file.max_age_days", 30)
viper.SetDefault("log.file.compress", true)
```

注意：移除 config.go 对 `"my-robot-backend/internal/platform/logging"` 的 import（logging 包初始化时会循环依赖，改为 config 包不依赖 logging）。

将 config.go 中现有的 `logging.Infof` / `logging.Warnf` 调用替换为 `fmt.Println` / `fmt.Fprintf(os.Stderr, ...)`，因为配置加载在日志初始化之前。

**Step 2: 更新 config.yaml**

在文件末尾添加：

```yaml
log:
  level: "debug"
  file:
    enabled: true
    path: "logs/app.log"
    max_size_mb: 50
    max_backups: 30
    max_age_days: 30
    compress: true
```

**Step 3: 验证编译**

```bash
cd backend-go && go build ./...
```

**Step 4: Commit**

```bash
git add backend-go/internal/platform/config/config.go backend-go/configs/config.yaml
git commit -m "feat: add LogConfig to config with file rotation defaults"
```

---

### Task 3: 重写 logging 包

**Files:**
- Modify: `backend-go/internal/platform/logging/logging.go`
- Modify: `backend-go/internal/platform/logging/logging_test.go`

**Step 1: 重写 logging.go**

完全替换为 slog 实现，关键设计：

```go
package logging

import (
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
}

type FileConfig struct {
	Enabled    bool
	Path       string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
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

// === 公开 API (保持调用方不变) ===

func ConfigureStdlib() {
	// slog 自带 stdlib bridge，不再需要 routeWriter hack
}

func SetWriters(info io.Writer, err io.Writer) {
	// 兼容测试：用提供的 writer 创建 handler
	// info writer 用于 info/warn, err writer 用于 error
	handlers := []slog.Handler{
		slog.NewTextHandler(info, &slog.HandlerOptions{Level: slog.LevelDebug}),
	}
	_ = err // Phase 1 简化：测试场景下统一写 info writer
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
```

注意需要 `import "context"` 到文件顶部。

**Step 2: 重写 logging_test.go**

测试需要适配 slog 输出格式（slog TextHandler 输出 `level=INFO msg="..."` 而非 `[INFO] ...`）：

```go
package logging

import (
	"bytes"
	"strings"
	"testing"
)

func TestInfoAndWarnOutput(t *testing.T) {
	var buf bytes.Buffer
	SetWriters(&buf, &buf)
	defer ResetWriters()

	Infof("server starting on %s", ":5000")
	Warnln("config fallback enabled")

	out := buf.String()
	if !strings.Contains(out, "server starting on :5000") {
		t.Fatalf("expected info output, got %q", out)
	}
	if !strings.Contains(out, "config fallback enabled") {
		t.Fatalf("expected warn output, got %q", out)
	}
	if !strings.Contains(out, "level=INFO") {
		t.Fatalf("expected INFO level, got %q", out)
	}
	if !strings.Contains(out, "level=WARN") {
		t.Fatalf("expected WARN level, got %q", out)
	}
}

func TestErrorOutput(t *testing.T) {
	var buf bytes.Buffer
	SetWriters(&buf, &buf)
	defer ResetWriters()

	Errorf("failed to start server: %v", "boom")

	out := buf.String()
	if !strings.Contains(out, "failed to start server: boom") {
		t.Fatalf("expected error output, got %q", out)
	}
	if !strings.Contains(out, "level=ERROR") {
		t.Fatalf("expected ERROR level, got %q", out)
	}
}

func TestInitWithFileRotation(t *testing.T) {
	tmpDir := t.TempDir()
	Init("debug", FileConfig{
		Enabled:    true,
		Path:       tmpDir + "/test.log",
		MaxSizeMB:  1,
		MaxBackups: 3,
		MaxAgeDays: 7,
		Compress:   false,
	})
	defer Close()

	Infof("hello file logging")

	Close()

	data, err := os.ReadFile(tmpDir + "/test.log")
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "hello file logging") {
		t.Fatalf("expected log in file, got %q", string(data))
	}
}
```

测试文件需要 `import "os"`。

**Step 3: 运行测试**

```bash
cd backend-go && go test ./internal/platform/logging/ -v
```

**Step 4: Commit**

```bash
git add backend-go/internal/platform/logging/
git commit -m "refactor: rewrite logging package with slog + lumberjack file rotation"
```

---

### Task 4: 接入 main.go 初始化

**Files:**
- Modify: `backend-go/cmd/server/main.go`

**Step 1: 修改 main.go**

将 `init()` 中的 `logging.ConfigureStdlib()` 替换为 `logging.Init()` 调用，放在 config 加载之后：

```go
func main() {
	if err := config.LoadConfig("./configs"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
	}

	logging.Init(
		config.AppConfig.Log.Level,
		logging.FileConfig{
			Enabled:    config.AppConfig.Log.File.Enabled,
			Path:       config.AppConfig.Log.File.Path,
			MaxSizeMB:  config.AppConfig.Log.File.MaxSizeMB,
			MaxBackups: config.AppConfig.Log.File.MaxBackups,
			MaxAgeDays: config.AppConfig.Log.File.MaxAgeDays,
			Compress:   config.AppConfig.Log.File.Compress,
		},
	)
	defer logging.Close()

	// ... 其余不变 ...
}
```

移除底部的 `init()` 函数。移除顶部 `"fmt"` import 如果不再需要（但上面用了，保留）。添加 `"os"` import。

**Step 2: 编译验证**

```bash
cd backend-go && go build ./...
```

**Step 3: Commit**

```bash
git add backend-go/cmd/server/main.go
git commit -m "feat: initialize slog logging with config-driven file rotation in main"
```

---

### Task 5: 更新 .gitignore + 验证端到端

**Files:**
- Modify: `.gitignore`

**Step 1: 添加 logs/ 到 .gitignore**

在 `.gitignore` 末尾添加：

```
# Log files
logs/
```

**Step 2: 运行全量测试**

```bash
cd backend-go && go test ./...
```

**Step 3: 启动服务器验证日志输出**

```bash
cd backend-go && go run cmd/server/main.go
```

验证：
- 终端看到 `level=INFO msg="Server starting on :5000"` 格式输出
- `logs/app.log` 文件被创建，内容与终端一致
- Ctrl+C 优雅退出后日志文件完整

**Step 4: Commit**

```bash
git add .gitignore
git commit -m "chore: add logs/ to gitignore"
```

---

### Task 6: 更新 slow_logger 适配（可选，但推荐）

**Files:**
- Modify: `backend-go/internal/platform/database/slow_logger.go`

**Step 1: 确认 slow_logger 调用 logging 包的方式**

当前 slow_logger 调用 `logging.Infof` / `logging.Warnf` / `logging.Errorf`，这些 API 签名不变，无需修改。只需要确认编译通过。

**Step 2: 编译验证**

```bash
cd backend-go && go build ./...
```

应该直接通过，因为 API 没变。

---

## 验证清单

- [ ] `go build ./...` 编译通过
- [ ] `go test ./...` 全部通过
- [ ] `golangci-lint run ./...` 无新增 warning
- [ ] 终端日志输出 `level=INFO msg="..."` 格式
- [ ] `logs/app.log` 文件创建并同步写入
- [ ] 日志级别通过 config.yaml `log.level` 可控
- [ ] 旧 `logging.Infof/Warnf/Errorf` 调用方零改动
