package app

import (
	"image"
	"reflect"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/settings"
)

func TestSettingsChoiceClicksPreviewAndCancel(t *testing.T) {
	for _, tc := range []struct {
		group, field, want string
		choice             int
	}{
		{"appearance.icons", "appearance.icons.enabled", "false", 1},
		{settingThemeGenerator, settingThemeGenerator + ".mode", "White", 4},
		{"guards.recycle_confirmation", "guards.recycle_confirmation.directories", "ON (Cancel default)", 2},
	} {
		t.Run(tc.field, func(t *testing.T) {
			v := newPreviewView()
			before := v.settingsValues.Clone()
			v.startSettingsWidget(tc.group)
			s := v.settingsInteraction()
			s.actionsFocused = true
			send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
			s.choices[tc.field].buttons[tc.choice].Click()
			send()
			if v.settingText(tc.field) != tc.want || s.fields[s.selected].Key != tc.field || s.actionsFocused || !s.dirty {
				t.Fatal("choice did not preview the requested value and focus its field", v.settingText(tc.field))
			}
			if !router.Source().Focused(&v.focus) {
				t.Fatal("keyboard focus did not follow choice click")
			}
			// Keyboard input continues from the clicked row.
			send(pressed(key.NameLeftArrow))
			if v.settingText(tc.field) == tc.want {
				t.Fatalf("keyboard focus did not follow the clicked row selected=%d editing=%v actions=%v current=%T", s.selected, s.editing, s.actionsFocused, v.interaction.current)
			}
			s.actions.buttons[1].Click()
			send()
			if !reflect.DeepEqual(v.settingsValues.Clone(), before) {
				t.Fatal("Cancel did not restore settings")
			}
		})
	}
}

func TestSettingsSelectedChoiceClickDoesNotDirty(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget("appearance.icons")
	s := v.settingsInteraction()
	send, _ := panelInputFrames(v)
	before := v.settingsValues.Clone()
	s.choices["appearance.icons.enabled"].buttons[0].Click()
	send()
	if s.dirty || !reflect.DeepEqual(before, v.settingsValues.Clone()) {
		t.Fatalf("clicking current value dirty=%v before=%#v after=%#v", s.dirty, before, v.settingsValues)
	}
}

func TestPendingSettingsChoiceDoesNotReplayAfterSave(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget("appearance.icons")
	s := v.settingsInteraction()
	send, _ := panelInputFrames(v)
	s.pending = true
	send()
	s.choices[settingIconsEnabled].buttons[1].Click()
	send()
	s.pending = false
	send()
	if s.dirty || !v.settingsValues.Get(settings.CatalogueDefault, settingIconsEnabled).Bool {
		t.Fatal("disabled choice replayed when the widget became available")
	}
}

func TestDeferredChoiceClickAppliesOnSave(t *testing.T) {
	deferWidget(t, "appearance.icons")
	v := newPreviewView()
	v.startSettingsWidget("appearance.icons")
	s := v.settingsInteraction()
	send, _ := panelInputFrames(v)
	s.choices["appearance.icons.enabled"].buttons[1].Click()
	send()
	if !v.settingsValues.Get(settings.CatalogueDefault, settingIconsEnabled).Bool || v.widgetValues().Get(settings.CatalogueDefault, "appearance.icons.enabled").Bool {
		t.Fatal("deferred click applied live or failed to stage")
	}
	send(modified(key.NameReturn, key.ModCtrl))
	if v.settingsValues.Get(settings.CatalogueDefault, settingIconsEnabled).Bool || v.settingsInteraction() != nil {
		t.Fatalf("Save failed current=%T selecting=%v actions=%v pending=%v value=%v err=%s", v.interaction.current, s.selecting, s.actionsFocused, s.pending, v.settingText(settingIconsEnabled), s.errorText)
	}
}

func TestSettingsChoicePointerFeedbackAndRelease(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget("appearance.icons")
	s := v.settingsInteraction()
	field := s.fields[0]
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Constraints{Max: image.Pt(400, 100)}}
		v.updateSettingsChoices(gtx, s)
		v.settingsChoices(gtx, field, true, v.textHeight(gtx, v.appearance.TextSize))
		router.Frame(&ops)
	}
	send := func(events ...event.Event) { router.Queue(events...); frame(); frame() }
	frame()
	frame()
	row := s.choices[field.Key]
	point := f32.Pt(float32(row.buttons[0].size.X+row.buttons[1].size.X/2), float32(row.buttons[1].size.Y/2))
	send(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: point})
	if !row.buttons[1].clickable.Hovered() {
		t.Fatal("choice did not hover")
	}
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Position: point, Buttons: pointer.ButtonPrimary})
	if !row.buttons[1].clickable.Pressed() || !v.settingsValues.Get(settings.CatalogueDefault, settingIconsEnabled).Bool {
		t.Fatal("press did not highlight or applied value before release")
	}
	send(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: point})
	if v.settingsValues.Get(settings.CatalogueDefault, settingIconsEnabled).Bool {
		t.Fatal("pointer release did not choose OFF")
	}
	// The measured button area must not spill into the rest of the row.
	outside := f32.Pt(300, point.Y)
	rowClick(send, outside, time.Second, 0)
	if v.settingsValues.Get(settings.CatalogueDefault, settingIconsEnabled).Bool {
		t.Fatal("click outside choice bounds changed value")
	}
}

func TestSettingsChoiceCommitKeepsOtherEditorValidation(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget("appearance.icons")
	s := v.settingsInteraction()
	s.selected = 1
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	s.editor.SetText("invalid")
	send()
	s.choices["appearance.icons.enabled"].buttons[1].Click()
	send()
	if !s.editing || s.selected != 1 || s.errorText == "" || !v.settingsValues.Get(settings.CatalogueDefault, settingIconsEnabled).Bool {
		t.Fatal("choice click bypassed invalid editor")
	}
	s.editor.SetText("25")
	send()
	s.choices["appearance.icons.enabled"].buttons[1].Click()
	send()
	if s.editing || s.selected != 0 || v.settingsValues.Get(settings.CatalogueDefault, settingIconsEnabled).Bool || v.settingText(s.fields[1].Key) != "25" {
		t.Fatal("valid draft was not accepted before choosing")
	}
}
