package app

import (
	"sync"

	"gioui.org/io/key"
)

// Gio does not translate Windows VK_INSERT. The window subclass forwards it
// here without accessing UI state from the native window thread.
const insertKey key.Name = "Insert"
const contextMenuKey key.Name = "ContextMenu"

type nativeKeyInput struct {
	mu         sync.Mutex
	events     []key.Event
	invalidate func()
	interacted bool
}

func (q *nativeKeyInput) noticeInteraction() {
	q.mu.Lock()
	q.interacted = true
	q.mu.Unlock()
	q.invalidate()
}

func (q *nativeKeyInput) takeInteraction() bool {
	if q == nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	interacted := q.interacted
	q.interacted = false
	return interacted
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
