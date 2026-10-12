package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestCopiedMovePreservesReplacedSourceWithMatchingMetadata(t *testing.T) {
	for _, folder := range []bool{false, true} {
		root, target := t.TempDir(), t.TempDir()
		source := filepath.Join(root, "item")
		if folder {
			if err := os.Mkdir(source, 0700); err != nil {
				t.Fatal(err)
			}
		} else {
			writeTransferFiles(t, root, map[string]string{"item": "original"})
		}
		outcome, err := stagedTransfer(source, filepath.Join(target, "item"), true,
			func(src, dst string, manifest *[]sourceEntry) error {
				if err := copyTree(src, dst, manifest); err != nil {
					return err
				}
				info := (*manifest)[0].info
				// Keep the original inode alive so its replacement cannot reuse it.
				if err := os.Rename(src, src+"-original"); err != nil {
					return err
				}
				if folder {
					if err := os.Mkdir(src, info.Mode().Perm()); err != nil {
						return err
					}
				} else if err := os.WriteFile(src, []byte("replaced"), info.Mode().Perm()); err != nil {
					return err
				}
				return os.Chtimes(src, info.ModTime(), info.ModTime())
			}, os.Remove)
		if !errors.Is(err, ErrSourceChanged) || !outcome.created || outcome.moved || len(outcome.changes) != 0 {
			t.Fatalf("replacement accepted: %+v %v", outcome, err)
		}
		if _, err := os.Lstat(source); err != nil {
			t.Fatal("replacement source removed:", err)
		}
		if !folder {
			assertTransferFile(t, source, "replaced")
			assertTransferFile(t, filepath.Join(target, "item"), "original")
		}
	}
}

func TestCopiedMoveRechecksDirectoryIdentityAfterDeletingChildren(t *testing.T) {
	root, target := t.TempDir(), t.TempDir()
	writeTransferFiles(t, root, map[string]string{"folder/child": "data"})
	source := filepath.Join(root, "folder")
	outcome, err := stagedTransfer(source, filepath.Join(target, "folder"), true, copyTree, func(path string) error {
		if err := os.Remove(path); err != nil {
			return err
		}
		if path == filepath.Join(source, "child") {
			if err := os.Rename(source, source+"-original"); err != nil {
				return err
			}
			return os.Mkdir(source, 0700)
		}
		return nil
	})
	if !errors.Is(err, ErrSourceChanged) || !outcome.created || outcome.moved || len(outcome.changes) != 1 || !outcome.changes[0].Moved {
		t.Fatalf("replaced directory accepted or child result lost: %+v %v", outcome, err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("replacement directory removed:", err)
	}
}

func TestPartialCopiedMoveReportsRemovedChildrenInSharedPipeline(t *testing.T) {
	for _, planned := range []bool{false, true} {
		root, target := t.TempDir(), t.TempDir()
		writeTransferFiles(t, root, map[string]string{"folder/a/child": "a", "folder/z/child": "z"})
		source := filepath.Join(root, "folder")
		perform := func(src, dst string, move bool) (transferOutcome, error) {
			return stagedTransfer(src, dst, move, copyTree, func(path string) error {
				if path == filepath.Join(source, "a", "child") {
					return fs.ErrPermission
				}
				return os.Remove(path)
			})
		}
		var result TransferResult
		var err error
		if planned {
			plan, planErr := PlanTransfer([]string{source}, target)
			if planErr != nil {
				t.Fatal(planErr)
			}
			result, err = executeTransfer(plan, true, TransferExclusive, perform)
		} else {
			result, err = transfer([]string{source}, target, true, perform)
		}
		if !errors.Is(err, fs.ErrPermission) || len(result.Items) != 1 || result.Items[0].Moved {
			t.Fatalf("partial top-level result: %+v %v", result, err)
		}
		var child, directory bool
		for _, change := range result.Changes {
			child = child || change.Source == filepath.Join(source, "z", "child") && change.Moved
			directory = directory || change.Source == filepath.Join(source, "z") && change.Directory && change.Moved
		}
		if !child || !directory {
			t.Fatalf("confirmed child moves lost: %+v", result.Changes)
		}
		assertTransferFile(t, filepath.Join(source, "a", "child"), "a")
		assertTransferFile(t, filepath.Join(target, "folder", "a", "child"), "a")
		assertTransferFile(t, filepath.Join(target, "folder", "z", "child"), "z")
	}
}

func TestPlannedMoveThroughParentAliasUsesRename(t *testing.T) {
	root, target := t.TempDir(), t.TempDir()
	writeTransferFiles(t, root, map[string]string{"folder/child": "data"})
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skip("parent aliases unavailable:", err)
	}
	before, err := os.Stat(filepath.Join(root, "folder", "child"))
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(before, before) {
		t.Fatal("cannot freeze original file identity")
	}
	plan, err := PlanTransfer([]string{filepath.Join(alias, "folder")}, target)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ExecuteTransfer(plan, true, TransferExclusive)
	if err != nil || len(result.Items) != 1 || !result.Items[0].Moved {
		t.Fatalf("alias move: %+v %v", result, err)
	}
	after, err := os.Stat(filepath.Join(target, "folder", "child"))
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("same-volume alias move copied instead of renaming:", err)
	}
}
