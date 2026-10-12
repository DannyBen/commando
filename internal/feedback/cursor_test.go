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

func TestPresentationEventsDoNotRepeatOrCatchUpInBursts(t *testing.T) {
	var c Channel
	start := time.Unix(1000, 0)
	profiles := Animations{Enabled: true, Info: Animation{Enabled: true, CharacterDelay: 35 * time.Millisecond}}
	c.Show(Info, "Å文Z")
	frame, _ := c.Frame(start, profiles)
	if frame.Typed || frame.Finished {
		t.Fatal("initial cursor frame produced a typing event")
	}
	frame, _ = c.Frame(start.Add(70*time.Millisecond), profiles)
	if !frame.Typed || frame.Finished || frame.Text != "Å文" {
		t.Fatal("delayed frame did not report one typing event for revealed Unicode text")
	}
	frame, _ = c.Frame(start.Add(70*time.Millisecond), profiles)
	if frame.Typed || frame.Finished {
		t.Fatal("redraw replayed typing")
	}
	frame, _ = c.Frame(start.Add(time.Second), profiles)
	if !frame.Typed || !frame.Finished {
		t.Fatal("last character did not finish the message")
	}
	frame, _ = c.Frame(start.Add(time.Hour), profiles)
	if frame.Typed || frame.Finished {
		t.Fatal("held message replayed audio events")
	}
	c.Show(Info, "Å文Z")
	c.Frame(start.Add(time.Hour), profiles)
	frame, _ = c.Frame(start.Add(time.Hour+105*time.Millisecond), profiles)
	if !frame.Typed || !frame.Finished {
		t.Fatal("replacement did not restart events")
	}
	c.Dismiss()
	if _, owns := c.Frame(start.Add(2*time.Hour), profiles); owns {
		t.Fatal("dismissed message still owns presentation")
	}
}
