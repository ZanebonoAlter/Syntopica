package analysis

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestAnalysisQueueDedupByKey(t *testing.T) {
	q := newInMemoryAnalysisQueue(nil, zap.NewNop())
	anchor := time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC)

	if err := q.Enqueue(&AnalysisJob{TopicTagID: 10, AnalysisType: AnalysisTypeEvent, WindowType: WindowTypeDaily, AnchorDate: anchor, Priority: AnalysisPriorityLow}); err != nil {
		t.Fatalf("enqueue first: %v", err)
	}
	if err := q.Enqueue(&AnalysisJob{TopicTagID: 10, AnalysisType: AnalysisTypeEvent, WindowType: WindowTypeDaily, AnchorDate: anchor, Priority: AnalysisPriorityHigh}); err != nil {
		t.Fatalf("enqueue duplicate: %v", err)
	}

	job, err := q.Dequeue()
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if job == nil {
		t.Fatal("expected dequeued job")
	}
	if job.Priority != AnalysisPriorityHigh {
		t.Fatalf("expected deduped job with upgraded priority=%d, got %d", AnalysisPriorityHigh, job.Priority)
	}
}

func TestAnalysisQueueRetryFlow(t *testing.T) {
	q := newInMemoryAnalysisQueue(nil, zap.NewNop())

	err := q.Enqueue(&AnalysisJob{TopicTagID: 5, AnalysisType: AnalysisTypeKeyword, WindowType: WindowTypeDaily, Priority: AnalysisPriorityMedium})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	job, err := q.Dequeue()
	if err != nil {
		t.Fatalf("dequeue: %v", err)
	}
	if job == nil {
		t.Fatal("expected job")
	}

	if err := q.UpdateStatus(job.ID, AnalysisStatusFailed, errors.New("temporary failure").Error()); err != nil {
		t.Fatalf("fail: %v", err)
	}

	current, err := q.Get(job.ID)
	if err != nil {
		t.Fatalf("status after fail: %v", err)
	}
	if current.Status != AnalysisStatusPending {
		t.Fatalf("expected pending after retryable fail, got %s", current.Status)
	}

	retryJob, err := q.Dequeue()
	if err != nil {
		t.Fatalf("dequeue retry: %v", err)
	}
	if retryJob == nil {
		t.Fatal("expected retried job")
	}
	if retryJob.RetryCount != 1 {
		t.Fatalf("expected retry_count=1, got %d", retryJob.RetryCount)
	}

	if err := q.Complete(retryJob.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}
	current, err = q.Get(retryJob.ID)
	if err != nil {
		t.Fatalf("status after complete: %v", err)
	}
	if current.Status != AnalysisStatusCompleted {
		t.Fatalf("expected completed, got %s", current.Status)
	}
}
