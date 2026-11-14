package sound

import (
	"bytes"
	"encoding/binary"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPendingAudioIsReplacedAndStopDropsIt(t *testing.T) {
	p := &Player{requests: make(chan playback, 1), stop: make(chan struct{})}
	p.Play(Opened, 50)
	p.Play(Closed, 50)
	p.Play(Click, 30)
	if len(p.requests) != 1 {
		t.Fatal("audio accumulated in a queue")
	}
	if request := <-p.requests; request.name != Closed || request.volume != 50 {
		t.Fatal("equal priority did not replace, or lower priority replaced pending audio")
	}
	p.Play(Opened, 50)
	p.Stop()
	if request := <-p.requests; request.name != "" {
		t.Fatal("Stop retained pending audio")
	}
}

// Model Oto's asynchronous reader and the join performed by PauseAndStopReading.
type testPlayback struct {
	reader              io.Reader
	volume              float64
	packet              chan []byte
	quit, done, stopped chan struct{}
	once                sync.Once
	finished            atomic.Bool
}

func (p *testPlayback) Play() {
	go func() {
		defer close(p.done)
		buffer := make([]byte, 513)
		n, _ := p.reader.Read(buffer)
		p.packet <- buffer[:n]
		<-p.quit
	}()
}
func (p *testPlayback) PauseAndStopReading() {
	p.once.Do(func() { close(p.quit); <-p.done; close(p.stopped) })
}
func (p *testPlayback) SetVolume(volume float64) { p.volume = volume }
func (p *testPlayback) Err() error               { return nil }
func (p *testPlayback) IsPlaying() bool {
	select {
	case <-p.stopped:
		return false
	default:
		return !p.finished.Load()
	}
}

type testOutput struct {
	players chan *testPlayback
	checks  chan struct{}
	current *testPlayback // Accessed only by the audio worker.
	overlap atomic.Bool
}

func (o *testOutput) err() error { o.checks <- struct{}{}; return nil }
func (o *testOutput) newPlayer(r io.Reader) audioPlayback {
	if o.current != nil {
		select {
		case <-o.current.stopped:
		default:
			source := o.current.reader.(*releaseReader)
			source.mu.Lock()
			o.overlap.Store(o.overlap.Load() || source.releaseStart < 0)
			source.mu.Unlock()
		}
	}
	p := &testPlayback{reader: r, packet: make(chan []byte, 1), quit: make(chan struct{}), done: make(chan struct{}), stopped: make(chan struct{})}
	o.current = p
	o.players <- p
	return p
}
func testPlayer(t *testing.T) (*Player, *testOutput) {
	t.Helper()
	p := &Player{requests: make(chan playback, 1), stop: make(chan struct{}), done: make(chan struct{}), sounds: make(map[Name][]byte)}
	for name, cue := range cues {
		pcm, err := synthesize(cue.cue())
		if err != nil {
			t.Fatal(err)
		}
		data, err := binary.Append(nil, binary.LittleEndian, pcm)
		if err != nil {
			t.Fatal(err)
		}
		p.sounds[name] = data
	}
	o := &testOutput{players: make(chan *testPlayback, 4), checks: make(chan struct{}, 1024)}
	go func() { defer close(p.done); p.runOutput(o) }()
	t.Cleanup(p.Close)
	return p, o
}
func nextPlayback(t *testing.T, o *testOutput) (*testPlayback, []byte) {
	t.Helper()
	select {
	case player := <-o.players:
		select {
		case packet := <-player.packet:
			return player, packet
		case <-time.After(2 * time.Second):
			t.Fatal("asynchronous source read did not complete")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("playback did not start")
	}
	return nil, nil
}
func assertPlaybackStopped(t *testing.T, player *testPlayback) {
	t.Helper()
	select {
	case <-player.stopped:
	default:
		t.Fatal("retired player still reading")
	}
}

func assertPlaybackReleasing(t *testing.T, player *testPlayback) {
	t.Helper()
	select {
	case <-player.stopped:
		t.Fatal("replacement abruptly stopped the outgoing sound")
	default:
	}
	source := player.reader.(*releaseReader)
	source.mu.Lock()
	released := source.releaseStart >= 0
	source.mu.Unlock()
	if !released {
		t.Fatal("replacement did not release the outgoing sound")
	}
}

func assertPlaybackReleased(t *testing.T, player *testPlayback) {
	t.Helper()
	assertPlaybackReleasing(t, player)
	source := player.reader.(*releaseReader)
	tail, err := io.ReadAll(source)
	if err != nil || len(tail) < 2 || binary.LittleEndian.Uint16(tail[len(tail)-2:]) != 0 {
		t.Fatal("outgoing sound did not finish its release at silence", err)
	}
	player.finished.Store(true)
}

func TestPlaybackUsesCachedPCMAndReplacesActiveSound(t *testing.T) {
	p, o := testPlayer(t)
	before := bytes.Clone(p.sounds[Opened])
	p.Play(Opened, 50)
	first, packet := nextPlayback(t, o)
	full, err := synthesize(cues[Opened].cue())
	if err != nil {
		t.Fatal(err)
	}
	var want bytes.Buffer
	if err := binary.Write(&want, binary.LittleEndian, full); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(packet, want.Bytes()[:len(packet)]) || first.volume != .5 {
		t.Fatal("playback did not use original PCM and master volume")
	}
	p.Play(Closed, 100)
	second, _ := nextPlayback(t, o)
	assertPlaybackReleased(t, first)
	if second.volume != 1 {
		t.Fatal("replacement lost master volume")
	}
	p.Close()
	p.Close()
	assertPlaybackStopped(t, second)
	if !bytes.Equal(p.sounds[Opened], before) {
		t.Fatal("playback changed the cached audio")
	}
}
func TestStopJoinsReading(t *testing.T) {
	p, o := testPlayer(t)
	p.Play(Opened, 100)
	active, _ := nextPlayback(t, o)
	p.Stop()
	select {
	case <-active.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not stop source reads")
	}
	p.Close()
	assertPlaybackStopped(t, active)
	select {
	case <-o.players:
		t.Fatal("Stop started another sound")
	default:
	}
}

func TestStopClearsReleasingSoundsAsWellAsCurrentSound(t *testing.T) {
	p, o := testPlayer(t)
	p.Play(Opened, 100)
	first, _ := nextPlayback(t, o)
	p.Play(Closed, 100)
	second, _ := nextPlayback(t, o)
	assertPlaybackReleasing(t, first)
	p.Stop()
	select {
	case <-second.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not stop current playback")
	}
	select {
	case <-first.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not stop the outgoing release")
	}
	p.Close()
	if len(o.players) != 0 {
		t.Fatal("Stop queued another sound")
	}
}

func TestCompletedReleaseIsRetiredOnNextReplacement(t *testing.T) {
	p, o := testPlayer(t)
	p.Play(Opened, 100)
	first, _ := nextPlayback(t, o)
	p.Play(Closed, 100)
	second, _ := nextPlayback(t, o)
	assertPlaybackReleased(t, first)
	p.Play(Opened, 100)
	third, _ := nextPlayback(t, o)
	assertPlaybackStopped(t, first)
	assertPlaybackReleased(t, second)
	p.Close()
	assertPlaybackStopped(t, second)
	assertPlaybackStopped(t, third)
}
