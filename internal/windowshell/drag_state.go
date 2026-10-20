package windowshell

const (
	dragContinue = 0
	dragDrop     = 0x40100
	dragCancel   = 0x40101
	leftButton   = 1
)

func continueDrag(escape bool, keys uint32) uint32 {
	if escape {
		return dragCancel
	}
	if keys&leftButton == 0 {
		return dragDrop
	}
	return dragContinue
}
