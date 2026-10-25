package sound

import (
	"slices"
	"strings"
	"testing"

	"github.com/ui-sound-system/uss-go/synth"
)

func TestEmbeddedUSSSoundsRender(t *testing.T) {
	files, err := soundFiles.ReadDir("sounds")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		data, _ := soundFiles.ReadFile("sounds/" + file.Name())
		pcm, err := renderUSS(data, nil)
		if err != nil {
			t.Fatalf("%s: %v", file.Name(), err)
		}
		if !slices.ContainsFunc(pcm, func(sample int16) bool { return sample != 0 }) {
			t.Fatalf("%s is silent", file.Name())
		}
	}
}

func TestUSSCueUsesSelectedFile(t *testing.T) {
	for _, file := range []string{"click.json", "tick.json"} {
		data, _ := soundFiles.ReadFile("sounds/" + file)
		want, err := renderUSS(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		definition := cues[Click]
		definition.File = file
		got, err := renderCue(Click, definition)
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("click did not use %s: %v", file, err)
		}
	}
	definition := cues[Click]
	definition.File = "missing.json"
	if _, err := renderCue(Click, definition); err == nil {
		t.Fatal("missing selected file silently fell back")
	}
}

func TestNavigationUsesTick(t *testing.T) {
	if cues[Navigation].File != "tick.json" || cues[Click].File != "" {
		t.Fatal("tick must replace navigation, not click")
	}
	data, err := soundFiles.ReadFile("sounds/tick.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := renderUSS(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := renderCue(Navigation, cues[Navigation])
	if err != nil || !slices.Equal(got, want) {
		t.Fatal("navigation did not render the exported tick", err)
	}
}

func TestMissingUSSFileIsRejected(t *testing.T) {
	if _, err := renderCue(Name("missing"), definition{}); err == nil {
		t.Fatal("missing implicit USS file accepted")
	}
}

func TestEveryConfiguredCueUsesUSS(t *testing.T) {
	for name, cue := range cues {
		file := cue.File
		if file == "" {
			file = string(name) + ".json"
		}
		data, err := soundFiles.ReadFile("sounds/" + file)
		if err != nil {
			t.Fatalf("%s has no USS asset: %v", name, err)
		}
		want, err := renderUSS(data, cue.Inputs)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got, err := renderCue(name, cue)
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("%s did not use USS: %v", name, err)
		}
		if !slices.ContainsFunc(got, func(sample int16) bool { return sample != 0 }) {
			t.Fatalf("%s is silent", name)
		}
	}
}

func TestPanelSizesUseUSSIndices(t *testing.T) {
	data, err := soundFiles.ReadFile("sounds/panel-size.json")
	if err != nil {
		t.Fatal(err)
	}
	var previous []int16
	for _, test := range []struct {
		name  Name
		index string
		note  string
	}{
		{PanelSize06, "-2", "A3"},
		{PanelSize08, "-1", "B3"},
		{PanelSize10, "0", "D4"},
		{PanelSize12, "1", "E4"},
		{PanelSize15, "2", "F#4"},
		{PanelSize20, "3", "A4"},
	} {
		cue := cues[test.name]
		if cue.Level != 4 || cue.File != "panel-size.json" || cue.Inputs["index"] != test.index {
			t.Fatalf("%s must select index %s at the interaction level", test.name, test.index)
		}
		want, err := renderUSS(data, synth.Inputs{"root": test.note, "index": "0"})
		if err != nil {
			t.Fatal(err)
		}
		got, err := renderCue(test.name, cue)
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("%s did not render %s: %v", test.name, test.note, err)
		}
		if !slices.ContainsFunc(got, func(sample int16) bool { return sample != 0 }) || slices.Equal(previous, got) {
			t.Fatal("panel size sounds must be audible and distinct")
		}
		previous = got
	}
}

func TestDirectoryNavigationUsesUSS(t *testing.T) {
	cue := cues[DirectoryNavigation]
	if cue.Level != 5 || cue.File != "dir-change.json" {
		t.Fatal("directory navigation must use dir-change at the navigation level")
	}
	data, err := soundFiles.ReadFile("sounds/dir-change.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := renderUSS(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := renderCue(DirectoryNavigation, cue)
	if err != nil || !slices.Equal(got, want) {
		t.Fatal("directory navigation did not render the exported sound", err)
	}
}

func TestInvalidUSSAudioIsRejected(t *testing.T) {
	// Keep rejection checks independent of editable production sounds.
	const data = `{
		"version": 1, "name": "test",
		"render": {"sample_rate": 48000, "channels": 1},
		"graph": {
			"tone": {"module": "oscillator", "inputs": {"waveform": "sine", "frequency_hz": 440, "phase_cycles": 0}},
			"envelope": {"module": "envelope_ad", "inputs": {"attack_ms": 2, "decay_ms": 10, "curve": "linear"}},
			"amp": {"module": "amplifier", "inputs": {"audio": {"ref": "tone.audio"}, "envelope": {"ref": "envelope.control"}, "gain": 0.1}}
		},
		"output": {"audio": {"ref": "amp.audio"}, "duration_ms": 13}
	}`
	if _, err := renderUSS([]byte(data), nil); err != nil {
		t.Fatal("invalid test fixture", err)
	}
	for _, invalid := range [][]byte{
		[]byte(`{}`),
		[]byte(strings.Replace(data, `"sine"`, `"unsupported"`, 1)),
		[]byte(strings.Replace(data, `"gain": 0.1`, `"gain": 10`, 1)),
	} {
		if _, err := renderUSS(invalid, nil); err == nil {
			t.Fatal("invalid or clipping USS sound accepted")
		}
	}
}

func TestOpenCloseUsesUSSWithOppositeDirections(t *testing.T) {
	data, err := soundFiles.ReadFile("sounds/open-close.json")
	if err != nil {
		t.Fatal(err)
	}
	up, err := renderUSS(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Swapping the notes should match selecting the descending branch.
	down, err := renderUSS(data, synth.Inputs{"note1": "G4", "note2": "G3"})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Equal(up, down) {
		t.Fatal("open and close must sound different")
	}
	for _, test := range []struct {
		name      Name
		direction string
		want      []int16
	}{
		{Opened, "up", up},
		{Closed, "down", down},
	} {
		cue := cues[test.name]
		if cue.Level != 4 || cue.File != "open-close.json" || cue.Inputs["direction"] != test.direction {
			t.Fatalf("%s must use open-close with direction %s at the interaction level", test.name, test.direction)
		}
		got, err := renderCue(test.name, cue)
		if err != nil || !slices.Equal(got, test.want) {
			t.Fatalf("%s did not render the expected direction: %v", test.name, err)
		}
	}
}

func TestEnableDisableUsesUSSWithOppositeDirections(t *testing.T) {
	data, err := soundFiles.ReadFile("sounds/enable-disable.json")
	if err != nil {
		t.Fatal(err)
	}
	var previous []int16
	for _, test := range []struct {
		name      Name
		direction string
	}{
		{Enabled, "up"},
		{Disabled, "down"},
	} {
		want, err := renderUSS(data, synth.Inputs{"direction": test.direction})
		if err != nil {
			t.Fatal(err)
		}
		got, err := renderCue(test.name, cues[test.name])
		if err != nil || !slices.Equal(got, want) {
			t.Fatalf("%s did not render the expected direction: %v", test.name, err)
		}
		if !slices.ContainsFunc(got, func(sample int16) bool { return sample != 0 }) || slices.Equal(previous, got) {
			t.Fatal("enable and disable must be audible and distinct")
		}
		previous = got
	}
}
