package sound

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"math"

	"github.com/ui-sound-system/uss-go/synth"
)

// Files default to cue names; sounds.toml can select another file per cue.
// They are embedded at build time; missing files retain the original cue.
//
//go:embed sounds/*.json
var soundFiles embed.FS

func renderCue(name Name, fallback definition) ([]int16, error) {
	file := fallback.File
	if file == "" {
		file = string(name) + ".json"
	}
	data, err := soundFiles.ReadFile("sounds/" + file)
	if errors.Is(err, fs.ErrNotExist) && fallback.File == "" {
		return synthesize(fallback.cue())
	}
	if err != nil {
		return nil, err
	}
	return renderUSS(data)
}

func renderUSS(data []byte) ([]int16, error) {
	definition, err := synth.Parse(data)
	if err != nil {
		return nil, err
	}
	rendered, err := definition.Render(nil)
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
