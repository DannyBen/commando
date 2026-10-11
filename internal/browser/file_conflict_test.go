package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func fileConflictFixture(t *testing.T) (*FileConflict, string, string) {
	t.Helper()
	source, target := filepath.Join(t.TempDir(), "file.txt"), filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(source, []byte("new contents"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old contents"), 0600); err != nil {
		t.Fatal(err)
	}
	conflict, err := InspectFileConflict(source, filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	return conflict, source, target
}

func TestOverwriteFileCopiesAndMovesCompleteReplacement(t *testing.T) {
	for _, move := range []bool{false, true} {
		conflict, source, target := fileConflictFixture(t)
		result, err := OverwriteFile(conflict, move)
		if err != nil || len(result.Items) != 1 || result.Items[0].Moved != move || !result.Items[0].Replaced {
			t.Fatalf("overwrite: %+v %v", result, err)
		}
		if data, err := os.ReadFile(target); err != nil || string(data) != "new contents" {
			t.Fatal("destination was not replaced with complete contents")
		}
		_, err = os.Stat(source)
		if move && !errors.Is(err, fs.ErrNotExist) || !move && err != nil {
			t.Fatalf("wrong source retention: %v", err)
		}
		entries, err := os.ReadDir(filepath.Dir(target))
		if err != nil || len(entries) != 1 {
			t.Fatal("overwrite leaked staging files")
		}
	}
}

func TestOverwriteCopyFailureLeavesBothFilesUntouched(t *testing.T) {
	conflict, source, target := fileConflictFixture(t)
	result, err := overwriteFile(conflict, true, func(string, string, fs.FileInfo) error { return fs.ErrPermission })
	if !errors.Is(err, fs.ErrPermission) || len(result.Items) != 0 {
		t.Fatalf("copy failure: %+v %v", result, err)
	}
	for path, want := range map[string]string{source: "new contents", target: "old contents"} {
		if data, err := os.ReadFile(path); err != nil || string(data) != want {
			t.Fatal("copy failure changed an original file")
		}
	}
}

func TestOverwriteRequiresFreshSourceAndDestination(t *testing.T) {
	for _, changeSource := range []bool{false, true} {
		conflict, source, target := fileConflictFixture(t)
		changed := target
		if changeSource {
			changed = source
		}
		if err := os.WriteFile(changed, []byte("externally changed"), 0600); err != nil {
			t.Fatal(err)
		}
		result, err := OverwriteFile(conflict, true)
		if !errors.Is(err, ErrConflictChanged) || len(result.Items) != 0 {
			t.Fatalf("stale decision accepted: %+v %v", result, err)
		}
		if data, err := os.ReadFile(changed); err != nil || string(data) != "externally changed" {
			t.Fatal("stale decision replaced a changed file")
		}
		if _, err := os.Stat(source); err != nil {
			t.Fatal("stale decision removed source")
		}
	}
}

func TestOverwriteDetectsReplacedIdentityWithMatchingMetadata(t *testing.T) {
	conflict, _, target := fileConflictFixture(t)
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(target, target+".previous"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("new arrival!"), info.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(target, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	result, err := OverwriteFile(conflict, false)
	if !errors.Is(err, ErrConflictChanged) || len(result.Items) != 0 {
		t.Fatalf("new file identity accepted: %+v %v", result, err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "new arrival!" {
		t.Fatal("new arrival replaced")
	}
}

func TestOverwriteRechecksDestinationAfterStaging(t *testing.T) {
	conflict, source, target := fileConflictFixture(t)
	result, err := overwriteFile(conflict, true, func(src, dst string, info fs.FileInfo) error {
		if err := copyFileExclusive(src, dst, info); err != nil {
			return err
		}
		return os.WriteFile(target, []byte("late change"), 0600)
	})
	if !errors.Is(err, ErrConflictChanged) || len(result.Items) != 0 {
		t.Fatalf("late change accepted: %+v %v", result, err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "late change" {
		t.Fatal("late change overwritten")
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatal("late change deleted source")
	}
}

func TestFileConflictRefusesSelfHardlinksFoldersAndLinks(t *testing.T) {
	_, source, target := fileConflictFixture(t)
	if _, err := InspectFileConflict(source, filepath.Dir(source)); !errors.Is(err, ErrSameTransferFile) {
		t.Fatalf("same-file overwrite accepted: %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(source, target); err == nil {
		if _, err := InspectFileConflict(source, filepath.Dir(target)); !errors.Is(err, ErrSameTransferFile) {
			t.Fatalf("hardlink overwrite accepted: %v", err)
		}
		if err := os.Remove(target); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := InspectFileConflict(source, filepath.Dir(target)); !errors.Is(err, ErrFileConflictType) {
		t.Fatalf("file/folder overwrite accepted: %v", err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, target); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if _, err := InspectFileConflict(source, filepath.Dir(target)); !errors.Is(err, ErrFileConflictType) {
		t.Fatalf("target link overwrite accepted: %v", err)
	}
}
