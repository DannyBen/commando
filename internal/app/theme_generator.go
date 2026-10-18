package app

import (
	"crypto/sha256"
	_ "embed"
	"encoding/binary"
	"fmt"
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
	Response struct {
		DynamicsExponent float64 `toml:"dynamics_exponent"`
		VibranceExponent float64 `toml:"vibrance_exponent"`
	}
	Modes map[string]struct {
		Background, Text, Direction, Filename float64
		FilenameSelectionOffset               float64  `toml:"filename_selection_offset"`
		LightnessSpread                       float64  `toml:"lightness_spread"`
		SelectionLightness                    *float64 `toml:"selection_lightness"`
		EdgeLightness                         *float64 `toml:"edge_lightness"`
	}
	Files struct {
		Saturation    float64
		SaturationMax float64 `toml:"saturation_max"`
		CutOffset     float64 `toml:"cut_offset"`
		HiddenOffset  float64 `toml:"hidden_offset"`
		DotfileOffset float64 `toml:"dotfile_offset"`
	}
	Colors map[string]themeRecipeColor
	order  []string // Dependency order, independent of TOML table order.
}

type themeRecipeColor struct {
	Base                       string
	Lightness, Saturation, Hue themeRecipeComponent
}

type themeRecipeComponent struct {
	Center *float64
	Offset float64
	Range  []float64
}

type themeGeneration struct {
	Mode                           string
	Seed                           uint64
	Vibrance, Dynamics, ColorShift float64
}

// A separate keyed draw for every coordinate keeps unrelated rules stable.
// Hashing explicit bytes also makes themes reproducible across platforms.
func themeRandom(seed uint64, key string) float64 {
	var encoded [8]byte
	binary.LittleEndian.PutUint64(encoded[:], seed)
	sum := sha256.Sum256(append(encoded[:], key...))
	return float64(binary.LittleEndian.Uint64(sum[:8])>>11) / (1 << 53)
}

func (c themeRecipeComponent) value(base, random, dynamics, direction float64) float64 {
	center := base
	if c.Center != nil {
		center = *c.Center
	}
	adjustment := 0.0
	if len(c.Range) == 2 {
		adjustment = (c.Range[0] + (c.Range[1]-c.Range[0])*random) * dynamics
	}
	return center + direction*(c.Offset+adjustment)
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
	for _, exponent := range []float64{r.Response.DynamicsExponent, r.Response.VibranceExponent} {
		if !finite(exponent) || exponent <= 0 || exponent > 4 {
			return r, fmt.Errorf("invalid response exponent")
		}
	}
	for _, name := range []string{"Black", "Dark", "Midtone", "Light", "White"} {
		m, ok := r.Modes[name]
		if !ok || !finite(m.Background) || m.Background < 0 || m.Background > 100 || !finite(m.Text) || m.Text < 0 || m.Text > 100 || !finite(m.Filename) || m.Filename < 0 || m.Filename > 100 || (m.Direction != 1 && m.Direction != -1) || !finite(m.LightnessSpread) || m.LightnessSpread <= 0 || m.LightnessSpread > 1 {
			return r, fmt.Errorf("invalid mode: %s", name)
		}
		if !finite(m.FilenameSelectionOffset) || math.Abs(m.FilenameSelectionOffset) > 100 {
			return r, fmt.Errorf("invalid filename selection offset: %s", name)
		}
		for _, n := range []*float64{m.SelectionLightness, m.EdgeLightness} {
			if n != nil && (!finite(*n) || *n < 0 || *n > 100) {
				return r, fmt.Errorf("invalid accent lightness: %s", name)
			}
		}
	}
	for _, offset := range []float64{r.Files.Saturation, r.Files.SaturationMax, r.Files.CutOffset, r.Files.HiddenOffset, r.Files.DotfileOffset} {
		if !finite(offset) || offset < 0 || offset > 100 {
			return r, fmt.Errorf("invalid file lightness offset")
		}
	}
	known := map[string]bool{"mode_background": true, "mode_text": true}
	for _, spec := range themeElements() {
		known[spec.Key[len(themeSettingPrefix):]] = true
	}
	for name, c := range r.Colors {
		if !known[name] || name == "mode_background" || name == "mode_text" || !known[c.Base] {
			return r, fmt.Errorf("invalid color or base: %s", name)
		}
		for component, w := range map[string]themeRecipeComponent{"lightness": c.Lightness, "saturation": c.Saturation, "hue": c.Hue} {
			limit := 100.0
			if component == "hue" {
				limit = 360
			}
			valid := finite(w.Offset) && (len(w.Range) == 0 || len(w.Range) == 2)
			if len(w.Range) == 2 {
				valid = valid && finite(w.Range[0]) && finite(w.Range[1]) && w.Range[0] <= w.Range[1]
			}
			if w.Center != nil {
				valid = valid && finite(*w.Center) && *w.Center >= 0 && *w.Center <= limit
			}
			if !valid {
				return r, fmt.Errorf("invalid %s component: %s", component, name)
			}
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

func (r themeRecipe) generate(options themeGeneration, retained map[string]lshColor) map[string]oklchColor {
	m := r.Modes[options.Mode]
	if retained == nil {
		retained = themePalette(settings.Values{})
	}
	colors := maps.Clone(retained)
	hue := 360 * themeRandom(options.Seed, "hue")
	colors["mode_background"] = lshColor{l: m.Background, h: hue}
	colors["mode_text"] = lshColor{l: m.Text, h: hue}
	dynamics := math.Pow((options.Dynamics-1)/99, r.Response.DynamicsExponent)
	for _, name := range r.order {
		c := r.Colors[name]
		value := colors[c.Base]
		var anchor *float64
		switch name {
		case "selection":
			anchor = m.SelectionLightness
		case "edge":
			anchor = m.EdgeLightness
		}
		if anchor != nil {
			c.Lightness.Center, c.Lightness.Offset = anchor, 0
		}
		value.l = max(0, min(100, c.Lightness.value(value.l, themeRandom(options.Seed, name+".lightness"), dynamics, m.Direction*m.LightnessSpread)))
		value.s = max(0, min(100, c.Saturation.value(value.s, themeRandom(options.Seed, name+".saturation"), dynamics, 1)))
		value.h = wrapHue(c.Hue.value(value.h, themeRandom(options.Seed, name+".hue"), dynamics, 1))
		colors[name] = value
	}
	// Global adjustments apply once, after all base dependencies resolve.
	for name := range r.Colors {
		value := colors[name]
		value.s = min(100, value.s*math.Pow(options.Vibrance/50, r.Response.VibranceExponent))
		value.h = wrapHue(value.h + options.ColorShift)
		colors[name] = value
	}
	resolved := make(map[string]oklchColor, len(colors))
	for name, value := range colors {
		resolved[name] = value.oklch()
	}

	for name := range resolved {
		if _, included := r.Colors[name]; !included {
			delete(resolved, name)
		}
	}

	return resolved
}

// Generator output becomes ordinary editable colors. Subsequent Customize
// edits stay intact until another generator value is applied.
func (v *view) generateTheme() {
	values := v.widgetValues()
	c := settings.CatalogueDefault
	get := func(key string) settings.Value { return values.Get(c, settingThemeGenerator+"."+key) }
	mode := get("mode").Text
	colors := defaultThemeRecipe.generate(themeGeneration{
		Mode: mode, Seed: uint64(get("seed").Number), Vibrance: get("vibrance").Number,
		Dynamics: get("dynamics").Number, ColorShift: get("color_shift").Number,
	}, themePalette(*values))
	for name, value := range colors {
		lsh := lshFromOKLCH(value)
		for component, n := range map[string]float64{"lightness": lsh.l, "saturation": lsh.s, "hue": lsh.h} {
			_ = values.Set(c, themeSettingPrefix+name+"."+component, settings.Value{Number: colorSettingNumber(n)})
		}
	}

	m := defaultThemeRecipe.Modes[mode]
	for key, n := range map[string]float64{
		settingLightness:          m.Filename,
		settingSaturation:         min(defaultThemeRecipe.Files.SaturationMax, defaultThemeRecipe.Files.Saturation*math.Pow(get("vibrance").Number/50, defaultThemeRecipe.Response.VibranceExponent)),
		settingSelectionLightness: m.FilenameSelectionOffset,
		settingCutLightness:       -m.Direction * defaultThemeRecipe.Files.CutOffset,
		settingHiddenLightness:    -m.Direction * defaultThemeRecipe.Files.HiddenOffset,
		settingDotfileLightness:   -m.Direction * defaultThemeRecipe.Files.DotfileOffset,
	} {
		_ = values.Set(c, key, settings.Value{Number: n})
	}
}
