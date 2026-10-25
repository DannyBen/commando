package sound

import (
	"embed"
	"math"

	"github.com/ui-sound-system/uss-go/synth"
)

const sampleRate = synth.SampleRate

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

// Files default to cue names; sounds.toml can select another file per cue.
// Assets are embedded; build checks reject missing or invalid sounds.
//
//go:embed sounds/*.json
var soundFiles embed.FS

func prepareCue(name Name, cue definition) (*synth.Prepared, error) {
	file := cue.File
	if file == "" {
		file = string(name) + ".json"
	}
	data, err := soundFiles.ReadFile("sounds/" + file)
	if err != nil {
		return nil, err
	}
	d, err := synth.Parse(data)
	if err != nil {
		return nil, err
	}
	prepared, err := d.Prepare()
	if err != nil {
		return nil, err
	}
	// Validate the configured variant without generating any samples.
	voice, err := prepared.Stream(cue.Inputs)
	if err != nil {
		return nil, err
	}
	voice.Close()
	return prepared, nil
}
