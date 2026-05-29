package logging

import (
	"bytes"
	"os"
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
	logPath := os.TempDir() + string(os.PathSeparator) + "test-logging-" + t.Name() + ".log"
	defer os.Remove(logPath)

	Init("debug", FileConfig{
		Enabled:    true,
		Path:       logPath,
		MaxSizeMB:  1,
		MaxBackups: 3,
		MaxAgeDays: 7,
		Compress:   false,
	})

	Infof("hello file logging")

	Close()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "hello file logging") {
		t.Fatalf("expected log in file, got %q", string(data))
	}
}
