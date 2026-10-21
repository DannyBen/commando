package feedback

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestFrameDistinguishesProgressFromNotifications(t *testing.T) {
	var c Channel
	due := time.Unix(1000, 0)
	c.ProgressAt("Reading folder", due)
	if frame, owns := c.Frame(due.Add(-time.Nanosecond), Animations{}); owns || frame.Progress {
		t.Fatal("pending progress claimed presentation before its deadline")
	}
	if frame, owns := c.Frame(due, Animations{}); !owns || !frame.Progress || !frame.Started {
		t.Fatal("visible progress did not identify itself")
	}
	c.ShowWithStyle(Info, "Ready", Immediate)
	if frame, owns := c.Frame(due, Animations{}); !owns || frame.Progress || !frame.Started {
		t.Fatal("ordinary immediate notification was mistaken for progress")
	}
	c.Complete()
	if c.Current() == nil {
		t.Fatal("progress completion erased a result notification")
	}
}

func TestImmediateStylePreservesAnnouncementAndLifecycle(t *testing.T) {
	var c Channel
	start := time.Unix(1000, 0)
	animations := Animations{Enabled: true, Info: Animation{
		Enabled: true, CharacterDelay: time.Second, Blinks: 2, BlinkInterval: time.Second,
		Cursor: "█", CursorBlinks: -1, CursorBlinkInterval: time.Second,
	}}
	c.ShowWithStyle(Info, "Ready", Immediate)
	for i := range 4 {
		frame, owns := c.Frame(start.Add(time.Duration(i)*time.Second), animations)
		if !owns || frame.Text != "Ready" || frame.Kind != Info || frame.Started != (i == 0) || !frame.Next.IsZero() {
			t.Fatalf("immediate presentation changed announcement or animated: %+v", frame)
		}
	}
	c.Show(Info, "Next")
	frame, _ := c.Frame(start, animations)
	if frame.Text != "█" || !frame.Started || frame.Next.IsZero() {
		t.Fatal("ordinary replacement lost its normal animation")
	}
	c.ShowWithStyle(Info, "Ready", Immediate)
	c.Acknowledge()
	if frame, _ := c.Frame(start, animations); frame.Started {
		t.Fatal("acknowledged immediate message announced again")
	}
	c.Dismiss()
	if _, owns := c.Frame(start, animations); owns {
		t.Fatal("immediate message did not dismiss")
	}
}

func TestFeedbackTypingAndTwoBlinksHoldIndefinitely(t *testing.T) {
	start := time.Unix(1000, 0)
	animation := Animation{Enabled: true, CharacterDelay: 10 * time.Millisecond, BlinkInterval: 20 * time.Millisecond, Blinks: 2}
	m := &Message{Text: "Å文Z"}
	for _, tc := range []struct {
		elapsed time.Duration
		want    string
		next    time.Duration
	}{
		{0, "", 10 * time.Millisecond}, {9 * time.Millisecond, "", 10 * time.Millisecond},
		{10 * time.Millisecond, "Å", 20 * time.Millisecond}, {20 * time.Millisecond, "Å文", 30 * time.Millisecond},
		{30 * time.Millisecond, "Å文Z", 50 * time.Millisecond}, {50 * time.Millisecond, "", 70 * time.Millisecond},
		{70 * time.Millisecond, "Å文Z", 90 * time.Millisecond}, {90 * time.Millisecond, "", 110 * time.Millisecond},
		{110 * time.Millisecond, "Å文Z", 0}, {24 * time.Hour, "Å文Z", 0},
	} {
		got, next := m.frame(start.Add(tc.elapsed), animation)
		if got != tc.want || !utf8.ValidString(got) {
			t.Fatalf("at %v got %q want %q", tc.elapsed, got, tc.want)
		}
		if tc.next == 0 {
			if !next.IsZero() {
				t.Fatalf("steady message still schedules frames: %v", next)
			}
		} else if !next.Equal(start.Add(tc.next)) {
			t.Fatalf("at %v deadline %v want %v", tc.elapsed, next, start.Add(tc.next))
		}
	}
}

func TestFeedbackLongMessagesKeepFixedTypingCadence(t *testing.T) {
	start := time.Unix(1000, 0)
	animation := Animation{Enabled: true, CharacterDelay: 35 * time.Millisecond}
	text := strings.Repeat("x", 200)
	m := &Message{Text: text}
	m.frame(start, animation)
	value, next := m.frame(start.Add(time.Second), animation)
	if len(value) != 28 || !next.Equal(start.Add(29*35*time.Millisecond)) {
		t.Fatalf("long message was accelerated: length=%d next=%v", len(value), next)
	}
	value, next = m.frame(start.Add(7*time.Second), animation)
	if value != text || !next.IsZero() {
		t.Fatal("long message did not finish typing at its fixed duration")
	}
	value, next = m.frame(start, Animation{})
	if value != text || !next.IsZero() {
		t.Fatal("disabled animation did not display full text immediately")
	}
}

func TestChannelLifecycle(t *testing.T) {
	var c Channel
	if _, owns := c.Frame(time.Unix(1000, 0), Animations{}); owns {
		t.Fatal("empty channel owns the footer")
	}
	c.Progress("Reading folder")
	c.Complete()
	if c.Current() != nil {
		t.Fatal("completed progress remains")
	}
	c.Progress("Renaming")
	c.Show(Error, "Name already exists")
	c.Complete()
	if m := c.Current(); m == nil || m.Kind != Error || m.Text != "Name already exists" {
		t.Fatal("completion erased the result")
	}
	c.Dismiss()
	if c.Current() != nil {
		t.Fatal("dismissed result remains")
	}
}

func TestKindsChooseIndependentAnimationProfiles(t *testing.T) {
	start := time.Unix(1000, 0)
	profiles := Animations{
		Enabled: true,
		Info:    Animation{Enabled: true, CharacterDelay: 10 * time.Millisecond},
		Success: Animation{}, // Immediate, steady success messages.
		Warning: Animation{Enabled: true, CharacterDelay: 20 * time.Millisecond},
		Error:   Animation{Enabled: true, BlinkInterval: 10 * time.Millisecond, Blinks: 1},
	}
	for _, tc := range []struct {
		kind Kind
		want string
	}{
		{Info, "ABC"}, {Success, "ABC"}, {Warning, "A"}, {Error, "ABC"},
	} {
		var c Channel
		c.Show(tc.kind, "ABC")
		c.Frame(start, profiles)
		got, owns := c.Frame(start.Add(30*time.Millisecond), profiles)
		if !owns || got.Text != tc.want || got.Kind != tc.kind {
			t.Fatalf("kind %v used wrong profile: %+v", tc.kind, got)
		}
	}
	var c Channel
	c.Show(Error, "ABC")
	c.Frame(start, profiles)
	if got, owns := c.Frame(start.Add(10*time.Millisecond), profiles); !owns || got.Text != "" || got.Next.IsZero() {
		t.Fatal("blink phase lost footer ownership or scheduling")
	}
	profiles.Enabled = false
	if got, owns := c.Frame(start.Add(10*time.Millisecond), profiles); !owns || got.Text != "ABC" || !got.Next.IsZero() {
		t.Fatal("master switch did not disable the kind's animation")
	}
	c.Show(Info, "XYZ")
	if got, _ := c.Frame(start.Add(time.Hour), Animations{Enabled: true, Info: profiles.Info}); got.Text != "" {
		t.Fatal("replacing a message did not restart its animation")
	}
}
