package sound

// Priority governs both pending and active cues. Smaller values win; equal
// priorities prefer the newest event. This is independent of sound profiles.
type Priority uint8

const (
	PriorityError Priority = iota + 1
	PriorityWarning
	PriorityNotification
	PriorityToggle
	PrioritySurface
	PriorityActivation
	PriorityNavigation
)

func (name Name) Priority() Priority {
	switch name {
	case MessageError:
		return PriorityError
	case MessageWarning:
		return PriorityWarning
	case MessageInfo, MessageSuccess:
		return PriorityNotification
	case Enabled, Disabled:
		return PriorityToggle
	case Opened, Closed:
		return PrioritySurface
	case Navigation, DirectoryNavigation:
		return PriorityNavigation
	default:
		return PriorityActivation // Click and specialized cues, including panel sizes.
	}
}
