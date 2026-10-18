//go:build !windows

package browser

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestUnsupportedRecycleLeavesFileUntouched(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Recycle(root, []string{"keep.txt"})
	if len(result.Names) != 0 || !errors.Is(err, ErrRecycleUnavailable) {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "keep" {
		t.Fatalf("file changed: %q, %v", contents, err)
	}
}

func TestUnsupportedPermanentDeleteLeavesSourcesUntouched(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Delete(root, []string{"keep.txt"})
	if len(result.Names) != 0 || !errors.Is(err, ErrDeleteUnavailable) {
		t.Fatal("unsupported deletion was not refused", result, err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "keep" {
		t.Fatal("unsupported deletion changed a source", err)
	}
}
