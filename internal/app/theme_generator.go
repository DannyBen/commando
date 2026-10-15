package app

import (
	_ "embed"
	"fmt"
	"image/color"
	"math"

	"github.com/BurntSushi/toml"
	"github.com/dannyben/commando/internal/settings"
)

const settingThemeGenerator = themeSettingPrefix + "generator"

//go:embed theme-generator.toml
var themeGeneratorTOML string

var defaultThemeRecipe = mustThemeRecipe()

type themeRecipe struct {
	Scale struct {
		GrowthPeriod                   float64 `toml:"growth_period"`
		MinimumStrength                float64 `toml:"minimum_strength"`
		HuePeriod                      float64 `toml:"hue_period"`
		BackgroundColorfulnessExponent float64 `toml:"background_colorfulness_exponent"`
		ColorfulnessExponent           float64 `toml:"colorfulness_exponent"`
	}
	Modes map[string]struct {
		Background, Text, Direction, Filename float64
		LightnessWaveMultiplier               float64 `toml:"lightness_wave_multiplier"`
	}
	Files struct {
		SelectionOffset float64 `toml:"selection_offset"`
		Chroma          float64
		CutOffset       float64 `toml:"cut_offset"`
		HiddenOffset    float64 `toml:"hidden_offset"`
		DotfileOffset   float64 `toml:"dotfile_offset"`
	}
	Colors []struct {
		Name, Base, Input string
		Chroma            *float64
		LightnessOffset   float64 `toml:"lightness_offset"`
		LightnessWave     float64 `toml:"lightness_wave"`
		ChromaWave        float64 `toml:"chroma_wave"`
		HueOffset         float64 `toml:"hue_offset"`
		HueWave           float64 `toml:"hue_wave"`
		Period, Phase     float64
	}
}

func mustThemeRecipe() themeRecipe {
	r, err := parseThemeRecipe(themeGeneratorTOML)
	if err != nil {
		panic(fmt.Errorf("embedded theme recipe: %w", err))
	}
	return r
}

func parseThemeRecipe(source string) (themeRecipe, error) {
	var r themeRecipe
	md, err := toml.Decode(source, &r)
	if err != nil {
		return r, err
	}
	if len(md.Undecoded()) != 0 {
		return r, fmt.Errorf("unknown recipe keys: %v", md.Undecoded())
	}
	finite := func(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) }
	if !finite(r.Scale.GrowthPeriod) || r.Scale.GrowthPeriod < 2 || !finite(r.Scale.HuePeriod) || r.Scale.HuePeriod <= 0 || !finite(r.Scale.MinimumStrength) || r.Scale.MinimumStrength < 0 || r.Scale.MinimumStrength > 1 || !finite(r.Scale.BackgroundColorfulnessExponent) || r.Scale.BackgroundColorfulnessExponent <= 0 || r.Scale.BackgroundColorfulnessExponent > 1 || !finite(r.Scale.ColorfulnessExponent) || r.Scale.ColorfulnessExponent <= 0 || r.Scale.ColorfulnessExponent > 4 {
		return r, fmt.Errorf("invalid variation scale")
	}
	for _, name := range []string{"Dark", "Midtone", "Light"} {
		m, ok := r.Modes[name]
		if !ok || !finite(m.Background) || m.Background < 0 || m.Background > 1 || !finite(m.Text) || m.Text < 0 || m.Text > 1 || !finite(m.Filename) || m.Filename < 0 || m.Filename > 1 || m.Direction != 1 && m.Direction != -1 || !finite(m.LightnessWaveMultiplier) || m.LightnessWaveMultiplier <= 0 || m.LightnessWaveMultiplier > 1 {
			return r, fmt.Errorf("invalid mode: %s", name)
		}
	}
	for _, offset := range []float64{r.Files.Chroma, r.Files.SelectionOffset, r.Files.CutOffset, r.Files.HiddenOffset, r.Files.DotfileOffset} {
		if !finite(offset) || offset < 0 || offset > 1 {
			return r, fmt.Errorf("invalid file lightness offset")
		}
	}
	seen := map[string]bool{"mode_background": true, "mode_text": true}
	for _, c := range r.Colors {
		_, known := settings.CatalogueDefault.Widget(themeSettingPrefix + c.Name)
		if !known || c.Name == "generator" || seen[c.Name] || !seen[c.Base] || c.Input != "background" && c.Input != "foreground" {
			return r, fmt.Errorf("invalid color or base: %s", c.Name)
		}
		for _, n := range []float64{c.Period, c.Phase, c.LightnessOffset, c.LightnessWave, c.ChromaWave, c.HueOffset, c.HueWave} {
			if !finite(n) {
				return r, fmt.Errorf("non-finite color parameter: %s", c.Name)
			}
		}
		if c.Period <= 0 || c.Chroma != nil && (!finite(*c.Chroma) || *c.Chroma < 0 || *c.Chroma > 0.5) {
			return r, fmt.Errorf("invalid period or chroma: %s", c.Name)
		}
		seen[c.Name] = true
	}
	for _, spec := range themeElements() {
		if !seen[spec.Key[len(themeSettingPrefix):]] {
			return r, fmt.Errorf("missing color: %s", spec.Key)
		}
	}
	return r, nil
}

func (r themeRecipe) strength(n float64) float64 {
	return r.Scale.MinimumStrength + (1-r.Scale.MinimumStrength)*math.Mod(n-1, r.Scale.GrowthPeriod)/(r.Scale.GrowthPeriod-1)
}

func (r themeRecipe) generate(mode string, background, foreground, colorfulness float64) map[string]oklchColor {
	m := r.Modes[mode]
	colors := map[string]oklchColor{
		"mode_background": {l: m.Background, h: wrapHue(360 * math.Mod(background-1, r.Scale.HuePeriod) / r.Scale.HuePeriod)},
		"mode_text":       {l: m.Text},
	}
	for _, c := range r.Colors {
		n := background
		if c.Input == "foreground" {
			n = foreground
		}
		wave := math.Sin(2*math.Pi*(math.Mod(n-1, c.Period)+math.Mod(c.Phase, c.Period))/c.Period) * r.strength(n)
		value := colors[c.Base]
		lightnessScale := 1.0
		if c.Input == "background" {
			lightnessScale = m.LightnessWaveMultiplier
		}
		value.l = clampLightness(value.l + m.Direction*(c.LightnessOffset+c.LightnessWave*wave*lightnessScale))
		if c.Chroma != nil {
			value.c = *c.Chroma
		}
		value.c = max(0, min(0.5, value.c+c.ChromaWave*wave))
		value.h = wrapHue(value.h + c.HueOffset + c.HueWave*wave)
		colors[c.Name] = value
	}
	// Apply chroma scaling after resolving bases so inherited colors scale once.
	for _, c := range r.Colors {
		value := colors[c.Name]
		multiplier := math.Pow(colorfulness/50, r.Scale.ColorfulnessExponent)
		if c.Input == "background" {
			multiplier = math.Pow(multiplier, r.Scale.BackgroundColorfulnessExponent)
		}
		value.c = min(0.5, value.c*multiplier)
		colors[c.Name] = value
	}
	// Measure the displayed sRGB colors, including gamut mapping and rounding.
	// Main text also appears in the location bar and native text selection.
	backgrounds := []color.NRGBA{colors["background"].rgb(), colors["alt_background"].rgb(), colors["text_selection"].rgb()}
	colors["text"] = readableThemeColor(colors["text"], backgrounds, 4.5)
	colors["secondary_text"] = readableThemeColor(colors["secondary_text"], backgrounds[:2], 4.5)
	colors["selected_text"] = readableThemeColor(colors["selected_text"], []color.NRGBA{colors["selection"].rgb()}, 4.5)
	for _, role := range []string{"error", "info", "success", "warning"} {
		colors[role] = readableThemeColor(colors[role], backgrounds[:2], 4.5)
	}
	delete(colors, "mode_background")
	delete(colors, "mode_text")
	return colors
}

func themeLuminance(c color.NRGBA) float64 {
	linear := func(n uint8) float64 {
		x := float64(n) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
}

func themeContrast(a, b color.NRGBA) float64 {
	x, y := themeLuminance(a), themeLuminance(b)
	return (max(x, y) + 0.05) / (min(x, y) + 0.05)
}

// Keep the intended color when readable; otherwise move its lightness toward
// the nearest readable endpoint. Chroma/hue survive, subject to gamut mapping.
func readableThemeColor(value oklchColor, backgrounds []color.NRGBA, ratio float64) oklchColor {
	score := func(c oklchColor) float64 {
		minimum := math.Inf(1)
		for _, bg := range backgrounds {
			minimum = min(minimum, themeContrast(c.rgb(), bg))
		}
		return minimum
	}
	if score(value) >= ratio {
		return value
	}
	best, distance := value, math.Inf(1)
	for _, endpoint := range []float64{0, 1} {
		candidate := value
		candidate.l = endpoint
		if score(candidate) < ratio {
			continue
		}
		low, high := 0.0, 1.0
		for range 24 {
			middle := (low + high) / 2
			candidate.l = value.l + (endpoint-value.l)*middle
			if score(candidate) >= ratio {
				high = middle
			} else {
				low = middle
			}
		}
		candidate.l = value.l + (endpoint-value.l)*high
		if d := math.Abs(candidate.l - value.l); d < distance {
			best, distance = candidate, d
		}
	}
	return best
}

// Generator output becomes ordinary editable colors. Subsequent Customize
// edits stay intact until another generator value is applied.
func (v *view) generateTheme() {
	c := settings.CatalogueDefault
	get := func(key string) settings.Value { return v.settingsValues.Get(c, settingThemeGenerator+"."+key) }
	mode := get("mode").Text
	colors := defaultThemeRecipe.generate(mode, get("background").Number, get("foreground").Number, get("colorfulness").Number)
	for name, value := range colors {
		for component, n := range map[string]float64{"lightness": value.l, "chroma": value.c, "hue": value.h} {
			_ = v.settingsValues.Set(c, themeSettingPrefix+name+"."+component, settings.Value{Number: n})
		}
	}
	m := defaultThemeRecipe.Modes[mode]
	for key, n := range map[string]float64{
		settingLightness:          m.Filename,
		settingChroma:             min(0.5, defaultThemeRecipe.Files.Chroma*math.Pow(get("colorfulness").Number/50, defaultThemeRecipe.Scale.ColorfulnessExponent)),
		settingSelectionLightness: m.Direction * defaultThemeRecipe.Files.SelectionOffset,
		settingCutLightness:       -m.Direction * defaultThemeRecipe.Files.CutOffset,
		settingHiddenLightness:    -m.Direction * defaultThemeRecipe.Files.HiddenOffset,
		settingDotfileLightness:   -m.Direction * defaultThemeRecipe.Files.DotfileOffset,
	} {
		_ = v.settingsValues.Set(c, key, settings.Value{Number: n})
	}
}
