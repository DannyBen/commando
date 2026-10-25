package sound

import (
	"io"
	"testing"
	"time"
)

func TestDevicePacketsMatchEveryCueAndSilenceTail(t *testing.T) {
	sizes := []int{13, 256, 480, 1024, 37, 512}
	for name, cue := range cues {
		t.Run(string(name), func(t *testing.T) {
			prepared, err := prepareCue(name, cue)
			if err != nil {
				t.Fatal(err)
			}
			full, err := prepared.Render(cue.Inputs)
			if err != nil {
				t.Fatal(err)
			}
			for _, gain := range []float32{1, .5} {
				stream, err := prepared.Stream(cue.Inputs)
				if err != nil {
					t.Fatal(err)
				}
				commands := make(chan *voice, 1)
				commands <- &voice{stream: stream, gain: gain}
				r := &audioRenderer{commands: commands}
				position := 0
				for packet := 0; position <= len(full.Samples); packet++ {
					frames := sizes[packet%len(sizes)]
					stereo := make([]float32, frames*2)
					for i := range stereo {
						stereo[i] = 123
					}
					if err := r.render(stereo); err != nil {
						t.Fatal(err)
					}
					for i := 0; i < frames; i++ {
						var want float32
						if position+i < len(full.Samples) {
							want = full.Samples[position+i] * gain
						}
						if stereo[i*2] != want || stereo[i*2+1] != want {
							t.Fatalf("gain %v frame %d: got %v/%v, want %v", gain, position+i, stereo[i*2], stereo[i*2+1], want)
						}
					}
					position += frames
				}
				if r.current != nil {
					t.Fatal("completed voice retained DSP state")
				}
				r.close()
			}
		})
	}
}

func TestDevicePacketReplacesActiveVoiceAndStopWritesSilence(t *testing.T) {
	prepared, err := prepareCue(Opened, cues[Opened])
	if err != nil {
		t.Fatal(err)
	}
	first, _ := prepared.Stream(cues[Opened].Inputs)
	commands := make(chan *voice, 1)
	r := &audioRenderer{commands: commands}
	defer r.close()
	commands <- &voice{stream: first, gain: 1}
	stereo := make([]float32, 128)
	if err := r.render(stereo); err != nil {
		t.Fatal(err)
	}
	second, _ := prepared.Stream(cues[Closed].Inputs)
	want, _ := prepared.Render(cues[Closed].Inputs)
	commands <- &voice{stream: second, gain: .5}
	if err := r.render(stereo); err != nil {
		t.Fatal(err)
	}
	var scratch [1]float32
	if _, err := first.ReadSamples(scratch[:]); err != io.EOF {
		t.Fatal("active voice not closed on replacement")
	}
	for i := 0; i < len(stereo)/2; i++ {
		if stereo[i*2] != want.Samples[i]*.5 {
			t.Fatal("replacement did not start from the beginning with volume")
		}
	}
	commands <- nil
	if err := r.render(stereo); err != nil {
		t.Fatal(err)
	}
	for _, sample := range stereo {
		if sample != 0 {
			t.Fatal("Stop left stale packet samples")
		}
	}
	if _, err := second.ReadSamples(scratch[:]); err != io.EOF {
		t.Fatal("Stop retained active synthesis")
	}
}

func TestAudioTimingCounters(t *testing.T) {
	var stats audioStatistics
	stats.wake(2*time.Millisecond, false)
	stats.wake(9*time.Millisecond, true)
	stats.wake(time.Millisecond, false)
	if stats.emptyQueue.Load() != 1 || stats.maxWakeNS.Load() != int64(9*time.Millisecond) {
		t.Fatal("incorrect audio timing counters")
	}
}
