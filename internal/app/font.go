package app

import (
	"strings"

	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/text"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/settings"
)

func bundledFont(data []byte) []font.FontFace {
	faces, err := opentype.ParseCollection(data)
	if err != nil {
		panic(err)
	}
	return faces
}

// Keep the bundled face last in the explicit family list so an unavailable
// choice has a predictable fallback. System fonts supply unsupported scripts,
// including when the bundled family is selected.
func fontShaper(data []byte, family string) (*text.Shaper, font.Typeface) {
	faces := bundledFont(data)
	bundled := faces[0].Font.Typeface
	family = strings.TrimSpace(family)
	if family == "" || strings.EqualFold(family, string(bundled)) {
		return text.NewShaper(text.WithCollection(faces)), bundled
	}
	quote := func(name string) string {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(name) + `"`
	}
	face := font.Typeface(quote(family) + ", " + quote(string(bundled)))
	return text.NewShaper(text.WithCollection(faces)), face
}

func (v *view) applyFontSettings() {
	c := settings.CatalogueDefault
	family := strings.TrimSpace(v.settingsValues.Get(c, settingFontFamily).Text)
	if family != v.appearance.FontFamily {
		v.appearance.FontFamily = family
		v.theme.Shaper, v.theme.Face = fontShaper(v.appearance.Font, family)
	}
	v.appearance.TextSize = unit.Sp(v.settingsValues.Get(c, settingFontTextSize).Number)
	v.appearance.SmallTextSize = unit.Sp(v.settingsValues.Get(c, settingFontSmallTextSize).Number)
	// Gio's zero weight is regular (CSS 400). Keep this as the shared default;
	// semantic styles can override it independently in the future.
	v.appearance.FontWeight = font.Weight(v.settingsValues.Get(c, settingFontWeight).Number - 400)
}
