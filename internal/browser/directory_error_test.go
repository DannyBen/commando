package browser

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMissingDirectoryIncludesFormerFolderReplacedByFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "folder")
	if err := os.WriteFile(path, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadDirectory(path); !IsMissingDirectory(err) {
		t.Fatalf("file replacement was not a missing directory: %v", err)
	}
	if _, err := ReadDirectory(filepath.Join(root, "missing")); !IsMissingDirectory(err) {
		t.Fatalf("absent folder was not a missing directory: %v", err)
	}
	for _, err := range []error{nil, os.ErrPermission, errors.New("unrelated read failure")} {
		if IsMissingDirectory(err) {
			t.Fatalf("unrelated read failure permits parent recovery: %v", err)
		}
	}
}
