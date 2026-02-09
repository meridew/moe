package server

import (
	"fmt"
	"sync"
	"time"
)

// ── Provider Status Tracker ─────────────────────────────────────────────

// ProviderStatus represents the current connectivity state of a provider.
type ProviderStatus struct {
	Name        string        `json:"name"`
	Type        string        `json:"type"`
	Status      string        `json:"status"` // "connected", "error", "unchecked", "checking"
	Error       string        `json:"error,omitempty"`
	CheckedAt   time.Time     `json:"checked_at"`
	Latency     time.Duration `json:"latency"`
	ConsecFails int           `json:"consec_fails"`
}

// statusTracker keeps an in-memory map of provider statuses, safe for
// concurrent reads and writes.
type statusTracker struct {
	mu       sync.RWMutex
	statuses map[string]*ProviderStatus
}

func newStatusTracker() *statusTracker {
	return &statusTracker{
		statuses: make(map[string]*ProviderStatus),
	}
}

// Set stores a status for a provider, replacing any existing entry.
func (st *statusTracker) Set(s *ProviderStatus) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.statuses[s.Name] = s
}

// Get returns the status for a single provider (nil if never checked).
func (st *statusTracker) Get(name string) *ProviderStatus {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.statuses[name]
}

// All returns a copy of all statuses.
func (st *statusTracker) All() map[string]*ProviderStatus {
	st.mu.RLock()
	defer st.mu.RUnlock()
	out := make(map[string]*ProviderStatus, len(st.statuses))
	for k, v := range st.statuses {
		out[k] = v
	}
	return out
}

// Remove deletes a provider's status from tracking.
func (st *statusTracker) Remove(name string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.statuses, name)
}

// ── Activity Log ────────────────────────────────────────────────────────

// ActivityEvent represents a single entry in the activity log.
type ActivityEvent struct {
	Time     time.Time `json:"time"`
	Provider string    `json:"provider"`
	Type     string    `json:"type"` // "info", "success", "error", "warning"
	Message  string    `json:"message"`
}

// activityLog is a thread-safe ring buffer of recent events.
type activityLog struct {
	mu     sync.RWMutex
	events []ActivityEvent
	cap    int
	seq    int64 // monotonic sequence for change detection
}

func newActivityLog(capacity int) *activityLog {
	return &activityLog{
		events: make([]ActivityEvent, 0, capacity),
		cap:    capacity,
	}
}

// Add appends an event, evicting the oldest if at capacity.
func (al *activityLog) Add(e ActivityEvent) {
	al.mu.Lock()
	defer al.mu.Unlock()
	if len(al.events) >= al.cap {
		al.events = al.events[1:]
	}
	al.events = append(al.events, e)
	al.seq++
}

// Recent returns up to n most recent events (newest first).
func (al *activityLog) Recent(n int) []ActivityEvent {
	al.mu.RLock()
	defer al.mu.RUnlock()

	total := len(al.events)
	if n > total {
		n = total
	}

	// Return in reverse chronological order.
	out := make([]ActivityEvent, n)
	for i := 0; i < n; i++ {
		out[i] = al.events[total-1-i]
	}
	return out
}

// Seq returns the current sequence number, useful for htmx polling to
// detect whether new events have arrived.
func (al *activityLog) Seq() int64 {
	al.mu.RLock()
	defer al.mu.RUnlock()
	return al.seq
}

// Logf is a convenience method that creates and adds an event.
func (al *activityLog) Logf(provider, eventType, format string, args ...any) {
	msg := format
	if len(args) > 0 {
		msg = fmt.Sprintf(format, args...)
	}
	al.Add(ActivityEvent{
		Time:     time.Now().UTC(),
		Provider: provider,
		Type:     eventType,
		Message:  msg,
	})
}
