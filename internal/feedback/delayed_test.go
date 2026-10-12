package feedback

import (
	"testing"
	"time"
)

func TestDelayedProgressYieldsAreaThenStartsNormalAnimation(t *testing.T) {
	var c Channel
	start := time.Unix(1000, 0)
	due := start.Add(500 * time.Millisecond)
	profiles := Animations{Enabled: true, Info: Animation{Enabled: true,
		CharacterDelay: 15 * time.Millisecond, Cursor: "█",
		CursorBlinkInterval: 350 * time.Millisecond, CursorBlinks: -1}}
	c.ProgressAt("Reading folder", due)
	for _, now := range []time.Time{start, due.Add(-time.Nanosecond)} {
		frame, owns := c.Frame(now, profiles)
		if owns || c.Current() != nil || frame.Started || frame.Text != "" || frame.Next != due {
			t.Fatalf("pending message took its area or lost its deadline: %+v, owns %v", frame, owns)
		}
	}
	frame, owns := c.Frame(due, profiles)
	if !owns || frame.Text != "█" || !frame.Started || frame.Kind != Info || c.Current().Text != "Reading folder" {
		t.Fatal("deadline did not start a normal Info message")
	}
	frame, _ = c.Frame(due.Add(15*time.Millisecond), profiles)
	if frame.Text != "R█" || frame.Started {
		t.Fatal("delay skipped typing or repeated the start event")
	}
	c.Complete()
	frame, owns = c.Frame(due.Add(time.Hour), profiles)
	if owns || c.Current() != nil || !frame.Next.IsZero() {
		t.Fatal("completed visible progress kept running")
	}
}

func TestPendingMessagesAreCancelledByCompletionDismissalOrReplacement(t *testing.T) {
	for _, action := range []string{"complete", "dismiss", "replace"} {
		t.Run(action, func(t *testing.T) {
			var c Channel
			due := time.Unix(1000, 0)
			c.ProgressAt("Reading folder", due)
			switch action {
			case "complete":
				c.Complete()
			case "dismiss":
				c.Dismiss()
			case "replace":
				c.Show(Error, "Access denied")
				c.Complete()
			}
			frame, owns := c.Frame(due.Add(time.Hour), Animations{})
			if action == "replace" {
				if !owns || frame.Text != "Access denied" || frame.Kind != Error {
					t.Fatal("late progress replaced the result message")
				}
			} else if owns || frame.Started || !frame.Next.IsZero() || c.Current() != nil {
				t.Fatal("cancelled progress reappeared")
			}
		})
	}
}

func TestScheduledResultSurvivesCompletionAndStartsWhenPresented(t *testing.T) {
	var c Channel
	start := time.Unix(1000, 0)
	profiles := Animations{Enabled: true, Warning: Animation{Enabled: true,
		CharacterDelay: 15 * time.Millisecond, Cursor: "█"}}
	c.ShowAt(Warning, "First", start)
	c.ShowAt(Warning, "Second", start.Add(time.Second))
	c.Complete() // Only progress is conditional on operation completion.
	frame, owns := c.Frame(start, profiles)
	if owns || frame.Next != start.Add(time.Second) {
		t.Fatal("replacement retained the old deadline or completion erased a result")
	}
	frame, owns = c.Frame(start.Add(time.Hour), profiles)
	if !owns || frame.Text != "█" || !frame.Started || c.Current().Text != "Second" {
		t.Fatal("late presentation skipped animation or displayed the replaced message")
	}
	frame, _ = c.Frame(start.Add(time.Hour), profiles)
	if frame.Started {
		t.Fatal("redraw repeated the delayed message's start event")
	}
}
