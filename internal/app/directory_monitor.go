package app

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/dannyben/commando/internal/browser"
)

const directoryChangeDelay = 150 * time.Millisecond

type directoryWatch struct {
	path    string
	cancel  context.CancelFunc
	dirtyAt time.Time
	failed  bool
}

// Monitor callbacks only mark directories dirty and wake the window. The UI
// consumes these signals; all listing reads still use the directory worker.
type directoryMonitor struct {
	mu         sync.Mutex
	watches    map[string]*directoryWatch
	watch      func(context.Context, string, func()) error
	invalidate func()
	workers    sync.WaitGroup
}

func directoryWatchKey(path string) string { return transferNameKey(filepath.Clean(path)) }

func newDirectoryMonitor(watch func(context.Context, string, func()) error, invalidate func()) *directoryMonitor {
	return &directoryMonitor{watches: make(map[string]*directoryWatch), watch: watch, invalidate: invalidate}
}

func (m *directoryMonitor) sync(paths []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	wanted := make(map[string]string)
	for _, path := range paths {
		if path != "" {
			wanted[directoryWatchKey(path)] = path
		}
	}
	for key, w := range m.watches {
		if _, keep := wanted[key]; !keep {
			delete(m.watches, key)
			w.cancel()
		}
	}
	for key, path := range wanted {
		if m.watches[key] != nil {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		w := &directoryWatch{path: path, cancel: cancel}
		m.watches[key] = w
		m.workers.Add(1)
		go func() {
			defer m.workers.Done()
			err := m.watch(ctx, path, func() { m.changed(w) })
			if err != nil && ctx.Err() == nil {
				if browser.IsMissingDirectory(err) {
					// A folder can vanish between its read and watch setup. Queue
					// one recovery read even when no native handle could be armed.
					m.changed(w)
				}
				m.mu.Lock()
				w.failed = true
				m.mu.Unlock()
			}
		}()
	}
}

func (m *directoryMonitor) changed(w *directoryWatch) {
	m.mu.Lock()
	if m.watches[directoryWatchKey(w.path)] != w || w.failed {
		m.mu.Unlock()
		return // A callback from a retired or suspended directory.
	}
	if w.dirtyAt.IsZero() {
		w.dirtyAt = time.Now()
	}
	m.mu.Unlock()
	if m.invalidate != nil {
		m.invalidate()
	}
}

// A missing location cannot produce useful updates. Retain the failed watch
// entry to prevent sync from immediately reopening it; Ctrl+R can re-arm it.
func (m *directoryMonitor) suspend(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w := m.watches[directoryWatchKey(path)]; w != nil {
		w.failed = true
		w.dirtyAt = time.Time{}
		w.cancel()
	}
}

// Take a burst once it is due. A fixed window also refreshes during sustained
// writes instead of waiting indefinitely for the directory to become quiet.
func (m *directoryMonitor) take(now time.Time) (paths []string, next time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, w := range m.watches {
		if w.dirtyAt.IsZero() {
			continue
		}
		due := w.dirtyAt.Add(directoryChangeDelay)
		if !now.Before(due) {
			paths = append(paths, w.path)
			w.dirtyAt = time.Time{}
		} else if next.IsZero() || due.Before(next) {
			next = due
		}
	}
	return
}

// A manual refresh can re-arm a watch whose native provider failed. There is
// no automatic retry loop or polling fallback for unsupported locations.
func (m *directoryMonitor) retry(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := directoryWatchKey(path)
	if w := m.watches[key]; w != nil && w.failed {
		delete(m.watches, key)
		w.cancel()
	}
}

func (m *directoryMonitor) close() {
	m.mu.Lock()
	for key, w := range m.watches {
		delete(m.watches, key)
		w.cancel()
	}
	m.mu.Unlock()
	m.workers.Wait()
}
