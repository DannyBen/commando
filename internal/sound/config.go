// Package sound synthesizes semantic application cues from simple oscillators.
package sound

import (
	_ "embed"
	"fmt"
	"time"

	"github.com/BurntSushi/toml"
)

type Name string

const (
	Navigation          Name = "navigation"
	Click               Name = "click"
	Enabled             Name = "enabled"
	Disabled            Name = "disabled"
	Opened              Name = "opened"
	Closed              Name = "closed"
	DirectoryNavigation Name = "directoryNavigation"
	MessageInfo         Name = "messageInfo"
	MessageSuccess      Name = "messageSuccess"
	MessageWarning      Name = "messageWarning"
	MessageError        Name = "messageError"
	PanelSize06         Name = "panelSize06"
	PanelSize08         Name = "panelSize08"
	PanelSize10         Name = "panelSize10"
	PanelSize12         Name = "panelSize12"
	PanelSize15         Name = "panelSize15"
	PanelSize20         Name = "panelSize20"
)

// All is independent of cue levels, including levels added in the future.
const All = -1

type definition struct {
	Level     int
	File      string
	Inputs    map[string]any
	Notes     []float64
	LengthMS  int `toml:"length_ms"`
	SpacingMS int `toml:"spacing_ms"`
	Waveform  Waveform
	Volume    float64
}

func (d definition) cue() Cue {
	return Cue{Notes: d.Notes, Length: time.Duration(d.LengthMS) * time.Millisecond,
		Spacing: time.Duration(d.SpacingMS) * time.Millisecond, Waveform: d.Waveform, Volume: d.Volume}
}

//go:embed sounds.toml
var soundConfig string

var cues = mustDefinitions(soundConfig)

func mustDefinitions(data string) map[Name]definition {
	definitions, err := parseDefinitions(data)
	if err != nil {
		panic(err)
	}
	return definitions
}

func parseDefinitions(data string) (map[Name]definition, error) {
	var config struct{ Sounds map[Name]definition }
	metadata, err := toml.Decode(data, &config)
	if err != nil {
		return nil, err
	}
	if len(metadata.Undecoded()) != 0 || len(config.Sounds) == 0 {
		return nil, fmt.Errorf("unknown or missing sound configuration")
	}
	for name, d := range config.Sounds {
		if name == "" || d.Level <= 0 {
			return nil, fmt.Errorf("sound %q needs a positive level", name)
		}
		if _, err := synthesize(d.cue()); err != nil {
			return nil, fmt.Errorf("sound %q: %w", name, err)
		}
	}
	return config.Sounds, nil
}

// Allows keeps the cumulative threshold rule independent of display names.
// Unknown cues stay silent even when All is selected.
func Allows(name Name, profile int) bool {
	d, ok := cues[name]
	return ok && (profile == All || profile > 0 && d.Level <= profile)
}
