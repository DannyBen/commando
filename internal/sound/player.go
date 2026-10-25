package sound

import (
	"io"
	"math"
	"sync"

	"github.com/dannyben/commando/internal/diagnostics"
	"github.com/ui-sound-system/uss-go/synth"
)

type playback struct {
	name   Name
	volume float64
}

// Player owns the audio device on one worker. Playback never blocks the UI;
// pending cues are replaced rather than accumulating behind the animation.
type Player struct {
	requests chan playback
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
	sounds   map[Name]*synth.Prepared
}

func NewPlayer() *Player {
	p := &Player{requests: make(chan playback, 1), stop: make(chan struct{}), done: make(chan struct{}), sounds: make(map[Name]*synth.Prepared)}
	for name, cue := range cues {
		prepared, err := prepareCue(name, cue)
		if err != nil {
			panic(string(name) + ": " + err.Error())
		}
		p.sounds[name] = prepared
	}
	go p.run()
	return p
}

func (p *Player) Play(name Name, volume float64) { p.submit(playback{name, volume}) }

// Stop drops pending audio and interrupts the current cue on the worker.
func (p *Player) Stop() { p.submit(playback{}) }

func (p *Player) submit(request playback) {
	select {
	case <-p.stop:
		return
	default:
	}
	select {
	case <-p.requests:
	default:
	}
	select {
	case p.requests <- request:
	default:
	}
}

func (p *Player) Close() {
	p.once.Do(func() { close(p.stop) })
	<-p.done
}

// The worker constructs voices and controls Oto; Oto owns device scheduling.
func (p *Player) run() {
	defer close(p.done)
	context, ready, err := sharedOutput()
	if err != nil {
		traceAudioError(err)
		return
	}
	select {
	case <-p.stop:
		return
	case <-ready:
	}
	if err := context.Err(); err != nil {
		traceAudioError(err)
		return
	}
	diagnostics.Record(diagnostics.Feedback, 0, "AudioOpened", map[string]any{"backend": "Oto", "sample_rate": sampleRate, "channels": 1})
	p.runOutput(otoOutput{context})
}

type audioPlayback interface {
	Play()
	PauseAndStopReading()
	SetVolume(float64)
	Err() error
}

type audioOutput interface {
	newPlayer(io.Reader) audioPlayback
	err() error
}

func (p *Player) runOutput(output audioOutput) {
	var active audioPlayback
	var reader io.ReadCloser
	stop := func() {
		if active != nil {
			// Pause alone permits read-ahead. Join the in-flight read before releasing
			// the USS voice, and retain the player until it has been stopped.
			active.PauseAndStopReading()
			traceAudioError(active.Err())
			reader.Close()
			active, reader = nil, nil
		}
	}
	defer stop()
	for {
		select {
		case <-p.stop:
			return
		case request := <-p.requests:
			stop()
			if err := output.err(); err != nil {
				traceAudioError(err)
				return
			}
			prepared := p.sounds[request.name]
			gain := masterGain(request.volume)
			if prepared == nil || gain == 0 {
				continue
			}
			stream, err := prepared.Stream(cues[request.name].Inputs)
			if err != nil {
				traceAudioError(err)
				continue
			}
			reader = stream.Float32LEReader()
			active = output.newPlayer(reader)
			active.SetVolume(float64(gain))
			active.Play()
		}
	}
}

func masterGain(volume float64) float32 {
	if !finite(volume) || volume <= 0 {
		return 0
	}
	return float32(math.Min(volume, 100) / 100)
}

func traceAudioError(err error) {
	if err != nil {
		diagnostics.Record(diagnostics.Feedback, 0, "AudioFailed", map[string]any{"error": err.Error()})
	}
}
