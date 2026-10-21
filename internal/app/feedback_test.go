package app

import (
	"errors"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/sound"
)

type recordedSounds struct {
	played  []sound.Name
	volumes []float64
	stops   int
}

func TestBackgroundFeedbackFollowsPanelWithoutLeakingIntoClone(t *testing.T) {
	v := selectionView()
	post := v.backgroundFeedback(0)
	v.insertPanel(0, v.panels[0].clone(), "", "")
	post(feedback.Error, "Failed to open")
	v.pollFeedback()
	if v.panels[0].feedback.Current() != nil {
		t.Fatal("cloned panel received original panel's background feedback")
	}
	message := v.panels[1].feedback.Current()
	if message == nil || message.Text != "Failed to open" || message.Kind != feedback.Error {
		t.Fatalf("moved panel feedback = %+v", message)
	}
}

func (s *recordedSounds) Play(name sound.Name, volume float64) {
	s.played = append(s.played, name)
	s.volumes = append(s.volumes, volume)
}
func (s *recordedSounds) Stop() { s.stops++ }

func TestMessageSoundsStartImmediatelyAndDoNotRepeat(t *testing.T) {
	for _, tc := range []struct {
		kind feedback.Kind
		cue  sound.Name
	}{
		{feedback.Info, sound.MessageInfo},
		{feedback.Success, sound.MessageSuccess},
		{feedback.Warning, sound.MessageWarning},
		{feedback.Error, sound.MessageError},
	} {
		v := newPreviewView()
		recorded := &recordedSounds{}
		v.sounds = recorded
		send, advance := panelInputFrames(v)
		v.showFeedback(0, tc.kind, "Å文Z")
		send()
		if len(recorded.played) != 1 || recorded.played[0] != tc.cue {
			t.Fatalf("initial cue for kind %v: %v", tc.kind, recorded.played)
		}
		send() // A redraw at the same time must not replay the cue.
		advance(15 * time.Millisecond)
		advance(30 * time.Millisecond)
		advance(350 * time.Millisecond)
		advance(350 * time.Millisecond)
		advance(time.Hour)
		if len(recorded.played) != 1 {
			t.Fatal("typing, completion or cursor blink replayed sound")
		}
		send(pressed(key.NameRightArrow))
		if recorded.stops == 0 || v.active != 1 || v.panels[0].feedback.Current() != nil {
			t.Fatal("dismissal did not stop audio and perform normal navigation")
		}
		if len(recorded.played) != 2 || recorded.played[1] != sound.Navigation {
			t.Fatal("navigation did not replace the dismissed message sound")
		}
		v.showFeedback(1, tc.kind, "ABCDE")
		send()
		if len(recorded.played) != 3 || recorded.played[2] != tc.cue {
			t.Fatal("new message did not play its initial cue")
		}
		advance(15 * time.Millisecond)
		send(pressed(key.NameF1))
		count := len(recorded.played)
		advance(time.Hour)
		if len(recorded.played) != count {
			t.Fatal("dismissed typing completed in the background")
		}
	}
}

func TestFeedbackNavigationDismissesWithoutConsumingInput(t *testing.T) {
	v := newPreviewView()
	send, advance := panelInputFrames(v)
	p := &v.panels[0]
	selected, position := p.selected, p.list.Position
	v.showFeedback(0, feedback.Success, "Renamed")
	send()
	advance(time.Hour)
	if p.feedback.Current() == nil || v.active != 0 || p.selected != selected || p.list.Position != position || len(p.marks) != 3 {
		t.Fatal("feedback expired or changed panel state")
	}
	send(pressed(key.NameRightArrow))
	if p.feedback.Current() != nil || v.active != 1 {
		t.Fatal("dismissal consumed panel navigation")
	}
}

func TestFeedbackPreviewShortcutsRemoved(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	for _, e := range []key.Event{pressed(key.NameF12), modified(key.NameF12, key.ModShift)} {
		send(e)
		if v.panels[0].feedback.Current() != nil {
			t.Fatal("removed shortcut still emits a preview message")
		}
	}
}

func TestFeedbackKeysStayInEditors(t *testing.T) {
	for _, name := range []string{"location", "rename", "create"} {
		t.Run(name, func(t *testing.T) {
			v := newPreviewView()
			if name == "rename" {
				v.panels[0].clearMarks()
			}
			v.nativeKeys = &nativeKeyInput{invalidate: func() {}}
			send, advance := panelInputFrames(v)
			if name == "location" {
				send(modified("D", key.ModAlt))
			} else if name == "rename" {
				send(pressed(key.NameF2))
			} else {
				send(newFolderKey())
			}
			editor := v.activeEditor()
			before := editor.Text()
			first, last := editor.Selection()
			selected := v.panels[0].selected
			v.showFeedback(v.active, feedback.Info, "Reading folder")
			send()
			gotFirst, gotLast := editor.Selection()
			if v.activeEditor() != editor || editor.Text() != before || gotFirst != first || gotLast != last || v.panels[0].selected != selected || v.panels[0].feedback.Current() == nil {
				t.Fatal("feedback changed editor or selection")
			}
			advance(time.Second)
			send(key.EditEvent{Range: key.Range{Start: 0, End: utf8.RuneCountInString(before)}, Text: "draft"})
			if v.panels[0].feedback.Current() != nil || editor.Text() != "draft" {
				t.Fatal("dismissal consumed text input")
			}
			v.showFeedback(v.active, feedback.Info, "Reading folder")
			send()
			// A native press notice must not consume the actual editor Ctrl+C.
			v.nativeKeys.noticeInteraction()
			send(modified("C", key.ModCtrl))
			if v.panels[0].feedback.Current() != nil || v.activeEditor() != editor || editor.Text() != "draft" {
				t.Fatal("editor command did not dismiss safely")
			}
			v.showFeedback(v.active, feedback.Info, "Reading folder")
			send()
			send(pressed(key.NameEscape))
			if v.panels[0].feedback.Current() != nil || v.activeEditor() != nil {
				t.Fatal("Escape did not both dismiss and cancel")
			}
		})
	}
}

func TestFeedbackClickObserverDoesNotConsumeClick(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	v.showFeedback(v.active, feedback.Info, "Reading folder")
	send()
	position := f32.Pt(330, 110)
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: position})
	send(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: position})
	if v.panels[0].feedback.Current() != nil || v.active != 1 || v.panels[1].selected != 1 {
		t.Fatal("click was swallowed by feedback")
	}
	v.showFeedback(v.active, feedback.Info, "Reading folder")
	send()
	send(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(340, 120)})
	if v.panels[1].feedback.Current() == nil {
		t.Fatal("mere pointer movement dismissed feedback")
	}
	send(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(340, 465)})
	if v.panels[1].feedback.Current() != nil {
		t.Fatal("click on footer did not dismiss")
	}
}

func TestFeedbackUnknownKeyDismissesButReleaseDoesNot(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	v.showFeedback(v.active, feedback.Info, "Reading folder")
	send()
	send(key.Event{Name: key.NameF1, State: key.Release})
	if v.panels[0].feedback.Current() == nil {
		t.Fatal("release dismissed feedback")
	}
	send(pressed(key.NameF1))
	if v.panels[0].feedback.Current() != nil {
		t.Fatal("unused key did not dismiss")
	}
}

func TestReadFeedbackDismissalDoesNotReappearFromStoredError(t *testing.T) {
	v := newPreviewView()
	send, advance := panelInputFrames(v)
	p := &v.panels[0]
	v.acceptLoad(loadResult{request: loadRequest{panel: 0}, err: errors.New("read failed")})
	send()
	if p.feedback.Current() == nil || p.feedback.Current().Kind != feedback.Error {
		t.Fatal("read failure did not emit feedback")
	}
	advance(time.Hour)
	if p.feedback.Current() == nil {
		t.Fatal("error expired without interaction")
	}
	send(pressed(key.NameF1))
	if p.feedback.Current() != nil || p.err == nil {
		t.Fatal("dismissal changed read result or failed to dismiss")
	}
	if status, failed := p.status(); failed || status != "" {
		t.Fatalf("normal footer did not resume: %q %v", status, failed)
	}
	advance(time.Hour)
	if p.feedback.Current() != nil {
		t.Fatal("stored read error resurrected dismissed message")
	}
}

func TestCompletedReadDoesNotEraseResultFeedback(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	v.showFeedback(v.active, feedback.Info, "Reading folder")
	send()
	message := v.panels[0].feedback.Current()
	v.acceptLoad(loadResult{request: loadRequest{panel: 0}})
	if v.panels[0].feedback.Current() != message {
		t.Fatal("completion erased an unrelated result")
	}
}
