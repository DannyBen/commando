// Package feedback owns contextual messages and their animation, independently
// of the window toolkit, footer layout, colors and input routing.
package feedback

import (
	"strings"
	"time"
)

type Kind uint8

const (
	Info Kind = iota
	Success
	Warning
	Error
)

// Style controls message presentation independently of its semantic kind.
type Style uint8

const (
	Animated  Style = iota
	Immediate       // Full, steady text without typing, pulses or cursor animation.
)

// Animation controls presentation only; messages never expire on time.
// Zero character delay skips typing; zero Blinks skips blinking.
// Negative CursorBlinks keeps the cursor blinking until dismissal.
type Animation struct {
	Enabled                       bool
	CharacterDelay, BlinkInterval time.Duration
	Blinks                        int
	Cursor                        string
	CursorBlinkInterval           time.Duration
	CursorBlinks                  int
}

// Animations selects presentation by meaning. Enabled is the master switch;
// each profile can also be disabled independently for immediate, steady text.
type Animations struct {
	Enabled                       bool
	Info, Success, Warning, Error Animation
}

func (a Animations) profile(kind Kind) Animation {
	if !a.Enabled {
		return Animation{}
	}
	switch kind {
	case Success:
		return a.Success
	case Warning:
		return a.Warning
	case Error:
		return a.Error
	default:
		return a.Info
	}
}

// Message is owned by its channel. Callers should treat Current's result as
// read-only and use Show or Progress to replace it and restart animation.
type Message struct {
	Text      string
	Kind      Kind
	Style     Style
	started   time.Time
	progress  bool
	presented bool
}

// Channel holds one contextual message. Its zero value is ready to use.
// A new message replaces the previous one; there is no queue or timer expiry.
// Use it on the UI thread; it is not safe for concurrent calls.
type Channel struct {
	message, pending *Message
	notBefore        time.Time
}

func (c *Channel) Show(kind Kind, text string) {
	c.ShowWithStyle(kind, text, Animated)
}

// ShowWithStyle changes presentation while retaining normal announcement,
// replacement and dismissal. Immediate messages still announce their kind.
func (c *Channel) ShowWithStyle(kind Kind, text string, style Style) {
	c.Dismiss()
	c.message = &Message{Text: text, Kind: kind, Style: style}
}

// ShowAt replaces the current message with one scheduled for notBefore.
// Until then the channel yields its display area and reports a redraw deadline.
// Dismiss or another message cancels the pending presentation.
func (c *Channel) ShowAt(kind Kind, text string, notBefore time.Time) {
	c.Dismiss()
	c.pending = &Message{Text: text, Kind: kind}
	c.notBefore = notBefore
}

func (c *Channel) Progress(text string) {
	c.Show(Info, text)
	c.message.progress = true
}

// ProgressAt shows delayed Info feedback only while the operation is pending.
// Complete before its deadline cancels it without animation or a Started event.
func (c *Channel) ProgressAt(text string, notBefore time.Time) {
	c.ShowAt(Info, text, notBefore)
	c.pending.progress = true
}

// Complete clears visible or pending progress without erasing a result message.
func (c *Channel) Complete() {
	if c.message != nil && c.message.progress || c.pending != nil && c.pending.progress {
		c.Dismiss()
	}
}

func (c *Channel) Dismiss() { *c = Channel{} }

// Current returns only visible feedback, never a scheduled message.
func (c *Channel) Current() *Message { return c.message }

// Acknowledge marks visible feedback as already announced by its caller.
// It suppresses the Started notification while retaining normal animation.
func (c *Channel) Acknowledge() {
	if c.message != nil {
		c.message.presented = true
	}
}

type Frame struct {
	Text    string
	Kind    Kind
	Next    time.Time // Zero means the presentation is steady; no redraw is needed.
	Started bool      // Newly announced feedback, once per message unless acknowledged.
}

// Frame reports whether the channel owns the footer, even during an empty
// typing/blink frame. The caller schedules the next redraw when Next is set.
func (c *Channel) Frame(now time.Time, animations Animations) (Frame, bool) {
	if c.pending != nil {
		if now.Before(c.notBefore) {
			return Frame{Next: c.notBefore}, false
		}
		c.message, c.pending = c.pending, nil
		c.notBefore = time.Time{}
	}
	if c.message == nil {
		return Frame{}, false
	}
	m := c.message
	animation := animations.profile(m.Kind)
	if m.Style == Immediate {
		animation = Animation{}
	}
	text, next := m.frame(now, animation)
	frame := Frame{Text: text, Kind: m.Kind, Next: next, Started: !m.presented}
	m.presented = true
	return frame, true
}

func (m *Message) frame(now time.Time, animation Animation) (value string, next time.Time) {
	if m.started.IsZero() {
		m.started = now
	}
	if !animation.Enabled {
		return m.Text, time.Time{}
	}
	elapsed := max(time.Duration(0), now.Sub(m.started))
	characters := []rune(m.Text)
	typingDuration := time.Duration(0)
	if animation.CharacterDelay > 0 {
		typingDuration = time.Duration(len(characters)) * animation.CharacterDelay
		if elapsed < typingDuration {
			count := int(elapsed / animation.CharacterDelay)
			return string(characters[:count]) + animation.Cursor, m.started.Add(time.Duration(count+1) * animation.CharacterDelay)
		}
	}
	value = m.Text
	if animation.Blinks > 0 && animation.BlinkInterval > 0 {
		phase := (elapsed - typingDuration) / animation.BlinkInterval
		if phase < time.Duration(animation.Blinks)*2 {
			if phase%2 != 0 {
				value = ""
			}
			next = m.started.Add(typingDuration + (phase+1)*animation.BlinkInterval)
		}
	}
	if animation.Cursor != "" && animation.CursorBlinks != 0 && animation.CursorBlinkInterval > 0 {
		phase := (elapsed - typingDuration) / animation.CursorBlinkInterval
		if animation.CursorBlinks < 0 || phase < time.Duration(animation.CursorBlinks)*2 {
			if phase%2 == 0 {
				// Keep the cursor at the text's end during the existing text pulse.
				if value == "" {
					value = strings.Repeat(" ", len(characters))
				}
				value += animation.Cursor
			}
			cursorNext := m.started.Add(typingDuration + (phase+1)*animation.CursorBlinkInterval)
			if next.IsZero() || cursorNext.Before(next) {
				next = cursorNext
			}
		}
	}
	return value, next
}
