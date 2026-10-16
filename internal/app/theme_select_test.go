package app

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

func TestThemeSelectCataloguePreviewCycleAndCancel(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	// A custom appearance must remain intact until a theme is chosen.
	_ = v.settingsValues.Set(settings.CatalogueDefault, themeSettingPrefix+"background.lightness", settings.Value{Number: 75})
	_ = v.settingsValues.Set(settings.CatalogueDefault, settingPaddingRows, settings.Value{Number: 4})
	v.applySettings()
	before, palette := v.settingsValues.Clone(), v.appearance.Colors
	found := false
	for _, cmd := range v.settingsCatalogue() {
		if cmd.id == "settings."+settingThemeSelect {
			if cmd.label() != "Appearance › Theme › Select" {
				t.Fatal(cmd.label())
			}
			cmd.execute(v)
			found = true
			break
		}
	}
	s := v.settingsInteraction()
	if !found || s == nil || s.title != "Appearance › Theme › Select" || len(s.fields) != 1 || s.dirty || !reflect.DeepEqual(before, v.settingsValues) {
		t.Fatal("opening theme selector changed the appearance or failed to open")
	}
	if v.settingText(settingThemeSelect) != "Current theme" || !reflect.DeepEqual(s.actionLabels(), []string{"Save", "Cancel"}) {
		t.Fatal("custom theme or actions incorrect")
	}
	send, _ := panelInputFrames(v)
	for _, step := range []struct {
		key  key.Name
		name string
	}{{key.NameRightArrow, "Built-in Light"}, {key.NameRightArrow, "Built-in Dark"}, {key.NameRightArrow, "Built-in Light"}, {key.NameLeftArrow, "Built-in Dark"}} {
		send(pressed(step.key))
		if v.settingText(settingThemeSelect) != step.name || !s.dirty {
			t.Fatal("theme did not cycle live", step.name)
		}
		for _, theme := range builtInThemes {
			if theme.name == step.name && !theme.matches(v.settingsValues) {
				t.Fatal("incomplete theme preview")
			}
		}
		for _, d := range settings.CatalogueDefault.Settings {
			if !themeSnapshotKey(d.Key) && before.Get(settings.CatalogueDefault, d.Key) != v.settingsValues.Get(settings.CatalogueDefault, d.Key) {
				t.Fatal("theme changed unrelated setting", d.Key)
			}
		}
	}
	if v.appearance.Colors == palette || v.appearance.FilenameColors.filenameLightness != 0.55 || v.appearance.FilenameColors.activeBackgroundLightnessOffset != -0.16 {
		t.Fatal("dark snapshot or Auto Colors did not apply")
	}
	v.activateSettingsAction(layout.Context{}, 1)
	if !v.interaction.idle() || !reflect.DeepEqual(v.settingsValues, before) || v.appearance.Colors != palette {
		t.Fatal("Cancel did not restore original appearance")
	}
}

func TestThemeSelectCompletionValidationSaveAndIndependentCustomize(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	before := v.settingsValues.Clone()
	v.startSettingsWidget(settingThemeSelect)
	s := v.settingsInteraction()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{End: len(s.editor.Text())}, Text: "missing"})
	send(pressed(key.NameReturn))
	if !s.editing || s.errorText != "Choose an available theme" || !reflect.DeepEqual(before, v.settingsValues.Clone()) {
		t.Fatalf("invalid theme: editing=%v error=%q text=%q unchanged=%v", s.editing, s.errorText, s.editor.Text(), reflect.DeepEqual(before, v.settingsValues.Clone()))
	}
	send(key.EditEvent{Range: key.Range{End: len(s.editor.Text())}, Text: "Built-in D"}, key.SelectionEvent{Start: 10, End: 10})
	if s.completion.suffix() != "ark" {
		t.Fatalf("completion missing: %q", s.completion.suffix())
	}
	send(pressed(key.NameLeftArrow))
	a, b := s.editor.Selection()
	if a != 9 || b != 9 || !reflect.DeepEqual(before, v.settingsValues.Clone()) {
		t.Fatal("Left did not retain native caret movement")
	}
	send(pressed(key.NameEnd), pressed(key.NameTab))
	if s.editor.Text() != "Built-in Dark" || !s.editing {
		t.Fatal("Tab did not complete theme name")
	}
	send(pressed(key.NameReturn))
	if s.editing || v.settingText(settingThemeSelect) != "Built-in Dark" || !builtInThemes[1].matches(v.settingsValues) {
		t.Fatal("typed theme did not preview")
	}
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	send(key.Event{Name: key.NameReturn, State: key.Press, Modifiers: key.ModCtrl})
	if !v.interaction.idle() {
		t.Fatal("Save did not finish selector")
	}
	loaded, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil || !builtInThemes[1].matches(loaded) {
		t.Fatal("saved appearance lost theme values", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "settings.toml"))
	if err != nil || strings.Contains(string(data), "[appearance.theme.select]") {
		t.Fatal("selector identity persisted", err)
	}
	restored := newPreviewView()
	restored.restoreSettings(loaded, nil)
	restored.startSettingsWidget(settingThemeSelect)
	if restored.settingText(settingThemeSelect) != "Built-in Dark" {
		t.Fatal("did not recognize saved palette")
	}
	restored.finishSettingsWidget(layout.Context{}, false)
	restored.startSettingsWidget(settingThemeCustomize)
	_ = restored.setWidgetValue(themeSettingPrefix+"background.lightness", settings.Value{Number: 15})
	restored.applySettings()
	restored.finishSettingsWidget(layout.Context{}, true)
	restored.startSettingsWidget(settingThemeSelect)
	if restored.settingText(settingThemeSelect) != "Current theme" {
		t.Fatal("Customize retained a stale theme identity")
	}
	restored.finishSettingsWidget(layout.Context{}, false)
	// Selecting/customizing never edits the built-in source snapshot.
	if !builtInThemes[1].matches(loaded) {
		t.Fatal("built-in snapshot was mutated")
	}
}

func TestThemeSnapshotsRejectIncompleteAndUnrelatedSettings(t *testing.T) {
	data, err := themeFiles.ReadFile("themes/light.toml")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{string(data) + "\n[appearance.font]\ntext_size=18\n", strings.Replace(string(data), "salt = 0\n", "", 1), string(data) + "\n[appearance.theme.generator]\nseed=5\n"} {
		if _, err := parseThemeSnapshot([]byte(source)); err == nil {
			t.Fatal("accepted an incomplete or unrelated theme snapshot")
		}
	}
}

func TestThemeSelectPreviews(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional theme selector previews")
	}
	for _, theme := range builtInThemes {
		v := newPreviewView()
		v.applySettings()
		v.startSettingsWidget(settingThemeSelect)
		if err := v.setWidgetValue(settingThemeSelect, settings.Value{Text: theme.name}); err != nil {
			t.Fatal(err)
		}
		v.applySettings()
		if err := renderViewPNG(filepath.Join(dir, strings.ReplaceAll(theme.name, " ", "-")+"-select.png"), v); err != nil {
			t.Fatal(err)
		}
	}
}
