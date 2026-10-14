package app

import (
	"io/fs"
	"slices"
	"strings"

	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/text"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/settings"
)

func bundledFonts(files fs.FS) []font.FontFace {
	names, err := fs.Glob(files, "fonts/*.ttf")
	if err != nil {
		panic(err)
	}
	var faces []font.FontFace
	for _, name := range names {
		data, err := fs.ReadFile(files, name)
		if err != nil {
			panic(err)
		}
		collection, err := opentype.ParseCollection(data)
		if err != nil {
			panic(err)
		}
		faces = append(faces, collection...)
	}
	// Gio uses the first face for empty lines as well as the default family.
	slices.SortStableFunc(faces, func(a, b font.FontFace) int {
		if a.Font.Weight == font.Normal && b.Font.Weight != font.Normal {
			return -1
		}
		if b.Font.Weight == font.Normal && a.Font.Weight != font.Normal {
			return 1
		}
		return int(a.Font.Weight - b.Font.Weight)
	})
	return faces
}

// Keep the bundled face last in the explicit family list so an unavailable
// choice has a predictable fallback. System fonts supply unsupported scripts,
// including when the bundled family is selected.
func fontShaper(faces []font.FontFace, family string) (*text.Shaper, font.Typeface) {
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
