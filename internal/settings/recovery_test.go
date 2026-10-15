package settings

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryBacksUpOriginalAndKeepsValidSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.toml")
	original := `# Preserve this original document in the backup.
appearance.autocolor.hue = 20
appearance.autocolor.lsh.saturation = 20
appearance.font.weight = "bold"
appearance.autocolor.lsh.lightness = 150
view.show_protected_system_files = true
[[programs.editors]]
ext = ["txt"]
path = 'C:\editor.exe'
obsolete = true
[[programs.editors]]
ext = ["png"]
path = 'C:\image.exe'
args = 123
[[programs.editors]]
ext = ["md"]
path = 'C:\markdown.exe'
`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	values, err := LoadAndRepair(dir, CatalogueDefault)
	if !errors.Is(err, ErrRepaired) {
		t.Fatalf("recovery = %v", err)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil || string(backup) != original {
		t.Fatal("backup did not retain the exact original document", err)
	}
	if values.Get(CatalogueDefault, "appearance.autocolor.lsh.saturation").Number != 20 ||
		!values.Get(CatalogueDefault, "view.show_protected_system_files").Bool ||
		values.Overridden("appearance.font.weight") || values.Overridden("appearance.autocolor.lsh.lightness") {
		t.Fatal("recovery lost valid settings or retained invalid ones")
	}
	editors := values.Collection(EditorCollection)
	if len(editors) != 2 || editors[0]["path"] != `C:\editor.exe` || editors[1]["path"] != `C:\markdown.exe` {
		t.Fatalf("recovery changed valid editor rules: %#v", editors)
	}
	if _, retained := editors[0]["obsolete"]; retained {
		t.Fatal("unknown editor field survived")
	}
	loaded, err := Load(dir, CatalogueDefault)
	if err != nil || loaded.Get(CatalogueDefault, "appearance.autocolor.lsh.saturation").Number != 20 || len(loaded.Collection(EditorCollection)) != 2 {
		t.Fatal("rewritten file did not load correctly", err)
	}
	if _, err := LoadAndRepair(dir, CatalogueDefault); err != nil {
		t.Fatal("clean startup repeated the warning", err)
	}
	if err := Save(dir, loaded); err != nil {
		t.Fatal(err)
	}
	backup, err = os.ReadFile(path + ".bak")
	if err != nil || string(backup) != original {
		t.Fatal("later startup or save changed the recovery backup", err)
	}
}

func TestRecoveryDoesNotRewriteWithoutBackupOrValidSyntax(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		dir := t.TempDir()
		path := filepath.Join(dir, "settings.toml")
		original := "appearance.autocolor.lsh.saturation = 20\nunknown = true\n"
		if malformed {
			original = "appearance.autocolor.lsh.saturation = 20\nbad = ["
		} else if err := os.Mkdir(path+".bak", 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(original), 0600); err != nil {
			t.Fatal(err)
		}
		values, err := LoadAndRepair(dir, CatalogueDefault)
		if err == nil || errors.Is(err, ErrRepaired) {
			t.Fatalf("failed recovery reported success: %v", err)
		}
		got, readErr := os.ReadFile(path)
		if readErr != nil || string(got) != original {
			t.Fatal("failed recovery changed the original file", readErr)
		}
		if malformed {
			if !errors.Is(err, ErrSyntax) || len(values.overrides) != 0 {
				t.Fatal("malformed document was partially applied", err)
			}
			if _, err := os.Stat(path + ".bak"); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("malformed document created a backup", err)
			}
		} else {
			if !errors.Is(err, ErrRepairFailed) {
				t.Fatal("backup failure was not identified as a repair failure", err)
			}
			if values.Get(CatalogueDefault, "appearance.autocolor.lsh.saturation").Number != 20 {
				t.Fatal("backup failure prevented valid settings from loading")
			}
		}
	}
}

func TestRecoveryMissingSettingsDoesNotCreateFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	if _, err := LoadAndRepair(dir, CatalogueDefault); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("loading missing settings created the data folder", err)
	}
}

func TestObsoleteRawColorEntriesResetWithoutReinterpretingUnits(t *testing.T) {
	dir := t.TempDir()
	source := "[appearance.theme.background]\nlightness = 0.3\nchroma = 0.1\nhue = 200\n[appearance.autocolor]\nlightness = 0.4\nchroma = 0.2\nsalt = 42\n[appearance.font]\ntext_size = 18\n"
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	values, err := LoadAndRepair(dir, CatalogueDefault)
	if !errors.Is(err, ErrRepaired) {
		t.Fatal("obsolete color format was accepted", err)
	}
	if values.Get(CatalogueDefault, "appearance.theme.colors.background.lightness").Number != 100 || values.Get(CatalogueDefault, "appearance.autocolor.lsh.lightness").Number != 36 || values.Get(CatalogueDefault, "appearance.autocolor.lsh.saturation").Number != 100 {
		t.Fatal("old units were interpreted as LSH")
	}
	if values.Get(CatalogueDefault, "appearance.font.text_size").Number != 18 || values.Get(CatalogueDefault, "appearance.autocolor.salt").Number != 42 {
		t.Fatal("reset lost independent preferences")
	}
}
