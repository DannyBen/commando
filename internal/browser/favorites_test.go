package browser

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestFavoriteStoreIsExplicitAndPreservesDestinations(t *testing.T) {
	root := t.TempDir()
	store, target := filepath.Join(root, "favorites"), filepath.Join(root, "target")
	if entries, err := ReadFavorites(store); err != nil || len(entries) != 0 {
		t.Fatal("missing favorites were not empty", entries, err)
	}
	if _, err := os.Stat(store); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("read created the favorite store")
	}
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	encode := func(path string) ([]byte, error) { return []byte(path), nil }
	file, err := createFavorite(store, "Work", target, encode)
	if err != nil || !file.Favorite || file.Name != "Work.lnk" || file.Target != target {
		t.Fatal("favorite did not capture the named folder", file, err)
	}
	if _, err := createFavorite(store, "Work", target, encode); !errors.Is(err, fs.ErrExist) {
		t.Fatal("favorite creation overwrote an existing shortcut", err)
	}
	if _, err := createFavorite(store, "../escape", target, encode); !errors.Is(err, ErrInvalidName) {
		t.Fatal("favorite accepted a path as its name", err)
	}
	if err := os.WriteFile(filepath.Join(store, "unrelated.txt"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	entries, err := readFavorites(store, func(path string) (string, error) {
		data, err := os.ReadFile(path)
		return string(data), err
	})
	if err != nil || len(entries) != 1 || entries[0].Target != target || entries[0].Path != file.Path {
		t.Fatal("listing lost an offline favorite or included unrelated data", entries, err)
	}
}
