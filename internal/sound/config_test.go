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

func TestOpenCloseCuesRiseAndFallAtInteractionLevel(t *testing.T) {
	on, off := cues[Opened], cues[Closed]
	if len(on.Notes) != 2 || len(off.Notes) != 2 || on.Notes[0] >= on.Notes[1] ||
		on.Notes[0] != off.Notes[1] || on.Notes[1] != off.Notes[0] || on.Level != 4 || off.Level != 4 {
		t.Fatal("Open/close cues must be a rising/falling pair at the interaction level")
	}
	if on.SpacingMS <= on.LengthMS || on.LengthMS+on.SpacingMS < 100 || on.LengthMS+on.SpacingMS > 200 {
		t.Fatal("open/close notes must be distinguishable while remaining brief")
	}
}

func TestBooleanCuesUseOneNoteAndHigherPitchForOn(t *testing.T) {
	on, off := cues[Enabled], cues[Disabled]
	if len(on.Notes) != 1 || len(off.Notes) != 1 || on.Notes[0] <= off.Notes[0] || on.Level != 4 || off.Level != 4 {
		t.Fatal("on/off must use single notes, with a higher pitch for on")
	}
}
