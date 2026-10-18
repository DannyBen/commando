package browser

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRecyclePreflightsWholeSelectionBeforeMutation(t *testing.T) {
	root := t.TempDir()
	var resolved []string
	called := false
	result, err := recycle(root, []string{"first", "missing"}, func(path string) (string, error) {
		resolved = append(resolved, path)
		if filepath.Base(path) == "missing" {
			return "", os.ErrNotExist
		}
		return path, nil
	}, func([]string) (removalBatch, error) { called = true; return removalBatch{}, nil })
	if called || len(result.Names) != 0 || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("preflight mutated or hid failure: result=%+v err=%v", result, err)
	}
	want := []string{root, filepath.Join(root, "first"), filepath.Join(root, "missing")}
	if !reflect.DeepEqual(resolved, want) {
		t.Fatalf("resolved %v, want %v", resolved, want)
	}
}

func TestRecyclePassesBackingPathsAndReportsPartialSuccess(t *testing.T) {
	root, backing := t.TempDir(), t.TempDir()
	for _, count := range []int{0, 1, 2} {
		failure := os.ErrPermission
		if count == 2 {
			failure = nil
		}
		result, err := recycle(root, []string{"a", "b"}, func(path string) (string, error) {
			relative, err := filepath.Rel(root, path)
			return filepath.Join(backing, relative), err
		}, func(paths []string) (removalBatch, error) {
			want := []string{filepath.Join(backing, "a"), filepath.Join(backing, "b")}
			if !reflect.DeepEqual(paths, want) {
				t.Fatalf("backend received %v, want %v", paths, want)
			}
			completed := make([]bool, len(paths))
			for i := range count {
				completed[i] = true
			}
			return removalBatch{completed: completed}, failure
		})
		if result.Directory != backing || len(result.Names) != count || !errors.Is(err, failure) {
			t.Fatalf("count=%d result=%+v err=%v", count, result, err)
		}
		if count < 2 {
			var pathError *os.PathError
			if !errors.As(err, &pathError) || pathError.Path != []string{"a", "b"}[count] {
				t.Fatalf("failed item was not identified: %v", err)
			}
		}
	}
}

func TestRecycleRejectsInvalidSelectionWithoutFilesystemCalls(t *testing.T) {
	for _, names := range [][]string{{".."}, {"."}, {""}, {"../outside"}, {`folder\outside`}, {"C:other"}, {"file:stream"}, {"first", "../outside"}, {"a", "A"}} {
		result, err := recycle("unused", names,
			func(string) (string, error) { t.Fatal("invalid selection resolved a path"); return "", nil },
			func([]string) (removalBatch, error) {
				t.Fatal("invalid selection reached backend")
				return removalBatch{}, nil
			})
		if len(result.Names) != 0 || !errors.Is(err, ErrInvalidName) {
			t.Fatalf("accepted %v: %+v, %v", names, result, err)
		}
	}
}

func TestRecycleMappingFailureNeverCallsBackend(t *testing.T) {
	for _, failure := range []error{ErrPathIdentity, ErrDriveMappingCycle, ErrDriveMapping} {
		result, err := recycle(t.TempDir(), []string{"a"},
			func(string) (string, error) { return "", failure },
			func([]string) (removalBatch, error) {
				t.Fatal("mapping failure reached backend")
				return removalBatch{}, nil
			})
		if len(result.Names) != 0 || !errors.Is(err, failure) {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	}
}

func TestPermanentDeleteSharesPreflightAndReportsOnlyConfirmedNames(t *testing.T) {
	root := t.TempDir()
	called := false
	result, err := removeSelection(root, []string{"first", "missing"}, "delete", func(path string) (string, error) {
		if filepath.Base(path) == "missing" {
			return "", os.ErrNotExist
		}
		return path, nil
	}, func([]string) (removalBatch, error) { called = true; return removalBatch{}, nil })
	if called || len(result.Names) != 0 || !errors.Is(err, os.ErrNotExist) {
		t.Fatal("delete bypassed whole-selection preflight")
	}
	result, err = removeSelection(root, []string{"first", "second"}, "delete", func(path string) (string, error) { return path, nil }, func([]string) (removalBatch, error) {
		return removalBatch{completed: []bool{true, false}}, os.ErrPermission
	})
	var pathError *os.PathError
	if !reflect.DeepEqual(result.Names, []string{"first"}) || !errors.As(err, &pathError) || pathError.Op != "delete" || pathError.Path != "second" {
		t.Fatal("delete lost confirmed partial result", result, err)
	}
}

func TestConfirmDeletedPathRequiresEntryToBeGone(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "folder with spaces")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(path, "child.txt")
	if err := os.WriteFile(child, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := confirmDeletedPath(path); !errors.Is(err, ErrDeleteUnconfirmed) {
		t.Fatal("existing root was reported deleted", err)
	}
	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if err := confirmDeletedPath(path); !errors.Is(err, ErrDeleteUnconfirmed) {
		t.Fatal("deleted child was mistaken for a deleted root", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := confirmDeletedPath(path); err != nil {
		t.Fatal("missing root was not confirmed", err)
	}
}

func TestRemovalReportsSuccessesAfterSkippedEntries(t *testing.T) {
	for _, operation := range []string{"recycle", "delete"} {
		result, err := removeSelection(t.TempDir(), []string{"locked", "removed", "cancelled"}, operation,
			func(path string) (string, error) { return path, nil },
			func([]string) (removalBatch, error) { return removalBatch{completed: []bool{false, true, false}}, nil })
		if err != nil || !result.Incomplete || !reflect.DeepEqual(result.Names, []string{"removed"}) {
			t.Fatalf("%s lost non-prefix successes or reported skipped entries: %+v %v", operation, result, err)
		}
	}
}
