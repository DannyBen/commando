package app

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

func TestContrastGuardToggleBypassesCorrectionsAndCancelRestores(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	foreground, background := rgb(0x222222), rgb(0x222222)
	assertGuard := func(enabled bool) {
		t.Helper()
		for _, role := range []contrastRole{contrastPrimary, contrastSecondary, contrastDimmedFile} {
			got := v.resolvedTextFor(role, foreground, background)
			if (got != foreground) != enabled {
				t.Fatalf("role %d: guard enabled=%t, got %v", role, enabled, got)
			}
		}
		selection := v.appearance.Colors.TextSelection
		got := v.resolvedTextSelection(selection, selection)
		if (got != selection) != enabled {
			t.Fatal("text selection did not follow contrast guard")
		}
	}
	assertGuard(true)
	found := false
	for _, cmd := range v.settingsCatalogue() {
		if cmd.label() == "Theme › Contrast guard" {
			cmd.execute(v)
			found = true
			break
		}
	}
	s := v.settingsInteraction()
	if !found || s == nil || len(s.fields) != 1 || s.fields[0].Control != "toggle" {
		t.Fatal("contrast guard did not open a single toggle widget")
	}
	v.adjustSetting(settingContrastGuard, 0)
	assertGuard(false)
	v.finishSettingsWidget(layout.Context{}, false)
	assertGuard(true)
}

func TestContrastGuardSaveThemeIndependenceAndReset(t *testing.T) {
	v := newPreviewView()
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.startSettingsWidget("appearance.contrast_guard")
	v.adjustSetting(settingContrastGuard, 0)
	v.finishSettingsWidget(layout.Context{}, true)
	values, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	v.restoreSettings(values, nil)
	if v.settingsValues.Get(settings.CatalogueDefault, settingContrastGuard).Bool {
		t.Fatal("guard Off did not persist")
	}
	v.startSettingsWidget(settingThemeSelect)
	for _, theme := range builtInThemes {
		if err := v.selectTheme(theme.name); err != nil {
			t.Fatal(err)
		}
		v.applySettings()
		if v.settingsValues.Get(settings.CatalogueDefault, settingContrastGuard).Bool {
			t.Fatal("selecting a theme changed the contrast preference")
		}
	}
	v.finishSettingsWidget(layout.Context{}, true)
	v.startSettingsWidget("appearance.contrast_guard")
	v.activateSettingsAction(layout.Context{}, 2)
	if !v.settingsValues.Get(settings.CatalogueDefault, settingContrastGuard).Bool {
		t.Fatal("Reset did not restore guard On")
	}
	v.finishSettingsWidget(layout.Context{}, false)
	if v.settingsValues.Get(settings.CatalogueDefault, settingContrastGuard).Bool {
		t.Fatal("Cancel did not undo Reset")
	}
}

func TestContrastGuardSettingsPreviews(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional contrast guard previews")
	}
	for _, theme := range builtInThemes {
		for _, enabled := range []bool{true, false} {
			v := newPreviewView()
			v.settingsValues = theme.values.Clone()
			_ = v.settingsValues.Set(settings.CatalogueDefault, settingContrastGuard, settings.Value{Bool: enabled})
			v.applySettings()
			v.startSettingsWidget("appearance.contrast_guard")
			name := fmt.Sprintf("%s-contrast-guard-%t.png", theme.name, enabled)
			if err := renderViewPNG(filepath.Join(dir, name), v); err != nil {
				t.Fatal(err)
			}
		}
	}
}
