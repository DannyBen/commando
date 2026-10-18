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

func TestSettingsValuesClickToEdit(t *testing.T) {
	for _, tc := range []struct{ group, field string }{
		{"application.data_folder", settingDataFolder},
		{"appearance.font", settingFontFamily},
		{"appearance.font", settingFontTextSize},
		{"feedback.sounds", settingSoundProfile},
		{settingThemeSelect, settingThemeSelect},
	} {
		t.Run(tc.field, func(t *testing.T) {
			v := newPreviewView()
			v.startSettingsWidget(tc.group)
			s := v.settingsInteraction()
			s.actionsFocused = true
			send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
			before := v.settingText(tc.field)
			s.values[tc.field].Click()
			send()
			if !s.editing || s.fields[s.selected].Key != tc.field || s.actionsFocused || s.editor.Text() != before || !router.Source().Focused(&s.editor) {
				t.Fatal("click did not enter and focus the existing editor")
			}
			s.editor.SetText("draft")
			s.editor.SetCaret(2, 2)
			send()
			s.values[tc.field].Click()
			send()
			if s.editor.Text() != "draft" || s.editor.SelectedText() != "" {
				t.Fatal("open editor was restarted")
			}
		})
	}
}

// Exercise actual pointer routing for wheel input without depending on dock geometry.
func valuePointerFrames(v *view, key string) func(...event.Event) {
	s := v.settingsInteraction()
	target := s.valueTarget(key)
	var router input.Router
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(300, 30))}
		v.updateSettingsWidget(gtx)
		event.Op(gtx.Ops, &v.focus)
		target.layout(gtx, !s.editing, func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: gtx.Constraints.Max} })
		router.Frame(&ops)
	}
	frame()
	frame()
	return func(events ...event.Event) { router.Queue(events...); frame(); frame() }
}

func TestSettingsWheelMatchesLeftAndRight(t *testing.T) {
	for _, tc := range []struct{ group, field string }{
		{"appearance.font", settingFontTextSize},
		{"appearance.font", settingFontFamily},
		{"feedback.sounds", settingSoundProfile},
		{settingThemeGenerator, settingThemeGenerator + ".color_shift"},
		{settingThemeSelect, settingThemeSelect},
		{"debug.trace", settingTraceMask},
	} {
		t.Run(tc.field, func(t *testing.T) {
			makeView := func() *view {
				v := newPreviewView()
				v.fontFamilies = []string{"Alpha", "Zulu"}
				v.startSettingsWidget(tc.group)
				if v.settingsInteraction() == nil {
					t.Fatal("missing widget", tc.group)
				}
				return v
			}
			mouse, keyboard := makeView(), makeView()
			for i, field := range keyboard.settingsInteraction().fields {
				if field.Key == tc.field {
					keyboard.settingsInteraction().selected = i
				}
			}
			keys, _ := panelInputFrames(keyboard)
			send := valuePointerFrames(mouse, tc.field)
			for _, step := range []struct {
				delta float32
				key   key.Name
			}{{-120, key.NameLeftArrow}, {120, key.NameRightArrow}, {120, key.NameRightArrow}} {
				keys(pressed(step.key))
				send(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: f32.Pt(20, 15), Scroll: f32.Pt(0, step.delta)})
				if mouse.settingText(tc.field) != keyboard.settingText(tc.field) || mouse.settingsInteraction().fields[mouse.settingsInteraction().selected].Key != tc.field {
					t.Fatalf("wheel %g gave %q; keyboard gave %q", step.delta, mouse.settingText(tc.field), keyboard.settingText(tc.field))
				}
			}
			if !reflect.DeepEqual(mouse.widgetValues().Clone(), keyboard.widgetValues().Clone()) {
				t.Fatal("wheel bypassed value transaction")
			}
		})
	}
}

func TestSettingsWheelAndNativeEditingInDock(t *testing.T) {
	v := newPreviewView()
	seed := settingThemeGenerator + ".seed"
	if err := v.settingsValues.Set(settings.CatalogueDefault, seed, settings.Value{Number: 50}); err != nil {
		t.Fatal(err)
	}
	v.startSettingsWidget(settingThemeGenerator)
	s := v.settingsInteraction()
	send, router := paletteFrames(v, image.Pt(800, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
	positions := []layout.Position{v.panels[0].list.Position, v.panels[1].list.Position, v.panels[2].list.Position}
	point := f32.Pt(320, 303)
	send(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: point, Scroll: f32.Pt(0, -120)})
	if v.settingText(seed) != "49" || s.selected != 1 || !router.Source().Focused(&v.focus) {
		t.Fatal("wheel did not reach hovered dock value", v.settingText(seed), s.selected)
	}
	for i := range v.panels {
		if v.panels[i].list.Position.First != positions[i].First || v.panels[i].list.Position.Offset != positions[i].Offset {
			t.Fatalf("wheel leaked to panel %d: before=%+v after=%+v", i, positions[i], v.panels[i].list.Position)
		}
	}
	rowClick(send, point, time.Second, 0)
	if !s.editing || !router.Source().Focused(&s.editor) {
		t.Fatal("value click did not start editing")
	}
	rowClick(send, point, 2*time.Second, 0)
	if !s.editing || s.editor.SelectedText() != "" {
		t.Fatal("second click did not use native caret placement")
	}
	send(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: point, Scroll: f32.Pt(0, 120)})
	if !s.editing || v.settingText(seed) != "49" {
		t.Fatal("wheel adjusted value while editing")
	}
}

func TestShortcutValueClickStartsCapture(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	_, s := openShortcutWidget(t, v, send, "Next metadata column")
	s.values[s.fields[0].Key].Click()
	send()
	if !s.capturing {
		t.Fatal("shortcut click did not start capture")
	}
	send(modified("J", key.ModCtrl))
	if s.capturing || v.shortcutSettingText(s, s.fields[0].Key) != "Ctrl+J" {
		t.Fatal("capture lost keyboard focus")
	}
}
