package app

import (
	"sync"
	"unicode"

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
	altText    map[rune]bool
	capturing  bool
}

func (q *nativeKeyInput) setShortcutCapture(capturing bool) {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.capturing = capturing
}

func (q *nativeKeyInput) setAltText(names []rune) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.altText = make(map[rune]bool, len(names))
	for _, name := range names {
		q.altText[unicode.ToLower(name)] = true
	}
}

// Gio receives the key event. Its extra Windows system-character message must
// not reach the default menu handler and ring a bell for an assigned shortcut.
func (q *nativeKeyInput) suppressesAltText(name rune) bool {
	if q == nil {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.capturing || q.altText[unicode.ToLower(name)]
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

// Ignore repeats, but retain releases so foreground sessions can track held keys.
// Browsing actions only execute on the press.
func (q *nativeKeyInput) altEnter(data uintptr, modifiers key.Modifiers) {
	if q == nil {
		return
	}
	state := key.Press
	if data&(1<<31) != 0 {
		state = key.Release
	} else if data&(1<<30) != 0 {
		return
	}
	q.push(key.Event{Name: key.NameReturn, Modifiers: modifiers, State: state})
}
