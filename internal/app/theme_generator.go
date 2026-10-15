package app

import (
	_ "embed"
	"fmt"
	"image/color"
	"maps"
	"math"
	"slices"

	"github.com/BurntSushi/toml"
	"github.com/dannyben/commando/internal/settings"
)

const settingThemeGenerator = "appearance.theme.generator"

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
		Saturation      float64
		CutOffset       float64 `toml:"cut_offset"`
		HiddenOffset    float64 `toml:"hidden_offset"`
		DotfileOffset   float64 `toml:"dotfile_offset"`
	}
	Colors map[string]themeRecipeColor
	order  []string // Dependency order, independent of TOML table order.
}

type themeRecipeColor struct {
	Base, Input     string
	Saturation      *float64
	LightnessOffset float64 `toml:"lightness_offset"`
	LightnessWave   float64 `toml:"lightness_wave"`
	SaturationWave  float64 `toml:"saturation_wave"`
	HueOffset       float64 `toml:"hue_offset"`
	HueWave         float64 `toml:"hue_wave"`
	Period, Phase   float64
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
		if !ok || !finite(m.Background) || m.Background < 0 || m.Background > 100 || !finite(m.Text) || m.Text < 0 || m.Text > 100 || !finite(m.Filename) || m.Filename < 0 || m.Filename > 100 || m.Direction != 1 && m.Direction != -1 || !finite(m.LightnessWaveMultiplier) || m.LightnessWaveMultiplier <= 0 || m.LightnessWaveMultiplier > 1 {
			return r, fmt.Errorf("invalid mode: %s", name)
		}
	}
	for _, offset := range []float64{r.Files.Saturation, r.Files.SelectionOffset, r.Files.CutOffset, r.Files.HiddenOffset, r.Files.DotfileOffset} {
		if !finite(offset) || offset < 0 || offset > 100 {
			return r, fmt.Errorf("invalid file lightness offset")
		}
	}
	known := map[string]bool{"mode_background": true, "mode_text": true}
	for _, spec := range themeElements() {
		known[spec.Key[len(themeSettingPrefix):]] = true
	}
	for name, c := range r.Colors {
		if !known[name] || name == "mode_background" || name == "mode_text" || !known[c.Base] || c.Input != "background" && c.Input != "foreground" {
			return r, fmt.Errorf("invalid color or base: %s", name)
		}
		for _, n := range []float64{c.Period, c.Phase, c.LightnessOffset, c.LightnessWave, c.SaturationWave, c.HueOffset, c.HueWave} {
			if !finite(n) {
				return r, fmt.Errorf("non-finite color parameter: %s", name)
			}
		}
		if c.Period <= 0 || c.Saturation != nil && (!finite(*c.Saturation) || *c.Saturation < 0 || *c.Saturation > 100) {
			return r, fmt.Errorf("invalid period or saturation: %s", name)
		}
	}
	// Only defined bases are dependencies. Omitted bases come from the palette.
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) error
	visit = func(name string) error {
		if done[name] {
			return nil
		}
		if visiting[name] {
			return fmt.Errorf("cyclic color base: %s", name)
		}
		visiting[name] = true
		base := r.Colors[name].Base
		if _, included := r.Colors[base]; included {
			if err := visit(base); err != nil {
				return err
			}
		}
		delete(visiting, name)
		done[name] = true
		r.order = append(r.order, name)
		return nil
	}
	for _, name := range slices.Sorted(maps.Keys(r.Colors)) {
		if err := visit(name); err != nil {
			return r, err
		}
	}

	return r, nil
}

func (r themeRecipe) strength(n float64) float64 {
	return r.Scale.MinimumStrength + (1-r.Scale.MinimumStrength)*math.Mod(n-1, r.Scale.GrowthPeriod)/(r.Scale.GrowthPeriod-1)
}

func (r themeRecipe) generate(mode string, background, foreground, colorfulness float64, retained map[string]lshColor) map[string]oklchColor {
	m := r.Modes[mode]
	if retained == nil {
		retained = themePalette(settings.Values{})
	}
	colors := maps.Clone(retained)
	colors["mode_background"] = lshColor{l: m.Background, h: wrapHue(360 * math.Mod(background-1, r.Scale.HuePeriod) / r.Scale.HuePeriod)}
	colors["mode_text"] = lshColor{l: m.Text}

	for _, name := range r.order {
		c := r.Colors[name]
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
		value.l = max(0, min(100, value.l+m.Direction*(c.LightnessOffset+c.LightnessWave*wave*lightnessScale)))
		if c.Saturation != nil {
			value.s = *c.Saturation
		}
		value.s = max(0, min(100, value.s+c.SaturationWave*wave))
		value.h = wrapHue(value.h + c.HueOffset + c.HueWave*wave)
		colors[name] = value
	}
	// Apply saturation scaling after resolving bases so inherited colors scale once.
	for name, c := range r.Colors {
		value := colors[name]
		multiplier := math.Pow(colorfulness/50, r.Scale.ColorfulnessExponent)
		if c.Input == "background" {
			multiplier = math.Pow(multiplier, r.Scale.BackgroundColorfulnessExponent)
		}
		value.s = min(100, value.s*multiplier)
		colors[name] = value
	}
	resolved := make(map[string]oklchColor, len(colors))
	for name, value := range colors {
		resolved[name] = value.oklch()
	}
	// Measure the displayed sRGB colors, including gamut mapping and rounding.
	// Main text also appears in the location bar and native text selection.
	backgrounds := []color.NRGBA{resolved["background"].rgb(), resolved["alt_background"].rgb(), resolved["text_selection"].rgb()}
	// Contrast correction may only update colors included in this recipe.
	for _, name := range []string{"text", "secondary_text", "error", "info", "success", "warning"} {
		if _, included := r.Colors[name]; !included {
			continue
		}
		against := backgrounds[:2]
		if name == "text" {
			against = backgrounds
		}
		resolved[name] = readableThemeColor(resolved[name], against, 4.5)
	}
	for name := range resolved {
		if _, included := r.Colors[name]; !included {
			delete(resolved, name)
		}
	}

	return resolved
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
	colors := defaultThemeRecipe.generate(mode, get("background").Number, get("foreground").Number, get("colorfulness").Number, themePalette(v.settingsValues))
	for name, value := range colors {
		lsh := lshFromOKLCH(value)
		for component, n := range map[string]float64{"lightness": lsh.l, "saturation": lsh.s, "hue": lsh.h} {
			_ = v.settingsValues.Set(c, themeSettingPrefix+name+"."+component, settings.Value{Number: colorSettingNumber(n)})
		}
	}

	m := defaultThemeRecipe.Modes[mode]
	for key, n := range map[string]float64{
		settingLightness:          m.Filename,
		settingSaturation:         min(100, defaultThemeRecipe.Files.Saturation*math.Pow(get("colorfulness").Number/50, defaultThemeRecipe.Scale.ColorfulnessExponent)),
		settingSelectionLightness: m.Direction * defaultThemeRecipe.Files.SelectionOffset,
		settingCutLightness:       -m.Direction * defaultThemeRecipe.Files.CutOffset,
		settingHiddenLightness:    -m.Direction * defaultThemeRecipe.Files.HiddenOffset,
		settingDotfileLightness:   -m.Direction * defaultThemeRecipe.Files.DotfileOffset,
	} {
		_ = v.settingsValues.Set(c, key, settings.Value{Number: n})
	}
}
