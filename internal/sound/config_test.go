package sound

import (
	"math"
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

func TestMasterVolumeBounds(t *testing.T) {
	for _, test := range []struct {
		volume float64
		want   float32
	}{
		{50, .5}, {100, 1}, {200, 1}, {0, 0}, {-1, 0}, {math.NaN(), 0}, {math.Inf(1), 0},
	} {
		if got := masterGain(test.volume); got != test.want {
			t.Fatalf("gain(%v)=%v, want %v", test.volume, got, test.want)
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
