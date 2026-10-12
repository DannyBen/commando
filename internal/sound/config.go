// Package sound synthesizes semantic application cues from simple oscillators.
package sound

import "time"

type Name string

const (
	MessageInfo    Name = "messageInfo"
	MessageSuccess Name = "messageSuccess"
	MessageWarning Name = "messageWarning"
	MessageError   Name = "messageError"
)

// Edit this table to tune sounds, then rebuild. Notes are frequencies in Hz;
// volume is a percentage. Spacing is measured between note starts.
var cues = map[Name]Cue{
	MessageInfo:    {Notes: []float64{440}, Length: 75 * time.Millisecond, Waveform: Triangle, Volume: 16},
	MessageSuccess: {Notes: []float64{660, 880}, Length: 60 * time.Millisecond, Spacing: 50 * time.Millisecond, Waveform: Triangle, Volume: 20},
	MessageWarning: {Notes: []float64{330, 247}, Length: 90 * time.Millisecond, Spacing: 80 * time.Millisecond, Waveform: Triangle, Volume: 18},
	MessageError:   {Notes: []float64{165, 110}, Length: 100 * time.Millisecond, Spacing: 85 * time.Millisecond, Waveform: Square, Volume: 12},
}
