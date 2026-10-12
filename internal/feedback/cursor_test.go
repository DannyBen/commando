package feedback

import (
	"testing"
	"time"
)

func TestDrawingCursorBlinksIndependentlyThenDisappears(t *testing.T) {
	start := time.Unix(1000, 0)
	animation := Animation{Enabled: true, CharacterDelay: 35 * time.Millisecond,
		Cursor: "█", CursorBlinkInterval: 350 * time.Millisecond, CursorBlinks: 2,
		BlinkInterval: 160 * time.Millisecond}
	m := &Message{Text: "He"}
	for _, tc := range []struct {
		ms     int
		text   string
		nextMS int
	}{
		{0, "█", 35}, {35, "H█", 70}, {70, "He█", 420},
		{230, "He█", 420}, {419, "He█", 420}, {420, "He", 770},
		{550, "He", 770}, {769, "He", 770}, {770, "He█", 1120}, {1120, "He", 1470},
		{1470, "He", 0}, {10000, "He", 0},
	} {
		value, next := m.frame(start.Add(time.Duration(tc.ms)*time.Millisecond), animation)
		if value != tc.text {
			t.Fatalf("at %d ms: %q, want %q", tc.ms, value, tc.text)
		}
		if tc.nextMS == 0 && !next.IsZero() || tc.nextMS != 0 && !next.Equal(start.Add(time.Duration(tc.nextMS)*time.Millisecond)) {
			t.Fatalf("at %d ms: next %v, want %d ms", tc.ms, next, tc.nextMS)
		}
	}
	value, next := m.frame(start, Animation{})
	if value != "He" || !next.IsZero() {
		t.Fatal("disabled animation left a cursor")
	}
}

func TestMessageStartsOnceRegardlessOfAnimation(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		var c Channel
		start := time.Unix(1000, 0)
		profiles := Animations{Enabled: enabled, Info: Animation{Enabled: true, CharacterDelay: 15 * time.Millisecond}}
		c.Show(Info, "Å文Z")
		frame, _ := c.Frame(start, profiles)
		if !frame.Started {
			t.Fatal("first frame did not announce the message")
		}
		for _, delay := range []time.Duration{0, 30 * time.Millisecond, time.Second, time.Hour} {
			frame, _ = c.Frame(start.Add(delay), profiles)
			if frame.Started {
				t.Fatal("redraw, typing or held message repeated its start")
			}
		}
		c.Show(Info, "Å文Z")
		frame, _ = c.Frame(start.Add(time.Hour), profiles)
		if !frame.Started {
			t.Fatal("replacement did not announce a new message")
		}
		c.Dismiss()
		if _, owns := c.Frame(start.Add(2*time.Hour), profiles); owns {
			t.Fatal("dismissed message still owns presentation")
		}
	}
}

func TestCursorKeepsBlinkingUntilDismissed(t *testing.T) {
	var c Channel
	start := time.Unix(1000, 0)
	profiles := Animations{Enabled: true, Info: Animation{Enabled: true,
		CharacterDelay: 15 * time.Millisecond, Cursor: "█",
		CursorBlinkInterval: 350 * time.Millisecond, CursorBlinks: -1}}
	c.Show(Info, "Å文")
	for _, tc := range []struct {
		delay time.Duration
		text  string
		next  time.Duration
	}{
		{0, "█", 15 * time.Millisecond},
		{15 * time.Millisecond, "Å█", 30 * time.Millisecond},
		{30 * time.Millisecond, "Å文█", 380 * time.Millisecond},
		{380 * time.Millisecond, "Å文", 730 * time.Millisecond},
		{730 * time.Millisecond, "Å文█", 1080 * time.Millisecond},
		{30*time.Millisecond + 10000*350*time.Millisecond, "Å文█", 30*time.Millisecond + 10001*350*time.Millisecond},
		{30*time.Millisecond + 10001*350*time.Millisecond, "Å文", 30*time.Millisecond + 10002*350*time.Millisecond},
	} {
		frame, owns := c.Frame(start.Add(tc.delay), profiles)
		if !owns || frame.Text != tc.text || !frame.Next.Equal(start.Add(tc.next)) {
			t.Fatalf("at %v: %+v, owns %v; want %q, next %v", tc.delay, frame, owns, tc.text, tc.next)
		}
	}
	c.Dismiss()
	frame, owns := c.Frame(start.Add(2*time.Hour), profiles)
	if owns || !frame.Next.IsZero() || frame.Started {
		t.Fatal("dismissed message kept blinking or announced itself")
	}
	c.Show(Info, "New")
	frame, owns = c.Frame(start.Add(2*time.Hour), profiles)
	if !owns || !frame.Started || frame.Text != "█" || !frame.Next.Equal(start.Add(2*time.Hour+15*time.Millisecond)) {
		t.Fatal("new message did not restart typing and cursor timing")
	}
}

func TestAcknowledgedFeedbackKeepsAnimationWithoutAnnouncingAgain(t *testing.T) {
	var c Channel
	start := time.Unix(1000, 0)
	profiles := Animations{Enabled: true, Info: Animation{Enabled: true,
		CharacterDelay: 15 * time.Millisecond, Cursor: "█"}}
	c.Show(Info, "Cancelled")
	c.Acknowledge()
	frame, owns := c.Frame(start, profiles)
	if !owns || frame.Text != "█" || frame.Started || frame.Next.IsZero() {
		t.Fatal("acknowledgement suppressed animation or repeated notification")
	}
	frame, _ = c.Frame(start.Add(15*time.Millisecond), profiles)
	if frame.Text != "C█" || frame.Started {
		t.Fatal("acknowledged message did not keep normal typing")
	}
	c.Show(Info, "Next")
	frame, _ = c.Frame(start.Add(time.Hour), profiles)
	if !frame.Started {
		t.Fatal("acknowledgement silenced a subsequent message")
	}
	c.ProgressAt("Reading folder", start.Add(2*time.Hour))
	c.Acknowledge() // Pending feedback has not been announced yet.
	frame, _ = c.Frame(start.Add(2*time.Hour), profiles)
	if !frame.Started {
		t.Fatal("acknowledgement silenced pending progress")
	}
}
