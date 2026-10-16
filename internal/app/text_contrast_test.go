package app

import (
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/settings"
)

func TestTextContrastIsIndependentForEverySurface(t *testing.T) {
	var cache textContrastCache
	yellow, black, nearYellow := rgb(0xffff00), rgb(0), rgb(0xffee33)
	if got := cache.resolve(yellow, black, contrastPrimary); got != yellow {
		t.Fatal("readable yellow was changed")
	}
	adjusted := cache.resolve(yellow, nearYellow, contrastPrimary)
	if adjusted == yellow || math.Abs(perceptualContrast(adjusted, nearYellow)) < themeContrastRules[contrastPrimary].targetLc {
		t.Fatal("unreadable footer text was not corrected")
	}
	if cache.resolve(yellow, black, contrastPrimary) != yellow {
		t.Fatal("footer correction leaked to main text")
	}
	if cache.resolve(yellow, nearYellow, contrastPrimary) != adjusted || len(cache) != 2 {
		t.Fatal("resolved pairs were not reused")
	}
	// Readable color is retained on white, independently of a dark footer.
	blue := rgb(0x003366)
	if cache.resolve(blue, rgb(0xffffff), contrastPrimary) != blue {
		t.Fatal("readable blue was changed")
	}
	if displayed := cache.resolve(blue, black, contrastPrimary); displayed == blue || math.Abs(perceptualContrast(displayed, black)) < themeContrastRules[contrastPrimary].targetLc {
		t.Fatal("dark footer did not get readable text")
	}
	if cache.resolve(blue, rgb(0xffffff), contrastPrimary) != blue {
		t.Fatal("dark footer affected readable blue elsewhere")
	}

}

func TestTextContrastRawPaletteAndSelectionSurviveRenderingAndSave(t *testing.T) {
	v := newPreviewView()
	for role, value := range map[string]uint32{"text": 0xffff00, "background": 0, "alt_background": 0xffee33, "text_selection": 0xffff00} {
		lsh := lshFromOKLCH(oklchFromRGB(rgb(value)))
		for component, n := range map[string]float64{"lightness": lsh.l, "saturation": lsh.s, "hue": lsh.h} {
			if err := v.settingsValues.Set(settings.CatalogueDefault, themeSettingPrefix+role+"."+component, settings.Value{Number: n}); err != nil {
				t.Fatal(err)
			}
		}
	}
	v.applySettings()
	before := v.settingsValues.Clone()
	v.startSettingsWidget(settingThemeGenerator)
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 480))}
	v.layout(gtx)
	c := v.appearance.Colors
	if displayed, ok := v.textContrast[textContrastPair{c.Foreground, c.Surface, themeContrastRules[contrastPrimary]}]; !ok || displayed != c.Foreground {
		t.Fatal("main text lost requested yellow")
	}
	if displayed, ok := v.textContrast[textContrastPair{c.Foreground, c.Footer, themeContrastRules[contrastPrimary]}]; !ok || displayed == c.Foreground {
		t.Fatal("footer did not get its own readable variant")
	}
	if _, guarded := v.textContrast[textContrastPair{c.EditorText, c.TextSelection, themeContrastRules[contrastPrimary]}]; guarded {
		t.Fatal("text selection was contrast-guarded")
	}
	if !reflect.DeepEqual(v.settingsValues.Clone(), before) {
		t.Fatal("rendering mutated saved palette")
	}
	dir := t.TempDir()
	if err := settings.Save(dir, v.settingsValues); err != nil {
		t.Fatal(err)
	}
	loaded, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil || !reflect.DeepEqual(themePalette(loaded), themePalette(before)) {
		t.Fatal("save wrote corrected display colors", err)
	}
	if len(v.textContrast) == 0 {
		t.Fatal("rendering did not cache contextual colors")
	}
	v.applySettings()
	if len(v.textContrast) != 0 {
		t.Fatal("theme application did not invalidate cached colors")
	}
}

func TestTextBackgroundScopesAndEditorSelectionExclusion(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	v.appearance.Colors.EditorText = rgb(0xffff00)
	v.appearance.Colors.TextSelection = rgb(0xffff00)
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(300, 50))}
	root := v.currentTextBackground()
	v.textOn(rgb(0), func(gtx layout.Context) layout.Dimensions {
		style := v.textEditor(&v.location, "Path")
		if style.Color != rgb(0xffff00) || style.SelectionColor != rgb(0xffff00) {
			t.Fatal("selection background influenced editor text or was itself corrected")
		}
		return v.band(gtx, 30, rgb(0xffff00), func(gtx layout.Context) layout.Dimensions {
			if v.resolvedText(rgb(0xffff00)) == rgb(0xffff00) {
				t.Fatal("nested band ignored its actual background")
			}
			return layout.Dimensions{}
		}, false)
	})(gtx)
	if v.currentTextBackground() != root {
		t.Fatal("background scope leaked into siblings")
	}
}

func TestTextContrastAccountsForOpacity(t *testing.T) {
	var cache textContrastCache
	text := color.NRGBA{A: 30}
	bg := rgb(0xffffff)
	resolved := cache.resolve(text, bg, contrastPrimary)
	if math.Abs(perceptualContrast(compositeText(resolved, bg), bg)) < themeContrastRules[contrastPrimary].targetLc {
		t.Fatal("dimmed text was checked as opaque")
	}
}

func BenchmarkTextContrast(b *testing.B) {
	for _, cached := range []bool{false, true} {
		name := "first resolution"
		if cached {
			name = "cached pair"
		}
		b.Run(name, func(b *testing.B) {
			var cache textContrastCache
			for i := 0; i < b.N; i++ {
				if !cached {
					clear(cache)
				}
				cache.resolve(rgb(0xbb00aa), rgb(0xbb00aa), contrastPrimary)
			}
		})
	}
}

func TestContextualContrastPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional contextual contrast preview")
	}
	v := newPreviewView()
	for role, c := range map[string]lshColor{
		"text": {54.5225, 100, 333.2329}, "background": {90.0936, 61.0815, 242.8279},
		"alt_background": {94.161, 74.864, 304.7657}, "text_selection": {63.2168, 100, 161.222},
	} {
		for component, n := range map[string]float64{"lightness": c.l, "saturation": c.s, "hue": c.h} {
			if err := v.settingsValues.Set(settings.CatalogueDefault, themeSettingPrefix+role+"."+component, settings.Value{Number: n}); err != nil {
				t.Fatal(err)
			}
		}
	}
	v.applySettings()
	v.startSettingsWidget(settingThemeGenerator)
	if err := renderViewPNG(filepath.Join(dir, "contextual-contrast.png"), v); err != nil {
		t.Fatal(err)
	}
}

func TestTextContrastAcrossGeneratedPalettes(t *testing.T) {
	for _, mode := range []string{"Dark", "Midtone", "Light"} {
		for seed := uint64(1); seed <= 20; seed++ {
			colors := defaultThemeRecipe.generate(themeGeneration{mode, seed, 100, 100, 96}, nil)
			var cache textContrastCache
			for _, role := range []string{"text", "secondary_text", "info", "success", "warning", "error"} {
				foreground := colors[role].rgb()
				policy := contrastPrimary
				if role == "secondary_text" {
					policy = contrastSecondary
				}
				target := themeContrastRules[policy].targetLc
				for _, surface := range []string{"background", "alt_background", "selection"} {
					background := colors[surface].rgb()
					resolved := cache.resolve(foreground, background, policy)
					originalScore := math.Abs(perceptualContrast(foreground, background))
					if originalScore >= target {
						if resolved != foreground {
							t.Fatalf("readable %s changed on %s", role, surface)
						}
					} else {
						bestExtreme := max(math.Abs(perceptualContrast(rgb(0), background)), math.Abs(perceptualContrast(rgb(0xffffff), background)))
						score := math.Abs(perceptualContrast(resolved, background))
						if bestExtreme >= target {
							if score < target {
								t.Fatalf("%s seed %d %s on %s failed reachable APCA target", mode, seed, role, surface)
							}
						} else if score < bestExtreme {
							t.Fatalf("%s seed %d %s on %s missed strongest fallback", mode, seed, role, surface)
						}
					}
				}
			}
		}
	}
}

func TestSelectedSeedUsesAPCAOnPinkBackground(t *testing.T) {
	v := newPreviewView()
	for role, value := range map[string]uint32{"text": 0, "background": 0xffffff, "selection": 0xed009e} {
		c := lshFromOKLCH(oklchFromRGB(rgb(value)))
		for component, n := range map[string]float64{"lightness": c.l, "saturation": c.s, "hue": c.h} {
			if err := v.settingsValues.Set(settings.CatalogueDefault, themeSettingPrefix+role+"."+component, settings.Value{Number: n}); err != nil {
				t.Fatal(err)
			}
		}
	}
	v.applySettings()
	v.startSettingsWidget(settingThemeGenerator)
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 480))}
	v.layout(gtx)
	c := v.appearance.Colors
	if c.SelectedName != c.Foreground {
		t.Fatal("selection text was adjusted in the palette")
	}
	if displayed, ok := v.textContrast[textContrastPair{c.SelectedName, c.Selected, themeContrastRules[contrastPrimary]}]; !ok || displayed != rgb(0xffffff) {
		t.Fatalf("selected Seed did not render white on pink: %v (cached %v)", displayed, ok)
	}
	if v.resolvedTextOn(c.Foreground, c.Surface) != c.Foreground {
		t.Fatal("selected-value fallback changed ordinary labels")
	}
	if dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS"); dir != "" {
		if err := renderViewPNG(filepath.Join(dir, "apca-selected-seed.png"), v); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPrimaryAndSecondaryContrastAreIndependentWhenColorsMatch(t *testing.T) {
	v := newPreviewView()
	for role, value := range map[string]uint32{"text": 0x777777, "secondary_text": 0x777777, "background": 0xffffff, "alt_background": 0xffffff} {
		c := lshFromOKLCH(oklchFromRGB(rgb(value)))
		for component, n := range map[string]float64{"lightness": c.l, "saturation": c.s, "hue": c.h} {
			if err := v.settingsValues.Set(settings.CatalogueDefault, themeSettingPrefix+role+"."+component, settings.Value{Number: n}); err != nil {
				t.Fatal(err)
			}
		}
	}
	v.applySettings()
	before := v.settingsValues.Clone()
	c := v.appearance.Colors
	score := math.Abs(perceptualContrast(c.Foreground, c.Footer))
	if c.Foreground != c.Muted || score < themeContrastRules[contrastSecondary].targetLc || score >= themeContrastRules[contrastPrimary].targetLc {
		t.Fatal("fixture does not separate the two targets")
	}
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 480))}
	v.layout(gtx)
	primary, ok := v.textContrast[textContrastPair{c.Foreground, c.Footer, themeContrastRules[contrastPrimary]}]
	if !ok || primary == c.Foreground || math.Abs(perceptualContrast(primary, c.Footer)) < themeContrastRules[contrastPrimary].targetLc {
		t.Fatal("primary footer text did not retain its stronger requirement")
	}
	secondary, ok := v.textContrast[textContrastPair{c.Muted, c.Footer, themeContrastRules[contrastSecondary]}]
	if !ok || secondary != c.Muted {
		t.Fatal("secondary footer position was corrected using the primary target")
	}
	if !reflect.DeepEqual(v.settingsValues.Clone(), before) {
		t.Fatal("per-role contrast changed saved colors")
	}
	if dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS"); dir != "" {
		v.startSettingsWidget(settingThemeGenerator)
		if err := renderViewPNG(filepath.Join(dir, "secondary-contrast.png"), v); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBackgroundContrastRuleUsesSameAPCASearch(t *testing.T) {
	foreground, background := rgb(0xffff00), rgb(0xffee33)
	rule := contrastRule{targetLc: 60, adjust: adjustBackground}
	resolved := contrastColor(foreground, background, rule)
	if resolved == background || math.Abs(perceptualContrast(foreground, resolved)) < rule.targetLc {
		t.Fatal("background rule did not adjust the requested side")
	}
	// No production role currently registers a background-adjusting rule.
	for _, active := range themeContrastRules {
		if active.adjust != adjustForeground {
			t.Fatal("background guard was enabled before selection work")
		}
	}
}
