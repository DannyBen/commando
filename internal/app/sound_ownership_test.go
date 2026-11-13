package app

import (
	"os"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/sound"
)

func TestPaletteNavigationHandsSoundToItsOutcome(t *testing.T) {
	for _, outcome := range []string{"notification", "navigation", "warning", "error"} {
		t.Run(outcome, func(t *testing.T) {
			v := newPreviewView()
			recorder := &recordedSounds{}
			v.sounds = recorder
			v.activeDataFolder = t.TempDir()
			v.loader = &directoryLoader{requests: make(chan loadRequest, 1), results: make(chan loadResult, 1)}
			send, advance := panelInputFrames(v)
			send(pressed("`"), key.EditEvent{Text: "Open data folder"})
			recorder.played = nil
			send(pressed(key.NameReturn))
			if !v.interaction.idle() || !v.panels[0].loading || len(recorder.played) != 0 {
				t.Fatal("palette or dispatch announced instead of handing off", recorder.played)
			}
			advance(time.Hour) // Even visible slow-read progress must stay silent.
			if len(recorder.played) != 0 {
				t.Fatal("progress announced before the outcome", recorder.played)
			}
			request := <-v.loader.requests
			result := loadResult{request: request, directory: browser.Directory{Path: v.activeDataFolder}}
			want := sound.MessageInfo
			switch outcome {
			case "navigation":
				result.request.successMessage = ""
				want = sound.DirectoryNavigation
			case "warning":
				result.pathMissing = true
				want = sound.MessageWarning
			case "error":
				result.err = os.ErrPermission
				want = sound.MessageError
			}
			v.acceptLoad(result)
			message := v.panels[0].feedback.Current()
			v.acceptLoad(result) // The same worker conclusion cannot replay its outcome.
			late := result
			late.progress = "Still reading"
			v.acceptLoad(late)
			if v.panels[0].feedback.Current() != message {
				t.Fatal("repeated completion or late progress replaced its concluded outcome")
			}
			send()
			advance(time.Hour)
			send(key.Event{Name: key.NameReturn, State: key.Release})
			if !slices.Equal(recorder.played, []sound.Name{want}) {
				t.Fatal("completion, duplicate delivery or rendering competed", recorder.played)
			}
		})
	}
}

func TestInteractionConclusionAndHandoffHaveDifferentOwnership(t *testing.T) {
	for _, handoff := range []bool{false, true} {
		v := newPreviewView()
		recorder := &recordedSounds{}
		v.sounds = recorder
		s := &paletteSession{panel: 0}
		v.startInteraction(s)
		recorder.played = nil
		if handoff {
			if !v.handoffInteraction(s) {
				t.Fatal("handoff rejected its owner")
			}
		} else if !v.finishInteraction(s) {
			t.Fatal("conclusion rejected its owner")
		}
		if v.finishInteraction(s) || v.handoffInteraction(s) {
			t.Fatal("a concluded interaction retained ownership")
		}
		want := []sound.Name{sound.Closed}
		if handoff {
			want = nil
		}
		if !slices.Equal(recorder.played, want) {
			t.Fatal("conclusion/handoff used the wrong sound policy", recorder.played)
		}
	}
}
