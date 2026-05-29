package tagging

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"syntopica-backend/internal/domain/models"
)

func TestConcurrentSemaphoreBehavior(t *testing.T) {
	var processed int64
	concurrency := 3

	processJobMock := func(job models.TagJob) {
		atomic.AddInt64(&processed, 1)
		time.Sleep(10 * time.Millisecond)
	}

	start := time.Now()
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			processJobMock(models.TagJob{})
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)

	if atomic.LoadInt64(&processed) != 6 {
		t.Fatalf("expected 6 processed, got %d", atomic.LoadInt64(&processed))
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("took %v, expected <50ms with concurrency=3", elapsed)
	}
}
