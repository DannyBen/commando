package app

import (
	"testing"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/sound"
)

func TestBrowsingNavigationPlaysOneTickPerMovement(t *testing.T) {
	for _, ke := range []key.Event{
		pressed(key.NameUpArrow), pressed(key.NameDownArrow),
		pressed(key.NameLeftArrow), pressed(key.NameRightArrow),
		pressed(key.NameTab), modified(key.NameTab, key.ModShift),
		pressed(key.NamePageUp), pressed(key.NamePageDown),
		pressed(key.NameHome), pressed(key.NameEnd),
	} {
		t.Run(string(ke.Name)+ke.Modifiers.String(), func(t *testing.T) {
			v := newPreviewView()
			v.active = 1
			p := &v.panels[1]
			p.selected = len(p.items) / 2
			recorded := &recordedSounds{}
			v.sounds = recorded
			send, _ := panelInputFrames(v)
			selected := p.selected
			send(ke)
			if v.active == 1 && p.selected == selected {
				t.Fatal("test navigation did not move")
			}
			if len(recorded.played) != 1 || recorded.played[0] != sound.Navigation {
				t.Fatalf("movement did not play exactly one tick: %v", recorded.played)
			}
			ke.State = key.Release
			send(ke)
			if len(recorded.played) != 1 {
				t.Fatal("key release replayed navigation sound")
			}
		})
	}
}

func TestNavigationEdgesAndEditorsStaySilent(t *testing.T) {
	v := newPreviewView()
	v.active, v.panels[0].selected = 0, 0
	recorded := &recordedSounds{}
	v.sounds = recorded
	send, _ := panelInputFrames(v)
	send(pressed(key.NameUpArrow), pressed(key.NameHome), pressed(key.NamePageUp), pressed(key.NameLeftArrow))
	v.active = len(v.panels) - 1
	p := &v.panels[v.active]
	p.selected = len(p.items) - 1
	send(pressed(key.NameDownArrow), pressed(key.NameEnd), pressed(key.NamePageDown), pressed(key.NameRightArrow))
	if len(recorded.played) != 0 {
		t.Fatal("navigation against an edge played sound")
	}
	for _, editor := range []string{"location", "name"} {
		t.Run(editor, func(t *testing.T) {
			v := newPreviewView()
			recorded := &recordedSounds{}
			v.sounds = recorded
			send, _ := panelInputFrames(v)
			if editor == "location" {
				send(modified("D", key.ModAlt))
			} else {
				send(pressed(key.NameF2))
			}
			send(pressed(key.NameLeftArrow), pressed(key.NameRightArrow), pressed(key.NameUpArrow), pressed(key.NameDownArrow),
				pressed(key.NameHome), pressed(key.NameEnd), pressed(key.NamePageUp), pressed(key.NamePageDown),
				pressed(key.NameTab), modified(key.NameTab, key.ModShift))
			if len(recorded.played) != 0 || v.activeEditor() == nil {
				t.Fatal("text editing played navigation sounds or left the editor")
			}
		})
	}
}

func TestPaletteNavigationTicksWhileSearchEditingStaysSilent(t *testing.T) {
	v := newPreviewView()
	recorded := &recordedSounds{}
	v.sounds = recorded
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	if len(recorded.played) != 1 || recorded.played[0] != sound.Palette {
		t.Fatalf("palette opening cue: %v", recorded.played)
	}
	recorded.played = nil
	send(pressed(key.NameDownArrow), pressed(key.NameEnd), pressed(key.NameHome))
	if len(recorded.played) != 3 {
		t.Fatalf("palette movement cues: %v", recorded.played)
	}
	send(pressed(key.NameHome), pressed(key.NameUpArrow), pressed(key.NameTab), modified(key.NameTab, key.ModShift),
		key.EditEvent{Text: "panel"}, modified(key.NameEnd, key.ModShift))
	if len(recorded.played) != 3 || v.paletteInteraction() == nil {
		t.Fatal("palette edge or search editing played a navigation sound")
	}
	for _, cue := range recorded.played {
		if cue != sound.Navigation {
			t.Fatal("palette used a different sound")
		}
	}
}

func TestPaletteOpeningCueForBothModes(t *testing.T) {
	for _, opening := range []event.Event{pressed("`"), modified("P", key.ModCtrl|key.ModShift), key.EditEvent{Text: "@"}} {
		v := newPreviewView()
		recorded := &recordedSounds{}
		v.sounds = recorded
		send, _ := panelInputFrames(v)
		send(opening)
		if v.paletteInteraction() == nil || len(recorded.played) != 1 || recorded.played[0] != sound.Palette {
			t.Fatalf("opening %v did not play one palette cue: %v", opening, recorded.played)
		}
	}
}

func TestPaletteTextOpeningDoesNotStopItsOwnCue(t *testing.T) {
	v := newPreviewView()
	recorded := &recordedSounds{}
	v.sounds = recorded
	send, _ := panelInputFrames(v)
	// The physical key remains available to deferred feedback handling after
	// its text event opens settings search.
	send(modified("2", key.ModShift), key.EditEvent{Text: "@"})
	if v.paletteInteraction() == nil || v.commandSearch.Text() != "@" ||
		len(recorded.played) != 1 || recorded.played[0] != sound.Palette || recorded.stops != 0 {
		t.Fatalf("settings opening played %v with %d stops", recorded.played, recorded.stops)
	}
}

func TestInlineNavigationPlaysOnceWithoutChangingFileSelection(t *testing.T) {
	v := newPreviewView()
	recorded := &recordedSounds{}
	v.sounds = recorded
	send, _ := panelInputFrames(v)
	selected := v.panels[0].selected
	v.startInlineActions(0, 4, "", "", []string{"First", "Second", "Third"}, nil, nil)
	send()
	send(pressed(key.NameDownArrow), pressed(key.NameEnd), pressed(key.NameHome), pressed(key.NameUpArrow))
	if len(recorded.played) != 3 || v.panels[0].selected != selected {
		t.Fatal("inline navigation replayed sound or changed the underlying selection")
	}
	send(pressed(key.NameRightArrow))
	if len(recorded.played) != 4 || v.active != 1 {
		t.Fatal("leaving inline choices did not play one panel navigation tick")
	}
}

func TestTransferTargetNavigationPlaysOnceAndIgnoresEdges(t *testing.T) {
	v, root, _ := clipboardView(t)
	v.load(1, root, "")
	waitLoaded(t, v, 1)
	selectName(t, &v.panels[0], "a.txt")
	recorded := &recordedSounds{}
	v.sounds = recorded
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF5))
	send(pressed(key.NameRightArrow), pressed(key.NameRightArrow), pressed(key.NameTab), modified(key.NameTab, key.ModShift))
	if v.active != 2 || v.inlineInteraction() == nil || len(recorded.played) != 3 {
		t.Fatalf("transfer target navigation did not play once per move: %v", recorded.played)
	}
	for _, cue := range recorded.played {
		if cue != sound.Navigation {
			t.Fatal("transfer targeting used a different sound")
		}
	}
}
