package app

import (
	"os"
	"path/filepath"
	"testing"

	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/settings"
)

func TestAutoColorChildWidgetsLiveCancelResetAndSave(t *testing.T) {
	for _, tc := range []struct {
		widget, title string
		keys          []string
	}{
		{"appearance.autocolor.selection", "Selection", []string{settingSelectionLightness, settingCutLightness}},
		{"appearance.autocolor.special_files", "Special files", []string{settingHiddenLightness, settingHiddenChroma, settingDotfileLightness, settingDotfileChroma}},
	} {
		t.Run(tc.title, func(t *testing.T) {
			v := newPreviewView()
			v.startSettingsWidget("appearance.autocolor")
			if len(v.settingsInteraction().fields) != 4 {
				t.Fatal("child controls leaked into the basic widget")
			}
			v.finishSettingsWidget(layout.Context{}, false)
			before := *v.appearance.FilenameColors
			probe := func(key string) oklchColor {
				s := *v.appearance.FilenameColors
				file := browser.Entry{Name: "file.json"}
				switch key {
				case settingHiddenLightness, settingHiddenChroma:
					file.Hidden = true
				case settingDotfileLightness, settingDotfileChroma:
					file.Name = ".file.json"
				}
				value := s.filename(file)
				if key == settingSelectionLightness {
					value = s.activeBackground(value)
				} else if key == settingCutLightness {
					value = s.cutColor(value)
				}
				return value
			}
			change := func() {
				v.startSettingsWidget(tc.widget)
				s := v.settingsInteraction()
				if s.title != "Appearance › Auto color › "+tc.title || len(s.fields) != len(tc.keys) {
					t.Fatalf("incorrect child widget: %#v", s)
				}
				for i, key := range tc.keys {
					original := probe(key)
					if s.fields[i].Key != key {
						t.Fatal("child field order changed")
					}
					v.adjustSetting(key, -1)
					if probe(key) == original || !s.dirty {
						t.Fatalf("%s did not preview its color change", key)
					}
				}
			}
			change()
			v.activateSettingsAction(layout.Context{}, 1)
			if *v.appearance.FilenameColors != before {
				t.Fatal("Cancel did not restore color settings")
			}
			change()
			// Reset the child without resetting a basic preference.
			_ = v.settingsValues.Set(settings.CatalogueDefault, settingSalt, settings.Value{Number: 42})
			v.activateSettingsAction(layout.Context{}, 2)
			if v.appearance.FilenameColors.salt != 42 {
				t.Fatal("child Reset changed a basic preference")
			}
			for _, key := range tc.keys {
				if v.settingsValues.Overridden(key) {
					t.Fatal("child Reset retained an override")
				}
			}
			v.finishSettingsWidget(layout.Context{}, false)
			change()
			dir := t.TempDir()
			v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
				return func() (string, error) { return "", settings.Save(dir, values) }
			}
			v.finishSettingsWidget(layout.Context{}, true)
			loaded, err := settings.Load(dir, settings.CatalogueDefault)
			if err != nil || !v.interaction.idle() {
				t.Fatal("child save failed", err)
			}
			restored := newPreviewView()
			restored.restoreSettings(loaded, nil)
			if *restored.appearance.FilenameColors != *v.appearance.FilenameColors {
				t.Fatal("saved child colors did not apply on startup")
			}
			restored.startSettingsWidget("appearance.autocolor")
			restored.adjustSetting(settingAutoColorEnabled, 0)
			if restored.appearance.FilenameColors.enabled {
				t.Fatal("Enabled did not disable generated colors")
			}
			for _, key := range tc.keys {
				if restored.settingsValues.Get(settings.CatalogueDefault, key) != v.settingsValues.Get(settings.CatalogueDefault, key) {
					t.Fatal("disabling auto color discarded a child preference")
				}
			}
		})
	}
}

func TestAutoColorChildWidgetPreviews(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional child widget previews")
	}
	for _, name := range []string{"selection", "special_files"} {
		v := newPreviewView()
		v.startSettingsWidget("appearance.autocolor." + name)
		if err := renderViewPNG(filepath.Join(dir, "settings-autocolor-"+name+".png"), v); err != nil {
			t.Fatal(err)
		}
	}
}
