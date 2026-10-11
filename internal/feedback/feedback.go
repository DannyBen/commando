// Package feedback owns contextual messages and their animation, independently
// of the window toolkit, footer layout, colors and input routing.
package feedback

import "time"

type Kind uint8

const (
	Info Kind = iota
	Success
	Warning
	Error
)

// Animation controls presentation only; messages never expire on time.
// Zero character delay skips typing; zero Blinks skips blinking.
type Animation struct {
	Enabled                       bool
	CharacterDelay, BlinkInterval time.Duration
	Blinks                        int
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
	Text     string
	Kind     Kind
	started  time.Time
	progress bool
}

// Channel holds one contextual message. Its zero value is ready to use.
// A new message replaces the previous one; there is no queue or timer expiry.
// Use it on the UI thread; it is not safe for concurrent calls.
type Channel struct{ message *Message }

func (c *Channel) Show(kind Kind, text string) {
	c.message = &Message{Text: text, Kind: kind}
}

func (c *Channel) Progress(text string) {
	c.Show(Info, text)
	c.message.progress = true
}

// Complete clears progress without erasing an intervening result message.
func (c *Channel) Complete() {
	if c.message != nil && c.message.progress {
		c.Dismiss()
	}
}

func (c *Channel) Dismiss()          { c.message = nil }
func (c *Channel) Current() *Message { return c.message }

type Frame struct {
	Text string
	Kind Kind
	Next time.Time // Zero means the presentation is steady; no redraw is needed.
}

// Frame reports whether the channel owns the footer, even during an empty
// typing/blink frame. The caller schedules the next redraw when Next is set.
func (c *Channel) Frame(now time.Time, animations Animations) (Frame, bool) {
	if c.message == nil {
		return Frame{}, false
	}
	text, next := c.message.frame(now, animations.profile(c.message.Kind))
	return Frame{Text: text, Kind: c.message.Kind, Next: next}, true
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
			return string(characters[:count]), m.started.Add(time.Duration(count+1) * animation.CharacterDelay)
		}
	}
	if animation.Blinks > 0 && animation.BlinkInterval > 0 {
		phase := (elapsed - typingDuration) / animation.BlinkInterval
		if phase < time.Duration(animation.Blinks)*2 {
			value = m.Text
			if phase%2 != 0 {
				value = ""
			}
			return value, m.started.Add(typingDuration + (phase+1)*animation.BlinkInterval)
		}
	}
	return m.Text, time.Time{}
}
