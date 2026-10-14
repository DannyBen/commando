package settings

import (
	"github.com/BurntSushi/toml"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSaveOverrides(t *testing.T) {
	c := CatalogueDefault
	c.Settings = append(append([]Definition(nil), c.Settings...), Definition{Key: "files.editor", Kind: String})
	var v Values
	for key, value := range map[string]Value{
		"view.show_protected_system_files": {Bool: false}, // An explicit default must remain explicit.
		"appearance.autocolor.lightness":   {Number: 0.5},
		"application.data_folder.path":     {Text: "C:\\folder: #1\\\"quoted\"\nnext\tline"},
		"files.editor":                     {Text: "null"},
	} {
		if err := v.Set(c, key, value); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(t.TempDir(), "data")
	if err := Save(dir, v); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"appearance":  map[string]any{"autocolor": map[string]any{"lightness": 0.5}},
		"files":       map[string]any{"editor": "null"},
		"view":        map[string]any{"show_protected_system_files": false},
		"application": map[string]any{"data_folder": map[string]any{"path": "C:\\folder: #1\\\"quoted\"\nnext\tline"}},
	}
	assertSavedTOML(t, dir, want)
	// Reset drops an override; saving replaces the previous document rather
	// than retaining stale keys or expanding inherited defaults.
	v.Reset("appearance.autocolor.lightness")
	if err := Save(dir, v); err != nil {
		t.Fatal(err)
	}
	delete(want, "appearance")
	assertSavedTOML(t, dir, want)
	if err := Save(dir, Values{}); err != nil {
		t.Fatal(err)
	}
	assertSavedTOML(t, dir, map[string]any{})
}

func TestSaveFailurePreservesDestination(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "settings.toml")
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(destination, "keep")
	if err := os.WriteFile(marker, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, Values{}); err == nil {
		t.Fatal("replaced an inaccessible destination")
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "original" {
		t.Fatalf("destination changed: %q, %v", got, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "settings.toml" {
		t.Fatalf("temporary file left behind: %v, %v", entries, err)
	}
	if err := Save("", Values{}); err == nil {
		t.Fatal("accepted empty data folder")
	}
}

func assertSavedTOML(t *testing.T, dir string, want map[string]any) {
	t.Helper()
	var got map[string]any
	_, err := toml.DecodeFile(filepath.Join(dir, "settings.toml"), &got)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("saved TOML = %#v, %v; want %#v", got, err, want)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("unexpected data files: %v, %v", entries, err)
	}
}
