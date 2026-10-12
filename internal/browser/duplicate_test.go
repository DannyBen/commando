package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assertNoTransferStages(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".commando-transfer-") {
			t.Fatal("transfer stage was not cleaned up")
		}
	}
}

func TestDuplicateNamingAndExclusiveCopies(t *testing.T) {
	root := t.TempDir()
	writeTransferFiles(t, root, map[string]string{
		"report.txt": "report", "report - Copy.txt": "existing", "report - Copy (2).txt": "second",
		".env": "hidden", "folder.v1/nested/file": "nested",
	})
	paths := []string{filepath.Join(root, "report.txt"), filepath.Join(root, ".env"), filepath.Join(root, "folder.v1")}
	plan, err := PlanDuplicate(paths, root)
	if err != nil || plan.Conflicts != 0 || plan.Merges {
		t.Fatalf("duplicate plan: %+v %v", plan, err)
	}
	if _, err := os.Stat(filepath.Join(root, "report - Copy (3).txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("planning created the copy")
	}
	result, err := ExecuteTransfer(plan, false, TransferExclusive)
	if err != nil || len(result.Items) != 3 {
		t.Fatalf("duplicate result: %+v %v", result, err)
	}
	assertTransferFile(t, filepath.Join(root, "report - Copy (3).txt"), "report")
	assertTransferFile(t, filepath.Join(root, ".env - Copy"), "hidden")
	assertTransferFile(t, filepath.Join(root, "folder.v1 - Copy", "nested", "file"), "nested")
	assertTransferFile(t, filepath.Join(root, "report - Copy.txt"), "existing")
	assertTransferFile(t, filepath.Join(root, "report.txt"), "report")
	for _, item := range result.Items {
		if item.Moved || item.Replaced {
			t.Fatal("duplication moved or replaced an entry")
		}
	}
	assertNoTransferStages(t, root)
}

func TestDuplicateEditedNameRejectsCollisionsAndChangedSources(t *testing.T) {
	root := t.TempDir()
	writeTransferFiles(t, root, map[string]string{"a.txt": "source", "existing.txt": "existing"})
	plan, err := PlanDuplicate([]string{filepath.Join(root, "a.txt")}, root)
	if err != nil {
		t.Fatal(err)
	}
	source, suggestion := plan.DuplicateSubject()
	if source.Name != "a.txt" || suggestion != "a - Copy.txt" {
		t.Fatalf("wrong duplicate subject: %+v %q", source, suggestion)
	}
	for _, name := range []string{"a.txt", "existing.txt"} {
		if result, err := DuplicateWithName(plan, name); !errors.Is(err, fs.ErrExist) || len(result.Items) != 0 {
			t.Fatal("existing name was replaced")
		}
	}
	if _, err := DuplicateWithName(plan, "../outside.txt"); !errors.Is(err, ErrInvalidName) {
		t.Fatal("duplicate accepted a navigation path")
	}
	if result, err := DuplicateWithName(plan, "custom.txt"); err != nil || len(result.Items) != 1 {
		t.Fatalf("custom duplicate: %+v %v", result, err)
	}
	assertTransferFile(t, filepath.Join(root, "custom.txt"), "source")
	assertTransferFile(t, filepath.Join(root, "existing.txt"), "existing")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("changed source"), 0600); err != nil {
		t.Fatal(err)
	}
	if result, err := DuplicateWithName(plan, "stale.txt"); !errors.Is(err, ErrConflictChanged) || len(result.Items) != 0 {
		t.Fatal("edited name ignored changed source")
	}
	if _, err := os.Stat(filepath.Join(root, "stale.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("stale copy was published")
	}
}

func TestDuplicateBatchLateCollisionNeverReplacesArrival(t *testing.T) {
	root := t.TempDir()
	writeTransferFiles(t, root, map[string]string{"a.txt": "a", "b.txt": "b"})
	plan, err := PlanDuplicate([]string{filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt")}, root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := executeTransfer(plan, false, TransferExclusive, func(source, destination string, move bool) (transferOutcome, error) {
		if filepath.Base(source) == "b.txt" {
			if err := os.WriteFile(destination, []byte("late arrival"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return transferOne(source, destination, move)
	})
	if !errors.Is(err, fs.ErrExist) || len(result.Items) != 1 {
		t.Fatalf("late collision: %+v %v", result, err)
	}
	assertTransferFile(t, filepath.Join(root, "a - Copy.txt"), "a")
	assertTransferFile(t, filepath.Join(root, "b - Copy.txt"), "late arrival")
	assertTransferFile(t, filepath.Join(root, "b.txt"), "b")
	assertNoTransferStages(t, root)
}

func TestDuplicateLateFolderArrivalDoesNotMerge(t *testing.T) {
	root := t.TempDir()
	writeTransferFiles(t, root, map[string]string{"folder/source.txt": "source"})
	plan, err := PlanDuplicate([]string{filepath.Join(root, "folder")}, root)
	if err != nil {
		t.Fatal(err)
	}
	writeTransferFiles(t, root, map[string]string{"folder - Copy/keep.txt": "arrival"})
	if result, err := ExecuteTransfer(plan, false, TransferExclusive); !errors.Is(err, ErrConflictChanged) || len(result.Items) != 0 {
		t.Fatal("unexpected duplicate folder was merged")
	}
	if _, err := DuplicateWithName(plan, "folder - Copy"); !errors.Is(err, fs.ErrExist) {
		t.Fatal("edited duplicate merged an existing folder")
	}
	if _, err := os.Stat(filepath.Join(root, "folder - Copy", "source.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("duplicate changed an unexpected existing folder")
	}
	assertTransferFile(t, filepath.Join(root, "folder - Copy", "keep.txt"), "arrival")
}

func TestSameFolderSourcesResolveAliases(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	writeTransferFiles(t, root, map[string]string{"a.txt": "a", "folder/file": "f"})
	if same, err := SourcesInDirectory([]string{filepath.Join(root, "a.txt"), filepath.Join(root, "folder")}, root); err != nil || !same {
		t.Fatal("same-folder sources were not detected")
	}
	if same, err := SourcesInDirectory([]string{filepath.Join(root, "a.txt")}, other); err != nil || same {
		t.Fatal("different folders were considered the same")
	}
	alias := filepath.Join(other, "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("directory symlink unavailable: %v", err)
	}
	if same, err := SourcesInDirectory([]string{filepath.Join(root, "a.txt")}, alias); err != nil || !same {
		t.Fatal("folder alias was not detected")
	}
	plan, err := PlanDuplicate([]string{filepath.Join(alias, "folder")}, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteTransfer(plan, false, TransferExclusive); err != nil {
		t.Fatal(err)
	}
	assertTransferFile(t, filepath.Join(root, "folder - Copy", "file"), "f")
}
