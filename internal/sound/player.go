package sound

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"sync"

	"github.com/dannyben/commando/internal/diagnostics"
)

type playback struct {
	name   Name
	volume float64
	reset  bool // An explicit Stop remains effective if a new cue replaces it.
}

// Player owns the audio device on one worker. Playback never blocks the UI;
// One pending cue retains the strongest/latest request. One active cue admits
// only higher/equal-priority replacements. Explicit Stop clears both.
type Player struct {
	requests chan playback
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
	mu       sync.Mutex // Serialize submissions, including replacement of pending cues.
	sounds   map[Name][]byte
}

func NewPlayer() *Player {
	p := &Player{requests: make(chan playback, 1), stop: make(chan struct{}), done: make(chan struct{}), sounds: make(map[Name][]byte)}
	for name, cue := range cues {
		pcm, err := synthesize(cue.cue())
		if err != nil {
			panic(string(name) + ": " + err.Error())
		}
		data, err := binary.Append(nil, binary.LittleEndian, pcm)
		if err != nil {
			panic(string(name) + ": " + err.Error())
		}
		p.sounds[name] = data
	}
	go p.run()
	return p
}

func (p *Player) Play(name Name, volume float64) {
	if Allows(name, All) && masterGain(volume) > 0 {
		p.submit(playback{name: name, volume: volume})
	}
}

// Stop drops pending audio and interrupts the current cue on the worker.
func (p *Player) Stop() { p.submit(playback{reset: true}) }

func (p *Player) submit(request playback) {
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case <-p.stop:
		return
	default:
	}
	select {
	case pending := <-p.requests:
		if !request.reset {
			if pending.name != "" && pending.name.Priority() < request.name.Priority() {
				request = pending
			} else {
				request.reset = pending.reset
			}
		}
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

// The worker controls Oto players; Oto owns device scheduling.
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
	IsPlaying() bool
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
	var priority Priority
	stop := func() {
		if active != nil {
			// Join any in-flight read before retiring the player.
			active.PauseAndStopReading()
			traceAudioError(active.Err())
			active = nil
		}
	}
	defer stop()
	for {
		select {
		case <-p.stop:
			return
		case request := <-p.requests:
			if err := output.err(); err != nil {
				traceAudioError(err)
				return
			}
			if !request.reset && active != nil && active.IsPlaying() && request.name.Priority() > priority {
				continue // Lower-priority requests are discarded, never deferred.
			}
			stop()
			pcm := p.sounds[request.name]
			gain := masterGain(request.volume)
			if len(pcm) == 0 || gain == 0 {
				continue
			}
			active = output.newPlayer(bytes.NewReader(pcm))
			priority = request.name.Priority()
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
