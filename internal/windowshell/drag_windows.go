package windowshell

import (
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"

	"golang.org/x/sys/windows"
)

var (
	iidSource  = windows.GUID{Data1: 0x121, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
	iidUnknown = windows.GUID{Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
)

type dropSource struct {
	table         *[5]uintptr
	refs          atomic.Int32
	state         dragState
	primaryButton uintptr
}

func (s *dropSource) primaryDown() bool {
	state, _, _ := getAsyncKeyState.Call(s.primaryButton)
	return state&0x8000 != 0
}

// Native clients can retain an interface. Root and pin the source until its
// final COM Release, instead of assuming all references end at SHDoDragDrop.
var sourcePins sync.Map // *dropSource -> *runtime.Pinner

func releaseSource(s *dropSource) uintptr {
	refs := s.refs.Add(-1)
	if refs == 0 {
		if value, ok := sourcePins.LoadAndDelete(s); ok {
			value.(*runtime.Pinner).Unpin()
		}
	}
	return uintptr(refs)
}

var sourceTable = [5]uintptr{
	syscall.NewCallback(func(s *dropSource, iid *windows.GUID, out **dropSource) uintptr {
		if out == nil || iid == nil {
			return 0x80004003
		}
		*out = nil
		if *iid != iidUnknown && *iid != iidSource {
			return 0x80004002
		}
		*out = s
		s.refs.Add(1)
		return 0
	}),
	syscall.NewCallback(func(s *dropSource) uintptr { return uintptr(s.refs.Add(1)) }),
	syscall.NewCallback(releaseSource),
	syscall.NewCallback(func(s *dropSource, escape, keys uintptr) uintptr {
		buttons := uint32(keys)
		if !s.state.standard {
			// Gio and OLE consume the same native message queue. For armed
			// dragging, queued flags must not masquerade as a fresh click.
			buttons = armedDragKeys(buttons, s.primaryDown())
		}
		return uintptr(s.state.next(escape != 0, buttons))
	}),
	syscall.NewCallback(func(_ *dropSource, _ uintptr) uintptr {
		return 0x40102 // DRAGDROP_S_USEDEFAULTCURSORS.
	}),
}
