package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// Opt-in real-volume coverage. All mutations stay inside newly created test
// folders; the supplied root must be on a different volume from os.TempDir().
func TestWindowsCrossVolumeMoveUsesStagedTransfer(t *testing.T) {
	root := os.Getenv("COMMANDO_TEST_TRANSFER_VOLUME")
	if root == "" {
		t.Skip("set COMMANDO_TEST_TRANSFER_VOLUME to a folder on a second volume")
	}
	target, err := os.MkdirTemp(root, "commando-transfer-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(target); err != nil {
			t.Error(err)
		}
	})
	for _, folder := range []bool{false, true} {
		sourceRoot := t.TempDir()
		name := "file.txt"
		if folder {
			name = "folder"
			writeTransferFiles(t, sourceRoot, map[string]string{"folder/child": "data"})
		} else {
			writeTransferFiles(t, sourceRoot, map[string]string{"file.txt": "data"})
		}
		source, destination := filepath.Join(sourceRoot, name), filepath.Join(target, name)
		if err := renameExclusive(source, destination); !errors.Is(err, windows.ERROR_NOT_SAME_DEVICE) {
			t.Fatalf("expected a real cross-volume rename refusal, got %v", err)
		}
		if _, err := os.Lstat(destination); !errors.Is(err, fs.ErrNotExist) {
			t.Fatal("rename performed an implicit copy:", err)
		}
		result, err := Transfer([]string{source}, target, true)
		if err != nil || len(result.Items) != 1 || !result.Items[0].Moved || len(result.Changes) != 1 || !result.Changes[0].Moved {
			t.Fatalf("staged cross-volume move: %+v %v", result, err)
		}
		if _, err := os.Lstat(source); !errors.Is(err, fs.ErrNotExist) {
			t.Fatal("verified moved source retained:", err)
		}
		if folder {
			destination = filepath.Join(destination, "child")
		}
		assertTransferFile(t, destination, "data")
	}
}

// Use an existing SUBST root without creating or changing drive mappings.
func TestWindowsSUBSTMoveRetainsFileIdentity(t *testing.T) {
	root := os.Getenv("COMMANDO_TEST_SUBST_ROOT")
	if root == "" {
		t.Skip("set COMMANDO_TEST_SUBST_ROOT to an existing SUBST drive root")
	}
	alias, err := os.MkdirTemp(root, "commando-transfer-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(alias); err != nil {
			t.Error(err)
		}
	})
	backing, err := ResolveOperationPath(alias)
	if err != nil {
		t.Fatal(err)
	}
	if sameTransferPath(alias, backing) {
		t.Fatal("supplied root is not a SUBST alias")
	}
	writeTransferFiles(t, alias, map[string]string{"source/folder/child": "data"})
	target := filepath.Join(backing, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(filepath.Join(alias, "source", "folder", "child"))
	if err != nil || !os.SameFile(before, before) {
		t.Fatal("cannot freeze original file identity:", err)
	}
	result, err := Transfer([]string{filepath.Join(alias, "source", "folder")}, target, true)
	if err != nil || len(result.Items) != 1 || !result.Items[0].Moved {
		t.Fatalf("SUBST move: %+v %v", result, err)
	}
	after, err := os.Stat(filepath.Join(target, "folder", "child"))
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("SUBST move copied instead of renaming:", err)
	}
}
