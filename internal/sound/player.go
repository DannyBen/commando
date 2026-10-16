package sound

import (
	"math"
	"sync"
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
	sounds   map[Name][]int16
}

func NewPlayer() *Player {
	p := &Player{requests: make(chan playback, 1), stop: make(chan struct{}), done: make(chan struct{}), sounds: make(map[Name][]int16)}
	for name, cue := range cues {
		pcm, err := synthesize(cue.cue())
		if err != nil {
			panic(string(name) + ": " + err.Error())
		}
		p.sounds[name] = pcm
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

func (p *Player) run() {
	defer close(p.done)
	output, err := openOutput()
	if err != nil {
		return // An unavailable audio device must not affect file operations.
	}
	defer output.close()
	for {
		select {
		case <-p.stop:
			return
		case request := <-p.requests:
			pcm := scalePCM(p.sounds[request.name], request.volume)
			if err := output.play(pcm); err != nil {
				return
			}
		}
	}
}

// Scale on the audio worker, leaving the cached cue and its relative volume
// untouched. Master volume 100 preserves the original samples exactly.
func scalePCM(pcm []int16, volume float64) []int16 {
	if !finite(volume) || volume <= 0 {
		return nil
	}
	if volume >= 100 {
		return pcm
	}
	scaled := make([]int16, len(pcm))
	for i, sample := range pcm {
		scaled[i] = int16(math.Round(float64(sample) * volume / 100))
	}
	return scaled
}
