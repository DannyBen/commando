package app

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/dannyben/commando/internal/settings"
)

func TestAPCAReferenceScores(t *testing.T) {
	// Independent evaluations of the published APCA 0.0.98G reference, including
	// the exact purple and yellow sampled from the user's contrast screenshots.
	for _, tc := range []struct {
		text, bg uint32
		want     float64
	}{
		{0, 0xffffff, 106.04067321268862},
		{0xffffff, 0, -107.88473318309848},
		{0, 0xbe47a8, 33.56256436000189},
		{0xffffff, 0xbe47a8, -75.99929535792218},
		{0, 0xe9b200, 67.00548537504525},
		{0xffffff, 0xe9b200, -41.219072762240756},
		{0x888888, 0x888888, 0},
		{0x111111, 0x121212, 0},
	} {
		if got := perceptualContrast(rgb(tc.text), rgb(tc.bg)); math.Abs(got-tc.want) > 1e-6 {
			t.Fatalf("%06x on %06x: Lc %.9f; want %.9f", tc.text, tc.bg, got, tc.want)
		}
	}
}

func TestAPCASelectionMatchesReportedPreferences(t *testing.T) {
	normal := rgb(0x334455)
	for _, tc := range []struct{ bg, want uint32 }{{0xbe47a8, 0xffffff}, {0xe9b200, 0}} {
		if got := contrastColor(normal, rgb(tc.bg), themeContrastRules[contrastPrimary]); math.Abs(perceptualContrast(got, rgb(tc.bg))) < min(themeContrastRules[contrastPrimary].targetLc, math.Abs(perceptualContrast(rgb(tc.want), rgb(tc.bg)))) {
			t.Fatalf("background %06x: got %v; want %06x", tc.bg, got, tc.want)
		}
	}
	// Keep an already clear, colored normal text rather than forcing a neutral.
	normal = rgb(0x003366)
	if contrastColor(normal, rgb(0xffffff), themeContrastRules[contrastPrimary]) != normal {
		t.Fatal("clear normal text was discarded")
	}
}

func TestAPCASelectionPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional perceptual selection previews")
	}
	for _, tc := range []struct {
		name       string
		background uint32
		want       uint32
	}{
		{"purple", 0x442277, 0xffffff},
		{"yellow", 0xffee33, 0},
	} {
		v := newPreviewView()
		v.applySettings()
		v.startSettingsWidget(settingThemeGenerator)
		for role, value := range map[string]uint32{"text": 0x888888, "selection": tc.background} {
			c := lshFromOKLCH(oklchFromRGB(rgb(value)))
			for component, n := range map[string]float64{"lightness": c.l, "saturation": c.s, "hue": c.h} {
				if err := v.settingsValues.Set(settings.CatalogueDefault, themeSettingPrefix+role+"."+component, settings.Value{Number: n}); err != nil {
					t.Fatal(err)
				}
			}
		}
		v.applySettings()
		v.settingsInteraction().selected = 2
		if selected := v.resolvedTextOn(v.appearance.Colors.SelectedName, v.appearance.Colors.Selected); math.Abs(perceptualContrast(selected, v.appearance.Colors.Selected)) < min(themeContrastRules[contrastPrimary].targetLc, math.Abs(perceptualContrast(rgb(tc.want), v.appearance.Colors.Selected))) {
			t.Fatalf("%s selection got %v", tc.name, selected)
		}
		if err := renderViewPNG(filepath.Join(dir, "contrast-apca-"+tc.name+".png"), v); err != nil {
			t.Fatal(err)
		}
	}
}
