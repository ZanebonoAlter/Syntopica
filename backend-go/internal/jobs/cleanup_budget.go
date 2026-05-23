package jobs

import (
	"math"
	"sync"
	"sync/atomic"
	"time"

	"syntopica-backend/internal/platform/logging"
)

type CleanupBudgetStats struct {
	TotalConsumed int            `json:"total_consumed"`
	TotalBudget   int            `json:"total_budget"`
	PhaseConsumed map[string]int `json:"phase_consumed"`
	PhaseBudget   map[string]int `json:"phase_budget"`
	TimedOut      bool           `json:"timed_out"`
}

type CleanupBudget struct {
	totalBudget atomic.Int32
	consumed    atomic.Int32
	deadline    time.Time
	mu          sync.Mutex
	phaseQuota  map[string]int
	phaseUsed   map[string]int
	timedOut    atomic.Bool
}

func NewCleanupBudget(totalBudget int, timeout time.Duration) *CleanupBudget {
	b := &CleanupBudget{
		deadline:   time.Now().Add(timeout),
		phaseQuota: make(map[string]int),
		phaseUsed:  make(map[string]int),
	}
	if totalBudget > math.MaxInt32 || totalBudget < math.MinInt32 {
		logging.Warnf("cleanup budget %d out of int32 range, clamping", totalBudget)
		if totalBudget > math.MaxInt32 {
			totalBudget = math.MaxInt32
		} else {
			totalBudget = math.MinInt32
		}
	}
	b.totalBudget.Store(int32(totalBudget)) //nolint:gosec
	return b
}

func (b *CleanupBudget) SetPhaseQuota(phase string, quota int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.phaseQuota[phase] = quota
}

func (b *CleanupBudget) Consume() bool {
	if b.checkTimeout() {
		return false
	}
	for {
		current := b.consumed.Load()
		if current >= b.totalBudget.Load() {
			return false
		}
		if b.consumed.CompareAndSwap(current, current+1) {
			return true
		}
	}
}

func (b *CleanupBudget) ConsumeForPhase(phase string) bool {
	b.mu.Lock()
	if quota, ok := b.phaseQuota[phase]; ok && b.phaseUsed[phase] >= quota {
		b.mu.Unlock()
		return false
	}
	if b.checkTimeout() {
		b.mu.Unlock()
		return false
	}
	current := b.consumed.Load()
	if current >= b.totalBudget.Load() {
		b.mu.Unlock()
		return false
	}
	b.consumed.Store(current + 1)
	b.phaseUsed[phase]++
	b.mu.Unlock()
	return true
}

func (b *CleanupBudget) IsTimedOut() bool {
	return b.checkTimeout()
}

func (b *CleanupBudget) Stats() CleanupBudgetStats {
	b.mu.Lock()
	defer b.mu.Unlock()
	phaseConsumed := make(map[string]int)
	for k, v := range b.phaseUsed {
		phaseConsumed[k] = v
	}
	phaseBudget := make(map[string]int)
	for k, v := range b.phaseQuota {
		phaseBudget[k] = v
	}
	return CleanupBudgetStats{
		TotalConsumed: int(b.consumed.Load()),
		TotalBudget:   int(b.totalBudget.Load()),
		PhaseConsumed: phaseConsumed,
		PhaseBudget:   phaseBudget,
		TimedOut:      b.timedOut.Load(),
	}
}

func (b *CleanupBudget) checkTimeout() bool {
	if b.timedOut.Load() {
		return true
	}
	if time.Now().After(b.deadline) {
		b.timedOut.Store(true)
		return true
	}
	return false
}
