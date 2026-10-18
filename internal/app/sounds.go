package app

import (
	"github.com/dannyben/commando/internal/diagnostics"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

type soundSettings struct {
	enabled bool
	profile int
	volume  float64
}

func (v *view) currentSoundSettings() soundSettings {
	c := settings.CatalogueDefault
	return soundSettings{
		enabled: v.settingsValues.Get(c, settingSoundEnabled).Bool,
		profile: int(v.settingsValues.Get(c, settingSoundProfile).Number),
		volume:  v.settingsValues.Get(c, settingSoundVolume).Number,
	}
}

func (v *view) applySoundSettings() {
	current := v.currentSoundSettings()
	if current != v.soundSettings {
		v.soundSettings = current
		if v.sounds != nil {
			v.sounds.Stop()
		}
	}
}

// Every semantic event shares this gate; profile names belong to settings UI.
func (v *view) playSound(cue sound.Name) {
	options := v.currentSoundSettings()
	allowed := v.sounds != nil && options.enabled && options.volume > 0 && sound.Allows(cue, options.profile)
	diagnostics.Record(diagnostics.Feedback, 0, "Sound", map[string]any{"cue": cue, "played": allowed, "profile": options.profile, "volume": options.volume})
	if allowed {
		v.sounds.Play(cue, options.volume)
	}
}

func (v *view) previewSoundSetting(key string) bool {
	if key != settingSoundEnabled && key != settingSoundProfile && key != settingSoundVolume {
		return false
	}
	for _, cue := range []sound.Name{sound.MessageInfo, sound.MessageWarning, sound.MessageError} {
		if sound.Allows(cue, v.currentSoundSettings().profile) {
			v.playSound(cue)
			return true
		}
	}
	return true
}
