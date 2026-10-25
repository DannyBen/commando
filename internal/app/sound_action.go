package app

import (
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

// An action collects intentions, not audio. Only its strongest outcome reaches
// the player. Nested commands share ownership; unrelated inputs never share a
// time window or a timer. The player replaces active audio rather than mixing.
type soundAction struct {
	cue          sound.Name
	priority     int
	widgetClosed bool // Generic cancellation must not replace an actual UI Close.
	handoff      bool // Moving an existing interaction is neither closing nor opening it.
}

func (a *soundAction) offer(cue sound.Name, priority int) {
	if sound.Allows(cue, sound.All) && (a.cue == "" || priority >= a.priority) {
		a.cue, a.priority = cue, priority
	}
}

func (v *view) beginSoundAction() func() {
	if v.soundAction != nil {
		return func() {}
	}
	action := &soundAction{}
	v.soundAction = action
	return func() {
		v.soundAction = nil
		if action.cue != "" {
			v.playSound(action.cue)
		}
	}
}

func soundPriority(cue sound.Name) int {
	switch cue {
	case sound.MessageError:
		return 120
	case sound.MessageWarning:
		return 110
	case sound.Enabled, sound.Disabled:
		return 90
	case sound.Opened, sound.Closed:
		return 70
	case sound.MessageSuccess:
		return 85
	case sound.MessageInfo:
		return 80
	case sound.DirectoryNavigation:
		return 40
	case sound.Navigation:
		return 20
	case sound.Click:
		return 10
	default:
		return 60 // Specialized cues, including panel-size pitches.
	}
}

func (v *view) announceBoolean(enabled bool) {
	if enabled {
		v.announceInteraction(eventEnabled)
	} else {
		v.announceInteraction(eventDisabled)
	}
}

// Both widgets and direct commands use the field's declared type. New boolean
// fields automatically join the on/off policy without adding feature sounds.
func (v *view) announceSettingChange(key string, value settings.Value) {
	definition, ok := settings.CatalogueDefault.Definition(key)
	if ok && definition.Kind == settings.Boolean {
		v.announceBoolean(value.Bool)
	} else {
		v.announceInteraction(eventValueChanged)
	}
}
