package app

import (
	"image/color"
	"math"

	"github.com/dannyben/commando/internal/settings"
)

const (
	settingThemeEffects = "appearance.theme.effects"
	settingDimmedText   = settingThemeEffects + ".dimmed_text_opacity"
	settingHover        = settingThemeEffects + ".hover_strength"
	settingBackdrop     = settingThemeEffects + ".backdrop_wash"
)

func (v *view) applyThemeEffects() {
	n := func(key string) float64 { return v.settingsValues.Get(settings.CatalogueDefault, key).Number / 100 }
	v.appearance.DimmedTextOpacity = n(settingDimmedText)
	v.appearance.HoverStrength = n(settingHover)
	v.appearance.OverlayBackdropOpacity = float32(n(settingBackdrop))
}

// De-emphasis changes opacity, not the meaning of the base color. Callers use
// the dimmed text contrast role so correction does not restore primary emphasis.
func (v *view) dimmedColor(fg color.NRGBA) color.NRGBA {
	fg.A = uint8(math.Round(float64(fg.A) * v.appearance.DimmedTextOpacity))
	return fg
}
