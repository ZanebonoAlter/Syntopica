package jobs

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestFirecrawlTriggerNowRejectsWhenAlreadyExecuting(t *testing.T) {
	scheduler := NewFirecrawlScheduler()
	scheduler.executionMutex.Lock()
	defer scheduler.executionMutex.Unlock()

	result := scheduler.TriggerNow()
	if result["accepted"] != false {
		t.Fatalf("accepted = %v, want false", result["accepted"])
	}
	if result["reason"] != "already_running" {
		t.Fatalf("reason = %v, want already_running", result["reason"])
	}
}

func TestFirecrawlTriggerNowBatchID(t *testing.T) {
	sourcePath := "firecrawl.go"
	content, err := os.ReadFile(sourcePath) //nolint:gosec
	if err != nil {
		t.Fatalf("read %s: %v", sourcePath, err)
	}

	source := string(content)

	batchIDPattern := regexp.MustCompile(`batchID := time\.Now\(\)\.Format\("20060102150405"\)`)
	if !batchIDPattern.MatchString(source) {
		t.Fatalf("TriggerNow should generate batchID with timestamp format YYYYMMDDHHMMSS")
	}

	if !strings.Contains(source, `go s.runCrawlCycle(batchID)`) {
		t.Fatalf("TriggerNow should pass batchID into runCrawlCycle")
	}

	if !strings.Contains(source, `"batch_id": batchID`) {
		t.Fatalf("TriggerNow should return batch_id in success response")
	}

	if !strings.Contains(source, `func (s *FirecrawlScheduler) runCrawlCycle(batchID string)`) {
		t.Fatalf("runCrawlCycle should accept batchID parameter")
	}
}

func TestFirecrawlRunCrawlCycleUsesInjectedBatchID(t *testing.T) {
	sourcePath := "firecrawl.go"
	content, err := os.ReadFile(sourcePath) //nolint:gosec
	if err != nil {
		t.Fatalf("read %s: %v", sourcePath, err)
	}

	source := string(content)

	if !strings.Contains(source, `func (s *FirecrawlScheduler) runCrawlCycle(batchID string)`) {
		t.Fatalf("runCrawlCycle should accept batchID parameter")
	}

	if strings.Contains(source, `batchID := time.Now().Format("20060102150405")`) && !strings.Contains(source, `func (s *FirecrawlScheduler) TriggerNow()`) {
		t.Fatalf("unexpected batchID generation location")
	}

	runCrawlCycleBodyStart := strings.Index(source, `func (s *FirecrawlScheduler) runCrawlCycle(batchID string) {`)
	if runCrawlCycleBodyStart == -1 {
		t.Fatalf("runCrawlCycle should accept batchID parameter")
	}

	runCrawlCycleBody := source[runCrawlCycleBodyStart:]
	if strings.Contains(runCrawlCycleBody, `time.Now().Format("20060102150405")`) {
		t.Fatalf("runCrawlCycle should use the passed batchID instead of generating a new one")
	}

	if !strings.Contains(source, `s.broadcastProgress(batchID, "processing"`) {
		t.Fatalf("runCrawlCycle should broadcast processing updates with batchID")
	}

	if !strings.Contains(source, `s.broadcastProgress(batchID, "completed"`) {
		t.Fatalf("runCrawlCycle should broadcast completed updates with batchID")
	}
}
