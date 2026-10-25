package sound

import (
	"io"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

// Oto permits one context per process. Players own individual readers, while
// the context lives until process exit. Previews never initialize audio.
var device struct {
	once    sync.Once
	context *oto.Context
	ready   chan struct{}
	err     error
}

func sharedOutput() (*oto.Context, <-chan struct{}, error) {
	device.once.Do(func() {
		device.context, device.ready, device.err = oto.NewContext(&oto.NewContextOptions{
			SampleRate: sampleRate, ChannelCount: 1, Format: oto.FormatSignedInt16LE,
			BufferSize: 20 * time.Millisecond, ApplicationName: "Commando",
		})
	})
	return device.context, device.ready, device.err
}

type otoOutput struct{ context *oto.Context }

func (o otoOutput) err() error { return o.context.Err() }
func (o otoOutput) newPlayer(reader io.Reader) audioPlayback {
	player := o.context.NewPlayer(reader)
	player.SetBufferSize(sampleRate * 2 / 100) // 10 ms of source PCM.
	return player
}
