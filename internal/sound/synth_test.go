package sound

import (
	"math"
	"testing"
	"time"
)

func TestConfiguredCuesHaveFiniteBoundedAudioAndSilentEdges(t *testing.T) {
	for name, definition := range cues {
		cue := definition.cue()
		t.Run(string(name), func(t *testing.T) {
			pcm, err := synthesize(cue)
			if err != nil {
				t.Fatal(err)
			}
			want := samples(cue.Length + time.Duration(len(cue.Notes)-1)*cue.Spacing)
			if len(pcm) != want || pcm[0] != 0 || pcm[len(pcm)-1] != 0 {
				t.Fatal("incorrect duration or abrupt audio edge")
			}
			peak := 0
			for _, value := range pcm {
				peak = max(peak, int(math.Abs(float64(value))))
			}
			if peak == 0 || peak >= 32767 {
				t.Fatalf("silent or clipped cue: peak %d", peak)
			}
		})
	}
}

func TestOscillatorFrequencyVolumeAndNoteSpacing(t *testing.T) {
	cue := Cue{Notes: []float64{1000}, Length: 50 * time.Millisecond, Waveform: Sine, Volume: 20}
	pcm, err := synthesize(cue)
	if err != nil {
		t.Fatal(err)
	}
	crossings := 0
	for i := 1; i < len(pcm); i++ {
		if pcm[i-1] <= 0 && pcm[i] > 0 {
			crossings++
		}
	}
	if crossings < 49 || crossings > 50 {
		t.Fatalf("incorrect pitch: %d cycles", crossings)
	}
	cue.Volume = 0
	pcm, _ = synthesize(cue)
	for _, sample := range pcm {
		if sample != 0 {
			t.Fatal("zero volume is audible")
		}
	}
	cue.Volume, cue.Notes, cue.Spacing = 20, []float64{1000, 1000}, 70*time.Millisecond
	pcm, _ = synthesize(cue)
	for _, sample := range pcm[samples(50*time.Millisecond):samples(70*time.Millisecond)] {
		if sample != 0 {
			t.Fatal("note spacing was measured from note ends")
		}
	}
	if pcm[samples(72*time.Millisecond)] == 0 {
		t.Fatal("second note missing")
	}
}

func TestInvalidCueConfiguration(t *testing.T) {
	for _, cue := range []Cue{
		{},
		{Notes: []float64{math.NaN()}, Length: time.Second, Waveform: Sine},
		{Notes: []float64{sampleRate}, Length: time.Second, Waveform: Sine},
		{Notes: []float64{440}, Length: time.Second, Waveform: "unknown"},
		{Notes: []float64{440}, Length: time.Second, Waveform: Sine, Volume: 101},
		{Notes: []float64{440}, Length: time.Second, Waveform: Sine, Spacing: -time.Second},
		{Notes: []float64{440, 440, 440, 440}, Length: 50 * time.Millisecond, Waveform: Square, Volume: 100},
	} {
		if _, err := synthesize(cue); err == nil {
			t.Fatalf("accepted invalid cue: %+v", cue)
		}
	}
}
