package sound

import (
	"embed"
	"fmt"
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

func renderCue(name Name, cue definition) ([]int16, error) {
	file := cue.File
	if file == "" {
		file = string(name) + ".json"
	}
	data, err := soundFiles.ReadFile("sounds/" + file)
	if err != nil {
		return nil, err
	}
	return renderUSS(data, cue.Inputs)
}

func renderUSS(data []byte, inputs synth.Inputs) ([]int16, error) {
	definition, err := synth.Parse(data)
	if err != nil {
		return nil, err
	}
	rendered, err := definition.Render(inputs)
	if err != nil {
		return nil, err
	}
	if !finite(rendered.Peak) || rendered.Peak > 1 {
		return nil, fmt.Errorf("sound clips at full master volume")
	}
	pcm := make([]int16, len(rendered.Samples))
	for i, value := range rendered.Samples {
		pcm[i] = int16(math.Round(float64(value) * 32767))
	}
	return pcm, nil
}
