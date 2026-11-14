package sound

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
	"time"
)

func constantPCM(length time.Duration) []byte {
	pcm := make([]byte, samples(length)*2)
	for offset := 0; offset < len(pcm); offset += 2 {
		binary.LittleEndian.PutUint16(pcm[offset:], 20000)
	}
	return pcm
}

func TestReplacementReleaseIsContinuousBoundedAndDoesNotChangeCachedPCM(t *testing.T) {
	for _, chunk := range []int{1, 3, 512, 513} {
		pcm := constantPCM(200 * time.Millisecond)
		before := bytes.Clone(pcm)
		r := newReleaseReader(pcm)
		prefix := make([]byte, 513) // Replacement in the middle of a PCM sample.
		if _, err := io.ReadFull(r, prefix); err != nil {
			t.Fatal(err)
		}
		if !r.release() {
			t.Fatal("started sound was treated as unheard")
		}
		var tail bytes.Buffer
		buffer := make([]byte, chunk)
		for {
			n, err := r.Read(buffer)
			tail.Write(buffer[:n])
			if err == io.EOF {
				break
			}
			if err != nil || n == 0 {
				t.Fatal("release did not finish", err)
			}
			r.release() // Repeated release must not restart or extend the envelope.
		}
		output := append(prefix, tail.Bytes()...)
		start := (len(prefix) + 1) / 2
		if len(output) != (start+samples(replacementRelease))*2 || !bytes.Equal(pcm, before) {
			t.Fatal("release length changed or cached PCM was modified", chunk)
		}
		previous := 20000
		for offset := 0; offset < len(output); offset += 2 {
			value := int(int16(binary.LittleEndian.Uint16(output[offset:])))
			if value < 0 || value > previous || previous-value > 100 {
				t.Fatal("release introduced an abrupt cut or broken sample", chunk, offset, value)
			}
			previous = value
		}
		if previous != 0 {
			t.Fatal("release did not reach silence")
		}
	}
}

func TestReleasePreservesNaturalPlaybackAndNeverExtendsOrStartsACue(t *testing.T) {
	pcm := constantPCM(10 * time.Millisecond)
	r := newReleaseReader(pcm)
	if got, err := io.ReadAll(r); err != nil || !bytes.Equal(got, pcm) {
		t.Fatal("ordinary playback changed", err)
	}
	r = newReleaseReader(pcm)
	if r.release() {
		t.Fatal("unheard sound acquired a release tail")
	}
	if got, err := io.ReadAll(r); err != nil || len(got) != 0 {
		t.Fatal("an unheard sound started during release", err)
	}
	r = newReleaseReader(pcm)
	prefix := make([]byte, 2)
	if _, err := io.ReadFull(r, prefix); err != nil {
		t.Fatal(err)
	}
	r.release()
	tail, err := io.ReadAll(r)
	if err != nil || len(prefix)+len(tail) != len(pcm) || binary.LittleEndian.Uint16(tail[len(tail)-2:]) != 0 {
		t.Fatal("release extended the original cue or failed to reach silence", err)
	}
}

func TestConcurrentAudioReadAndRelease(t *testing.T) {
	r := newReleaseReader(constantPCM(time.Second))
	ready, start := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		buffer := make([]byte, 513)
		_, err := io.ReadFull(r, buffer)
		close(ready)
		<-start
		if err == nil {
			_, err = io.Copy(io.Discard, r)
		}
		done <- err
	}()
	<-ready
	close(start)
	for range 100 {
		r.release()
	}
	if err := <-done; err != nil {
		t.Fatal("audio read failed during release", err)
	}
}
