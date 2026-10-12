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

func TestWindowsDirectoryWatchStopsAfterWatchedFolderMoves(t *testing.T) {
	root := t.TempDir()
	path, moved := filepath.Join(root, "watched"), filepath.Join(root, "moved")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	changes, done := make(chan struct{}, 8), make(chan error, 1)
	go func() {
		done <- WatchDirectory(ctx, path, func() {
			select {
			case changes <- struct{}{}:
			default:
			}
		})
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("watch did not release its handles")
		}
	})
	select {
	case <-changes:
	case err := <-done:
		t.Fatalf("watch did not arm: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("watch did not arm")
	}
	if err := os.Rename(path, moved); err != nil {
		t.Fatal(err)
	}
	// A retained notification handle may still observe the moved directory.
	if err := os.WriteFile(filepath.Join(moved, "changed.txt"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing watched path was not detected: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watch continued monitoring the moved directory")
	}
}
