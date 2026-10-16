package app

import (
	"math"
	"reflect"
	"strings"
	"testing"

	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

func partialThemeRecipe(t *testing.T, tables string) themeRecipe {
	t.Helper()
	top, _, ok := strings.Cut(themeGeneratorTOML, "\n[colors.")
	if !ok {
		t.Fatal("embedded recipe has no color tables")
	}
	r, err := parseThemeRecipe(top + "\n" + tables)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPartialThemeRecipeUsesRetainedBaseAndOnlyReturnsDefinedColors(t *testing.T) {
	r := partialThemeRecipe(t, `[colors.border]
base = "background"
lightness.offset = 5
lightness.range = [-8, 8]
`)
	retained := themePalette(settings.Values{})
	// A gray base still retains its chosen hue in LSH coordinates.
	retained["background"] = lshColor{52, 0, 213}
	before := retained["background"]
	first := r.generate(themeGeneration{"Dark", 1, 50, 1, 0}, retained)
	if len(first) != 1 {
		t.Fatalf("partial recipe generated %d colors", len(first))
	}
	border, exists := first["border"]
	if !exists || math.Abs(border.l-0.57) > 1e-9 || border.h != 213 || border.c != 0 {
		t.Fatalf("border did not inherit retained base: %+v", border)
	}
	next := r.generate(themeGeneration{"Dark", 1, 50, 100, 0}, retained)
	if next["border"] == border {
		t.Fatal("isolated border did not vary")
	}
	if retained["background"] != before || len(retained) != len(themeElements()) {
		t.Fatal("generation modified the retained palette")
	}
	if !reflect.DeepEqual(first, r.generate(themeGeneration{"Dark", 1, 50, 1, 0}, retained)) {
		t.Fatal("stepping forward and back accumulated changes")
	}
	// No color tables is also a valid way to hold the entire palette fixed.
	if got := partialThemeRecipe(t, "").generate(themeGeneration{"Dark", 2, 100, 100, 0}, retained); len(got) != 0 {
		t.Fatal("empty recipe generated colors")
	}
}

func TestThemeRecipeDictionaryResolvesForwardBasesIndependentlyOfTableOrder(t *testing.T) {
	background := `[colors.background]
base = "mode_background"
saturation.center = 20
hue.center = 120
`
	border := `[colors.border]
base = "background"
lightness.offset = 10
`
	a, b := partialThemeRecipe(t, border+background), partialThemeRecipe(t, background+border)
	first, second := a.generate(themeGeneration{"Dark", 1, 50, 1, 0}, nil), b.generate(themeGeneration{"Dark", 1, 50, 1, 0}, nil)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("TOML table order changed the generated palette")
	}
	expected := (lshColor{35, 20, 120}).oklch()
	if first["border"] != expected {
		t.Fatalf("forward base was not generated first: %+v", first["border"])
	}
}

func TestThemeRecipeDictionaryRejectsCyclesAndUnknownColors(t *testing.T) {
	top, _, _ := strings.Cut(themeGeneratorTOML, "\n[colors.")
	for _, tables := range []string{
		"[colors.border]\nbase='border'\n",
		"[colors.border]\nbase='background'\n[colors.background]\nbase='border'\n",
		"[colors.unknown]\nbase='background'\n",
		"[colors.border]\nbase='missing'\n",
		"[colors.border]\nbase='background'\n[colors.border]\nbase='background'\n",
	} {
		if _, err := parseThemeRecipe(top + "\n" + tables); err == nil {
			t.Fatal("accepted invalid dictionary recipe")
		}
	}
}

func TestPartialThemeRecipePreviewSaveAndCancelKeepOmittedColors(t *testing.T) {
	original := defaultThemeRecipe
	defaultThemeRecipe = partialThemeRecipe(t, `[colors.warning]
base = "text"
saturation.center = 80
hue.offset = 60
hue.range = [-30, 30]
lightness.range = [-10, 10]
`)
	t.Cleanup(func() { defaultThemeRecipe = original })
	v := newPreviewView()
	v.applySettings()
	// Give an omitted base a deliberate custom value, distinct from mode defaults.
	for component, n := range map[string]float64{"lightness": 25, "saturation": 35, "hue": 170} {
		if err := v.settingsValues.Set(settings.CatalogueDefault, themeSettingPrefix+"text."+component, settings.Value{Number: n}); err != nil {
			t.Fatal(err)
		}
	}
	v.applySettings()
	before := themePalette(v.settingsValues)
	v.startSettingsWidget(settingThemeGenerator)
	v.adjustSetting(settingThemeGenerator+".seed", 1)
	after := themePalette(v.settingsValues)
	changed := false
	for name, value := range before {
		if name == "warning" {
			changed = after[name] != value
			continue
		}
		if after[name] != value {
			t.Fatalf("partial generation changed omitted %s", name)
		}
	}
	if !changed {
		t.Fatal("included warning did not change")
	}
	v.activateSettingsAction(layout.Context{}, 1)
	if !reflect.DeepEqual(themePalette(v.settingsValues), before) {
		t.Fatal("Cancel did not restore the isolated preview")
	}
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.startSettingsWidget(settingThemeGenerator)
	v.adjustSetting(settingThemeGenerator+".seed", 1)
	expected := themePalette(v.settingsValues)
	v.finishSettingsWidget(layout.Context{}, true)
	loaded, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil || !reflect.DeepEqual(themePalette(loaded), expected) {
		t.Fatal("saving partial generation lost retained colors", err)
	}
}

func TestThemeRecipeRangesAndStableAdjustments(t *testing.T) {
	r := partialThemeRecipe(t, `[colors.border]
base = "mode_background"
lightness.center = 50
lightness.offset = 2
lightness.range = [-10, 20]
saturation.center = 30
saturation.range = [-10, 20]
hue.center = 350
hue.range = [-80, 80]
`)
	for seed := uint64(1); seed <= 100; seed++ {
		options := themeGeneration{"Dark", seed, 50, 1, 0}
		tame := lshFromOKLCH(r.generate(options, nil)["border"])
		if math.Abs(tame.l-52) > 1e-7 || math.Abs(tame.s-30) > 1e-7 || tame.h != 350 {
			t.Fatalf("minimum Dynamics moved baseline: %+v", tame)
		}
		options.Dynamics = 100
		wild := lshFromOKLCH(r.generate(options, nil)["border"])
		if wild.l < 42 || wild.l > 72 || wild.s < 20 || wild.s > 50 {
			t.Fatalf("outside configured ranges: %+v", wild)
		}
		options.Dynamics = 50.5
		middle := lshFromOKLCH(r.generate(options, nil)["border"])
		if math.Abs(middle.l-(tame.l+wild.l)/2) > 1e-7 || math.Abs(middle.s-(tame.s+wild.s)/2) > 1e-7 {
			t.Fatal("Dynamics redrew random choices")
		}
		options.Dynamics = 100
		options.Vibrance = 25
		muted := lshFromOKLCH(r.generate(options, nil)["border"])
		if muted.h != wild.h || math.Abs(muted.l-wild.l) > 1e-7 || math.Abs(muted.s-wild.s*math.Pow(0.5, 1.5)) > 1e-7 {
			t.Fatal("Vibrance changed identity or used wrong response")
		}
		options.Vibrance = 50
		options.ColorShift = 30
		shifted := lshFromOKLCH(r.generate(options, nil)["border"])
		if math.Abs(shifted.l-wild.l) > 1e-7 || math.Abs(shifted.s-wild.s) > 1e-7 || math.Abs(shifted.h-wrapHue(wild.h+30)) > 1e-7 {
			t.Fatal("Color Shift changed more than hue")
		}
		options.ColorShift = 0
		options.Mode = "Light"
		light := lshFromOKLCH(r.generate(options, nil)["border"])
		if math.Abs(light.l+wild.l-100) > 1e-7 {
			t.Fatal("Light mode did not reverse lightness adjustments around absolute center")
		}
	}
}

func TestThemeRecipeModifiersApplyOnceAndRulesDoNotReshuffle(t *testing.T) {
	tables := `[colors.background]
base = "mode_background"
saturation.center = 10
hue.range = [-30, 30]
[colors.alt_background]
base = "background"
[colors.border]
base = "alt_background"
`
	r := partialThemeRecipe(t, tables)
	extra := partialThemeRecipe(t, tables+"\n[colors.edge]\nbase='mode_text'\nhue.range=[-180, 180]\n")
	options := themeGeneration{"Dark", 42, 100, 80, 320}
	a, b := r.generate(options, nil), extra.generate(options, nil)
	for name, c := range a {
		if c != b[name] {
			t.Fatal("adding unrelated rule reshuffled palette")
		}
		if math.Abs(lshFromOKLCH(c).s-10*math.Pow(2, 1.5)) > 1e-7 || c.h != a["background"].h {
			t.Fatal("global modifiers compounded through inheritance")
		}
	}
	if !reflect.DeepEqual(a, r.generate(options, nil)) {
		t.Fatal("same seed and controls did not reproduce palette")
	}
	options.Seed++
	if reflect.DeepEqual(a, r.generate(options, nil)) {
		t.Fatal("Seed did not choose new values")
	}
}

func TestThemeRecipeRejectsInvalidComponentsAndResponses(t *testing.T) {
	top, _, _ := strings.Cut(themeGeneratorTOML, "\n[colors.")
	for _, field := range []string{
		"lightness.range=[1]", "lightness.range=[1,2,3]", "lightness.range=[2,1]",
		"lightness.range=[nan,1]", "hue.range=[0,inf]", "saturation.center=101",
		"lightness.center=-1", "hue.center=361", "lightness.offset=inf", "hue.period=10",
	} {
		if _, err := parseThemeRecipe(top + "\n[colors.border]\nbase='background'\n" + field); err == nil {
			t.Fatalf("accepted %s", field)
		}
	}
	for _, field := range []string{"dynamics_exponent", "vibrance_exponent"} {
		for _, value := range []string{"0", "-1", "4.1", "inf", "nan"} {
			source := strings.Replace(themeGeneratorTOML, field+" = "+map[string]string{"dynamics_exponent": "1.0", "vibrance_exponent": "1.5"}[field], field+" = "+value, 1)
			if _, err := parseThemeRecipe(source); err == nil {
				t.Fatalf("accepted %s=%s", field, value)
			}
		}
	}
}

func TestThemeGeneratorDeterministicAndGray(t *testing.T) {
	for _, mode := range []string{"Dark", "Midtone", "Light"} {
		for seed := uint64(1); seed <= 30; seed++ {
			for _, dynamics := range []float64{1, 35, 100} {
				for _, vibrance := range []float64{0, 50, 100} {
					options := themeGeneration{mode, seed, vibrance, dynamics, 170}
					colors := defaultThemeRecipe.generate(options, nil)
					if !reflect.DeepEqual(colors, defaultThemeRecipe.generate(options, nil)) {
						t.Fatal("non-deterministic palette")
					}
					for name, c := range colors {
						if math.IsNaN(c.l) || c.l < 0 || c.l > 1 || c.c < 0 || c.h < 0 || c.h >= 360 {
							t.Fatalf("invalid %s: %+v", name, c)
						}
						rgb := c.rgb()
						if vibrance == 0 && (rgb.R != rgb.G || rgb.G != rgb.B) {
							t.Fatalf("%s colored at zero Vibrance", name)
						}
					}
				}
			}
		}
	}
}

func TestThemeGeneratorPreservesRequestedColorsDespitePoorContrast(t *testing.T) {
	r := partialThemeRecipe(t, `[colors.background]
base = "mode_background"
lightness.center = 50
[colors.alt_background]
base = "background"
[colors.text_selection]
base = "background"
[colors.text]
base = "background"
`)
	options := themeGeneration{"Light", 127, 50, 100, 96}
	before := r.generate(options, nil)
	for _, name := range []string{"background", "alt_background", "text_selection", "text"} {
		if before[name].l != 0.5 {
			t.Fatalf("generator contrast-corrected %s", name)
		}
	}
	// Changing selection cannot change the generated primary text or surfaces.
	rule := r.Colors["text_selection"]
	center := 10.0
	rule.Lightness.Center = &center
	r.Colors["text_selection"] = rule
	after := r.generate(options, nil)
	for _, name := range []string{"background", "alt_background", "text"} {
		if before[name] != after[name] {
			t.Fatalf("selection influenced generated %s", name)
		}
	}
}
