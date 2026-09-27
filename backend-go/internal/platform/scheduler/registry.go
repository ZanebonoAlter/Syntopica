package scheduler

import (
	"fmt"
	"sync"
	"time"

	"syntopica-backend/internal/platform/logging"
)

// Scheduler is the interface that all schedulers must implement.
// GetStatus is intentionally not part of this interface — handlers
// access scheduler status through type assertion on the concrete type.
type Scheduler interface {
	Start() error
	Stop()
	TriggerNow() map[string]interface{}
	UpdateInterval(seconds int) error
	ResetStats() error
}

// Registry manages named scheduler instances.
type Registry struct {
	mu    sync.RWMutex
	items map[string]Scheduler
	order []string // registration order, for stable /status output
}

// NewRegistry creates a new scheduler registry.
func NewRegistry() *Registry {
	return &Registry{
		items: make(map[string]Scheduler),
	}
}

// Register adds a scheduler to the registry under the given name.
// Panics if a scheduler with the same name is already registered.
func (r *Registry) Register(name string, s Scheduler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[name]; exists {
		panic(fmt.Sprintf("scheduler %q already registered", name))
	}
	r.items[name] = s
	r.order = append(r.order, name)
}

// Get returns the scheduler registered under the given name.
func (r *Registry) Get(name string) (interface{}, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.items[name]
	return s, ok
}

// OrderedNames returns scheduler keys in registration order. The admin
// handler iterates this to render /api/schedulers/status in a stable order
// (registry insertion order) without maintaining a separate descriptor list.
func (r *Registry) OrderedNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// StartAll starts all registered schedulers in registration order.
// Failures are logged but do not prevent subsequent schedulers from starting.
func (r *Registry) StartAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for name, s := range r.items {
		if err := s.Start(); err != nil {
			logging.Errorf("Failed to start scheduler %q: %v", name, err)
		}
	}
}

// StopAll stops all registered schedulers with a timeout.
func (r *Registry) StopAll(timeout time.Duration) {
	done := make(chan struct{})
	go func() {
		r.mu.RLock()
		defer r.mu.RUnlock()
		for name, s := range r.items {
			s.Stop()
			logging.Infof("Stopped scheduler: %s", name)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		logging.Warnf("Scheduler shutdown timed out after %v", timeout)
	}
}

// All returns a copy of all registered schedulers.
func (r *Registry) All() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]interface{}, len(r.items))
	for name, s := range r.items {
		result[name] = s
	}
	return result
}
