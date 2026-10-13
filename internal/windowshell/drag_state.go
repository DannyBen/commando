package windowshell

const (
	dragContinue = 0
	dragDrop     = 0x40100
	dragCancel   = 0x40101
	leftButton   = 1
)

// An armed drag persists with no button down, then drops on completion of the
// next click. Ignore a button held at invocation until it has been released.
type dragState struct {
	standard    bool
	waitRelease bool
	clicked     bool
}

func armedDragKeys(queued uint32, primaryDown bool) uint32 {
	keys := queued &^ leftButton
	if primaryDown {
		keys |= leftButton
	}
	return keys
}

func (s *dragState) dropAt(internal bool) uint32 {
	if internal && !s.standard {
		s.clicked = false
		return dragContinue
	}
	return dragDrop
}

func (s *dragState) next(escape bool, keys uint32) uint32 {
	if escape {
		return dragCancel
	}
	down := keys&leftButton != 0
	if s.standard {
		if !down {
			return dragDrop
		}
		return dragContinue
	}
	if s.waitRelease {
		if !down {
			s.waitRelease = false
		}
		return dragContinue
	}
	if down {
		s.clicked = true
	} else if s.clicked {
		return dragDrop
	}
	return dragContinue
}
