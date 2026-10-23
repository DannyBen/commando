package sound

import (
	"slices"
	"strings"
	"testing"
)

func TestEmbeddedUSSSoundsRender(t *testing.T) {
	files, err := soundFiles.ReadDir("sounds")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		data, _ := soundFiles.ReadFile("sounds/" + file.Name())
		pcm, err := renderUSS(data)
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
		want, err := renderUSS(data)
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
	want, err := renderUSS(data)
	if err != nil {
		t.Fatal(err)
	}
	got, err := renderCue(Navigation, cues[Navigation])
	if err != nil || !slices.Equal(got, want) {
		t.Fatal("navigation did not render the exported tick", err)
	}
}

func TestMissingUSSFileRetainsOriginalCue(t *testing.T) {
	want, _ := synthesize(cues[Opened].cue())
	got, err := renderCue(Opened, cues[Opened])
	if err != nil || !slices.Equal(got, want) {
		t.Fatal("missing JSON changed the original cue", err)
	}
}

func TestInvalidUSSAudioIsRejected(t *testing.T) {
	data, _ := soundFiles.ReadFile("sounds/click.json")
	for _, invalid := range [][]byte{
		[]byte(`{}`),
		[]byte(strings.Replace(string(data), `"sine"`, `"unsupported"`, 1)),
		[]byte(strings.Replace(string(data), `"gain": 0.1`, `"gain": 10`, 1)),
	} {
		if _, err := renderUSS(invalid); err == nil {
			t.Fatal("invalid or clipping USS sound accepted")
		}
	}
}
