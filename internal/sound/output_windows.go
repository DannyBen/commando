package sound

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	winmm         = windows.NewLazySystemDLL("winmm.dll")
	waveOpen      = winmm.NewProc("waveOutOpen")
	wavePrepare   = winmm.NewProc("waveOutPrepareHeader")
	waveWrite     = winmm.NewProc("waveOutWrite")
	waveReset     = winmm.NewProc("waveOutReset")
	waveUnprepare = winmm.NewProc("waveOutUnprepareHeader")
	waveClose     = winmm.NewProc("waveOutClose")
	// Retain pinned memory if a broken driver refuses to release it.
	unreleasedOutputs sync.Map
)

// Native WAVEFORMATEX and WAVEHDR layouts from mmeapi.h.
type waveFormat struct {
	format, channels   uint16
	rate, bytes        uint32
	align, bits, extra uint16
}

type waveHeader struct {
	data             *int16
	length, recorded uint32
	user             uintptr
	flags, loops     uint32
	next, reserved   uintptr
}

type output struct {
	handle  uintptr
	current *waveBuffer
}

type waveBuffer struct {
	header waveHeader
	pin    runtime.Pinner
}

func openOutput() (*output, error) {
	o := &output{}
	format := waveFormat{format: 1, channels: 1, rate: sampleRate, bytes: sampleRate * 2, align: 2, bits: 16}
	result, _, _ := waveOpen.Call(uintptr(unsafe.Pointer(&o.handle)), 0xffffffff, uintptr(unsafe.Pointer(&format)), 0, 0, 0)
	if result != 0 {
		return nil, fmt.Errorf("open audio: %d", result)
	}
	return o, nil
}

func (o *output) play(pcm []int16) error {
	if err := o.release(); err != nil {
		return err
	}
	if len(pcm) == 0 {
		return nil
	}
	// The driver retains both pointers after write returns. Pin the header
	// and samples until reset and unprepare have released the buffer.
	b := &waveBuffer{header: waveHeader{data: &pcm[0], length: uint32(len(pcm) * 2)}}
	b.pin.Pin(&b.header)
	b.pin.Pin(&pcm[0])
	result, _, _ := wavePrepare.Call(o.handle, uintptr(unsafe.Pointer(&b.header)), unsafe.Sizeof(b.header))
	if result != 0 {
		b.pin.Unpin()
		return fmt.Errorf("prepare audio: %d", result)
	}
	o.current = b
	result, _, _ = waveWrite.Call(o.handle, uintptr(unsafe.Pointer(&b.header)), unsafe.Sizeof(b.header))
	if result != 0 {
		return fmt.Errorf("play audio: %d", result)
	}
	return nil
}

func (o *output) release() error {
	if o.current == nil {
		return nil
	}
	result, _, _ := waveReset.Call(o.handle)
	if result != 0 {
		return fmt.Errorf("reset audio: %d", result)
	}
	b := o.current
	result, _, _ = waveUnprepare.Call(o.handle, uintptr(unsafe.Pointer(&b.header)), unsafe.Sizeof(b.header))
	if result != 0 {
		return fmt.Errorf("release audio: %d", result)
	}
	b.pin.Unpin()
	o.current = nil
	return nil
}

func (o *output) close() {
	// Do not free a buffer if the driver has failed to release it.
	if o.release() == nil {
		waveClose.Call(o.handle)
	} else {
		unreleasedOutputs.Store(o, true)
	}
}
