package app

import (
	"sync"

	"gioui.org/io/key"
)

// Gio does not translate Windows VK_INSERT. The window subclass forwards it
// here without accessing UI state from the native window thread.
const insertKey key.Name = "Insert"

type nativeKeyInput struct {
	mu         sync.Mutex
	events     []key.Event
	invalidate func()
}

func (q *nativeKeyInput) push(e key.Event) {
	q.mu.Lock()
	q.events = append(q.events, e)
	q.mu.Unlock()
	q.invalidate()
}

func (q *nativeKeyInput) drain() []key.Event {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	events := q.events
	q.events = nil
	return events
}
