package logging

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout 把 os.Stdout 临时换成管道，返回 fn 期间写入的内容。
// 注意：TextHandler 在 Init/SetWriters 调用时对 os.Stdout 求值，所以必须在
// fn 内部完成 Init —— 捕获的是当时的变量值。测试不可并行（全局状态）。
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	_ = r.Close()
	return string(data)
}

// TestInitWithFileDoesNotWriteStdout 锁定「文件启用时默认不写 stdout」——这是
// nohup.out 不再无界增长的前提（fix-log-stdout-rotation D2）。
func TestInitWithFileDoesNotWriteStdout(t *testing.T) {
	t.Setenv("LOG_CONSOLE", "")
	logPath := filepath.Join(t.TempDir(), "app.log")

	out := captureStdout(t, func() {
		Init("debug", FileConfig{Enabled: true, Path: logPath, MaxSizeMB: 1, MaxBackups: 2, MaxAgeDays: 1})
		Infof("file-only marker")
		Close()
	})

	if strings.Contains(out, "file-only marker") {
		t.Fatalf("file logging should not fan out to stdout, got %q", out)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "file-only marker") {
		t.Fatalf("expected marker in log file, got %q", string(data))
	}
}

// TestInitWithLogConsoleWritesStdout 锁定显式开关：LOG_CONSOLE=true 时
// 文件与 stdout 双写都成立，供临时调试 tail stdout。
func TestInitWithLogConsoleWritesStdout(t *testing.T) {
	t.Setenv("LOG_CONSOLE", "true")
	logPath := filepath.Join(t.TempDir(), "app.log")

	out := captureStdout(t, func() {
		Init("debug", FileConfig{Enabled: true, Path: logPath, MaxSizeMB: 1, MaxBackups: 2, MaxAgeDays: 1})
		Infof("console-and-file marker")
		Close()
	})

	if !strings.Contains(out, "console-and-file marker") {
		t.Fatalf("LOG_CONSOLE=true should write stdout, got %q", out)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "console-and-file marker") {
		t.Fatalf("expected marker in log file, got %q", string(data))
	}
}

// TestInitWithoutFileKeepsStdout 锁定历史行为：文件未启用时纯 stdout（脚本与
// 既有测试依赖此语义）。
func TestInitWithoutFileKeepsStdout(t *testing.T) {
	t.Setenv("LOG_CONSOLE", "")

	out := captureStdout(t, func() {
		Init("debug", FileConfig{Enabled: false})
		Infof("stdout-only marker")
		ResetWriters()
	})

	if !strings.Contains(out, "stdout-only marker") {
		t.Fatalf("file-disabled logging should write stdout, got %q", out)
	}
}

// TestFileWriterExposure 锁定 FileWriter 的三态：未启用 nil、启用非 nil、
// Close 后回到 nil（Gin 注入依赖这个契约）。
func TestFileWriterExposure(t *testing.T) {
	ResetWriters()
	if w := FileWriter(); w != nil {
		t.Fatalf("FileWriter should be nil before any file-enabled Init")
	}

	Init("info", FileConfig{Enabled: false})
	if w := FileWriter(); w != nil {
		t.Fatalf("FileWriter should stay nil when file logging is disabled")
	}

	logPath := filepath.Join(t.TempDir(), "app.log")
	Init("info", FileConfig{Enabled: true, Path: logPath, MaxSizeMB: 1, MaxBackups: 1, MaxAgeDays: 1})
	if w := FileWriter(); w == nil {
		t.Fatalf("FileWriter should be non-nil after file-enabled Init")
	}

	Close()
	if w := FileWriter(); w != nil {
		t.Fatalf("FileWriter should reset to nil after Close")
	}
}
