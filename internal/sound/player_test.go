package sound

import "testing"

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
