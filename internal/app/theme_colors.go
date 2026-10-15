package app

import (
	"image/color"
	"strings"

	"github.com/dannyben/commando/internal/settings"
)

func themeColorsFromSettings(values settings.Values) ThemeColors {
	get := func(role string) color.NRGBA { return themeColorValue(values, themeSettingPrefix+role) }
	c := ThemeColors{
		Surface: get("background"), Header: get("alt_background"), Border: get("border"),
		Foreground: get("text"), Muted: get("secondary_text"), Edge: get("edge"),
		Selected: get("selection"), TextSelection: get("text_selection"),
		Info: get("info"), Success: get("success"), Warning: get("warning"), Error: get("error"),
	}
	c.SelectedName = selectionTextColor(c.Foreground, c.Selected)
	c.Footer = c.Header
	c.Filename, c.Metadata = c.Foreground, c.Muted
	c.ActiveLocation = c.Edge
	c.SelectedMetadata = c.SelectedName
	c.NameRow, c.EditorText = c.Surface, c.Foreground
	c.InlineSurface, c.InlineText, c.InlineEdge = c.Header, c.Foreground, c.Edge
	c.InlineInactiveSelected = c.Header
	c.Scrollbar, c.ScrollbarHover = c.Border, c.Edge
	return c
}

func themeColorValue(values settings.Values, prefix string) color.NRGBA {
	return themeColorCoordinates(values, prefix).oklch().rgb()
}

func themeColorCoordinates(values settings.Values, prefix string) lshColor {
	n := func(component string) float64 {
		return values.Get(settings.CatalogueDefault, prefix+"."+component).Number
	}
	return lshColor{n("lightness"), n("saturation"), n("hue")}
}

func (v *view) settingSwatch(key string) (color.NRGBA, bool) {
	prefix, hue := strings.CutSuffix(key, ".hue")
	if !hue || !strings.HasPrefix(prefix, themeSettingPrefix) {
		return color.NRGBA{}, false
	}
	return themeColorValue(v.settingsValues, prefix), true
}

// Preserve LSH coordinates directly, including hue/saturation on achromatic
// colors. Sampling rendered RGB here would lose those editable coordinates.
func themePalette(values settings.Values) map[string]lshColor {
	result := make(map[string]lshColor)
	for _, spec := range themeElements() {
		result[strings.TrimPrefix(spec.Key, themeSettingPrefix)] = themeColorCoordinates(values, spec.Key)
	}
	return result
}
