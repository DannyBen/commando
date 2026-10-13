package windowshell

import "testing"

func TestArmedInternalClickLeavesNextExternalClickAvailable(t *testing.T) {
	s := dragState{}
	for range 2 {
		s.next(false, leftButton)
		if s.next(false, 0) != dragDrop || s.dropAt(true) != dragContinue {
			t.Fatal("internal click ended armed drag")
		}
		if s.next(false, 0) != dragContinue {
			t.Fatal("moving after ignored click ended armed drag")
		}
	}
	s.next(false, leftButton)
	if s.next(false, 0) != dragDrop || s.dropAt(false) != dragDrop || s.next(true, 0) != dragCancel {
		t.Fatal("external drop or Escape changed after ignored internal clicks")
	}
	standard := dragState{standard: true}
	if standard.dropAt(true) != dragDrop {
		t.Fatal("regular drag acquired armed behavior")
	}
}

func TestArmedDragIgnoresQueuedButtonFlags(t *testing.T) {
	s := dragState{}
	// Stale press/release flags and movement with the physical button up must
	// never arm completion. Only a real new press followed by release drops.
	for _, queued := range []uint32{leftButton, 0, leftButton | 8, 8, 0} {
		if s.next(false, armedDragKeys(queued, false)) != dragContinue {
			t.Fatal("queued mouse state completed armed drag without a click")
		}
	}
	if s.next(false, armedDragKeys(0, true)) != dragContinue ||
		s.next(false, armedDragKeys(leftButton, false)) != dragDrop {
		t.Fatal("physical press/release failed to complete armed drag")
	}
}

func TestArmedDragWaitsForACompleteNewClick(t *testing.T) {
	s := dragState{}
	// No button, modifier transitions and switching windows cannot complete it.
	for _, keys := range []uint32{0, 0x20, 0, 8, 0} {
		if got := s.next(false, keys); got != dragContinue {
			t.Fatalf("premature drop for keys %x", keys)
		}
	}
	if s.next(false, 1) != dragContinue || s.next(false, 1|8) != dragContinue || s.next(false, 0) != dragDrop {
		t.Fatal("next press/release did not complete the armed drag")
	}
}

func TestInvocationClickDoesNotDrop(t *testing.T) {
	s := dragState{waitRelease: true}
	for _, keys := range []uint32{1, 0, 0, 8, 0, 1} {
		if s.next(false, keys) != dragContinue {
			t.Fatal("invocation release or next press dropped")
		}
	}
	if s.next(false, 0) != dragDrop {
		t.Fatal("new click release did not drop")
	}
}

func TestEscapeCancelsBeforeOrDuringClick(t *testing.T) {
	for _, state := range []dragState{{}, {clicked: true}, {waitRelease: true}, {standard: true}} {
		if state.next(true, 1) != dragCancel {
			t.Fatal("Escape failed to cancel")
		}
	}
}

func TestStandardDragUsesButtonRelease(t *testing.T) {
	s := dragState{standard: true}
	if s.next(false, 1) != dragContinue || s.next(false, 0) != dragDrop {
		t.Fatal("held-drag baseline changed")
	}
}
