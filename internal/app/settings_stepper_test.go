package app

import (
	"reflect"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

func TestSettingsPageAndEndpointKeys(t *testing.T) {
	for _, tc := range []struct {
		name, widget, key                        string
		initial, forward, back, minimum, maximum float64
	}{
		{"points", "appearance.autocolor", settingLightness, 36, 61, 36, 0, 100},
		{"clamping", "appearance.autocolor", settingLightness, 98, 100, 75, 0, 100},
		{"signed", "appearance.autocolor.selection", settingCutLightness, -90, -65, -90, -100, 100},
		{"fractional", "appearance.autocolor.special_files", settingHiddenSaturation, 0.5, 0.75, 0.5, 0, 1},
		{"font size", "appearance.font", settingFontTextSize, 15, 40, 15, 6, 48},
		{"unbounded", "appearance.autocolor", settingSalt, 0, 25, 0, 0, 0},
		{"hue", colorPrefix + "background", colorPrefix + "background.hue", 355, 20, 355, 0, 359},
		{"color shift", settingThemeGenerator, settingThemeGenerator + ".color_shift", 355, 20, 355, 0, 359},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newPreviewView()
			if err := v.settingsValues.Set(settings.CatalogueDefault, tc.key, settings.Value{Number: tc.initial}); err != nil {
				t.Fatal(err)
			}
			v.startSettingsWidget(tc.widget)
			s := v.settingsInteraction()
			found := false
			for i, field := range s.fields {
				if field.Key == tc.key {
					s.selected, found = i, true
					break
				}
			}
			if !found {
				t.Fatal("stepper not found in widget")
			}
			send, _ := panelInputFrames(v)
			for _, action := range []struct {
				key  key.Name
				want float64
			}{
				{key.NamePageDown, tc.forward}, {key.NamePageUp, tc.back},
				{key.NameHome, tc.minimum}, {key.NameEnd, tc.maximum},
			} {
				send(pressed(action.key))
				if got := v.settingsValues.Get(settings.CatalogueDefault, tc.key).Number; got != action.want {
					t.Fatalf("%s = %g; want %g", action.key, got, action.want)
				}
			}
		})
	}
}

func TestColorShiftWrapsWithAllAdjustmentSpeeds(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget(settingThemeGenerator)
	v.settingsInteraction().selected = 4
	send, _ := panelInputFrames(v)
	for _, action := range []struct {
		event key.Event
		want  float64
	}{
		{pressed(key.NameLeftArrow), 359},
		{pressed(key.NameRightArrow), 0},
		{modified(key.NameLeftArrow, key.ModCtrl), 350},
		{modified(key.NameRightArrow, key.ModCtrl), 0},
		{pressed(key.NamePageUp), 335},
		{pressed(key.NamePageDown), 0},
	} {
		send(action.event)
		if got := v.settingsValues.Get(settings.CatalogueDefault, settingThemeGenerator+".color_shift").Number; got != action.want {
			t.Fatalf("%v = %g; want %g", action.event, got, action.want)
		}
	}
}

func TestSeedEndRandomizesReproduciblyAndCancelRestores(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	before, palette := v.settingsValues.Clone(), v.appearance.Colors
	v.startSettingsWidget(settingThemeGenerator)
	s := v.settingsInteraction()
	send, _ := panelInputFrames(v)
	s.selected = 1 // Seed follows Mode.
	previous := v.settingsValues.Get(settings.CatalogueDefault, settingThemeGenerator+".seed").Number
	for range 5 {
		send(pressed(key.NameEnd))
		seed := v.settingsValues.Get(settings.CatalogueDefault, settingThemeGenerator+".seed").Number
		if seed == previous || seed < 1 || seed > 999999 || !s.dirty {
			t.Fatalf("End did not choose a different random seed: %g -> %g", previous, seed)
		}
		for _, control := range []string{"mode", "vibrance", "dynamics", "color_shift"} {
			name := settingThemeGenerator + "." + control
			if v.settingsValues.Get(settings.CatalogueDefault, name) != before.Get(settings.CatalogueDefault, name) {
				t.Fatalf("randomizing Seed also changed %s", control)
			}
		}
		other := newPreviewView()
		other.settingsValues = before.Clone()
		other.startSettingsWidget(settingThemeGenerator)
		if err := other.setWidgetValue(settingThemeGenerator+".seed", settings.Value{Number: seed}); err != nil {
			t.Fatal(err)
		}
		other.applySettings()
		if v.appearance.Colors != other.appearance.Colors {
			t.Fatal("displayed random seed did not reproduce the preview")
		}
		previous = seed
	}
	send(key.Event{Name: key.NameEnd, State: key.Release})
	if seed := v.settingsValues.Get(settings.CatalogueDefault, settingThemeGenerator+".seed").Number; seed != previous {
		t.Fatal("key release randomized Seed again")
	}
	v.activateSettingsAction(layout.Context{}, 1)
	if !reflect.DeepEqual(v.settingsValues.Clone(), before) || v.appearance.Colors != palette {
		t.Fatal("Cancel did not restore the original seed and palette")
	}
}

func TestStepperKeysKeepNativeEditingChoicesAndActions(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget(settingThemeGenerator)
	s := v.settingsInteraction()
	send, _ := panelInputFrames(v)
	before := v.settingsValues.Clone()
	s.selected = 1 // Seed follows Mode.
	send(pressed(key.NameReturn))
	s.editor.SetCaret(0, 0)
	send()
	send(pressed(key.NameEnd))
	if a, b := s.editor.Selection(); a != 1 || b != 1 {
		t.Fatal("End did not retain native caret movement")
	}
	send(pressed(key.NameHome))
	if a, b := s.editor.Selection(); a != 0 || b != 0 {
		t.Fatal("Home did not retain native caret movement")
	}
	send(pressed(key.NamePageUp), pressed(key.NamePageDown))
	if !s.editing || s.editor.Text() != "1" || s.dirty || !reflect.DeepEqual(v.settingsValues.Clone(), before) {
		t.Fatal("stepper keys changed a setting while typing")
	}
	send(pressed(key.NameEscape))
	s.selected = 0 // Mode is a choice, not a stepper.
	send()
	send(pressed(key.NameHome), pressed(key.NameEnd), pressed(key.NamePageUp), pressed(key.NamePageDown))
	if s.selected != 0 || s.dirty || !reflect.DeepEqual(v.settingsValues.Clone(), before) {
		t.Fatal("stepper keys affected a choice")
	}
	s.selected, s.actionsFocused = 0, true
	send()
	send(pressed(key.NameHome), pressed(key.NameEnd), pressed(key.NamePageUp), pressed(key.NamePageDown))
	if !s.actionsFocused || s.dirty || !reflect.DeepEqual(v.settingsValues.Clone(), before) {
		t.Fatal("stepper keys affected the action bar")
	}
	v.activateSettingsAction(layout.Context{}, 1)
	v.startSettingsWidget("appearance.autocolor")
	v.settingsInteraction().selected = 3 // Salt has no maximum and no random action.
	send()
	send(pressed(key.NameEnd))
	if v.settingsInteraction().dirty || !reflect.DeepEqual(v.settingsValues.Clone(), before) {
		t.Fatal("End changed an unbounded value other than Seed")
	}
}
