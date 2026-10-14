package app

import (
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

// Every semantic cue shares this gate; individual cue volumes stay unchanged.
func (v *view) playSound(cue sound.Name) {
	if v.sounds != nil && v.settingsValues.Get(settings.CatalogueDefault, settingSoundsEnabled).Bool {
		v.sounds.Play(cue)
	}
}
