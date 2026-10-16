package app

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

func TestThemeGeneratorLiveControlsAndCancel(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	before, palette, filenames := v.settingsValues.Clone(), v.appearance.Colors, *v.appearance.FilenameColors
	selected := v.panels[0].selected
	v.startSettingsWidget(settingThemeGenerator)
	s := v.settingsInteraction()
	if s == nil || s.fields[0].Key != settingThemeGenerator+".mode" || s.fields[1].Key != settingThemeGenerator+".seed" || s.title != "Appearance › Theme › Generator" || len(s.fields) != 5 || isThemeWidget(s) {
		t.Fatal("Generator did not open its own five-field widget")
	}
	if !reflect.DeepEqual(v.settingsValues.Clone(), before) || s.dirty {
		t.Fatal("opening Generator overwrote the active palette")
	}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameLeftArrow)) // Light -> Midtone.
	send(pressed(key.NameLeftArrow)) // Midtone -> Dark.
	if v.settingText(settingThemeGenerator+".mode") != "Dark" || v.appearance.Colors == palette || !s.dirty {
		t.Fatal("mode did not preview live")
	}
	dark := v.appearance.Colors
	send(pressed(key.NameDownArrow), pressed(key.NameRightArrow))
	if v.settingText(settingThemeGenerator+".seed") != "2" || v.appearance.Colors == dark || v.panels[0].selected != selected {
		t.Fatal("Seed step failed to preview or disturbed browsing")
	}
	send(pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{End: 1}, Text: "600"}, pressed(key.NameReturn))
	if s.editing || v.settingText(settingThemeGenerator+".seed") != "600" {
		t.Fatal("typed Seed did not apply")
	}
	send(pressed(key.NameDownArrow), pressed(key.NameRightArrow))
	if v.settingText(settingThemeGenerator+".vibrance") != "51" {
		t.Fatal("Vibrance did not adjust live")
	}
	send(pressed(key.NameDownArrow), pressed(key.NameRightArrow))
	if v.settingText(settingThemeGenerator+".dynamics") != "36" {
		t.Fatal("Dynamics did not adjust live")
	}
	send(pressed(key.NameDownArrow), pressed(key.NameRightArrow))
	if v.settingText(settingThemeGenerator+".color_shift") != "1" {
		t.Fatal("Color Shift did not adjust live")
	}
	v.activateSettingsAction(layout.Context{}, 1)
	if v.settingsInteraction() != nil || !reflect.DeepEqual(v.settingsValues.Clone(), before) || v.appearance.Colors != palette || *v.appearance.FilenameColors != filenames {
		t.Fatal("Cancel did not restore colors, controls and filename preferences")
	}
}

func TestThemeGeneratorSaveCustomizeReopenAndReset(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.startSettingsWidget(settingThemeGenerator)
	v.adjustSetting(settingThemeGenerator+".mode", -1)
	v.adjustSetting(settingThemeGenerator+".seed", 1)
	v.adjustSetting(settingThemeGenerator+".dynamics", 1)
	_ = v.setWidgetValue(settingThemeGenerator+".color_shift", settings.Value{Number: 123})
	_ = v.setWidgetValue(settingThemeGenerator+".vibrance", settings.Value{Number: 75})
	v.applySettings()
	generated, filenames := v.appearance.Colors, *v.appearance.FilenameColors
	v.finishSettingsWidget(layout.Context{}, true)
	loaded, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	other := newPreviewView()
	other.restoreSettings(loaded, nil)
	if other.appearance.Colors != generated || *other.appearance.FilenameColors != filenames || other.settingText(settingThemeGenerator+".seed") != "2" || other.settingText(settingThemeGenerator+".vibrance") != "75" || other.settingText(settingThemeGenerator+".dynamics") != "36" || other.settingText(settingThemeGenerator+".color_shift") != "123" {
		t.Fatal("saved generated colors or inputs did not survive restart")
	}
	v.startSettingsWidget(settingThemeCustomize)
	_ = v.setWidgetValue(themeSettingPrefix+"background.lightness", settings.Value{Number: 30})
	v.applySettings()
	v.finishSettingsWidget(layout.Context{}, true)
	custom := v.appearance.Colors
	v.startSettingsWidget(settingThemeGenerator)
	if v.appearance.Colors != custom || v.settingsInteraction().dirty {
		t.Fatal("reopening Generator discarded Customize edits")
	}
	v.activateSettingsAction(layout.Context{}, 2)
	if v.settingText(settingThemeGenerator+".mode") != "Light" || v.settingText(settingThemeGenerator+".seed") != "1" || !v.settingsInteraction().dirty || v.appearance.Colors == custom {
		t.Fatal("Reset did not preview the default recipe inputs")
	}
	v.activateSettingsAction(layout.Context{}, 1)
	if v.appearance.Colors != custom {
		t.Fatal("Cancel after Reset lost the customized palette")
	}
}

func TestThemeGeneratorChoiceEdgesAndCycle(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	v.startSettingsWidget(settingThemeGenerator)
	before := v.settingsValues.Clone()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameRightArrow)) // Light -> White.
	if v.settingText(settingThemeGenerator+".mode") != "White" || !v.settingsInteraction().dirty {
		t.Fatal("White did not preview live")
	}
	before = v.settingsValues.Clone()
	send(pressed(key.NameRightArrow)) // White is already the final choice.
	if !reflect.DeepEqual(v.settingsValues.Clone(), before) {
		t.Fatal("choice at an edge regenerated the palette")
	}
	send(pressed(key.NameReturn)) // Enter cycles to Black, without a text editor.
	if v.settingText(settingThemeGenerator+".mode") != "Black" || v.settingsInteraction().editing {
		t.Fatal("Enter did not cycle the mode")
	}
	send(pressed(key.NameSpace))
	if v.settingText(settingThemeGenerator+".mode") != "Dark" {
		t.Fatal("Space did not cycle the mode")
	}
}

func TestThemeGeneratorPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional generator previews")
	}
	for _, mode := range []string{"Black", "Dark", "Midtone", "Light", "White"} {
		v := newPreviewView()
		v.applySettings()
		v.startSettingsWidget(settingThemeGenerator)
		_ = v.setWidgetValue(settingThemeGenerator+".mode", settings.Value{Text: mode})
		_ = v.setWidgetValue(settingThemeGenerator+".seed", settings.Value{Number: 600})
		_ = v.setWidgetValue(settingThemeGenerator+".dynamics", settings.Value{Number: 80})
		v.applySettings()
		if err := renderViewPNG(filepath.Join(dir, fmt.Sprintf("generator-%s.png", mode)), v); err != nil {
			t.Fatal(err)
		}
	}
}

func TestThemeGeneratorCommitsRoundedAutoColors(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	v.startSettingsWidget(settingThemeGenerator)
	if err := v.setWidgetValue(settingThemeGenerator+".vibrance", settings.Value{Number: 46}); err != nil {
		t.Fatal(err)
	}
	v.applySettings()
	if v.settingText(settingSaturation) != "65.3" {
		t.Fatal("generator saturation has excess decimals", v.settingText(settingSaturation))
	}
	for _, theme := range builtInThemes {
		for _, d := range settings.CatalogueDefault.Settings {
			if !themeSnapshotKey(d.Key) || d.Kind != settings.Number || d.Integer {
				continue
			}
			n := theme.values.Get(settings.CatalogueDefault, d.Key).Number
			if n != colorSettingNumber(n) {
				t.Fatalf("%s %s retains decimal noise: %g", theme.name, d.Key, n)
			}
		}
	}
}
