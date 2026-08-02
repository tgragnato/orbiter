package signal

import "sync"

// MemoryDispatcher stores signal messages in memory for Bubble Tea UI consumption.
type MemoryDispatcher struct {
	mu       sync.Mutex
	messages []Message
}

// NewMemoryDispatcher creates an empty in-memory signal queue.
func NewMemoryDispatcher() *MemoryDispatcher {
	return &MemoryDispatcher{}
}

// Dispatch appends one message to the queue.
func (d *MemoryDispatcher) Dispatch(message Message) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.messages = append(d.messages, message)
	return nil
}

// Messages returns a copy of all queued messages.
func (d *MemoryDispatcher) Messages() []Message {
	d.mu.Lock()
	defer d.mu.Unlock()

	clone := make([]Message, len(d.messages))
	copy(clone, d.messages)
	return clone
}

// Drain returns all queued messages and clears the queue.
func (d *MemoryDispatcher) Drain() []Message {
	d.mu.Lock()
	defer d.mu.Unlock()

	clone := make([]Message, len(d.messages))
	copy(clone, d.messages)
	d.messages = nil
	return clone
}
