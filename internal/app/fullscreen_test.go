package app

import (
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/settings"
)

func TestFullScreenBindingsAndPalette(t *testing.T) {
	v := newPreviewView()
	calls := 0
	v.toggleFullScreen = func() { calls++ }
	send, _ := panelInputFrames(v)
	send(modified(key.NameReturn, key.ModAlt))
	send(pressed(key.NameF11))
	if calls != 2 {
		t.Fatalf("default shortcuts executed %d times", calls)
	}

	catalogue := bindingSettingsCatalogue()
	for slot := range 2 {
		if err := v.settingsValues.Set(catalogue, bindingSettingKey("application.toggle_full_screen", slot), settings.Value{Text: "none"}); err != nil {
			t.Fatal(err)
		}
	}
	// A saved Alt+Enter assignment is accepted as an ordinary shortcut.
	if err := v.settingsValues.Set(catalogue, bindingSettingKey("view.next_metadata_column", 0), settings.Value{Text: "Alt+Enter"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := catalogue.Save(dir, v.settingsValues); err != nil {
		t.Fatal(err)
	}
	values, err := settings.Load(dir, catalogue)
	if err != nil {
		t.Fatal(err)
	}
	v.restoreSettings(values, nil)
	before := v.panels[v.active].metadata
	send(modified(key.NameReturn, key.ModAlt))
	send(pressed(key.NameF11))
	if calls != 2 || v.panels[v.active].metadata == before {
		t.Fatal("unbinding or reassigning Alt+Enter failed")
	}
	send(pressed("`"))
	send(key.EditEvent{Text: "Toggle full screen"})
	if s := v.paletteInteraction(); len(s.results) == 0 || s.results[0].command.id != "application.toggle_full_screen" {
		t.Fatal("fullscreen command missing from palette")
	}
	send(pressed(key.NameReturn))
	if calls != 3 {
		t.Fatal("unbinding disabled palette execution")
	}
}

func TestNativeAltEnterForwardsPressAndRelease(t *testing.T) {
	q := &nativeKeyInput{invalidate: func() {}}
	for _, extra := range []uintptr{0, 1 << 24} {
		modifiers := key.ModAlt | key.ModShift
		q.altEnter(1|extra|1<<29, modifiers)
		q.altEnter(1|extra|1<<29|1<<30, modifiers)
		q.altEnter(1|extra|1<<29|1<<30|1<<31, modifiers)
		events := q.drain()
		if len(events) != 2 || events[0] != (key.Event{Name: key.NameReturn, Modifiers: modifiers, State: key.Press}) || events[1] != (key.Event{Name: key.NameReturn, Modifiers: modifiers, State: key.Release}) {
			t.Fatalf("unexpected bridged events: %+v", events)
		}
	}
}

func TestNativeAltEnterTogglesFullScreenOnce(t *testing.T) {
	v := newPreviewView()
	v.nativeKeys = &nativeKeyInput{invalidate: func() {}}
	calls := 0
	v.toggleFullScreen = func() { calls++ }
	send, _ := panelInputFrames(v)
	for _, data := range []uintptr{1 | 1<<29, 1 | 1<<29 | 1<<30, 1 | 1<<29 | 1<<30 | 1<<31} {
		v.nativeKeys.altEnter(data, key.ModAlt)
		send()
	}
	if calls != 1 {
		t.Fatalf("one native Alt+Enter toggled fullscreen %d times", calls)
	}
}
