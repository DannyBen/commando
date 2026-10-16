package app

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dannyben/commando/internal/settings"
)

func TestGeneratedAutoColorsTrackAcceptedLightAndDarkReferences(t *testing.T) {
	for _, tc := range []struct {
		mode                     string
		seed, vibrance, dynamics float64
		theme                    int
	}{
		{"White", 142, 94, 50, 0}, {"Black", 194, 46, 18, 1},
	} {
		v := newPreviewView()
		v.startSettingsWidget(settingThemeGenerator)
		for key, n := range map[string]float64{"seed": tc.seed, "vibrance": tc.vibrance, "dynamics": tc.dynamics} {
			if err := v.setWidgetValue(settingThemeGenerator+"."+key, settings.Value{Number: n}); err != nil {
				t.Fatal(err)
			}
		}
		if err := v.setWidgetValue(settingThemeGenerator+".mode", settings.Value{Text: tc.mode}); err != nil {
			t.Fatal(err)
		}
		reference := builtInThemes[tc.theme].values
		for _, key := range []string{settingLightness, settingSelectionLightness, settingSaturation} {
			got, want := v.settingsValues.Get(settings.CatalogueDefault, key).Number, reference.Get(settings.CatalogueDefault, key).Number
			if math.Abs(got-want) > 1 {
				t.Fatalf("%s %s: generated %g too far from accepted %g", tc.mode, key, got, want)
			}
		}
	}
}

func TestGeneratedFileSelectionMovesTowardSurfaceAcrossModes(t *testing.T) {
	v := newPreviewView()
	v.startSettingsWidget(settingThemeGenerator)
	// Auto Colors preferences outside the generator's ownership stay independent.
	for key, value := range map[string]settings.Value{settingAutoColorEnabled: {Bool: false}, settingSalt: {Number: 73}, settingHiddenSaturation: {Number: 0.4}, settingDotfileSaturation: {Number: 0.6}} {
		if err := v.settingsValues.Set(settings.CatalogueDefault, key, value); err != nil {
			t.Fatal(err)
		}
	}
	before := v.settingsValues.Clone()
	for _, mode := range []string{"Black", "Dark", "Midtone", "Light", "White"} {
		if err := v.setWidgetValue(settingThemeGenerator+".mode", settings.Value{Text: mode}); err != nil {
			t.Fatal(err)
		}
		for seed := 1; seed <= 30; seed++ {
			if err := v.setWidgetValue(settingThemeGenerator+".seed", settings.Value{Number: float64(seed)}); err != nil {
				t.Fatal(err)
			}
			for _, dynamics := range []float64{1, 50, 100} {
				if err := v.setWidgetValue(settingThemeGenerator+".dynamics", settings.Value{Number: dynamics}); err != nil {
					t.Fatal(err)
				}
				surface := themeColorCoordinates(v.settingsValues, themeSettingPrefix+"background").l
				filename := v.settingsValues.Get(settings.CatalogueDefault, settingLightness).Number
				selection := filename + v.settingsValues.Get(settings.CatalogueDefault, settingSelectionLightness).Number
				if selection <= math.Min(surface, filename) || selection >= math.Max(surface, filename) {
					t.Fatalf("%s selection %g is not between filename %g and surface %g", mode, selection, filename, surface)
				}
				for _, key := range []string{settingCutLightness, settingHiddenLightness, settingDotfileLightness} {
					faded := filename + v.settingsValues.Get(settings.CatalogueDefault, key).Number
					if math.Abs(faded-surface) >= math.Abs(filename-surface) {
						t.Fatalf("%s %s no longer softens file contrast", mode, key)
					}
				}
				for _, key := range []string{settingAutoColorEnabled, settingSalt, settingHiddenSaturation, settingDotfileSaturation} {
					if v.settingsValues.Get(settings.CatalogueDefault, key) != before.Get(settings.CatalogueDefault, key) {
						t.Fatal("generator changed independent preference", key)
					}
				}
			}
		}
	}
	previous := -1.0
	for _, vibrance := range []float64{0, 25, 46, 50, 75, 94, 100} {
		if err := v.setWidgetValue(settingThemeGenerator+".vibrance", settings.Value{Number: vibrance}); err != nil {
			t.Fatal(err)
		}
		saturation := v.settingsValues.Get(settings.CatalogueDefault, settingSaturation).Number
		if saturation < previous || saturation > 93 || vibrance == 0 && saturation != 0 {
			t.Fatal("saturation response lost grayscale, monotonicity or its softer ceiling", saturation)
		}
		previous = saturation
	}
}

func TestThemeRecipeRejectsInvalidFileTuning(t *testing.T) {
	for _, field := range []string{"filename_selection_offset = -16", "saturation_max = 93"} {
		for _, value := range []string{"-101", "101", "nan", "inf"} {
			key, _, _ := strings.Cut(field, " = ")
			if _, err := parseThemeRecipe(strings.Replace(themeGeneratorTOML, field, key+" = "+value, 1)); err == nil {
				t.Fatal("invalid file tuning accepted", key, value)
			}
		}
	}
}

func TestGeneratedAutoColorsPreviews(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional Auto Colors previews")
	}
	for _, mode := range []string{"Black", "Dark", "Midtone", "Light", "White"} {
		for _, vibrance := range []float64{0, 50, 100} {
			v := newPreviewView()
			v.startSettingsWidget(settingThemeGenerator)
			_ = v.setWidgetValue(settingThemeGenerator+".mode", settings.Value{Text: mode})
			_ = v.setWidgetValue(settingThemeGenerator+".seed", settings.Value{Number: 194})
			_ = v.setWidgetValue(settingThemeGenerator+".dynamics", settings.Value{Number: 50})
			_ = v.setWidgetValue(settingThemeGenerator+".vibrance", settings.Value{Number: vibrance})
			v.applySettings()
			if err := renderViewPNG(filepath.Join(dir, mode+"-autocolor-"+v.settingText(settingThemeGenerator+".vibrance")+".png"), v); err != nil {
				t.Fatal(err)
			}
		}
	}
}
