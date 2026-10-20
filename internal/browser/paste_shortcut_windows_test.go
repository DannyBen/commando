package browser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsPastedShortcutsRoundTripThroughShell(t *testing.T) {
	root := resolvedTempDir(t)
	folder, file := filepath.Join(root, "תיקיה with spaces"), filepath.Join(root, "notes.txt")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := CreateShortcuts([]string{folder, file}, root)
	if err != nil || result.Summary.Completed != 2 {
		t.Fatal("Windows shortcut creation failed", result, err)
	}
	for i, item := range result.Items {
		savedTarget, savedErr := ShortcutTarget(filepath.Join(root, item.Entry.Name))
		wantSaved := file
		if i == 0 {
			wantSaved = folder
		}
		if savedErr != nil || !strings.EqualFold(savedTarget, wantSaved) {
			t.Fatalf("saved shortcut target = %q, %v; want %q", savedTarget, savedErr, wantSaved)
		}
		target, err := FolderShortcutTarget(filepath.Join(root, item.Entry.Name))
		want := ""
		if i == 0 {
			want = folder
		}
		if err != nil || !strings.EqualFold(target, want) {
			t.Fatalf("created shortcut target = %q, %v; want %q", target, err, want)
		}
	}
}
