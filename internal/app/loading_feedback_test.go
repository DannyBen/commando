package app

import (
	"errors"
	"image"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/sound"
)

// Hold the real directory worker until the test explicitly completes the read.
// Presentation time advances independently, without sleeps or filesystem caches.
func pendingDirectoryRead(t *testing.T, v *view) func() {
	t.Helper()
	release := make(chan struct{})
	entered := make(chan struct{})
	v.loader = newDirectoryLoader(func(path string) (browser.Directory, error) {
		close(entered)
		<-release
		return browser.Directory{Path: path, Entries: []browser.Entry{{Name: "result.txt"}}}, nil
	}, nil)
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		v.finishOperations()
	})
	return func() {
		t.Helper()
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("directory worker did not begin the read")
		}
		close(release)
		waitLoaded(t, v, 0)
	}
}

func drawLoadingFrame(v *view, now time.Time) {
	var ops op.Ops
	v.layout(layout.Context{Ops: &ops, Now: now,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(800, 480))})
}

func TestDirectoryReadProgressIsDelayedAndUsesNormalInfoFeedback(t *testing.T) {
	for _, delay := range []time.Duration{500 * time.Millisecond, 2 * time.Second} {
		t.Run(delay.String(), func(t *testing.T) {
			v := newPreviewView()
			v.appearance.FeedbackProgressDelay = delay
			recorded := &recordedSounds{}
			v.sounds = recorded
			finish := pendingDirectoryRead(t, v)
			before := time.Now()
			v.load(0, "next-folder", "")
			v.load(0, "ignored-while-loading", "")
			frame, owns := v.panels[0].feedback.Frame(before, v.appearance.Feedback)
			due := frame.Next
			if owns || due.Before(before.Add(delay)) || due.After(time.Now().Add(delay)) {
				t.Fatal("directory progress ignored the configured delay")
			}
			drawLoadingFrame(v, due.Add(-time.Nanosecond))
			if v.panels[0].feedback.Current() != nil || len(recorded.played) != 1 || recorded.played[0] != sound.DirectoryNavigation {
				t.Fatal("pending read displayed progress or lost its immediate directory cue")
			}
			drawLoadingFrame(v, due)
			message := v.panels[0].feedback.Current()
			if message == nil || message.Kind != feedback.Info || message.Text != "Reading folder" || len(recorded.played) != 2 || recorded.played[1] != sound.MessageInfo {
				t.Fatal("slow read did not start normal Info feedback and sound")
			}
			frame, _ = v.panels[0].feedback.Frame(due, v.appearance.Feedback)
			if frame.Text != "█" || frame.Started {
				t.Fatal("slow read skipped its initial cursor or repeated its cue")
			}
			drawLoadingFrame(v, due.Add(15*time.Millisecond))
			frame, _ = v.panels[0].feedback.Frame(due.Add(15*time.Millisecond), v.appearance.Feedback)
			if frame.Text != "R█" || len(recorded.played) != 2 {
				t.Fatal("slow-read animation restarted or replayed sound")
			}
			finish()
			drawLoadingFrame(v, due.Add(time.Hour))
			if v.panels[0].feedback.Current() != nil || len(recorded.played) != 2 || v.panels[0].items[0].name != "result.txt" {
				t.Fatal("completion did not clear progress and install the listing")
			}
		})
	}
}

func TestFastReadDismissalAndReplacementCancelDelayedProgress(t *testing.T) {
	for _, action := range []string{"finish", "dismiss", "replace", "fail"} {
		t.Run(action, func(t *testing.T) {
			v := newPreviewView()
			recorded := &recordedSounds{}
			v.sounds = recorded
			finish := pendingDirectoryRead(t, v)
			v.load(0, "next-folder", "")
			frame, _ := v.panels[0].feedback.Frame(time.Time{}, v.appearance.Feedback)
			due := frame.Next
			switch action {
			case "finish":
				finish()
			case "dismiss":
				v.dismissFeedback()
			case "replace":
				v.showFeedback(0, feedback.Success, "Renamed")
			case "fail":
				v.acceptLoad(loadResult{request: loadRequest{panel: 0}, err: errors.New("read failed")})
			}
			drawLoadingFrame(v, due.Add(time.Second))
			if message := v.panels[0].feedback.Current(); message != nil && message.Text == "Reading folder" {
				t.Fatal("cancelled progress appeared after its deadline")
			}
			for _, cue := range recorded.played {
				if cue == sound.MessageInfo {
					t.Fatal("cancelled progress played an Info sound")
				}
			}
		})
	}
}

func TestRefreshDelaysProgressWithoutDirectoryNavigationCue(t *testing.T) {
	v := newPreviewView()
	recorded := &recordedSounds{}
	v.sounds = recorded
	finish := pendingDirectoryRead(t, v)
	v.refreshPanel()
	frame, _ := v.panels[0].feedback.Frame(time.Time{}, v.appearance.Feedback)
	drawLoadingFrame(v, frame.Next.Add(-time.Nanosecond))
	if len(recorded.played) != 0 || v.panels[0].feedback.Current() != nil {
		t.Fatal("manual refresh played a directory cue or displayed early progress")
	}
	drawLoadingFrame(v, frame.Next)
	if len(recorded.played) != 1 || recorded.played[0] != sound.MessageInfo {
		t.Fatal("slow refresh did not play the delayed Info cue")
	}
	finish()
}

func TestRejectedDirectoryReadDoesNotScheduleFeedbackOrPlaySound(t *testing.T) {
	v := newPreviewView()
	v.loader = newDirectoryLoader(browser.ReadLocation, nil)
	v.loader.close()
	<-v.loader.stopped
	recorded := &recordedSounds{}
	v.sounds = recorded
	v.load(0, "next-folder", "")
	frame, owns := v.panels[0].feedback.Frame(time.Now().Add(time.Hour), v.appearance.Feedback)
	if v.panels[0].loading || owns || !frame.Next.IsZero() || len(recorded.played) != 0 {
		t.Fatal("rejected read made the panel busy, scheduled progress or played a cue")
	}
}
