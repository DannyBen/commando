package settings

import (
	"math"
	"strings"
	"testing"
)

func TestLinkedColorRangesUseActualValues(t *testing.T) {
	c := CatalogueDefault
	var v Values
	set := func(key string, n float64) {
		t.Helper()
		if err := v.Set(c, themePrefix+key, Value{Number: n}); err != nil {
			t.Fatal(err)
		}
	}
	set("background.lightness", 0.4)
	set("alt_background.lightness", 0.9)
	ClampThemeColors(&v, c)
	if got := v.Get(c, themePrefix+"alt_background.lightness").Number; math.Abs(got-0.52) > 1e-9 {
		t.Fatalf("child not clamped: %g", got)
	}
	r, _ := ThemeColorRange(v, c, themePrefix+"alt_background.lightness")
	if math.Abs(r.Minimum-0.28) > 1e-9 || math.Abs(r.Maximum-0.52) > 1e-9 {
		t.Fatalf("wrong actual range: %+v", r)
	}
	_ = v.Set(c, themePrefix+"alt_background.linked", Value{Bool: false})
	ClampThemeColors(&v, c)
	if got := v.Get(c, themePrefix+"alt_background.lightness").Number; math.Abs(got-0.52) > 1e-9 {
		t.Fatal("unlink changed color")
	}
	set("alt_background.lightness", 0.8)
	_ = v.Set(c, themePrefix+"alt_background.linked", Value{Bool: true})
	ClampThemeColors(&v, c)
	if got := v.Get(c, themePrefix+"alt_background.lightness").Number; math.Abs(got-0.52) > 1e-9 {
		t.Fatal("enabling link did not clamp")
	}
	set("text.lightness", 0.8)
	set("cut_text.lightness", 0.2)
	ClampThemeColors(&v, c)
	if got := v.Get(c, themePrefix+"cut_text.lightness").Number; math.Abs(got-0.45) > 1e-9 {
		t.Fatalf("chain used unresolved parent: %g", got)
	}
}

func TestColorHueBoundsCrossZero(t *testing.T) {
	r := ColorRange{Minimum: 340, Maximum: 380, Circular: true}
	for _, tc := range []struct{ in, want float64 }{{350, 350}, {10, 10}, {330, 340}, {30, 20}, {180, 20}} {
		if got := r.Clamp(tc.in); got != tc.want {
			t.Fatalf("clamp %g = %g, want %g", tc.in, got, tc.want)
		}
	}
	if r.Step(359, 1) != 0 || r.Step(0, -1) != 359 || r.Step(20, 1) != 20 || r.Step(340, -1) != 340 {
		t.Fatal("hue stepping escaped the arc")
	}
	if r.Progress(0) != 0.5 || r.Progress(20) != 1 {
		t.Fatal("hue progress did not follow the arc")
	}
	full := ColorRange{0, 360, true}
	if full.Step(359, 1) != 0 || full.Step(0, -1) != 359 {
		t.Fatal("independent hue did not wrap")
	}
}

func TestColorLinksSupportInversionExactAndIndependentComponents(t *testing.T) {
	source := `[alt_background.lightness]
source = "background"
scale = [-1.0, -1.0]
offset = [1.0, 1.0]
`
	links, err := parseColorLinks(source, CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	var values Values
	_ = values.Set(CatalogueDefault, themePrefix+"background.lightness", Value{Number: 0.3})
	r, _ := links.bounds(values, CatalogueDefault, themePrefix+"alt_background.lightness")
	if r.Minimum != 0.7 || r.Maximum != 0.7 || r.Clamp(0.2) != 0.7 || r.Step(0.7, 0.01) != 0.7 {
		t.Fatal("exact inverse bound failed")
	}
	chroma, _ := links.bounds(values, CatalogueDefault, themePrefix+"alt_background.chroma")
	if chroma.Minimum != 0 || chroma.Maximum != 0.5 {
		t.Fatal("missing component link was not independent")
	}
	for _, bad := range []string{
		strings.Replace(source, "background\"", "missing\"", 1),
		strings.Replace(source, "[-1.0, -1.0]", "[1.0, -1.0]", 1),
		strings.Replace(source, "[1.0, 1.0]", "[nan, 1.0]", 1),
		source + "misspelled = 1\n",
		source + `[background.lightness]
source = "alt_background"
scale = [1.0,1.0]
offset = [0.0,0.0]
`,
	} {
		if _, err := parseColorLinks(bad, CatalogueDefault); err == nil {
			t.Fatal("accepted invalid links")
		}
	}
	cycle := strings.Replace(source, "source = \"background\"", "source = \"border\"", 1) + `[border.lightness]
source = "alt_background"
scale = [1.0,1.0]
offset = [0.0,0.0]
`
	if _, err := parseColorLinks(cycle, CatalogueDefault); err == nil {
		t.Fatal("accepted component cycle")
	}
}

func TestDefaultColorLinksNeedNoOverrides(t *testing.T) {
	var values Values
	ClampThemeColors(&values, CatalogueDefault)
	if len(values.overrides) != 0 {
		t.Fatalf("defaults violate their own links: %#v", values.overrides)
	}
}
