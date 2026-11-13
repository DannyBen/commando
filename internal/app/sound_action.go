package app

import (
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

// An action collects intentions, not audio. Only its strongest outcome reaches
// the player. Nested commands share ownership; unrelated inputs never share a
// time window or a timer. Playback uses the same priorities across actions.
type soundAction struct {
	cue          sound.Name
	priority     sound.Priority
	widgetClosed bool // Generic cancellation must not replace an actual UI Close.
	handoff      bool // Moving an existing interaction is neither closing nor opening it.
}

func (a *soundAction) offer(cue sound.Name, priority sound.Priority) {
	if sound.Allows(cue, sound.All) && (a.cue == "" || priority <= a.priority) {
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

func (v *view) announceBoolean(enabled bool) {
	if enabled {
		v.concludeSound(interactionSounds[eventEnabled])
	} else {
		v.concludeSound(interactionSounds[eventDisabled])
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
