package sound

import "sync"

// Player owns the audio device on one worker. Playback never blocks the UI;
// pending cues are replaced rather than accumulating behind the animation.
type Player struct {
	requests chan []int16
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
	sounds   map[Name][]int16
}

func NewPlayer() *Player {
	p := &Player{requests: make(chan []int16, 1), stop: make(chan struct{}), done: make(chan struct{}), sounds: make(map[Name][]int16)}
	for name, cue := range cues {
		pcm, err := synthesize(cue)
		if err != nil {
			panic(string(name) + ": " + err.Error())
		}
		p.sounds[name] = pcm
	}
	go p.run()
	return p
}

func (p *Player) Play(name Name) { p.submit(p.sounds[name]) }

// Stop drops pending audio and interrupts the current cue on the worker.
func (p *Player) Stop() { p.submit(nil) }

func (p *Player) submit(pcm []int16) {
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
	case p.requests <- pcm:
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
		case pcm := <-p.requests:
			if err := output.play(pcm); err != nil {
				return
			}
		}
	}
}
