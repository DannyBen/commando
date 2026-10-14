package settings

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDottedAndSectionOverrides(t *testing.T) {
	for _, document := range []string{
		`appearance.autocolor.lightness = 0.5
appearance.autocolor.hue = 20
view.show_protected_system_files = false
application.data_folder.path = 'C:\user folder\#data'
`,
		`[appearance.autocolor]
lightness = 0.5
hue = 20
[view]
show_protected_system_files = false
[application.data_folder]
path = 'C:\user folder\#data'
`,
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(document), 0600); err != nil {
			t.Fatal(err)
		}
		values, err := Load(dir, CatalogueDefault)
		if err != nil {
			t.Fatal(err)
		}
		for key, want := range map[string]Value{
			"appearance.autocolor.lightness":   {Number: 0.5},
			"appearance.autocolor.hue":         {Number: 20},
			"view.show_protected_system_files": {Bool: false},
			"application.data_folder.path":     {Text: `C:\user folder\#data`},
		} {
			if !values.Overridden(key) || values.Get(CatalogueDefault, key) != want {
				t.Fatalf("override %s not restored", key)
			}
		}
		if values.Overridden("appearance.autocolor.chroma") || values.Get(CatalogueDefault, "appearance.autocolor.chroma").Number != 0.35 {
			t.Fatal("missing setting did not inherit its default")
		}
		if err := Save(dir, values); err != nil {
			t.Fatal(err)
		}
		reloaded, err := Load(dir, CatalogueDefault)
		if err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"appearance.autocolor.lightness", "appearance.autocolor.hue", "view.show_protected_system_files", "application.data_folder.path"} {
			if reloaded.Get(CatalogueDefault, key) != values.Get(CatalogueDefault, key) {
				t.Fatalf("save/load changed effective value %s", key)
			}
		}
		if reloaded.Overridden("appearance.autocolor.hue") || reloaded.Overridden("view.show_protected_system_files") {
			t.Fatal("Save retained explicit default values")
		}
	}
}

func TestLoadRejectsEntireInvalidFile(t *testing.T) {
	for _, tc := range []struct {
		name, document string
		reason         error
	}{
		{"syntax", "bad = [", ErrSyntax},
		{"duplicate", "view.show_protected_system_files = true\nview.show_protected_system_files = false", ErrSyntax},
		{"unknown key", "files.extra = true", ErrUnknownKey},
		{"unknown empty table", "[files.extra]", ErrUnknownKey},
		{"literal dotted key", `"view.show_protected_system_files" = true`, ErrUnknownKey},
		{"numeric string", `appearance.autocolor.hue = "20"`, ErrType},
		{"boolean string", `view.show_protected_system_files = "true"`, ErrType},
		{"number as boolean", "view.show_protected_system_files = 1", ErrType},
		{"array as path", "application.data_folder.path = []", ErrType},
		{"table as value", "[application.data_folder.path]", ErrType},
		{"value as group", "application = 1", ErrType},
		{"upper bound", "appearance.autocolor.lightness = 1.1", ErrRange},
		{"lower bound", "appearance.autocolor.chroma = -0.1", ErrRange},
		{"nan", "appearance.autocolor.hue = nan", ErrRange},
		{"infinity", "appearance.autocolor.hue = inf", ErrRange},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "settings.toml")
			document := "appearance.autocolor.chroma = 0.2\n" + tc.document
			// Avoid defining chroma twice in the lower-bound case.
			if tc.name == "lower bound" {
				document = "view.show_protected_system_files = true\n" + tc.document
			}
			if err := os.WriteFile(path, []byte(document), 0600); err != nil {
				t.Fatal(err)
			}
			values, err := Load(dir, CatalogueDefault)
			if !errors.Is(err, tc.reason) {
				t.Fatalf("error = %v; want %v", err, tc.reason)
			}
			if len(values.overrides) != 0 || values.Get(CatalogueDefault, "appearance.autocolor.chroma").Number != 0.35 {
				t.Fatal("invalid file partially applied")
			}
			if got, err := os.ReadFile(path); err != nil || string(got) != document {
				t.Fatal("invalid file was changed", err)
			}
		})
	}
}

func TestLoadMissingEmptyAndUnreadableFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	values, err := Load(dir, CatalogueDefault)
	if err != nil || len(values.overrides) != 0 {
		t.Fatal("missing file did not use defaults", err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("reading created the data folder")
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.toml")
	if err := os.WriteFile(path, []byte("# no overrides\n[appearance.autocolor]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	values, err = Load(dir, CatalogueDefault)
	if err != nil || len(values.overrides) != 0 {
		t.Fatal("empty settings did not use defaults", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir, CatalogueDefault); err == nil {
		t.Fatal("ignored unreadable file")
	}
	if _, err := Load("", CatalogueDefault); err == nil {
		t.Fatal("accepted empty data folder")
	}
}
