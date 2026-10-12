package browser

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWindowsDirectoryNotifications(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := t.TempDir()
	changes, done := make(chan struct{}, 8), make(chan error, 1)
	go func() {
		done <- WatchDirectory(ctx, root, func() {
			select {
			case changes <- struct{}{}:
			default:
			}
		})
	}()
	wait := func() {
		t.Helper()
		select {
		case <-changes:
		case err := <-done:
			t.Fatalf("watch ended before notification: %v", err)
		case <-time.After(3 * time.Second):
			t.Fatal("directory notification missing")
		}
	}
	wait() // Watch is armed before the filesystem mutation.
	if err := os.WriteFile(filepath.Join(root, "external.txt"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	wait()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("watch did not cancel cleanly: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not release the native wait")
	}
}
