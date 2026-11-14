package sound

import (
	"sync"
	"testing"
	"time"
)

// Exercise each category through the real pending queue and playback worker.
// The ordering here is the public policy, independent of the implementation.
var priorityOrder = []Name{MessageError, MessageWarning, MessageInfo, Enabled, Opened, Click, Navigation}

func TestPendingPriorityAcrossCuePairs(t *testing.T) {
	for firstRank, first := range priorityOrder {
		for nextRank, next := range priorityOrder {
			p := &Player{requests: make(chan playback, 1), stop: make(chan struct{})}
			p.Play(first, 40)
			p.Play(next, 80)
			want := first
			if nextRank <= firstRank {
				want = next
			}
			if got := <-p.requests; got.name != want || len(p.requests) != 0 {
				t.Fatalf("pending %s then %s: got %s, want %s", first, next, got.name, want)
			}
		}
	}
	// Different cues with equal importance follow the same newest-wins rule.
	p := &Player{requests: make(chan playback, 1), stop: make(chan struct{})}
	p.Play(MessageInfo, 40)
	p.Play(MessageSuccess, 80)
	if got := <-p.requests; got.name != MessageSuccess {
		t.Fatal("equal-priority notifications did not prefer the newest")
	}
}

func waitAudioRequest(t *testing.T, o *testOutput) {
	t.Helper()
	select {
	case <-o.checks:
	case <-time.After(2 * time.Second):
		t.Fatal("audio worker did not consume the request")
	}
}

func TestActivePriorityAcrossCuePairs(t *testing.T) {
	for firstRank, first := range priorityOrder {
		for nextRank, next := range priorityOrder {
			t.Run(string(first)+"/"+string(next), func(t *testing.T) {
				p, o := testPlayer(t)
				p.Play(first, 40)
				active, _ := nextPlayback(t, o)
				waitAudioRequest(t, o)
				p.Play(next, 80)
				waitAudioRequest(t, o) // Consume it before a later request can replace it.
				if nextRank > firstRank {
					// A new equal-priority event is also a barrier: any wrongly
					// accepted lower cue would appear before it in the output.
					p.Play(first, 60)
					player, _ := nextPlayback(t, o)
					if player.volume != float64(masterGain(60)) {
						t.Fatal("a lower-priority cue interrupted active audio")
					}
				} else {
					player, _ := nextPlayback(t, o)
					if player.volume != float64(masterGain(80)) {
						t.Fatal("higher/equal-priority replacement was lost")
					}
				}
				assertPlaybackReleased(t, active)
				p.Close()
				if o.overlap.Load() || len(o.players) != 0 {
					t.Fatal("unreleased playback overlapped or discarded audio was replayed")
				}
			})
		}
	}
}

func TestCompletedSoundNoLongerBlocksLowerPriority(t *testing.T) {
	p, o := testPlayer(t)
	p.Play(MessageError, 50)
	active, _ := nextPlayback(t, o)
	active.finished.Store(true)
	p.Play(Navigation, 30)
	player, _ := nextPlayback(t, o)
	assertPlaybackStopped(t, active)
	if player.volume != float64(masterGain(30)) {
		t.Fatal("completed audio retained priority ownership")
	}
}

func TestStopRemainsEffectiveWhenPendingCueReplacesIt(t *testing.T) {
	p, o := testPlayer(t)
	p.Play(MessageError, 50)
	active, _ := nextPlayback(t, o)
	// This is the single request produced by coalescing Stop and a new cue.
	p.submit(playback{reset: true, name: Navigation, volume: 30})
	player, _ := nextPlayback(t, o)
	assertPlaybackStopped(t, active)
	if player.volume != float64(masterGain(30)) {
		t.Fatal("explicit dismissal lost to active priority")
	}
	// Also verify the public submission path without a consumer.
	q := &Player{requests: make(chan playback, 1), stop: make(chan struct{})}
	q.Stop()
	q.Play(Navigation, 30)
	if request := <-q.requests; !request.reset || request.name != Navigation {
		t.Fatal("new pending cue erased explicit dismissal")
	}
}

func TestConcurrentSubmissionsPreserveStrongestPendingCue(t *testing.T) {
	p := &Player{requests: make(chan playback, 1), stop: make(chan struct{})}
	var submissions sync.WaitGroup
	for range 20 {
		for _, cue := range priorityOrder {
			submissions.Go(func() { p.Play(cue, 50) })
		}
	}
	submissions.Wait()
	if request := <-p.requests; request.name != MessageError || len(p.requests) != 0 {
		t.Fatal("concurrent requests lost priority or accumulated audio")
	}
}

func TestInvalidOrMutedRequestsLeavePlaybackAlone(t *testing.T) {
	p, o := testPlayer(t)
	p.Play(MessageError, 50)
	active, _ := nextPlayback(t, o)
	p.Play("unknown", 100)
	p.Play(MessageError, 0)
	p.Play(MessageError, -1)
	p.Play(MessageError, 100) // Barrier after requests which must be ignored.
	player, _ := nextPlayback(t, o)
	assertPlaybackReleased(t, active)
	if player.volume != 1 || len(o.players) != 0 {
		t.Fatal("invalid or muted request affected active playback")
	}
	pending := &Player{requests: make(chan playback, 1), stop: make(chan struct{})}
	pending.Play(MessageInfo, 50)
	pending.Play("unknown", 100)
	pending.Play(MessageError, 0)
	if request := <-pending.requests; request.name != MessageInfo {
		t.Fatal("invalid or muted request erased a valid pending cue")
	}
}
