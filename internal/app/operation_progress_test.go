package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/sound"
)

func TestQuietOperationProgressPreview(t *testing.T) {
	directory := os.Getenv("COMMANDO_PREVIEW_MESSAGES")
	if directory == "" {
		t.Skip("optional quiet progress preview")
	}
	v := newPreviewView()
	v.operationProgress(0, "Recycling")
	frame, _ := v.panels[0].feedback.Frame(time.Time{}, v.appearance.Feedback)
	v.panels[0].feedback.Frame(frame.Next, v.appearance.Feedback)
	if err := renderViewPNG(filepath.Join(directory, "operation-progress.png"), v); err != nil {
		t.Fatal(err)
	}
}

func TestRemovalProgressIsDelayedAndQuietWithNormalResults(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		for _, slow := range []bool{false, true} {
			v := newPreviewView()
			v.appearance.FeedbackProgressDelay = time.Hour
			v.loader = &directoryLoader{requests: make(chan loadRequest, 1)}
			recorder := &recordedSounds{}
			v.sounds = recorder
			if permanent {
				for _, name := range []string{"single_files", "directories", "multiple_entries"} {
					setGuard(t, v, true, name, "OFF")
				}
			}
			send, _ := panelInputFrames(v)
			event := pressed(key.NameDeleteForward)
			if permanent {
				event = modified(key.NameDeleteForward, key.ModShift)
			}
			before := time.Now()
			send(event)
			request := <-v.loader.requests
			pending, owns := v.panels[0].feedback.Frame(before, v.appearance.Feedback)
			due := pending.Next
			if owns || due.Before(before.Add(v.appearance.FeedbackProgressDelay)) || len(recorder.played) != 0 {
				t.Fatal("operation immediately displayed or announced progress")
			}
			if slow {
				drawLoadingFrame(v, due)
				frame, owns := v.panels[0].feedback.Frame(due, v.appearance.Feedback)
				want := "Recycling"
				if permanent {
					want = "Deleting"
				}
				if !owns || frame.Text != want || frame.Started || !frame.Next.IsZero() || len(recorder.played) != 0 {
					t.Fatal("slow operation progress was not steady and quiet", frame, recorder.played)
				}
			}
			v.acceptLoad(loadResult{request: request, removed: browser.RemovalResult{Names: request.removal.names}})
			drawLoadingFrame(v, due.Add(time.Second))
			message := v.panels[0].feedback.Current()
			if message == nil || message.Kind != feedback.Success || len(recorder.played) != 1 || recorder.played[0] != sound.MessageSuccess {
				t.Fatal("completion lost its normal notification", message, recorder.played)
			}
			drawLoadingFrame(v, due.Add(time.Hour))
			if len(recorder.played) != 1 || v.panels[0].feedback.Current().Kind != feedback.Success {
				t.Fatal("pending progress resurfaced after completion")
			}
		}
	}
}

func TestSubmittedNameMutationUsesDelayedQuietProgress(t *testing.T) {
	for _, kind := range []nameOperation{nameRename, nameCreateFile, nameCreateFolder, nameDuplicate} {
		v := newPreviewView()
		v.panels[0].clearMarks()
		v.appearance.FeedbackProgressDelay = time.Hour
		v.loader = &directoryLoader{requests: make(chan loadRequest, 1)}
		send, _ := panelInputFrames(v)
		var ops op.Ops
		v.startName(layout.Context{Ops: &ops}, kind)
		send()
		v.nameEditor.SetText("new-name")
		send(pressed(key.NameReturn))
		if len(v.loader.requests) != 1 {
			t.Fatal("name mutation was not submitted", kind)
		}
		frame, owns := v.panels[0].feedback.Frame(time.Now(), v.appearance.Feedback)
		if owns || frame.Next.IsZero() {
			t.Fatal("name mutation did not delay progress", kind)
		}
		frame, owns = v.panels[0].feedback.Frame(frame.Next, v.appearance.Feedback)
		if !owns || frame.Started || frame.Text == "" || !frame.Next.IsZero() {
			t.Fatal("name mutation progress animated or announced itself", kind, frame)
		}
	}
}
