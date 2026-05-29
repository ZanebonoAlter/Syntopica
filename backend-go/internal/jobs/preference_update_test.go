package jobs

import (
	"testing"
	"time"
)

func TestPreferenceUpdateSchedulerStartReturnsWithoutDeadlock(t *testing.T) {
	scheduler := NewPreferenceUpdateScheduler(1800)

	done := make(chan error, 1)
	go func() {
		done <- scheduler.Start()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Start did not return; possible mutex deadlock")
	}

	if scheduler.nextRun == nil {
		t.Fatal("nextRun was not initialized")
	}
	if !scheduler.running {
		t.Fatal("scheduler should be marked running after Start")
	}

	scheduler.Stop()
}
