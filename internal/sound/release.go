package sound

import (
	"encoding/binary"
	"io"
	"math"
	"sync"
	"time"
)

const replacementRelease = 40 * time.Millisecond

// Each playback has its own cursor and release envelope over immutable PCM.
// Oto reads on its audio thread; the request worker can release it concurrently.
type releaseReader struct {
	mu                       sync.Mutex
	pcm                      []byte
	position                 int
	releaseStart, releaseEnd int // Sample positions; -1 means not released.
}

func newReleaseReader(pcm []byte) *releaseReader {
	return &releaseReader{pcm: pcm, releaseStart: -1}
}

// release starts from the next complete sample, preserving a partially read
// sample. An unheard source can be retired without playing a release tail.
func (r *releaseReader) release() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.releaseStart < 0 {
		r.releaseStart = (r.position + 1) / 2
		r.releaseEnd = min(len(r.pcm)/2, r.releaseStart+samples(replacementRelease))
		if r.position == 0 {
			r.releaseEnd = 0
		}
	}
	return r.position > 0
}

func (r *releaseReader) Read(dst []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(dst) == 0 {
		return 0, nil
	}
	end := len(r.pcm)
	if r.releaseStart >= 0 {
		end = r.releaseEnd * 2
	}
	n := copy(dst, r.pcm[r.position:end])
	if r.releaseStart >= 0 {
		for sample := max(r.position/2, r.releaseStart); sample*2 < r.position+n; sample++ {
			value := int16(binary.LittleEndian.Uint16(r.pcm[sample*2:]))
			gain := float64(r.releaseEnd-1-sample) / float64(max(1, r.releaseEnd-1-r.releaseStart))
			value = int16(math.Round(float64(value) * gain))
			// Reads need not end on a PCM sample boundary.
			for b := range 2 {
				if offset := sample*2 + b - r.position; offset >= 0 && offset < n {
					dst[offset] = byte(uint16(value) >> (8 * b))
				}
			}
		}
	}
	r.position += n
	if r.position == end {
		return n, io.EOF
	}
	return n, nil
}
