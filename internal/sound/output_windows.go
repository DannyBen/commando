//go:build windows && amd64

package sound

import (
	"fmt"
	"github.com/dannyben/commando/internal/diagnostics"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

// Adapted from uss-go/internal/latency, tested with Commando assets on Windows.
// COM vtable layouts are defined by audioclient.h and mmdeviceapi.h.
type comObject struct{ table *[21]uintptr }

// Fixed arguments avoid the heap allocation of an escaping variadic slice.
// Windows x64 callees ignore unused argument slots (this file excludes x86).
//
//go:uintptrescapes
func invoke(object *comObject, index int, a, b, c, d, e, f uintptr) uint32 {
	result, _, _ := syscall.SyscallN(object.table[index], uintptr(unsafe.Pointer(object)), a, b, c, d, e, f)
	runtime.KeepAlive(object)
	return uint32(result)
}

func check(operation string, hr uint32) error {
	if int32(hr) < 0 {
		return fmt.Errorf("%s: HRESULT 0x%08X", operation, hr)
	}
	return nil
}
func release(object *comObject) {
	if object != nil {
		invoke(object, 2, 0, 0, 0, 0, 0, 0)
	}
}

var (
	ole32             = syscall.NewLazyDLL("ole32.dll")
	coInitialize      = ole32.NewProc("CoInitializeEx")
	coUninitialize    = ole32.NewProc("CoUninitialize")
	coCreate          = ole32.NewProc("CoCreateInstance")
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	createEvent       = kernel32.NewProc("CreateEventW")
	waitForSingle     = kernel32.NewProc("WaitForSingleObject")
	closeHandle       = kernel32.NewProc("CloseHandle")
	avrt              = syscall.NewLazyDLL("avrt.dll")
	setAudioThread    = avrt.NewProc("AvSetMmThreadCharacteristicsW")
	revertAudioThread = avrt.NewProc("AvRevertMmThreadCharacteristics")
	clsidEnumerator   = syscall.GUID{Data1: 0xbcde0395, Data2: 0xe52f, Data3: 0x467c, Data4: [8]byte{0x8e, 0x3d, 0xc4, 0x57, 0x92, 0x91, 0x69, 0x2e}}
	iidEnumerator     = syscall.GUID{Data1: 0xa95664d2, Data2: 0x9614, Data3: 0x4f35, Data4: [8]byte{0xa7, 0x46, 0xde, 0x8d, 0xb6, 0x36, 0x17, 0xe6}}
	iidClient         = syscall.GUID{Data1: 0x1cb9ad4c, Data2: 0xdbfa, Data3: 0x4c32, Data4: [8]byte{0xb1, 0x78, 0xc2, 0xf5, 0x68, 0xa7, 0x03, 0xb2}}
	iidClient3        = syscall.GUID{Data1: 0x7ed4ee07, Data2: 0x8e67, Data3: 0x4cd4, Data4: [8]byte{0x8c, 0x1a, 0x2b, 0x7a, 0x59, 0x87, 0xad, 0x42}}
	iidRender         = syscall.GUID{Data1: 0xf294acfc, Data2: 0x3146, Data3: 0x4483, Data4: [8]byte{0xa7, 0xbf, 0xad, 0xdc, 0xa7, 0xc2, 0x60, 0xe2}}
	floatSubtype      = syscall.GUID{Data1: 3, Data3: 0x10, Data4: [8]byte{0x80, 0, 0, 0xaa, 0, 0x38, 0x9b, 0x71}}
)

type waveFormat struct {
	tag, channels                     uint16
	rate, bytesPerSecond              uint32
	alignment, bits, extra, validBits uint16
	mask                              uint32
	subtype                           syscall.GUID
}

var _ [40]byte = [unsafe.Sizeof(waveFormat{})]byte{}

func floatFormat() *waveFormat {
	return &waveFormat{tag: 0xfffe, channels: 2, rate: 48000, bytesPerSecond: 48000 * 8, alignment: 8, bits: 32, extra: 22, validBits: 32, mask: 3, subtype: floatSubtype}
}

type deviceInfo struct {
	backend, fallback string
	period, capacity  uint32
	latency           time.Duration
	mmcss             bool
}
type device struct {
	client, render *comObject
	event          uintptr
	info           deviceInfo
	padding        uint32
	data           *float32
}

func activate(endpoint *comObject) (*comObject, error) {
	var client *comObject
	err := check("activate audio client", invoke(endpoint, 3, uintptr(unsafe.Pointer(&iidClient)), 23, 0, uintptr(unsafe.Pointer(&client)), 0, 0))
	return client, err
}
func openDevice(requested int) (*device, error) {
	var enumerator, endpoint *comObject
	hr, _, _ := coCreate.Call(uintptr(unsafe.Pointer(&clsidEnumerator)), 0, 23, uintptr(unsafe.Pointer(&iidEnumerator)), uintptr(unsafe.Pointer(&enumerator)))
	if err := check("create audio device enumerator", uint32(hr)); err != nil {
		return nil, err
	}
	defer release(enumerator)
	if err := check("default multimedia output", invoke(enumerator, 4, 0, 1, uintptr(unsafe.Pointer(&endpoint)), 0, 0, 0)); err != nil {
		return nil, err
	}
	defer release(endpoint)
	client, err := activate(endpoint)
	if err != nil {
		return nil, err
	}
	d := &device{client: client}
	success := false
	defer func() {
		if !success {
			d.close()
		}
	}()
	format := floatFormat()
	var modern *comObject
	hr3 := invoke(client, 0, uintptr(unsafe.Pointer(&iidClient3)), uintptr(unsafe.Pointer(&modern)), 0, 0, 0, 0)
	if int32(hr3) >= 0 {
		var normal, fundamental, minimum, maximum uint32
		hr3 = invoke(modern, 18, uintptr(unsafe.Pointer(format)), uintptr(unsafe.Pointer(&normal)), uintptr(unsafe.Pointer(&fundamental)), uintptr(unsafe.Pointer(&minimum)), uintptr(unsafe.Pointer(&maximum)), 0)
		if int32(hr3) >= 0 {
			selected, err := choosePeriod(requested, fundamental, minimum, maximum)
			if err != nil {
				release(modern)
				return nil, err
			}
			hr3 = invoke(modern, 20, 0x00040000, uintptr(selected), uintptr(unsafe.Pointer(format)), 0, 0, 0)
			if int32(hr3) >= 0 {
				release(client)
				d.client = modern
				d.info.backend = "WASAPI IAudioClient3 shared/event"
				d.info.period = selected
			}
		}
		if d.info.backend == "" {
			release(modern)
		}
	}
	if d.info.backend == "" {
		d.info.fallback = fmt.Sprintf("low-period 48 kHz stereo initialization unavailable (0x%08X)", hr3)
		// Use a fresh client after a failed initialization attempt. Windows performs
		// format conversion here if the default device is not running at 48 kHz.
		release(d.client)
		d.client = nil
		d.client, err = activate(endpoint)
		if err != nil {
			return nil, err
		}
		if err = check("initialize standard shared audio", invoke(d.client, 3, 0, 0x00040000|0x80000000|0x08000000, 0, 0, uintptr(unsafe.Pointer(format)), 0)); err != nil {
			return nil, err
		}
		var normal, minimum int64
		if err = check("device period", invoke(d.client, 9, uintptr(unsafe.Pointer(&normal)), uintptr(unsafe.Pointer(&minimum)), 0, 0, 0, 0)); err != nil {
			return nil, err
		}
		d.info.period = uint32(normal * 48000 / 10000000)
		d.info.backend = "WASAPI standard shared/event (format conversion enabled)"
	}
	if err = check("audio buffer size", invoke(d.client, 4, uintptr(unsafe.Pointer(&d.info.capacity)), 0, 0, 0, 0, 0)); err != nil {
		return nil, err
	}
	if d.info.capacity == 0 || d.info.capacity > 48000 {
		return nil, fmt.Errorf("unsupported device buffer size %d", d.info.capacity)
	}
	var latency int64
	if err = check("reported stream latency", invoke(d.client, 5, uintptr(unsafe.Pointer(&latency)), 0, 0, 0, 0, 0)); err != nil {
		return nil, err
	}
	d.info.latency = time.Duration(latency) * 100
	d.event, _, err = createEvent.Call(0, 0, 0, 0)
	if d.event == 0 {
		return nil, fmt.Errorf("create audio event: %w", err)
	}
	if err = check("audio event handle", invoke(d.client, 13, d.event, 0, 0, 0, 0, 0)); err != nil {
		return nil, err
	}
	if err = check("audio render service", invoke(d.client, 14, uintptr(unsafe.Pointer(&iidRender)), uintptr(unsafe.Pointer(&d.render)), 0, 0, 0, 0)); err != nil {
		return nil, err
	}
	success = true
	return d, nil
}
func (d *device) close() {
	if d.client != nil {
		invoke(d.client, 11, 0, 0, 0, 0, 0, 0)
	}
	release(d.render)
	release(d.client)
	d.render, d.client = nil, nil
	if d.event != 0 {
		closeHandle.Call(d.event)
		d.event = 0
	}
}
func (d *device) buffer(frames uint32) ([]float32, error) {
	if err := check("get render buffer", invoke(d.render, 3, uintptr(frames), uintptr(unsafe.Pointer(&d.data)), 0, 0, 0, 0)); err != nil {
		return nil, err
	}
	return unsafe.Slice(d.data, int(frames)*2), nil
}
func (d *device) submit(frames uint32, silent bool) error {
	flags := uintptr(0)
	if silent {
		flags = 2
	}
	err := check("submit render buffer", invoke(d.render, 4, uintptr(frames), flags, 0, 0, 0, 0))
	d.data = nil
	return err
}
func prioritize() func() {
	if setAudioThread.Find() != nil {
		return nil
	}
	name, _ := syscall.UTF16PtrFromString("Pro Audio")
	var task uint32
	handle, _, _ := setAudioThread.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&task)))
	if handle == 0 {
		return nil
	}
	return func() { revertAudioThread.Call(handle) }
}
func runAudio(commands <-chan *voice, stop <-chan struct{}, stats *audioStatistics) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	r := &audioRenderer{commands: commands}
	defer r.close()
	hr, _, _ := coInitialize.Call(0, 0)
	if err := check("initialize audio COM", uint32(hr)); err != nil {
		return err
	}
	defer coUninitialize.Call()
	d, err := openDevice(0)
	if err != nil {
		return err
	}
	defer d.close()
	if restore := prioritize(); restore != nil {
		d.info.mmcss = true
		defer restore()
	}
	// Log before starting audio; never perform trace writes in the refill loop.
	diagnostics.Record(diagnostics.Feedback, 0, "AudioOpened", d.info.traceData())
	buffer, err := d.buffer(d.info.capacity)
	if err != nil {
		return err
	}
	clear(buffer)
	if err = d.submit(d.info.capacity, true); err != nil {
		return err
	}
	if err = check("start audio", invoke(d.client, 10, 0, 0, 0, 0, 0, 0)); err != nil {
		return err
	}
	waitAddress := waitForSingle.Addr()
	previous := time.Now()
	wasActive := false
	for {
		select {
		case <-stop:
			return nil
		default:
		}
		// The timeout bounds shutdown if a disconnected device stops signaling.
		result, _, callErr := syscall.SyscallN(waitAddress, d.event, 50)
		if result == 258 {
			if time.Since(previous) > time.Second {
				return fmt.Errorf("no audio event for one second")
			}
			continue
		}
		if result != 0 {
			return fmt.Errorf("wait for audio: %v (0x%X)", callErr, result)
		}
		now := time.Now()
		if err = check("audio padding", invoke(d.client, 6, uintptr(unsafe.Pointer(&d.padding)), 0, 0, 0, 0, 0)); err != nil {
			return err
		}
		if d.padding > d.info.capacity {
			return fmt.Errorf("invalid device padding %d", d.padding)
		}
		stats.wake(now.Sub(previous), wasActive && d.padding == 0)
		previous = now
		frames := d.info.capacity - d.padding
		if frames == 0 {
			continue
		}
		buffer, err = d.buffer(frames)
		if err != nil {
			return err
		}
		if err = r.render(buffer); err != nil {
			d.submit(frames, true)
			return err
		}
		wasActive = r.current != nil
		if err = d.submit(frames, false); err != nil {
			return err
		}
	}
}

func (i deviceInfo) traceData() map[string]any {
	return map[string]any{"backend": i.backend, "fallback": i.fallback,
		"period_frames": i.period, "capacity_frames": i.capacity,
		"latency_us": i.latency.Microseconds(), "mmcss": i.mmcss}
}
