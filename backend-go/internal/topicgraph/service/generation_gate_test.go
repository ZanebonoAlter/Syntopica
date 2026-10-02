package service

import (
	"sync"
	"sync/atomic"
	"testing"
)

// ── guard-daily-report-generate 1.1：日报生成域闸（CAS 互斥）──
//
// 契约锚：change specs/daily-report-system/spec.md「日报生成 API — 异步触发」
// （MODIFIED）——同一时刻至多一条日报生成管线；闸在管线结束后必须释放。

func TestDailyReportGenerationGate(t *testing.T) {
	t.Run("first acquire succeeds", func(t *testing.T) {
		if !TryAcquireDailyReportGeneration() {
			t.Fatal("first TryAcquire on a free gate must succeed")
		}
		ReleaseDailyReportGeneration()
	})

	t.Run("second acquire while held fails", func(t *testing.T) {
		if !TryAcquireDailyReportGeneration() {
			t.Fatal("setup: first acquire must succeed")
		}
		if TryAcquireDailyReportGeneration() {
			t.Fatal("TryAcquire while the gate is held must fail")
		}
		ReleaseDailyReportGeneration()
	})

	t.Run("acquire succeeds after release", func(t *testing.T) {
		if !TryAcquireDailyReportGeneration() {
			t.Fatal("setup: first acquire must succeed")
		}
		ReleaseDailyReportGeneration()
		if !TryAcquireDailyReportGeneration() {
			t.Fatal("TryAcquire after Release must succeed")
		}
		ReleaseDailyReportGeneration()
	})

	t.Run("100 concurrent acquires exactly one winner", func(t *testing.T) {
		var winners atomic.Int32
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if TryAcquireDailyReportGeneration() {
					winners.Add(1)
				}
			}()
		}
		close(start)
		wg.Wait()

		if got := winners.Load(); got != 1 {
			t.Fatalf("exactly 1 winner expected among 100 concurrent TryAcquire, got %d", got)
		}
		ReleaseDailyReportGeneration()
	})
}
