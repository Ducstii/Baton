package daemon

import "sync"

// SSEEvent represents a server-sent event payload.
type SSEEvent struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// SSEBroker fans out events to subscribers per run ID.
type SSEBroker struct {
	mu   sync.RWMutex
	subs map[string][]chan SSEEvent
}

// NewSSEBroker creates a new SSE broker.
func NewSSEBroker() *SSEBroker {
	return &SSEBroker{
		subs: make(map[string][]chan SSEEvent),
	}
}

// Publish sends an event to all subscribers of the given run ID.
func (b *SSEBroker) Publish(runID string, event SSEEvent) {
	b.mu.RLock()
	chs := b.subs[runID]
	b.mu.RUnlock()
	for _, ch := range chs {
		select {
		case ch <- event:
		default:
		}
	}
}

// Subscribe adds a new subscriber channel for the given run ID.
// Returns a read-only channel that receives SSEEvent values.
func (b *SSEBroker) Subscribe(runID string) <-chan SSEEvent {
	ch := make(chan SSEEvent, 16)
	b.mu.Lock()
	b.subs[runID] = append(b.subs[runID], ch)
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber channel for the given run ID and closes it.
func (b *SSEBroker) Unsubscribe(runID string, ch <-chan SSEEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	chs := b.subs[runID]
	for i, c := range chs {
		if c == ch {
			b.subs[runID] = append(chs[:i], chs[i+1:]...)
			close(c)
			break
		}
	}
}
