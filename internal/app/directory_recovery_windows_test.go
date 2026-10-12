package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dannyben/commando/internal/browser"
)

// Exercise native notifications, the actual directory worker and panel recovery
// together. The ancestor moves without any writes in the watched empty child.
func TestWindowsNotificationsRecoverOpenDescendantsAutomatically(t *testing.T) {
	root := t.TempDir()
	ancestor := filepath.Join(root, "ancestor")
	child := filepath.Join(ancestor, "empty")
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView([]string{child, child, root}, nil)
	t.Cleanup(v.finishOperations)
	for pi := range v.panels {
		waitLoaded(t, v, pi)
	}
	armed, wake := make(chan string, 16), make(chan struct{}, 16)
	v.monitor = newDirectoryMonitor(func(ctx context.Context, path string, changed func()) error {
		first := true
		return browser.WatchDirectory(ctx, path, func() {
			if first {
				first = false
				changed()
				armed <- path
				return
			}
			changed()
		})
	}, func() {
		select {
		case wake <- struct{}{}:
		default:
		}
	})
	v.monitor.sync(v.fileOperationLocations())
	for range 2 { // One shared child watch and one root watch.
		select {
		case <-armed:
		case <-time.After(3 * time.Second):
			t.Fatal("native notification provider did not arm")
		}
	}
	// Consume initial baselines so only actual removal notifications can pass.
	runAutomaticRefresh(v, time.Now().Add(time.Second))
	finishAutomaticRefresh(t, v)
	for len(wake) != 0 {
		<-wake
	}
	if err := os.Rename(ancestor, filepath.Join(root, "moved")); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for v.panels[0].path != root || v.panels[1].path != root {
		select {
		case <-wake:
			runAutomaticRefresh(v, time.Now().Add(time.Second))
			finishAutomaticRefresh(t, v)
		case <-deadline.C:
			t.Fatal("missing open descendants did not automatically recover")
		}
	}
	if v.active != 0 || v.panels[0].err != nil || v.panels[1].err != nil {
		t.Fatal("native recovery stole focus or left stale errors")
	}
}
