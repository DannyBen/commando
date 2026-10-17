package app

import (
	"reflect"
	"testing"
	"unicode/utf8"

	"gioui.org/io/key"
)

func TestChoiceHomeEndSelectFirstAndLastAcrossWidgets(t *testing.T) {
	for _, tc := range []struct{ group, setting, first, last string }{
		{"feedback.sounds", settingSoundProfile, "Errors only", "All sounds"},
		{"appearance.font", settingFontFamily, "Alpha", "Zulu"},
		{settingThemeSelect, settingThemeSelect, "Built-in Light", "Built-in Dark"},
		{settingThemeGenerator, settingThemeGenerator + ".mode", "Black", "White"},
	} {
		t.Run(tc.group, func(t *testing.T) {
			v := newPreviewView()
			v.fontFamilies = []string{"Zulu", "Alpha"}
			recorder := &recordedSounds{}
			v.sounds = recorder
			v.startSettingsWidget(tc.group)
			for i, field := range v.settingsInteraction().fields {
				if field.Key == tc.setting {
					v.settingsInteraction().selected = i
					break
				}
			}
			send, _ := panelInputFrames(v)
			for _, step := range []struct {
				name key.Name
				want string
			}{{key.NameHome, tc.first}, {key.NameEnd, tc.last}} {
				send(pressed(step.name))
				if v.settingText(tc.setting) != step.want {
					t.Fatal("boundary did not select expected choice", step.want)
				}
				before, played := v.settingsValues.Clone(), len(recorder.played)
				send(pressed(step.name), key.Event{Name: step.name, State: key.Release})
				if !reflect.DeepEqual(before, v.settingsValues.Clone()) || len(recorder.played) != played {
					t.Fatal("repeating a boundary or releasing its key changed values or played a cue")
				}
			}
		})
	}
}

func TestChoiceBoundariesKeepEditorCaretActionsAndModifiers(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget("feedback.sounds")
	s := v.settingsInteraction()
	s.selected = 1 // Profile follows Enabled.
	recorder := &recordedSounds{}
	v.sounds = recorder
	send, _ := panelInputFrames(v)
	before := v.settingsValues.Clone()
	played := len(recorder.played)
	// Already at All: End must not create an explicit override or dirty draft.
	send(pressed(key.NameEnd), modified(key.NameHome, key.ModCtrl), modified(key.NameEnd, key.ModShift))
	if s.dirty || len(recorder.played) != played || !reflect.DeepEqual(before, v.settingsValues.Clone()) {
		t.Fatalf("unchanged or modified boundaries affected the profile: dirty=%v cues=%v values=%v", s.dirty, recorder.played[played:], v.settingsValues)
	}
	send(pressed(key.NameReturn))
	send(pressed(key.NameHome))
	if a, b := s.editor.Selection(); a != 0 || b != 0 {
		t.Fatal("Home did not retain native caret movement")
	}
	send(pressed(key.NameEnd))
	end := utf8.RuneCountInString(s.editor.Text())
	if a, b := s.editor.Selection(); a != end || b != end || !s.editing || s.dirty || !reflect.DeepEqual(before, v.settingsValues.Clone()) {
		t.Fatal("End changed the setting while editing")
	}
	send(pressed(key.NameEscape))
	s.actionsFocused = true
	send()
	send(pressed(key.NameHome), pressed(key.NameEnd))
	if !s.actionsFocused || s.dirty || !reflect.DeepEqual(before, v.settingsValues.Clone()) {
		t.Fatal("action bar boundaries changed a choice")
	}
}

func TestThemeElementHomeEndUsesSharedWidgetSwitching(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget(settingThemeCustomize)
	send, _ := panelInputFrames(v)
	elements := themeElements()
	for _, step := range []struct {
		name key.Name
		want string
	}{{key.NameEnd, elements[len(elements)-1].Key}, {key.NameHome, elements[0].Key}} {
		send(pressed(step.name))
		s := v.settingsInteraction()
		if themeElementKey(s) != step.want {
			t.Fatal("theme element did not switch to boundary")
		}
		send(pressed(step.name))
		if v.settingsInteraction() != s || s.dirty {
			t.Fatal("same element reopened the widget or changed its draft")
		}
	}
}
