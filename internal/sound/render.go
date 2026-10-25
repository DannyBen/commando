package sound

import (
	"io"
	"math"
	"sync/atomic"
	"time"

	"github.com/dannyben/commando/internal/diagnostics"
	"github.com/ui-sound-system/uss-go/synth"
)

type voice struct {
	stream *synth.Stream
	gain   float32
}

func (v *voice) close() {
	if v != nil {
		v.stream.Close()
	}
}

func masterGain(volume float64) float32 {
	if !finite(volume) || volume <= 0 {
		return 0
	}
	return float32(math.Min(volume, 100) / 100)
}

// Only the audio thread owns the current voice. Commands replace it at the
// next device packet; nil is Stop. Each packet is fully filled, including silence.
type audioRenderer struct {
	commands <-chan *voice
	current  *voice
	scratch  [bufferSamples]float32
}

func (r *audioRenderer) close() { r.current.close(); r.current = nil }

func (r *audioRenderer) render(stereo []float32) error {
	select {
	case next := <-r.commands:
		r.close()
		r.current = next
	default:
	}
	clear(stereo)
	for offset := 0; offset < len(stereo)/2 && r.current != nil; {
		v := r.current
		count := min(len(r.scratch), len(stereo)/2-offset)
		n, err := v.stream.ReadSamples(r.scratch[:count])
		if err != nil && err != io.EOF {
			return err
		}
		for i, sample := range r.scratch[:n] {
			value := max(float32(-1), min(float32(1), sample)) * v.gain
			stereo[(offset+i)*2], stereo[(offset+i)*2+1] = value, value
		}
		offset += n
		if err == io.EOF {
			r.close()
		}
	}
	return nil
}

// Counters are observational. File writes happen only on the control worker.
// An empty queue during an ongoing voice is a possible underrun, not a
// measurement of physical speaker output.
type audioStatistics struct {
	emptyQueue atomic.Uint64
	maxWakeNS  atomic.Int64
}

func (s *audioStatistics) wake(gap time.Duration, empty bool) {
	if empty {
		s.emptyQueue.Add(1)
	}
	for old := s.maxWakeNS.Load(); int64(gap) > old; old = s.maxWakeNS.Load() {
		if s.maxWakeNS.CompareAndSwap(old, int64(gap)) {
			break
		}
	}
}

func (s *audioStatistics) trace() {
	if diagnostics.Enabled(diagnostics.Feedback) {
		diagnostics.Record(diagnostics.Feedback, 0, "AudioTiming", map[string]any{
			"empty_queue_while_active": s.emptyQueue.Load(),
			"max_wake_gap_us":          s.maxWakeNS.Load() / 1000,
		})
	}
}
