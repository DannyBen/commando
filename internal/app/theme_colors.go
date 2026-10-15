package app

import (
	"image/color"

	"github.com/dannyben/commando/internal/settings"
)

func themeColorsFromSettings(values settings.Values) ThemeColors {
	values = values.Clone()
	settings.ClampThemeColors(&values, settings.CatalogueDefault)
	get := func(role string) color.NRGBA {
		n := func(component string) float64 {
			return values.Get(settings.CatalogueDefault, "appearance.theme."+role+"."+component).Number
		}
		return oklchColor{n("lightness"), n("chroma"), n("hue")}.rgb()
	}
	c := ThemeColors{
		Surface: get("background"), Header: get("alt_background"), Border: get("border"),
		Foreground: get("text"), Muted: get("secondary_text"), Edge: get("edge"), Marked: get("marked"),
		Selected: get("selection"), SelectedName: get("selected_text"), TextSelection: get("text_selection"),
		CutFilename: get("cut_text"), SelectedCutFilename: get("selected_cut_text"),
		Info: get("info"), Success: get("success"), Warning: get("warning"), Error: get("error"),
	}
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
