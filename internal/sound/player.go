package sound

import (
	"sync"

	"github.com/dannyben/commando/internal/diagnostics"
	"github.com/ui-sound-system/uss-go/synth"
)

const bufferSamples = 512

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

// Voice construction and trace writes stay off the device's audio thread.
func (p *Player) run() {
	defer close(p.done)
	commands := make(chan *voice, 1)
	audioDone := make(chan struct{})
	stats := &audioStatistics{}
	go func() {
		defer close(audioDone)
		if err := runAudio(commands, p.stop, stats); err != nil {
			diagnostics.Record(diagnostics.Feedback, 0, "AudioFailed", map[string]any{"error": err.Error()})
		}
	}()
	defer func() {
		<-audioDone
		select {
		case pending := <-commands:
			pending.close()
		default:
		}
		stats.trace()
	}()
	for {
		select {
		case <-p.stop:
			return
		case <-audioDone:
			return
		case request := <-p.requests:
			stats.trace()
			var next *voice
			if prepared := p.sounds[request.name]; prepared != nil && masterGain(request.volume) > 0 {
				stream, err := prepared.Stream(cues[request.name].Inputs)
				if err != nil {
					diagnostics.Record(diagnostics.Feedback, 0, "AudioFailed", map[string]any{"error": err.Error()})
				} else {
					next = &voice{stream: stream, gain: masterGain(request.volume)}
				}
			}
			replaceVoice(commands, next)
		}
	}
}

func replaceVoice(commands chan *voice, next *voice) {
	select {
	case pending := <-commands:
		pending.close()
	default:
	}
	commands <- next
}
