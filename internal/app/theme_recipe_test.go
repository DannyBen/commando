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
input = "foreground"
lightness_offset = 5
lightness_wave = 8
period = 8
`)
	retained := themePalette(settings.Values{})
	// A gray base still retains its chosen hue in LSH coordinates.
	retained["background"] = lshColor{52, 0, 213}
	before := retained["background"]
	first := r.generate("Dark", 1, 1, 50, retained)
	if len(first) != 1 {
		t.Fatalf("partial recipe generated %d colors", len(first))
	}
	border, exists := first["border"]
	if !exists || math.Abs(border.l-0.57) > 1e-9 || border.h != 213 || border.c != 0 {
		t.Fatalf("border did not inherit retained base: %+v", border)
	}
	next := r.generate("Dark", 1, 2, 50, retained)
	if next["border"] == border {
		t.Fatal("isolated border did not vary")
	}
	if retained["background"] != before || len(retained) != len(themeElements()) {
		t.Fatal("generation modified the retained palette")
	}
	if !reflect.DeepEqual(first, r.generate("Dark", 1, 1, 50, retained)) {
		t.Fatal("stepping forward and back accumulated changes")
	}
	// No color tables is also a valid way to hold the entire palette fixed.
	if got := partialThemeRecipe(t, "").generate("Dark", 1, 2, 100, retained); len(got) != 0 {
		t.Fatal("empty recipe generated colors")
	}
}

func TestThemeRecipeDictionaryResolvesForwardBasesIndependentlyOfTableOrder(t *testing.T) {
	background := `[colors.background]
base = "mode_background"
input = "background"
saturation = 20
hue_offset = 120
period = 8
`
	border := `[colors.border]
base = "background"
input = "foreground"
lightness_offset = 10
period = 8
`
	a, b := partialThemeRecipe(t, border+background), partialThemeRecipe(t, background+border)
	first, second := a.generate("Dark", 1, 1, 50, nil), b.generate("Dark", 1, 1, 50, nil)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("TOML table order changed the generated palette")
	}
	expected := (lshColor{40, 20, 120}).oklch()
	if first["border"] != expected {
		t.Fatalf("forward base was not generated first: %+v", first["border"])
	}
}

func TestThemeRecipeDictionaryRejectsCyclesAndUnknownColors(t *testing.T) {
	top, _, _ := strings.Cut(themeGeneratorTOML, "\n[colors.")
	for _, tables := range []string{
		"[colors.border]\nbase='border'\ninput='foreground'\nperiod=8\n",
		"[colors.border]\nbase='background'\ninput='foreground'\nperiod=8\n[colors.background]\nbase='border'\ninput='background'\nperiod=8\n",
		"[colors.unknown]\nbase='background'\ninput='foreground'\nperiod=8\n",
		"[colors.border]\nbase='missing'\ninput='foreground'\nperiod=8\n",
		"[colors.border]\nbase='background'\ninput='foreground'\nperiod=8\n[colors.border]\nbase='background'\ninput='foreground'\nperiod=8\n",
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
input = "foreground"
saturation = 80
hue_offset = 60
hue_wave = 30
lightness_wave = 10
period = 8
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
	v.adjustSetting(settingThemeGenerator+".foreground", 1)
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
	v.adjustSetting(settingThemeGenerator+".foreground", 1)
	expected := themePalette(v.settingsValues)
	v.finishSettingsWidget(layout.Context{}, true)
	loaded, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil || !reflect.DeepEqual(themePalette(loaded), expected) {
		t.Fatal("saving partial generation lost retained colors", err)
	}
}
