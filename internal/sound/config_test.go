package sound

import (
	"math"
	"slices"
	"strings"
	"testing"
)

func TestAllIncludesFutureLevelsAndUnknownCuesStaySilent(t *testing.T) {
	name := Name("future")
	definition := cues[MessageError]
	definition.Level = 100
	cues[name] = definition
	t.Cleanup(func() { delete(cues, name) })
	if !Allows(name, All) || Allows(name, 4) || Allows(name, 0) || Allows("unknown", All) {
		t.Fatal("All depends on a fixed maximum or unknown sounds bypass policy")
	}
}

func TestSoundConfigurationRequiresPositiveLevelsAndKnownParameters(t *testing.T) {
	const valid = `[sounds.example]
level = 5
notes = [440]
length_ms = 12
waveform = "triangle"
volume = 10
`
	for _, data := range []string{
		strings.Replace(valid, "level = 5", "level = 0", 1),
		strings.Replace(valid, "level = 5", "level = -1", 1),
		strings.Replace(valid, "level = 5\n", "", 1),
		strings.Replace(valid, "level = 5", "level = 1.5", 1),
		strings.Replace(valid, "volume = 10", "volume = 101", 1),
		strings.Replace(valid, "length_ms = 12", "length = 12", 1),
		"",
	} {
		if _, err := parseDefinitions(data); err == nil {
			t.Fatal("invalid sound definition accepted")
		}
	}
}

func TestMasterVolumeScalesWithoutMutatingCachedAudio(t *testing.T) {
	original := []int16{-32768, -1000, 0, 1000, 32767}
	before := slices.Clone(original)
	if got := scalePCM(original, 50); !slices.Equal(got, []int16{-16384, -500, 0, 500, 16384}) {
		t.Fatal("half volume did not scale PCM", got)
	}
	if !slices.Equal(original, before) || !slices.Equal(scalePCM(original, 100), before) {
		t.Fatal("master volume changed cached samples or full-volume playback")
	}
	for _, volume := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		if len(scalePCM(original, volume)) != 0 {
			t.Fatal("invalid or zero volume was audible")
		}
	}
}

func TestPanelSizeCuesRiseFromG3ThroughNormalC4ToG4(t *testing.T) {
	previous := 0.0
	for _, name := range []Name{PanelSize06, PanelSize08, PanelSize10, PanelSize12, PanelSize15, PanelSize20} {
		cue := cues[name]
		if len(cue.Notes) != 1 || cue.Notes[0] <= previous {
			t.Fatalf("size cue %s is not a single rising pitch", name)
		}
		previous = cue.Notes[0]
		if cue.Level != 4 || cue.LengthMS != 45 || cue.Volume != 28 || cue.Waveform != Triangle {
			t.Fatalf("size cue %s does not use the agreed sound profile", name)
		}
	}
	if math.Abs(cues[PanelSize10].Notes[0]-261.6256) > 0.01 {
		t.Fatal("normal size is not C4")
	}
}
