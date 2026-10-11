package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func transferFixture(t *testing.T) (string, string, []string) {
	t.Helper()
	source, target := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(source, "folder", "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	for path, data := range map[string]string{"Å file.txt": "hello", "folder/nested/data": "nested contents"} {
		if err := os.WriteFile(filepath.Join(source, filepath.FromSlash(path)), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return source, target, []string{filepath.Join(source, "Å file.txt"), filepath.Join(source, "folder")}
}

func TestTransferRealMixedFilesAndFolders(t *testing.T) {
	for _, move := range []bool{false, true} {
		source, target, paths := transferFixture(t)
		result, err := Transfer(paths, target, move)
		if err != nil || result.Directory != target || len(result.Items) != 2 {
			t.Fatalf("transfer: %+v %v", result, err)
		}
		for i, item := range result.Items {
			if item.Source != paths[i] || item.Moved != move || item.Entry.Name != filepath.Base(paths[i]) {
				t.Fatalf("wrong confirmation: %+v", item)
			}
		}
		for name, want := range map[string]string{"Å file.txt": "hello", "folder/nested/data": "nested contents"} {
			data, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(name)))
			if err != nil || string(data) != want {
				t.Fatalf("wrong destination %q: %q %v", name, data, err)
			}
			_, err = os.Stat(filepath.Join(source, filepath.FromSlash(name)))
			if move && !errors.Is(err, fs.ErrNotExist) || !move && err != nil {
				t.Fatalf("wrong source retention: %v", err)
			}
		}
		if !result.Items[1].Entry.Directory || !result.Items[0].Entry.SizeKnown {
			t.Fatal("missing transferred metadata")
		}
	}
}

func TestTransferConflictsPreflightWholeBatch(t *testing.T) {
	for _, move := range []bool{false, true} {
		source, target, paths := transferFixture(t)
		if err := os.Mkdir(filepath.Join(target, "folder"), 0700); err != nil {
			t.Fatal(err)
		}
		keep := filepath.Join(target, "folder", "keep")
		if err := os.WriteFile(keep, []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
		result, err := Transfer(paths, target, move)
		if !errors.Is(err, fs.ErrExist) || len(result.Items) != 0 {
			t.Fatalf("conflict was not refused: %+v %v", result, err)
		}
		if _, err := os.Stat(filepath.Join(target, "Å file.txt")); !errors.Is(err, fs.ErrNotExist) {
			t.Fatal("first item transferred before later conflict")
		}
		if data, err := os.ReadFile(keep); err != nil || string(data) != "original" {
			t.Fatal("existing folder changed")
		}
		if _, err := os.Stat(filepath.Join(source, "Å file.txt")); err != nil {
			t.Fatal("conflict removed source")
		}
	}
}

func TestTransferRejectsMissingDuplicatesAndOverlappingSources(t *testing.T) {
	source, target, paths := transferFixture(t)
	for _, tc := range []struct {
		paths  []string
		target string
		want   error
	}{
		{append(paths[:1:1], filepath.Join(source, "missing")), target, fs.ErrNotExist},
		{[]string{paths[0], paths[0]}, target, fs.ErrExist},
		{[]string{paths[1], filepath.Join(paths[1], "nested", "data")}, target, ErrOverlappingSources},
		{[]string{paths[1]}, filepath.Join(paths[1], "nested"), ErrTransferIntoSelf},
		{[]string{"relative-file"}, target, fs.ErrInvalid},
	} {
		calls := 0
		result, err := transfer(tc.paths, tc.target, true, func(string, string, bool) (bool, bool, error) { calls++; return false, false, nil })
		if !errors.Is(err, tc.want) || calls != 0 || len(result.Items) != 0 {
			t.Fatalf("preflight: %+v %v calls=%d", result, err, calls)
		}
	}
}

func TestTransferRuntimeFailureRetainsConfirmedPrefix(t *testing.T) {
	_, target, paths := transferFixture(t)
	result, err := transfer(paths, target, false, func(source, destination string, move bool) (bool, bool, error) {
		if filepath.Base(source) == "folder" {
			return false, false, fs.ErrPermission
		}
		return transferOne(source, destination, move)
	})
	if !errors.Is(err, fs.ErrPermission) || len(result.Items) != 1 || result.Items[0].Source != paths[0] {
		t.Fatalf("partial result: %+v %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(target, "folder")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("failed folder appeared")
	}
}

func TestTransferLateConflictNeverReplacesDestination(t *testing.T) {
	for _, move := range []bool{false, true} {
		_, target, paths := transferFixture(t)
		if err := os.Chmod(paths[0], 0444); err != nil {
			t.Fatal(err)
		}
		result, err := transfer(paths[:1], target, move, func(source, destination string, move bool) (bool, bool, error) {
			if err := os.WriteFile(destination, []byte("late arrival"), 0600); err != nil {
				t.Fatal(err)
			}
			return transferOne(source, destination, move)
		})
		if !errors.Is(err, fs.ErrExist) || len(result.Items) != 0 {
			t.Fatalf("late conflict: %+v %v", result, err)
		}
		data, err := os.ReadFile(filepath.Join(target, filepath.Base(paths[0])))
		if err != nil || string(data) != "late arrival" {
			t.Fatal("late arrival replaced")
		}
		if _, err := os.Stat(paths[0]); err != nil {
			t.Fatal("late conflict removed source")
		}
		entries, _ := os.ReadDir(target)
		if len(entries) != 1 {
			t.Fatal("staging leaked after conflict")
		}
	}
}

func TestCopyTransferCrossVolumeMovePath(t *testing.T) {
	_, target, paths := transferFixture(t)
	created, moved, err := copyTransfer(paths[1], filepath.Join(target, "folder"), true)
	if err != nil || !created || !moved {
		t.Fatalf("copy/delete move: %t %t %v", created, moved, err)
	}
	if _, err := os.Stat(paths[1]); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("source tree retained after successful move")
	}
	if data, err := os.ReadFile(filepath.Join(target, "folder", "nested", "data")); err != nil || string(data) != "nested contents" {
		t.Fatal("copy/delete move lost nested data")
	}
}

func TestSourceMutationPreventsMoveDeletion(t *testing.T) {
	_, target, paths := transferFixture(t)
	var manifest []sourceEntry
	if err := copyTree(paths[1], filepath.Join(target, "folder"), &manifest); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(paths[1], "nested", "data")
	if err := os.WriteFile(file, []byte("new contents"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := removeCopiedSource(manifest); !errors.Is(err, ErrSourceChanged) {
		t.Fatalf("changed source not refused: %v", err)
	}
	if data, err := os.ReadFile(file); err != nil || string(data) != "new contents" {
		t.Fatal("changed source was deleted")
	}
}

func TestCopyPreservesModifiedTimeAndFinalLinks(t *testing.T) {
	_, target, paths := transferFixture(t)
	modified := time.Date(2020, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := os.Chtimes(paths[0], modified, modified); err != nil {
		t.Fatal(err)
	}
	if _, err := Transfer(paths[:1], target, false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(target, filepath.Base(paths[0])))
	if err != nil || !info.ModTime().Equal(modified) {
		t.Fatal("copy lost modified time")
	}
	link := filepath.Join(filepath.Dir(paths[0]), "link")
	if err := os.Symlink("missing-target", link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if _, err := Transfer([]string{link}, target, false); err != nil {
		t.Fatal(err)
	}
	if value, err := os.Readlink(filepath.Join(target, "link")); err != nil || value != "missing-target" {
		t.Fatal("copy followed or altered final link")
	}
}

func TestTransferDirectoryAliasAndDestinationSelf(t *testing.T) {
	source, target, paths := transferFixture(t)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	result, err := Transfer([]string{filepath.Join(alias, "folder")}, filepath.Join(paths[1], "nested"), true)
	if !errors.Is(err, ErrTransferIntoSelf) || len(result.Items) != 0 {
		t.Fatalf("alias self-copy: %+v %v", result, err)
	}
	result, err = Transfer([]string{filepath.Join(alias, filepath.Base(paths[0]))}, target, false)
	if err != nil || !reflect.DeepEqual([]string{result.Items[0].Source}, []string{filepath.Join(alias, filepath.Base(paths[0]))}) {
		t.Fatalf("clipboard source identity lost: %+v %v", result, err)
	}
}
