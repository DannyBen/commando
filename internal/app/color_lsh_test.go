package app

import (
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dannyben/commando/internal/settings"
)

func TestLSHCoordinatesFitAndRoundTrip(t *testing.T) {
	for l := 0.0; l <= 100; l += 10 {
		for h := 0.0; h < 360; h += 5 {
			for s := 0.0; s <= 100; s += 10 {
				value := (lshColor{l, s, h}).oklch()
				if !inSRGB(value.linearRGB()) {
					t.Fatalf("LSH %g/%g/%g left sRGB: %+v", l, s, h, value)
				}
				if math.Abs(value.l-l/100) > 1e-12 || value.h != h {
					t.Fatal("LSH changed lightness or hue")
				}
				if l == 0 || l == 100 {
					if value.c != 0 {
						t.Fatal("black/white retained chroma")
					}
					continue
				}
				roundTrip := lshFromOKLCH(value)
				if math.Abs(roundTrip.s-s) > 1e-5 {
					t.Fatalf("saturation roundtrip: %+v", roundTrip)
				}
				if s == 0 && value.c != 0 {
					t.Fatal("zero saturation is colored")
				}
			}
		}
	}
	if (lshColor{50, 80, 360}).oklch() != (lshColor{50, 80, 0}).oklch() {
		t.Fatal("hue did not wrap")
	}
	// At equal L/S, different hues need different absolute chroma.
	a, b := (lshColor{50, 100, 90}).oklch(), (lshColor{50, 100, 270}).oklch()
	if math.Abs(a.c-b.c) < 0.01 {
		t.Fatal("saturation used a fixed chroma range")
	}
}

func TestSelectionTextRetainsReadableNormalOrAdjustsColor(t *testing.T) {
	for _, tc := range []struct {
		name             string
		normal, bg, want color.NRGBA
	}{
		{"normal on light", rgb(0x222222), rgb(0xeeeeee), rgb(0x222222)},
		{"normal on dark", rgb(0xeeeeee), rgb(0x222222), rgb(0xeeeeee)},
		{"fallback white", rgb(0x222222), rgb(0x223366), rgb(0xffffff)},
		{"fallback black", rgb(0xeeeeee), rgb(0xffbbdd), rgb(0)},
	} {
		if got := contrastColor(tc.normal, tc.bg, themeContrastRules[contrastPrimary]); math.Abs(perceptualContrast(got, tc.bg)) < min(themeContrastRules[contrastPrimary].targetLc, math.Abs(perceptualContrast(tc.want, tc.bg))) || math.Abs(perceptualContrast(got, tc.bg)) < math.Abs(perceptualContrast(tc.normal, tc.bg)) {
			t.Fatalf("%s: got %v", tc.name, got)
		}
	}
	// The render guard covers Customize and
	// restored settings, including the pink selection from the reported example.
	var values settings.Values
	for role, value := range map[string]lshColor{"text": {45, 80, 280}, "selection": {65, 90, 5}} {
		for component, n := range map[string]float64{"lightness": value.l, "saturation": value.s, "hue": value.h} {
			if err := values.Set(settings.CatalogueDefault, themeSettingPrefix+role+"."+component, settings.Value{Number: n}); err != nil {
				t.Fatal(err)
			}
		}
	}
	c := themeColorsFromSettings(values)
	var cache textContrastCache
	selected := cache.resolve(c.SelectedName, c.Selected, contrastPrimary)
	if c.SelectedName != c.Foreground || selected != contrastColor(c.Foreground, c.Selected, themeContrastRules[contrastPrimary]) || math.Abs(perceptualContrast(selected, c.Selected)) < math.Abs(perceptualContrast(c.Foreground, c.Selected)) {
		t.Fatal("theme resolution missed selection contrast")
	}
	if _, exists := settings.CatalogueDefault.Widget(themeSettingPrefix + "selected_text"); exists {
		t.Fatal("derived selection text remains independently editable")
	}
}

func TestLSHStoredUnitsAndHueWheel(t *testing.T) {
	var values settings.Values
	for component, n := range map[string]float64{"lightness": 30, "saturation": 75, "hue": 360} {
		if err := values.Set(settings.CatalogueDefault, themeSettingPrefix+"edge."+component, settings.Value{Number: n}); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	if err := settings.Save(dir, values); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "settings.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"lightness = 30\n", "saturation = 75\n", "hue = 0\n"} {
		if !strings.Contains(string(data), text) {
			t.Fatalf("missing human value %q: %s", text, data)
		}
	}
	loaded, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Get(settings.CatalogueDefault, themeSettingPrefix+"edge.hue").Number != 0 || themeColorValue(loaded, themeSettingPrefix+"edge") != themeColorValue(values, themeSettingPrefix+"edge") {
		t.Fatal("LSH palette changed on reload")
	}
}

func TestLSHContrastPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional reported contrast preview")
	}
	v := newPreviewView()
	v.applySettings()
	v.startSettingsWidget(settingThemeGenerator)
	for key, value := range map[string]settings.Value{"mode": {Text: "Light"}, "seed": {Number: 24}, "dynamics": {Number: 39}, "vibrance": {Number: 50}} {
		if err := v.setWidgetValue(settingThemeGenerator+"."+key, value); err != nil {
			t.Fatal(err)
		}
	}
	v.applySettings()
	v.settingsInteraction().selected = 2
	c := v.appearance.Colors
	selected := v.resolvedTextOn(c.SelectedName, c.Selected)
	if selected != contrastColor(c.Foreground, c.Selected, themeContrastRules[contrastPrimary]) || math.Abs(perceptualContrast(selected, c.Selected)) < math.Abs(perceptualContrast(c.Foreground, c.Selected)) {
		t.Fatal("reported selection remains unreadable")
	}
	if err := renderViewPNG(filepath.Join(dir, "contrast-lsh.png"), v); err != nil {
		t.Fatal(err)
	}
}

func TestSelectionUsesPerceptualContrastForYellowAndDarkBackgrounds(t *testing.T) {
	// A bright yellow selection must get readable text even with gray normal text.
	v := newPreviewView()
	for role, c := range map[string]lshColor{"text": {55, 0, 0}, "selection": {92, 99, 100}} {
		for component, n := range map[string]float64{"lightness": c.l, "saturation": c.s, "hue": c.h} {
			if err := v.settingsValues.Set(settings.CatalogueDefault, themeSettingPrefix+role+"."+component, settings.Value{Number: n}); err != nil {
				t.Fatal(err)
			}
		}
	}
	v.applySettings()
	c := v.appearance.Colors
	selected := v.resolvedTextOn(c.SelectedName, c.Selected)
	if math.Abs(perceptualContrast(selected, c.Selected)) < min(themeContrastRules[contrastPrimary].targetLc, math.Abs(perceptualContrast(rgb(0), c.Selected))) {
		t.Fatalf("yellow selection retained colored text: normal %v selected %v background %v", c.Foreground, selected, c.Selected)
	}
	if math.Abs(perceptualContrast(selected, c.Selected)) <= math.Abs(perceptualContrast(c.Foreground, c.Selected)) {
		t.Fatal("yellow selection fallback did not improve contrast")
	}
	// A middling gray does not have enough perceptual contrast on dark blue.
	normal, bg := rgb(0x888888), rgb(0x111111)
	if math.Abs(perceptualContrast(normal, bg)) >= 75 {
		t.Fatal("fixture already has strong contrast")
	}

	if selected := contrastColor(normal, bg, themeContrastRules[contrastPrimary]); math.Abs(perceptualContrast(selected, bg)) < themeContrastRules[contrastPrimary].targetLc {
		t.Fatal("dark selection did not get readable text")
	}
}
