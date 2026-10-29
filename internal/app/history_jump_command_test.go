package app

import (
	"testing"

	"gioui.org/io/key"
)

func TestHistoryJumpCommandOpensLocationSearch(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: ">comm"})
	if !v.locationEditing() || v.location.Text() != ">comm" {
		t.Fatalf("default binding lost initial query: editing=%v text=%q", v.locationEditing(), v.location.Text())
	}
	send(pressed(key.NameEscape))
	send(pressed("`"))
	send(key.EditEvent{Text: "History jump"})
	p := v.paletteInteraction()
	if p == nil || len(p.results) != 1 || p.results[0].command.id != "view.history_jump" {
		t.Fatal("History jump was not found in the command palette")
	}
	send(pressed(key.NameReturn))
	if v.paletteInteraction() != nil || !v.locationEditing() || v.location.Text() != ">" {
		t.Fatalf("palette command did not open search: palette=%v editing=%v text=%q", v.paletteInteraction() != nil, v.locationEditing(), v.location.Text())
	}
}

func TestHistoryJumpRebindingKeepsQueryCue(t *testing.T) {
	v := newPreviewView()
	setTestShortcut(t, v, "view.history_jump", 0, "Ctrl+H")
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: ">comm"})
	if v.locationEditing() {
		t.Fatal("old > binding still opened history search")
	}
	send(modified("H", key.ModCtrl))
	if !v.locationEditing() || v.location.Text() != ">" {
		t.Fatalf("replacement binding lost fixed cue: editing=%v text=%q", v.locationEditing(), v.location.Text())
	}
}
