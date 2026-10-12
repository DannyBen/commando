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
	// No writes inside the moved directory: the existence check must detect
	// the missing original path even when the directory was empty.
	select {
	case err := <-done:
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("missing watched path was not detected: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watch continued monitoring the moved directory")
	}
}

func startWindowsLocationWatch(t *testing.T, path string) (<-chan struct{}, <-chan error) {
	t.Helper()
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
			t.Error("watch did not stop and release its handles")
		}
	})
	select {
	case <-changes:
	case err := <-done:
		t.Fatalf("watch could not arm: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("watch did not establish its baseline")
	}
	return changes, done
}

func waitWindowsMissingLocation(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if !IsMissingDirectory(err) {
			t.Fatalf("watch did not detect the unavailable location: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watch missed the removed location")
	}
}

func TestWindowsDirectoryWatchDetectsEmptyFolderDeletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	_, done := startWindowsLocationWatch(t, path)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	waitWindowsMissingLocation(t, done)
}

func TestWindowsDirectoryWatchDetectsRenamedAncestor(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "ancestor")
	child := filepath.Join(parent, "child", "empty")
	if err := os.MkdirAll(child, 0700); err != nil {
		t.Fatal(err)
	}
	_, done := startWindowsLocationWatch(t, child)
	if err := os.Rename(parent, filepath.Join(root, "moved")); err != nil {
		t.Fatal(err)
	}
	waitWindowsMissingLocation(t, done)
}

func TestWindowsDirectoryWatchDetectsMovedAliasTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	_, done := startWindowsLocationWatch(t, alias)
	if err := os.Rename(target, filepath.Join(root, "moved")); err != nil {
		t.Fatal(err)
	}
	waitWindowsMissingLocation(t, done)
}
