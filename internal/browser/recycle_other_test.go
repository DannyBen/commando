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
