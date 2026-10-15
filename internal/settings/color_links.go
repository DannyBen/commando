package settings

import (
	_ "embed"
	"fmt"
	"math"
	"strings"

	"github.com/BurntSushi/toml"
)

const themePrefix = "appearance.theme."

//go:embed theme-links.toml
var themeLinksTOML string

type colorLink struct {
	Source string
	Scale  []float64
	Offset []float64
}
type colorLinks map[string]map[string]colorLink

var themeLinks = mustColorLinks(themeLinksTOML, CatalogueDefault)

func mustColorLinks(source string, c Catalogue) colorLinks {
	links, err := parseColorLinks(source, c)
	if err != nil {
		panic(err)
	}
	return links
}

func parseColorLinks(source string, c Catalogue) (colorLinks, error) {
	var links colorLinks
	md, err := toml.Decode(source, &links)
	if err != nil {
		return nil, err
	}
	if len(md.Undecoded()) > 0 {
		return nil, fmt.Errorf("unknown color link field: %s", md.Undecoded()[0])
	}
	for role, components := range links {
		if d, ok := c.Definition(themePrefix + role + ".linked"); !ok || d.Kind != Boolean {
			return nil, fmt.Errorf("color link needs toggle: %s", role)
		}
		for component, link := range components {
			if _, ok := colorComponent(component); !ok {
				return nil, fmt.Errorf("unknown color component: %s", component)
			}
			for _, name := range []string{role, link.Source} {
				if d, ok := c.Definition(themePrefix + name + "." + component); !ok || d.Kind != Number {
					return nil, fmt.Errorf("unknown color link role: %s", name)
				}
			}
			for _, pair := range [][]float64{link.Scale, link.Offset} {
				if len(pair) != 2 || !finite(pair[0]) || !finite(pair[1]) || pair[0] > pair[1] {
					return nil, fmt.Errorf("invalid color link range: %s.%s", role, component)
				}
			}
			seen := map[string]bool{role: true}
			for parent := link.Source; parent != ""; parent = links[parent][component].Source {
				if seen[parent] {
					return nil, fmt.Errorf("color link cycle: %s.%s", role, component)
				}
				seen[parent] = true
			}
		}
	}
	return links, nil
}

func colorComponent(name string) (float64, bool) {
	switch name {
	case "lightness":
		return 1, true
	case "chroma":
		return 0.5, true
	case "hue":
		return 360, true
	}
	return 0, false
}

// ColorRange stores actual values. A hue arc may extend past 360 internally;
// controls and persisted colors always use canonical angles in [0,360).
type ColorRange struct {
	Minimum, Maximum float64
	Circular         bool
}

func wrapColorHue(n float64) float64 { return math.Mod(math.Mod(n, 360)+360, 360) }
func (r ColorRange) position(n float64) float64 {
	if r.Circular {
		return r.Minimum + wrapColorHue(n-r.Minimum)
	}
	return n
}
func (r ColorRange) Clamp(n float64) float64 {
	if !r.Circular {
		return max(r.Minimum, min(r.Maximum, n))
	}
	n = r.position(n)
	if n > r.Maximum {
		if n-r.Maximum <= r.Minimum+360-n {
			n = r.Maximum
		} else {
			n = r.Minimum
		}
	}
	return wrapColorHue(n)
}
func (r ColorRange) Step(n, delta float64) float64 {
	if r.Circular && r.Maximum-r.Minimum >= 360 {
		return wrapColorHue(n + delta)
	}
	n = r.position(r.Clamp(n))
	n = max(r.Minimum, min(r.Maximum, n+delta))
	if r.Circular {
		return wrapColorHue(n)
	}
	return n
}
func (r ColorRange) Progress(n float64) float64 {
	if r.Maximum == r.Minimum {
		return 1
	}
	return max(0, min(1, (r.position(r.Clamp(n))-r.Minimum)/(r.Maximum-r.Minimum)))
}
func (r ColorRange) Label() string {
	if r.Circular && r.Maximum > 360 {
		return fmt.Sprintf("%g–360 / 0–%g", roundColor(r.Minimum), roundColor(r.Maximum-360))
	}
	return fmt.Sprintf("%g–%g", roundColor(r.Minimum), roundColor(r.Maximum))
}
func roundColor(n float64) float64 { return math.Round(n*10000) / 10000 }

func ThemeColorRange(values Values, c Catalogue, key string) (ColorRange, bool) {
	return themeLinks.bounds(values, c, key)
}
func (links colorLinks) bounds(values Values, c Catalogue, key string) (ColorRange, bool) {
	path, ok := strings.CutPrefix(key, themePrefix)
	if !ok {
		return ColorRange{}, false
	}
	role, component, ok := strings.Cut(path, ".")
	maximum, valid := colorComponent(component)
	if !ok || !valid {
		return ColorRange{}, false
	}
	r := ColorRange{0, maximum, component == "hue"}
	link, linked := links[role][component]
	if !linked || !values.Get(c, themePrefix+role+".linked").Bool {
		return r, true
	}
	sourceKey := themePrefix + link.Source + "." + component
	sourceRange, _ := links.bounds(values, c, sourceKey)
	source := sourceRange.Clamp(values.Get(c, sourceKey).Number)
	a, b := source*link.Scale[0], source*link.Scale[1]
	r.Minimum, r.Maximum = min(a, b)+link.Offset[0], max(a, b)+link.Offset[1]
	if r.Circular {
		span := r.Maximum - r.Minimum
		if span >= 360 {
			r.Minimum, r.Maximum = 0, 360
		} else {
			r.Minimum = wrapColorHue(r.Minimum)
			r.Maximum = r.Minimum + span
		}
	} else {
		r.Minimum = max(0, min(maximum, r.Minimum))
		r.Maximum = max(r.Minimum, min(maximum, r.Maximum))
	}
	return r, true
}

// Normalize every dependent component against resolved parents. A clamp is a
// real value change, included in the same Save/Cancel transaction as its cause.
func ClampThemeColors(values *Values, c Catalogue) {
	resolved := map[string]float64{}
	var resolve func(string) float64
	resolve = func(key string) float64 {
		if n, ok := resolved[key]; ok {
			return n
		}
		path := strings.TrimPrefix(key, themePrefix)
		role, component, _ := strings.Cut(path, ".")
		if link, ok := themeLinks[role][component]; ok && values.Get(c, themePrefix+role+".linked").Bool {
			parent := themePrefix + link.Source + "." + component
			n := resolve(parent)
			if n != values.Get(c, parent).Number {
				_ = values.Set(c, parent, Value{Number: n})
			}
		}
		n := values.Get(c, key).Number
		if bounds, ok := ThemeColorRange(*values, c, key); ok {
			n = bounds.Clamp(n)
		}
		resolved[key] = n
		return n
	}
	for _, d := range c.Settings {
		if _, ok := ThemeColorRange(*values, c, d.Key); !ok {
			continue
		}
		n := resolve(d.Key)
		if n != values.Get(c, d.Key).Number {
			_ = values.Set(c, d.Key, Value{Number: n})
		}
	}
}
