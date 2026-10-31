package app

import (
	"image/color"

	"github.com/dannyben/commando/internal/settings"
)

const (
	settingActiveHeader = "appearance.theme.active_header_colors"
	settingActiveFooter = "appearance.theme.active_footer_colors"
	settingHeaderOffset = settingActiveHeader + ".lightness_points"
	settingFooterOffset = settingActiveFooter + ".lightness_points"
)

func (v *view) activeBandColors(base color.NRGBA, offsetKey string) (background, text color.NRGBA) {
	value := oklchFromRGB(base)
	value.l = clampLightness(value.l + v.settingsValues.Get(settings.CatalogueDefault, offsetKey).Number/100)
	return value.rgb(), v.appearance.Colors.Surface
}

func restoreLegacyBandOffsets(values settings.Values) settings.Values {
	// Compatibility: themes/settings saved before separate header/footer offsets
	// used the file-selection offset for both. Copy missing values once so later
	// file-color edits stay independent. Remove with legacy theme support.
	values = values.Clone()
	for _, key := range []string{settingHeaderOffset, settingFooterOffset} {
		if !values.Overridden(key) {
			_ = values.Set(settings.CatalogueDefault, key, values.Get(settings.CatalogueDefault, settingSelectionLightness))
		}
	}
	return values
}
