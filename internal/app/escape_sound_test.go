package app

import (
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/sound"
)

func TestEscapePlaysOneCueAcrossInteractions(t *testing.T) {
	for _, context := range []string{"browsing", "prefix", "location", "rename", "create", "palette", "inline", "duplicate"} {
		t.Run(context, func(t *testing.T) {
			v := newPreviewView()
			send, advance := panelInputFrames(v)
			selected := v.panels[0].selected
			switch context {
			case "prefix":
				send(key.EditEvent{Text: "c"})
				selected = v.panels[0].selected
			case "location":
				send(modified("D", key.ModAlt))
			case "rename":
				send(pressed(key.NameF2))
			case "create":
				send(newFolderKey())
			case "palette":
				send(pressed("`"))
			case "inline":
				v.startInlineActions(0, 4, "", "", []string{"Choose"}, nil, nil)
			case "duplicate":
				var ops op.Ops
				v.beginName(layout.Context{Ops: &ops}, &nameSession{
					kind: nameDuplicate, panel: 0, row: 4, path: v.panels[0].path,
					previousPosition: v.panels[0].list.Position}, "copy.txt", false)
				v.transferPending = true
			}
			send()
			v.showFeedback(0, feedback.Error, "Access denied")
			send()
			recorded := &recordedSounds{}
			v.sounds = recorded
			send(pressed(key.NameEscape))
			advance(time.Hour)
			if len(recorded.played) != 1 || recorded.played[0] != sound.Cancel {
				t.Fatalf("Escape did not produce exactly one cancellation cue: %v", recorded.played)
			}
			if !v.interaction.idle() || v.panels[0].selected != selected || v.panels[0].prefix != "" || v.active != 0 {
				t.Fatal("Escape changed selection or failed to cancel its interaction")
			}
			if context == "duplicate" {
				message := v.panels[0].feedback.Current()
				if v.transferPending || message == nil || message.Text != "Copy cancelled" {
					t.Fatal("Escape lost the duplicate cancellation result")
				}
			} else if v.panels[0].feedback.Current() != nil {
				t.Fatal("Escape did not dismiss feedback")
			}
			send(key.Event{Name: key.NameEscape, State: key.Release})
			if len(recorded.played) != 1 {
				t.Fatal("Escape release replayed the cue")
			}
		})
	}
}

func TestEscapeRemainsSilentWhileSubmittedNameIsProtected(t *testing.T) {
	v := newPreviewView()
	v.panels[0].clearMarks()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 1)}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameF2))
	send(replaceRename(v, "new-name"), pressed(key.NameReturn))
	owner := v.nameInteraction()
	if owner == nil || !owner.pending {
		t.Fatal("test name mutation was not submitted")
	}
	recorded := &recordedSounds{}
	v.sounds = recorded
	send(pressed(key.NameEscape))
	if len(recorded.played) != 0 || v.nameInteraction() != owner || !owner.pending {
		t.Fatal("ignored Escape played a cue or cancelled submitted work")
	}
}

func TestMouseCancellationDoesNotPlayEscapeCue(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	recorded := &recordedSounds{}
	v.sounds = recorded
	var ops op.Ops
	if !v.cancelInteraction(layout.Context{Ops: &ops}) || !v.interaction.idle() || len(recorded.played) != 0 {
		t.Fatal("non-keyboard cancellation emitted an Escape sound")
	}
}
