package sound

import (
	"fmt"
	"math"
	"time"
)

const sampleRate = 44100

type Waveform string

const (
	Sine     Waveform = "sine"
	Triangle Waveform = "triangle"
	Square   Waveform = "square"
	Sawtooth Waveform = "sawtooth"
)

type Cue struct {
	Notes           []float64
	Length, Spacing time.Duration
	Waveform        Waveform
	Volume          float64
}

func synthesize(cue Cue) ([]int16, error) {
	if len(cue.Notes) == 0 || cue.Length <= 0 || cue.Spacing < 0 || !finite(cue.Volume) || cue.Volume < 0 || cue.Volume > 100 {
		return nil, fmt.Errorf("invalid sound timing or volume")
	}
	switch cue.Waveform {
	case Sine, Triangle, Square, Sawtooth:
	default:
		return nil, fmt.Errorf("unknown waveform %q", cue.Waveform)
	}
	for _, frequency := range cue.Notes {
		if !finite(frequency) || frequency <= 0 || frequency >= sampleRate/2 {
			return nil, fmt.Errorf("invalid frequency %v", frequency)
		}
	}
	length := samples(cue.Length)
	if length < 2 {
		return nil, fmt.Errorf("sound is too short")
	}
	total := samples(cue.Length + time.Duration(len(cue.Notes)-1)*cue.Spacing)
	mixed := make([]float64, total)
	for note, frequency := range cue.Notes {
		start := samples(time.Duration(note) * cue.Spacing)
		for i := 0; i < length && start+i < total; i++ {
			phase := float64(i) * frequency / sampleRate
			// Short attack and release avoid clicks; the exponential decay gives
			// the same dry oscillator character as the poker game's cues.
			attack := min(1.0, float64(i)/(sampleRate*0.002))
			release := min(1.0, float64(length-1-i)/(sampleRate*0.001))
			envelope := attack * release * math.Exp(-6*float64(i)/float64(length))
			mixed[start+i] += oscillator(cue.Waveform, phase) * envelope * cue.Volume / 100
		}
	}
	pcm := make([]int16, total)
	for i, value := range mixed {
		if !finite(value) || math.Abs(value) > 1 {
			return nil, fmt.Errorf("sound clips at full master volume")
		}
		pcm[i] = int16(math.Round(max(-1.0, min(1.0, value)) * 32767))
	}
	return pcm, nil
}

func samples(duration time.Duration) int {
	return int(duration * sampleRate / time.Second)
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func oscillator(waveform Waveform, phase float64) float64 {
	phase -= math.Floor(phase)
	switch waveform {
	case Triangle:
		return 1 - 4*math.Abs(phase-0.5)
	case Square:
		if phase < 0.5 {
			return 1
		}
		return -1
	case Sawtooth:
		return 2*phase - 1
	default:
		return math.Sin(2 * math.Pi * phase)
	}
}
