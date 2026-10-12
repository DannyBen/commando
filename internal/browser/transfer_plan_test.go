package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func writeTransferFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func assertTransferFile(t *testing.T, path, want string) {
	t.Helper()
	if data, err := os.ReadFile(path); err != nil || string(data) != want {
		t.Fatalf("%s: got %q, %v; want %q", path, data, err, want)
	}
}

func TestPlanTransferCountsFilesAndAnchorsNestedConflicts(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	writeTransferFiles(t, source, map[string]string{"folder/nested/same.txt": "new", "folder/new.txt": "new", "a.txt": "new", "z.txt": "new"})
	writeTransferFiles(t, target, map[string]string{"folder/nested/same.txt": "old", "folder/keep.txt": "keep", "a.txt": "old"})
	plan, err := PlanTransfer([]string{filepath.Join(source, "folder"), filepath.Join(source, "a.txt"), filepath.Join(source, "z.txt")}, target)
	if err != nil || plan.Conflicts != 2 || plan.SingleFile || !plan.Merges || plan.Subject.Name != "folder" || !plan.Subject.Directory {
		t.Fatalf("wrong conflict scope: %+v %v", plan, err)
	}
	assertTransferFile(t, filepath.Join(target, "folder", "nested", "same.txt"), "old")
	if _, err := os.Stat(filepath.Join(target, "folder", "new.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("planning changed destination")
	}
	result, err := ExecuteTransfer(plan, false, TransferExclusive)
	if !errors.Is(err, fs.ErrExist) || len(result.Items) != 0 {
		t.Fatal("conflicts ran without an explicit policy")
	}
}

func TestPlannedBatchOverwriteAndSkipMergeWithoutRemovingExtras(t *testing.T) {
	for _, move := range []bool{false, true} {
		for _, policy := range []TransferPolicy{TransferOverwrite, TransferSkipExisting} {
			source, target := t.TempDir(), t.TempDir()
			writeTransferFiles(t, source, map[string]string{"folder/nested/same.txt": "new", "folder/new.txt": "added", "a.txt": "new a", "z.txt": "z"})
			writeTransferFiles(t, target, map[string]string{"folder/nested/same.txt": "old", "folder/keep.txt": "keep", "a.txt": "old a"})
			paths := []string{filepath.Join(source, "folder"), filepath.Join(source, "a.txt"), filepath.Join(source, "z.txt")}
			plan, err := PlanTransfer(paths, target)
			if err != nil {
				t.Fatal(err)
			}
			result, err := ExecuteTransfer(plan, move, policy)
			if err != nil {
				t.Fatal(err)
			}
			assertTransferFile(t, filepath.Join(target, "folder", "keep.txt"), "keep")
			assertTransferFile(t, filepath.Join(target, "folder", "new.txt"), "added")
			assertTransferFile(t, filepath.Join(target, "z.txt"), "z")
			if policy == TransferSkipExisting {
				if result.Skipped != 2 || len(result.Items) != 2 || result.Items[0].Moved {
					t.Fatalf("wrong skipped/partial results: %+v", result)
				}
				assertTransferFile(t, filepath.Join(target, "folder", "nested", "same.txt"), "old")
				assertTransferFile(t, filepath.Join(target, "a.txt"), "old a")
				assertTransferFile(t, filepath.Join(source, "folder", "nested", "same.txt"), "new")
				assertTransferFile(t, filepath.Join(source, "a.txt"), "new a")
			} else {
				if result.Skipped != 0 || len(result.Items) != 3 {
					t.Fatalf("wrong overwrite results: %+v", result)
				}
				assertTransferFile(t, filepath.Join(target, "folder", "nested", "same.txt"), "new")
				assertTransferFile(t, filepath.Join(target, "a.txt"), "new a")
				if move {
					for _, path := range paths {
						if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
							t.Fatal("fully moved source remained")
						}
					}
				} else {
					assertTransferFile(t, filepath.Join(source, "folder", "nested", "same.txt"), "new")
				}
			}
			_, err = os.Stat(filepath.Join(source, "folder", "new.txt"))
			if move && !errors.Is(err, fs.ErrNotExist) || !move && err != nil {
				t.Fatal("new child had wrong source retention")
			}
			entries, err := os.ReadDir(target)
			if err != nil || len(entries) != 3 {
				t.Fatal("batch leaked staging files")
			}
		}
	}
}

func TestPlannedMergeWithoutFileCollisionsAndEmptyFolders(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	writeTransferFiles(t, source, map[string]string{"folder/new.txt": "new"})
	writeTransferFiles(t, target, map[string]string{"folder/extra.txt": "extra"})
	if err := os.Mkdir(filepath.Join(source, "folder", "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanTransfer([]string{filepath.Join(source, "folder")}, target)
	if err != nil || plan.Conflicts != 0 || !plan.Merges {
		t.Fatalf("directory merge required choice: %+v %v", plan, err)
	}
	result, err := ExecuteTransfer(plan, true, TransferExclusive)
	if err != nil || len(result.Items) != 1 || !result.Items[0].Moved {
		t.Fatalf("merge move: %+v %v", result, err)
	}
	assertTransferFile(t, filepath.Join(target, "folder", "extra.txt"), "extra")
	if info, err := os.Stat(filepath.Join(target, "folder", "empty")); err != nil || !info.IsDir() {
		t.Fatal("empty source directory was lost")
	}
}

func TestBatchTypeMismatchPreflightsBeforeAnyTransfer(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	writeTransferFiles(t, source, map[string]string{"new.txt": "new", "folder/child": "child"})
	writeTransferFiles(t, target, map[string]string{"folder": "existing file"})
	if _, err := PlanTransfer([]string{filepath.Join(source, "new.txt"), filepath.Join(source, "folder")}, target); !errors.Is(err, ErrTransferType) {
		t.Fatalf("file/folder mismatch accepted: %v", err)
	}
	assertTransferFile(t, filepath.Join(target, "folder"), "existing file")
	if _, err := os.Stat(filepath.Join(target, "new.txt")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("type mismatch partially executed batch")
	}
}

func TestBatchChangedDecisionRefusesWholeBatch(t *testing.T) {
	for _, changeSource := range []bool{false, true} {
		source, target := t.TempDir(), t.TempDir()
		writeTransferFiles(t, source, map[string]string{"first.txt": "first", "same.txt": "new"})
		writeTransferFiles(t, target, map[string]string{"same.txt": "old"})
		plan, err := PlanTransfer([]string{filepath.Join(source, "first.txt"), filepath.Join(source, "same.txt")}, target)
		if err != nil {
			t.Fatal(err)
		}
		changed := filepath.Join(target, "same.txt")
		if changeSource {
			changed = filepath.Join(source, "same.txt")
		}
		if err := os.WriteFile(changed, []byte("external change"), 0600); err != nil {
			t.Fatal(err)
		}
		result, err := ExecuteTransfer(plan, true, TransferOverwrite)
		if !errors.Is(err, ErrConflictChanged) || len(result.Items) != 0 {
			t.Fatalf("stale batch ran: %+v %v", result, err)
		}
		assertTransferFile(t, changed, "external change")
		assertTransferFile(t, filepath.Join(source, "first.txt"), "first")
		if _, err := os.Stat(filepath.Join(target, "first.txt")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatal("earlier batch item ran before stale decision was rejected")
		}
	}
}

func TestPlannedBatchLateArrivalStopsAndRetainsConfirmedPrefix(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	writeTransferFiles(t, source, map[string]string{"first.txt": "first", "second.txt": "second"})
	plan, err := PlanTransfer([]string{filepath.Join(source, "first.txt"), filepath.Join(source, "second.txt")}, target)
	if err != nil {
		t.Fatal(err)
	}
	result, err := executeTransfer(plan, true, TransferOverwrite, func(src, dst string, move bool) (transferOutcome, error) {
		if filepath.Base(src) == "first.txt" {
			if err := os.WriteFile(filepath.Join(target, "second.txt"), []byte("late arrival"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		return transferOne(src, dst, move)
	})
	if !errors.Is(err, fs.ErrExist) || len(result.Items) != 1 || !result.Items[0].Moved {
		t.Fatalf("late arrival: %+v %v", result, err)
	}
	assertTransferFile(t, filepath.Join(target, "second.txt"), "late arrival")
	assertTransferFile(t, filepath.Join(source, "second.txt"), "second")
}

func TestPartialMergeFailureRetainsSourceAndConfirmedChildren(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	writeTransferFiles(t, source, map[string]string{"folder/a.txt": "new", "folder/b.txt": "remaining"})
	writeTransferFiles(t, target, map[string]string{"folder/a.txt": "old"})
	plan, err := PlanTransfer([]string{filepath.Join(source, "folder")}, target)
	if err != nil {
		t.Fatal(err)
	}
	result, err := executeTransfer(plan, true, TransferOverwrite, func(string, string, bool) (transferOutcome, error) { return transferOutcome{}, fs.ErrPermission })
	if !errors.Is(err, fs.ErrPermission) || len(result.Items) != 1 || result.Items[0].Moved || len(result.Changes) != 1 || !result.Changes[0].Moved {
		t.Fatalf("partial merge: %+v %v", result, err)
	}
	assertTransferFile(t, filepath.Join(target, "folder", "a.txt"), "new")
	assertTransferFile(t, filepath.Join(source, "folder", "b.txt"), "remaining")
}

func TestMergeMoveNeverRemovesNewSourceChildren(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	writeTransferFiles(t, source, map[string]string{"folder/a.txt": "a"})
	writeTransferFiles(t, target, map[string]string{"folder/extra.txt": "extra"})
	plan, err := PlanTransfer([]string{filepath.Join(source, "folder")}, target)
	if err != nil {
		t.Fatal(err)
	}
	result, err := executeTransfer(plan, true, TransferExclusive, func(src, dst string, move bool) (transferOutcome, error) {
		outcome, err := transferOne(src, dst, move)
		writeTransferFiles(t, source, map[string]string{"folder/new-arrival.txt": "keep"})
		return outcome, err
	})
	if err == nil || len(result.Items) != 1 || result.Items[0].Moved {
		t.Fatalf("new source child was accepted as complete move: %+v %v", result, err)
	}
	assertTransferFile(t, filepath.Join(source, "folder", "new-arrival.txt"), "keep")
	assertTransferFile(t, filepath.Join(target, "folder", "a.txt"), "a")
}

func TestTransferPlanRejectsSelfOverlapAndFolderLinks(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	writeTransferFiles(t, source, map[string]string{"folder/a.txt": "a"})
	for _, paths := range [][]string{{filepath.Join(source, "folder"), filepath.Join(source, "folder", "a.txt")}, {filepath.Join(source, "folder"), filepath.Join(source, "folder")}} {
		if _, err := PlanTransfer(paths, target); err == nil {
			t.Fatal("overlapping or duplicate sources accepted")
		}
	}
	if _, err := PlanTransfer([]string{filepath.Join(source, "folder")}, source); !errors.Is(err, ErrSameTransferFile) {
		t.Fatalf("same directory accepted: %v", err)
	}
	if err := os.Symlink(filepath.Join(source, "folder"), filepath.Join(target, "folder")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if _, err := PlanTransfer([]string{filepath.Join(source, "folder")}, target); !errors.Is(err, ErrTransferType) {
		t.Fatalf("destination link was followed for merging: %v", err)
	}
}
