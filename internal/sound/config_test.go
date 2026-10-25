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
file = "click.json"
`
	for _, data := range []string{
		strings.Replace(valid, "level = 5", "level = 0", 1),
		strings.Replace(valid, "level = 5", "level = -1", 1),
		strings.Replace(valid, "level = 5\n", "", 1),
		strings.Replace(valid, "level = 5", "level = 1.5", 1),
		valid + "volume = 10\n",
		valid + "notes = [440]\n",
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

func TestBooleanCuesUseOppositeUSSDirectionsAtInteractionLevel(t *testing.T) {
	on, off := cues[Enabled], cues[Disabled]
	if on.File != "enable-disable.json" || off.File != on.File ||
		on.Inputs["direction"] != "up" || off.Inputs["direction"] != "down" || on.Level != 4 || off.Level != 4 {
		t.Fatal("on/off must use enable-disable with up/down directions at the interaction level")
	}
}
