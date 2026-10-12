package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestDirectoryMonitorSharesWatchesAndRejectsRetiredCallbacks(t *testing.T) {
	type armed struct {
		path    string
		ctx     context.Context
		changed func()
	}
	started := make(chan armed, 3)
	var wakes atomic.Int32
	m := newDirectoryMonitor(func(ctx context.Context, path string, changed func()) error {
		started <- armed{path, ctx, changed}
		<-ctx.Done()
		return ctx.Err()
	}, func() { wakes.Add(1) })
	t.Cleanup(m.close)
	m.sync([]string{"one", "one", "two"})
	watches := make(map[string]armed)
	for range 2 {
		select {
		case w := <-started:
			watches[w.path] = w
		case <-time.After(3 * time.Second):
			t.Fatal("directory notification source did not start")
		}
	}
	m.sync([]string{"one"})
	select {
	case <-watches["two"].ctx.Done():
	default:
		t.Fatal("leaving a directory did not cancel its notification source")
	}
	watches["two"].changed()
	if wakes.Load() != 0 {
		t.Fatal("retired callback woke the window")
	}
	for range 10 {
		watches["one"].changed()
	}
	paths, next := m.take(time.Now())
	if len(paths) != 0 || next.IsZero() {
		t.Fatal("notification burst was not delayed")
	}
	paths, _ = m.take(time.Now().Add(directoryChangeDelay))
	if len(paths) != 1 || paths[0] != "one" {
		t.Fatal("burst did not become one directory change")
	}
	paths, next = m.take(time.Now().Add(time.Second))
	if len(paths) != 0 || !next.IsZero() {
		t.Fatal("consumed directory change was repeated")
	}
	m.close()
	select {
	case <-watches["one"].ctx.Done():
	default:
		t.Fatal("shutdown did not stop the remaining watcher")
	}
}
