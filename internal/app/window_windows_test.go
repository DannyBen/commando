package app

import (
	"testing"
	"unsafe"
)

func TestPassivePointerUpdate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		message uint32
		flags   uintptr
		want    bool
	}{
		{"hover", 0x0245, 0x2003, true},
		{"contact", 0x0245, 0x0004, false},
		{"left drag", 0x0245, 0x0010, false},
		{"right drag", 0x0245, 0x0020, false},
		{"middle drag", 0x0245, 0x0040, false},
		{"fourth button", 0x0245, 0x0080, false},
		{"fifth button", 0x0245, 0x0100, false},
		{"cancel", 0x0245, 0x0800, false},
		{"press", 0x0246, 0, false},
		{"release", 0x0247, 0, false},
		{"capture changed", 0x024c, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := passivePointerUpdate(tc.message, tc.flags<<16|7); got != tc.want {
				t.Fatalf("passivePointerUpdate = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWindowsPlacementRoundTrip(t *testing.T) {
	want := windowState{X: -1300, Y: 56, Width: 900, Height: 500, Positioned: true}
	for _, maximized := range []bool{false, true} {
		want.Maximized = maximized
		placement := want.placement()
		if placement.Length != 44 || placement.Length != uint32(unsafe.Sizeof(placement)) {
			t.Fatal("WINDOWPLACEMENT does not match the native ABI")
		}
		if got := placement.state(); got != want {
			t.Fatalf("normal/maximized placement changed: got=%+v want=%+v", got, want)
		}
		placement.ShowCmd = 2
		if maximized {
			placement.Flags = 2
		}
		if got := placement.state(); got != want {
			t.Fatalf("minimized placement lost its restored state: got=%+v want=%+v", got, want)
		}
		if restored := placement.state().placement(); restored.ShowCmd == 2 {
			t.Fatal("a closed minimized window reopens minimized")
		}
	}
}

func TestNativeAltEnterIsConsumed(t *testing.T) {
	const alt = 1 << 29
	for _, data := range []uintptr{
		alt | 1,                         // Press.
		alt | 1 | 1<<30,                 // Repeat.
		alt | 1 | 1<<30 | 1<<31,         // Release.
		alt | 1 | 1<<24,                 // Keypad Enter.
		alt | 1 | 1<<24 | 1<<30 | 1<<31, // Keypad release.
	} {
		for _, code := range []int32{0, 3} { // GetMessage and PeekMessage(PM_NOREMOVE).
			if got := shortcutKeyboardProc(code, 0x0d, data); got != 1 {
				t.Fatalf("Alt+Enter reached the next hook: code=%d data=%#x result=%d", code, data, got)
			}
		}
	}
}

func TestNativeAltEnterLeavesOtherKeysAlone(t *testing.T) {
	for _, tc := range []struct {
		code      int32
		key, data uintptr
	}{
		{0, 0x0d, 1},          // Plain Enter.
		{0, 'D', 1 | 1<<29},   // Alt+D.
		{0, 0x73, 1 | 1<<29},  // Alt+F4.
		{-1, 0x0d, 1 | 1<<29}, // Hooks must forward negative codes.
	} {
		if got := shortcutKeyboardProc(tc.code, tc.key, tc.data); got != 0 {
			t.Fatalf("unreserved key was consumed: %+v result=%d", tc, got)
		}
	}
}
