package handlers

import "sync"

// FailureTracker keeps count of consecutive ingress failures per logical key.
type FailureTracker struct {
	mu        sync.Mutex
	counts    map[string]int
}

func NewFailureTracker() *FailureTracker {
	return &FailureTracker{
		counts: make(map[string]int),
	}
}

func (ft *FailureTracker) RecordFailure(key string) int {
	if key == "" {
		return 0
	}
	ft.mu.Lock()
	defer ft.mu.Unlock()
	ft.counts[key]++
	return ft.counts[key]
}

func (ft *FailureTracker) RecordSuccess(key string) {
	if key == "" {
		return
	}
	ft.mu.Lock()
	defer ft.mu.Unlock()
	delete(ft.counts, key)
}

func (ft *FailureTracker) Current(key string) int {
	ft.mu.Lock()
	defer ft.mu.Unlock()
	return ft.counts[key]
}
