package windowshell

import "testing"

func TestDragContinuesUntilReleaseOrEscape(t *testing.T) {
	for _, tc := range []struct {
		name   string
		escape bool
		keys   uint32
		want   uint32
	}{
		{"held", false, leftButton, dragContinue},
		{"held with modifiers", false, leftButton | 8, dragContinue},
		{"released", false, 0, dragDrop},
		{"released with modifiers", false, 8, dragDrop},
		{"cancel while held", true, leftButton, dragCancel},
		{"cancel on release", true, 0, dragCancel},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := continueDrag(tc.escape, tc.keys); got != tc.want {
				t.Fatalf("got %x, want %x", got, tc.want)
			}
		})
	}
}
