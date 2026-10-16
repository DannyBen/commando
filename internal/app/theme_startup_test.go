package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/dannyben/commando/internal/settings"
)

func TestInitialThemeSavedOnceAndSystemChangesIgnored(t *testing.T) {
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "data")
			calls := 0
			values, err := loadStartupSettings(dir, func() bool { calls++; return dark })
			theme := builtInThemes[0]
			if dark {
				theme = builtInThemes[1]
			}
			if err != nil || calls != 1 || !theme.matches(values) {
				t.Fatal("initial palette not applied", err)
			}
			loaded, err := settings.Load(dir, bindingSettingsCatalogue())
			if err != nil || !theme.matches(loaded) {
				t.Fatal("initial palette not persisted", err)
			}
			same, err := loadStartupSettings(dir, func() bool { t.Fatal("unchanged saved theme rechecked system preference"); return !dark })
			if err != nil || !theme.matches(same) {
				t.Fatal("later launch changed the initial palette", err)
			}
			_ = loaded.Set(settings.CatalogueDefault, themeSettingPrefix+"background.lightness", settings.Value{Number: 45})
			if err := bindingSettingsCatalogue().Save(dir, loaded); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(dir, "settings.toml"))
			if err != nil {
				t.Fatal(err)
			}
			again, err := loadStartupSettings(dir, func() bool { t.Fatal("rechecked system preference"); return !dark })
			if err != nil || again.Get(settings.CatalogueDefault, themeSettingPrefix+"background.lightness").Number != 45 {
				t.Fatal("custom palette overwritten", err)
			}
			after, err := os.ReadFile(filepath.Join(dir, "settings.toml"))
			if err != nil || string(after) != string(before) {
				t.Fatal("startup rewrote existing settings", err)
			}
		})
	}
}

func TestExistingEmptyInvalidAndInaccessibleSettingsNeverAutoSelect(t *testing.T) {
	for _, source := range []string{"", "[broken", "appearance.font.text_size = 18\n"} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := loadStartupSettings(dir, func() bool { t.Fatal("existing settings triggered auto-selection"); return true })
		if source == "[broken" && !errors.Is(err, settings.ErrSyntax) {
			t.Fatal("lost syntax error", err)
		}
		if source != "[broken" && err != nil {
			t.Fatal(err)
		}
		after, err := os.ReadFile(filepath.Join(dir, "settings.toml"))
		if err != nil || string(after) != source {
			t.Fatal("existing document changed", err)
		}
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "settings.toml"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := loadStartupSettings(dir, func() bool { t.Fatal("inaccessible settings triggered auto-selection"); return true }); err == nil {
		t.Fatal("lost load error")
	}
}

func TestInitialThemeSaveFailureIsReportedAndRetainsChosenPalette(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	values, err := loadStartupSettings(dir, func() bool {
		if err := os.WriteFile(dir, nil, 0600); err != nil {
			t.Fatal(err)
		}
		return true
	})
	if !errors.Is(err, errInitialThemeSave) || !builtInThemes[1].matches(values) {
		t.Fatal("save failure discarded palette or error", err)
	}
}
