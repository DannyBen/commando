package app

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

const colorPrefix = "appearance.theme."

func TestThemeColorWidgetsAndLinkedTransactions(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	before, files := v.appearance.Colors, *v.appearance.FilenameColors
	v.startSettingsWidget(colorPrefix + "background")
	s := v.settingsInteraction()
	if s == nil || len(s.fields) != 3 {
		t.Fatal("background needs three color controls")
	}
	if err := v.setWidgetValue(colorPrefix+"background.lightness", settings.Value{Number: 0.4}); err != nil {
		t.Fatal(err)
	}
	v.applySettings()
	if got := v.settingText(colorPrefix + "alt_background.lightness"); got != "0.52" {
		t.Fatalf("parent did not clamp child: %s", got)
	}
	if v.appearance.Colors == before || *v.appearance.FilenameColors != files {
		t.Fatal("color did not preview or changed file settings")
	}
	v.finishSettingsWidget(layout.Context{}, false)
	if v.appearance.Colors != before || v.settingText(colorPrefix+"alt_background.lightness") != "0.9" {
		t.Fatal("Cancel did not restore child and parent")
	}
	v.startSettingsWidget(colorPrefix + "alt_background")
	if len(v.settingsInteraction().fields) != 4 {
		t.Fatal("linked widget needs toggle plus LCH")
	}
	colors := v.appearance.Colors
	v.adjustSetting(colorPrefix+"alt_background.linked", 0)
	if v.appearance.Colors != colors {
		t.Fatal("unlink changed appearance")
	}
	_ = v.setWidgetValue(colorPrefix+"alt_background.lightness", settings.Value{Number: 0.2})
	v.adjustSetting(colorPrefix+"alt_background.linked", 0)
	if got := v.settingText(colorPrefix + "alt_background.lightness"); got != "0.88" {
		t.Fatalf("link-on did not clamp: %s", got)
	}
	v.finishSettingsWidget(layout.Context{}, false)
	v.startSettingsWidget(colorPrefix + "background")
	_ = v.setWidgetValue(colorPrefix+"background.lightness", settings.Value{Number: 0.4})
	v.applySettings()
	want := v.appearance.Colors
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.finishSettingsWidget(layout.Context{}, true)
	values, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	restored := newPreviewView()
	restored.restoreSettings(values, nil)
	if restored.appearance.Colors != want {
		t.Fatal("linked colors did not persist")
	}
	restored.startSettingsWidget(colorPrefix + "background")
	restored.activateSettingsAction(layout.Context{}, 2)
	// Reset expands the child range; a valid child must retain its actual value.
	if got := restored.settingText(colorPrefix + "alt_background.lightness"); got != "0.88" {
		t.Fatalf("Reset failed to re-clamp child: %s", got)
	}
	restored.finishSettingsWidget(layout.Context{}, false)
	if restored.appearance.Colors != want {
		t.Fatal("Cancel did not undo Reset and its dependent changes")
	}
}

func TestThemeColorKeyboardUsesActualRanges(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	_ = v.setWidgetValue(colorPrefix+"background.hue", settings.Value{Number: 355})
	v.startSettingsWidget(colorPrefix + "alt_background")
	send, _ := panelInputFrames(v)
	// Toggle, Lightness, Chroma, Hue. Input retains actual degrees at the seam.
	for range 3 {
		send(pressed(key.NameDownArrow))
	}
	_ = v.setWidgetValue(colorPrefix+"alt_background.hue", settings.Value{Number: 359})
	send(pressed(key.NameRightArrow))
	if v.settingText(colorPrefix+"alt_background.hue") != "0" {
		t.Fatal("hue arrow did not cross zero")
	}
	send(pressed(key.NameLeftArrow))
	if v.settingText(colorPrefix+"alt_background.hue") != "359" {
		t.Fatal("hue arrow did not cross zero backwards")
	}
	send(pressed(key.NameUpArrow))
	send(pressed(key.NameUpArrow))
	send(pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{Start: 0, End: 3}, Text: "0.1"})
	send(pressed(key.NameReturn))
	if v.settingsInteraction().editing || v.settingText(colorPrefix+"alt_background.lightness") != "0.88" {
		t.Fatal("typed actual value did not clamp to linked range")
	}
}

func TestNotificationsUsePaletteBoundsAndCanUnlink(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	_ = v.setWidgetValue(colorPrefix+"notification.hue", settings.Value{Number: 200})
	for _, role := range []string{"info", "success", "warning", "error"} {
		key := colorPrefix + role + ".hue"
		r, _ := settings.ThemeColorRange(v.settingsValues, settings.CatalogueDefault, key)
		n := v.settingsValues.Get(settings.CatalogueDefault, key).Number
		if math.Abs(r.Clamp(n)-n) > 1e-9 {
			t.Fatalf("%s escaped notification bounds", role)
		}
	}
	v.startSettingsWidget(colorPrefix + "error")
	v.adjustSetting(colorPrefix+"error.linked", 0)
	_ = v.setWidgetValue(colorPrefix+"error.hue", settings.Value{Number: 42})
	_ = v.setWidgetValue(colorPrefix+"notification.hue", settings.Value{Number: 300})
	if v.settingText(colorPrefix+"error.hue") != "42" {
		t.Fatal("unlinked notification followed palette")
	}
}

func TestThemeColorPreviews(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional color widget previews")
	}
	for _, role := range []string{"background", "alt_background", "error"} {
		v := newPreviewView()
		v.applySettings()
		v.startSettingsWidget(colorPrefix + role)
		if err := renderViewPNG(filepath.Join(dir, fmt.Sprintf("theme-color-%s.png", role)), v); err != nil {
			t.Fatal(err)
		}
	}
}
