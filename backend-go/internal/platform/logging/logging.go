package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
)

const stdlibFlags = log.LstdFlags | log.Lshortfile

var (
	mu          sync.RWMutex
	infoWriter  io.Writer = os.Stdout
	errorWriter io.Writer = os.Stderr
	infoLogger            = log.New(infoWriter, "", stdlibFlags)
	errorLogger           = log.New(errorWriter, "", stdlibFlags)
)

type routeWriter struct{}

func ConfigureStdlib() {
	log.SetFlags(stdlibFlags)
	log.SetOutput(routeWriter{})
}

func SetWriters(info io.Writer, err io.Writer) {
	mu.Lock()
	defer mu.Unlock()

	if info == nil {
		info = io.Discard
	}
	if err == nil {
		err = io.Discard
	}

	infoWriter = info
	errorWriter = err
	infoLogger = log.New(infoWriter, "", stdlibFlags)
	errorLogger = log.New(errorWriter, "", stdlibFlags)
}

func ResetWriters() {
	SetWriters(os.Stdout, os.Stderr)
}

func Infof(format string, args ...any) {
	writef(infoLogger, "[INFO] "+format, args...)
}

func Infoln(args ...any) {
	writeln(infoLogger, "[INFO] "+fmt.Sprint(args...))
}

func Warnf(format string, args ...any) {
	writef(infoLogger, "[WARN] "+format, args...)
}

func Warnln(args ...any) {
	writeln(infoLogger, "[WARN] "+fmt.Sprint(args...))
}

func Errorf(format string, args ...any) {
	writef(errorLogger, "[ERROR] "+format, args...)
}

func Errorln(args ...any) {
	writeln(errorLogger, "[ERROR] "+fmt.Sprint(args...))
}

func Fatalf(format string, args ...any) {
	errorLogger.Output(2, fmt.Sprintf("[FATAL] "+format, args...)) //nolint:errcheck,gosec
	os.Exit(1)
}

func (routeWriter) Write(p []byte) (int, error) {
	line := strings.ToLower(string(p))
	writer := infoOutput()
	if isErrorLine(line) {
		writer = errorOutput()
	}
	return writer.Write(p)
}

func isErrorLine(line string) bool {
	for _, marker := range []string{"[error]", "[fatal]", "[panic]", "[warn]", "warning", "panic", "fatal", "error", "failed"} {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

func writef(logger *log.Logger, format string, args ...any) {
	logger.Output(3, fmt.Sprintf(format, args...)) //nolint:errcheck,gosec
}

func writeln(logger *log.Logger, message string) {
	logger.Output(3, message) //nolint:errcheck,gosec
}

func infoOutput() io.Writer {
	mu.RLock()
	defer mu.RUnlock()
	return infoWriter
}

func errorOutput() io.Writer {
	mu.RLock()
	defer mu.RUnlock()
	return errorWriter
}
