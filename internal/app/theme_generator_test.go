package app

import (
	"fmt"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

func TestThemeGeneratorLiveControlsAndCancel(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	before, palette, filenames := v.settingsValues.Clone(), v.appearance.Colors, *v.appearance.FilenameColors
	selected := v.panels[0].selected
	v.startSettingsWidget(settingThemeGenerator)
	s := v.settingsInteraction()
	if s == nil || s.title != "Appearance › Theme › Generator" || len(s.fields) != 4 || isThemeWidget(s) {
		t.Fatal("Generator did not open its own four-field widget")
	}
	if !reflect.DeepEqual(v.settingsValues.Clone(), before) || s.dirty {
		t.Fatal("opening Generator overwrote the active palette")
	}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameLeftArrow)) // Light -> Midtone.
	send(pressed(key.NameLeftArrow)) // Midtone -> Dark.
	if v.settingText(settingThemeGenerator+".mode") != "Dark" || v.appearance.Colors == palette || !s.dirty {
		t.Fatal("mode did not preview live")
	}
	dark := v.appearance.Colors
	send(pressed(key.NameDownArrow), pressed(key.NameRightArrow))
	if v.settingText(settingThemeGenerator+".background") != "2" || v.appearance.Colors == dark || v.panels[0].selected != selected {
		t.Fatal("background step failed to preview or disturbed browsing")
	}
	background := v.appearance.Colors.Surface
	send(pressed(key.NameDownArrow), pressed(key.NameReturn))
	send(key.EditEvent{Range: key.Range{End: 1}, Text: "600"}, pressed(key.NameReturn))
	if s.editing || v.settingText(settingThemeGenerator+".foreground") != "600" || v.appearance.Colors.Surface != background {
		t.Fatal("typed foreground variation did not apply independently")
	}
	send(pressed(key.NameDownArrow), pressed(key.NameRightArrow))
	if v.settingText(settingThemeGenerator+".colorfulness") != "51" {
		t.Fatal("Colorfulness did not adjust live")
	}
	v.activateSettingsAction(layout.Context{}, 1)
	if v.settingsInteraction() != nil || !reflect.DeepEqual(v.settingsValues.Clone(), before) || v.appearance.Colors != palette || *v.appearance.FilenameColors != filenames {
		t.Fatal("Cancel did not restore colors, controls and filename preferences")
	}
}

func TestThemeGeneratorSaveCustomizeReopenAndReset(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	dir := t.TempDir()
	v.prepareSettingsSave = func(values settings.Values) func() (string, error) {
		return func() (string, error) { return "", settings.Save(dir, values) }
	}
	v.startSettingsWidget(settingThemeGenerator)
	v.adjustSetting(settingThemeGenerator+".mode", -1)
	v.adjustSetting(settingThemeGenerator+".background", 1)
	v.adjustSetting(settingThemeGenerator+".foreground", 1)
	_ = v.setWidgetValue(settingThemeGenerator+".colorfulness", settings.Value{Number: 75})
	v.applySettings()
	generated, filenames := v.appearance.Colors, *v.appearance.FilenameColors
	v.finishSettingsWidget(layout.Context{}, true)
	loaded, err := settings.Load(dir, settings.CatalogueDefault)
	if err != nil {
		t.Fatal(err)
	}
	other := newPreviewView()
	other.restoreSettings(loaded, nil)
	if other.appearance.Colors != generated || *other.appearance.FilenameColors != filenames || other.settingText(settingThemeGenerator+".background") != "2" || other.settingText(settingThemeGenerator+".colorfulness") != "75" {
		t.Fatal("saved generated colors or inputs did not survive restart")
	}
	v.startSettingsWidget(settingThemeCustomize)
	_ = v.setWidgetValue(themeSettingPrefix+"background.lightness", settings.Value{Number: 0.3})
	v.applySettings()
	v.finishSettingsWidget(layout.Context{}, true)
	custom := v.appearance.Colors
	v.startSettingsWidget(settingThemeGenerator)
	if v.appearance.Colors != custom || v.settingsInteraction().dirty {
		t.Fatal("reopening Generator discarded Customize edits")
	}
	v.activateSettingsAction(layout.Context{}, 2)
	if v.settingText(settingThemeGenerator+".mode") != "Light" || v.settingText(settingThemeGenerator+".background") != "1" || !v.settingsInteraction().dirty || v.appearance.Colors == custom {
		t.Fatal("Reset did not preview the default recipe inputs")
	}
	v.activateSettingsAction(layout.Context{}, 1)
	if v.appearance.Colors != custom {
		t.Fatal("Cancel after Reset lost the customized palette")
	}
}

func TestThemeGeneratorChoiceEdgesAndCycle(t *testing.T) {
	v := newPreviewView()
	v.applySettings()
	v.startSettingsWidget(settingThemeGenerator)
	before := v.settingsValues.Clone()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameRightArrow)) // Light is already the final choice.
	if v.settingsInteraction().dirty || !reflect.DeepEqual(v.settingsValues.Clone(), before) {
		t.Fatal("choice at an edge regenerated the palette")
	}
	send(pressed(key.NameReturn)) // Enter cycles to Dark, without a text editor.
	if v.settingText(settingThemeGenerator+".mode") != "Dark" || v.settingsInteraction().editing {
		t.Fatal("Enter did not cycle the mode")
	}
	send(pressed(key.NameSpace))
	if v.settingText(settingThemeGenerator+".mode") != "Midtone" {
		t.Fatal("Space did not cycle the mode")
	}
}

func TestThemeRecipeRepeatableVariedAndReadable(t *testing.T) {
	r := defaultThemeRecipe
	steps := []float64{1, 2, 17, 123, 359, 360, 361, 650, 719, 720, 721, 1440, 12345, 1<<53 - 1}
	for _, mode := range []string{"Dark", "Midtone", "Light"} {
		for i, bg := range steps {
			fg := steps[(i+5)%len(steps)]
			colors := r.generate(mode, bg, fg, 50)
			if !reflect.DeepEqual(colors, r.generate(mode, bg, fg, 50)) {
				t.Fatal("same inputs produced different palettes")
			}
			for name, c := range colors {
				if math.IsNaN(c.l) || c.l < 0 || c.l > 1 || c.c < 0 || c.c > 0.5 || c.h < 0 || c.h >= 360 {
					t.Fatalf("invalid %s color at %s %g/%g: %+v", name, mode, bg, fg, c)
				}
			}
			pairs := map[string][]string{
				"text":           {"background", "alt_background", "text_selection"},
				"secondary_text": {"background", "alt_background"},
				"selected_text":  {"selection"},
				"error":          {"alt_background"}, "info": {"alt_background"},
				"success": {"alt_background"}, "warning": {"alt_background"},
			}
			for text, backgrounds := range pairs {
				for _, background := range backgrounds {
					if ratio := themeContrast(colors[text].rgb(), colors[background].rgb()); ratio < 4.5 {
						t.Fatalf("%s %g/%g: %s on %s contrast %.3f", mode, bg, fg, text, background, ratio)
					}
				}
			}
		}
		tame, wild := r.generate(mode, 1, 1, 50), r.generate(mode, 600, 600, 50)
		if reflect.DeepEqual(tame, wild) {
			t.Fatal("distant inputs generated the same palette")
		}
		// Foreground changes retain the background family; variations are reversible.
		if tame["background"] != r.generate(mode, 1, 400, 50)["background"] {
			t.Fatal("foreground control changed the background")
		}
		// Early steps should feel related rather than redraw the palette.
		next := r.generate(mode, 2, 2, 50)
		for _, role := range []string{"background", "alt_background", "edge", "selection"} {
			a, b := tame[role], next[role]
			if math.Abs(a.l-b.l) > 0.02 || math.Abs(a.c-b.c) > 0.02 || math.Min(wrapHue(a.h-b.h), wrapHue(b.h-a.h)) > 30 {
				t.Fatalf("early %s step jumped in %s", role, mode)
			}
		}
	}
}

func TestThemeRecipeRejectsBrokenDefinitions(t *testing.T) {
	for _, source := range []string{
		strings.Replace(themeGeneratorTOML, "growth_period = 720.0", "growth_period = 0.0", 1),
		strings.Replace(themeGeneratorTOML, "period = 31.0", "period = 0.0", 1),
		strings.Replace(themeGeneratorTOML, "base = \"background\"", "base = \"unknown\"", 1),
		strings.Replace(themeGeneratorTOML, "chroma_wave = 0.035", "chroma_wav = 0.035", 1),
		strings.Replace(themeGeneratorTOML, "name = \"warning\"", "name = \"info\"", 1),
	} {
		if _, err := parseThemeRecipe(source); err == nil {
			t.Fatal("accepted invalid recipe")
		}
	}
}

func TestThemeGeneratorPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional generator previews")
	}
	for _, mode := range []string{"Dark", "Midtone", "Light"} {
		v := newPreviewView()
		v.applySettings()
		v.startSettingsWidget(settingThemeGenerator)
		_ = v.setWidgetValue(settingThemeGenerator+".mode", settings.Value{Text: mode})
		_ = v.setWidgetValue(settingThemeGenerator+".background", settings.Value{Number: 600})
		_ = v.setWidgetValue(settingThemeGenerator+".foreground", settings.Value{Number: 340})
		v.applySettings()
		if err := renderViewPNG(filepath.Join(dir, fmt.Sprintf("generator-%s.png", mode)), v); err != nil {
			t.Fatal(err)
		}
	}
}

func TestThemeGeneratorRespondsEarlyAndExploresLightness(t *testing.T) {
	distance := func(a, b color.NRGBA) float64 {
		return math.Abs(float64(a.R)-float64(b.R)) + math.Abs(float64(a.G)-float64(b.G)) + math.Abs(float64(a.B)-float64(b.B))
	}
	for _, mode := range []string{"Dark", "Midtone", "Light"} {
		first := defaultThemeRecipe.generate(mode, 1, 1, 50)
		for _, control := range []string{"background", "foreground"} {
			bg, fg := 1.0, 1.0
			roles := []string{"background", "alt_background", "border"}
			if control == "background" {
				bg = 2
			} else {
				fg = 2
				roles = []string{"edge", "selection", "text_selection"}
			}
			next := defaultThemeRecipe.generate(mode, bg, fg, 50)
			change := 0.0
			for _, role := range roles {
				change += distance(first[role].rgb(), next[role].rgb())
			}
			if change < 6 {
				t.Fatalf("%s %s first step is effectively invisible: RGB change %g", mode, control, change)
			}
		}
		if mode == "Midtone" {
			continue
		}
		low, high := 1.0, 0.0
		for n := 1.0; n <= 720; n += 23 {
			l := defaultThemeRecipe.generate(mode, n, 1, 50)["background"].l
			low, high = min(low, l), max(high, l)
		}
		if high-low < 0.12 {
			t.Fatalf("%s background lightness stays confined to %.3f–%.3f", mode, low, high)
		}
	}
}

func TestThemeColorfulnessAndForegroundVariation(t *testing.T) {
	roles := []string{"border", "edge", "error", "info", "success", "warning"}
	for _, mode := range []string{"Dark", "Midtone", "Light"} {
		for _, n := range []float64{1, 17, 70, 340, 719} {
			muted := defaultThemeRecipe.generate(mode, n, n, 20)
			vivid := defaultThemeRecipe.generate(mode, n, n, 100)
			low, high := 0.0, 0.0
			for _, role := range roles {
				low += oklchFromRGB(muted[role].rgb()).c
				high += oklchFromRGB(vivid[role].rgb()).c
			}
			if high < low*1.2 {
				t.Fatalf("%s/%g: Colorfulness failed to increase displayed chroma: %.3f to %.3f", mode, n, low, high)
			}
			gray := defaultThemeRecipe.generate(mode, n, n, 0)
			for role, value := range gray {
				rgb := value.rgb()
				if rgb.R != rgb.G || rgb.G != rgb.B {
					t.Fatalf("%s is colored at Colorfulness 0", role)
				}
			}
			for _, palette := range []map[string]oklchColor{muted, vivid, gray} {
				for _, text := range []string{"text", "secondary_text", "error", "info", "success", "warning"} {
					for _, bg := range []string{"background", "alt_background"} {
						if ratio := themeContrast(palette[text].rgb(), palette[bg].rgb()); ratio < 4.5 {
							t.Fatalf("%s/%g %s on %s: contrast %.3f", mode, n, text, bg, ratio)
						}
					}
				}
				if themeContrast(palette["text"].rgb(), palette["text_selection"].rgb()) < 4.5 {
					t.Fatalf("%s/%g text selection lost contrast", mode, n)
				}
			}
		}
		first := defaultThemeRecipe.generate(mode, 1, 1, 50)
		next := defaultThemeRecipe.generate(mode, 1, 2, 50)
		if first["border"].rgb() == next["border"].rgb() {
			t.Fatal("Foreground did not change the border")
		}
		changed := 0
		for _, role := range []string{"error", "info", "success", "warning"} {
			if first[role].rgb() != next[role].rgb() {
				changed++
			}
		}
		if changed < 3 {
			t.Fatalf("%s: first Foreground step changed only %d notification colors", mode, changed)
		}
	}
}
