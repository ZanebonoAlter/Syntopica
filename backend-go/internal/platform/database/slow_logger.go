package database

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"syntopica-backend/internal/platform/logging"
)

func findCaller() string {
	const maxDepth = 16
	pcs := make([]uintptr, maxDepth)
	n := runtime.Callers(4, pcs)
	pcs = pcs[:n]
	for _, pc := range pcs {
		fn := runtime.FuncForPC(pc)
		if fn == nil {
			continue
		}
		name := fn.Name()
		if strings.Contains(name, "gorm.io") ||
			strings.Contains(name, "database.slow_logger") ||
			strings.Contains(name, "database.(*SlowLogger)") {
			continue
		}
		file, line := fn.FileLine(pc)
		return filepath.Base(file) + ":" + fmt.Sprintf("%d", line)
	}
	return ""
}

type SlowLogger struct {
	slowThreshold time.Duration
}

func NewSlowLogger(slowThreshold time.Duration) *SlowLogger {
	return &SlowLogger{slowThreshold: slowThreshold}
}

func (l *SlowLogger) LogMode(logger.LogLevel) logger.Interface {
	return l
}

func (l *SlowLogger) Info(_ context.Context, msg string, args ...any) {
	logging.Infof(msg, args...)
}

func (l *SlowLogger) Warn(_ context.Context, msg string, args ...any) {
	logging.Warnf(msg, args...)
}

func (l *SlowLogger) Error(_ context.Context, msg string, args ...any) {
	logging.Errorf(msg, args...)
}

func (l *SlowLogger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	elapsed := time.Since(begin)

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return
		}
		sql, rows := fc()
		caller := findCaller()
		if caller != "" {
			logging.Errorf("[SQL] %.3fms | rows=%d | err=%v | caller=%s | %s", float64(elapsed.Microseconds())/1000.0, rows, err, caller, sql)
		} else {
			logging.Errorf("[SQL] %.3fms | rows=%d | err=%v | %s", float64(elapsed.Microseconds())/1000.0, rows, err, sql)
		}
		return
	}

	if l.slowThreshold > 0 && elapsed > l.slowThreshold {
		sql, rows := fc()
		logging.Warnf("[SLOW SQL] %.3fms > %v | rows=%d | %s", float64(elapsed.Microseconds())/1000.0, l.slowThreshold, rows, sql)
		return
	}

	_ = fmt.Sprintf("%.3fms", float64(elapsed.Microseconds())/1000.0)
}
