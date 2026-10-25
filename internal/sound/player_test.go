package sound

import (
	"io"
	"testing"
)

func TestPendingAudioIsReplacedAndStopDropsIt(t *testing.T) {
	p := &Player{requests: make(chan playback, 1), stop: make(chan struct{})}
	p.Play(Opened, 50)
	p.Play(Closed, 50)
	p.Play(Click, 30)
	if len(p.requests) != 1 {
		t.Fatal("audio accumulated in a queue")
	}
	if request := <-p.requests; request.name != Click || request.volume != 30 {
		t.Fatal("newest cue did not replace pending audio")
	}
	p.Play(Opened, 50)
	p.Stop()
	if request := <-p.requests; request.name != "" {
		t.Fatal("Stop retained pending audio")
	}
}

func TestSupersededVoicesAreClosed(t *testing.T) {
	prepared, err := prepareCue(Opened, cues[Opened])
	if err != nil {
		t.Fatal(err)
	}
	first, _ := prepared.Stream(cues[Opened].Inputs)
	second, _ := prepared.Stream(cues[Opened].Inputs)
	commands := make(chan *voice, 1)
	replaceVoice(commands, &voice{stream: first, gain: 1})
	replaceVoice(commands, &voice{stream: second, gain: 1})
	var scratch [1]float32
	if _, err := first.ReadSamples(scratch[:]); err != io.EOF {
		t.Fatal("superseded pending voice remained live")
	}
	replaceVoice(commands, nil)
	if _, err := second.ReadSamples(scratch[:]); err != io.EOF {
		t.Fatal("Stop retained pending synthesis")
	}
	if <-commands != nil {
		t.Fatal("Stop did not replace the pending voice")
	}
}

func TestPlayerCloseIsIdempotent(t *testing.T) {
	p := NewPlayer()
	p.Play(Opened, 100)
	p.Close()
	p.Close()
	select {
	case <-p.done:
	default:
		t.Fatal("audio worker did not finish")
	}
}
