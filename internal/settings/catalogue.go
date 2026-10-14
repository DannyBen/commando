package settings

import (
	_ "embed"
	"fmt"
	"math"
	"strings"

	"github.com/BurntSushi/toml"
)

//go:embed settings.toml
var defaultTOML string

//go:embed settings-ui.toml
var uiTOML string

var CatalogueDefault = embeddedCatalogue()

func embeddedCatalogue() Catalogue {
	c, err := ParseCatalogue(defaultTOML, uiTOML)
	if err != nil {
		panic(fmt.Errorf("embedded settings: %w", err))
	}
	return c
}

type fieldMetadata struct {
	Label, Description, Control string
	Minimum                     *float64 `toml:"min"`
	Maximum                     *float64 `toml:"max"`
	Step                        float64
}

// ParseCatalogue compiles nested defaults and matching UI metadata into the
// ordered catalogue used by generic widgets. Value paths are independent of
// control choices. Palette groups follow parent value paths.
func ParseCatalogue(defaults, presentation string) (Catalogue, error) {
	var tree map[string]any
	values, err := toml.Decode(defaults, &tree)
	if err != nil {
		return Catalogue{}, err
	}
	var ui map[string]toml.Primitive
	metadata, err := toml.Decode(presentation, &ui)
	if err != nil {
		return Catalogue{}, err
	}
	var c Catalogue
	groups := map[string]bool{}
	knownMetadata := map[string]bool{}
	for _, path := range values.Keys() {
		scalar, ok := lookup(tree, path)
		if !ok {
			continue
		}
		if _, table := scalar.(map[string]any); table {
			continue
		}
		for _, part := range path {
			if part == "" || strings.Contains(part, ".") {
				return c, fmt.Errorf("ambiguous setting path: %s", path)
			}
		}
		d := Definition{Key: strings.Join(path, "."), Default: valueFromScalar(scalar), Minimum: math.Inf(-1)}
		switch scalar.(type) {
		case bool:
			d.Kind = Boolean
		case float64:
			d.Kind = Number
		case string:
			d.Kind = String
		default:
			return c, fmt.Errorf("unsupported value type: %s", d.Key)
		}
		// Every parent on the value path is a group. Decode only its label;
		// descendants are validated as their own groups or fields below.
		for depth := 1; depth < len(path); depth++ {
			parentPath := path[:depth]
			key := strings.Join(parentPath, ".")
			if groups[key] {
				continue
			}
			primitive, err := metadataAt(&metadata, ui, parentPath)
			if err != nil {
				return c, err
			}
			var group struct{ Label string }
			if err := metadata.PrimitiveDecode(primitive, &group); err != nil {
				return c, err
			}
			if group.Label == "" {
				return c, fmt.Errorf("missing group label: %s", key)
			}
			c.UI.Groups = append(c.UI.Groups, Group{Key: key, Parent: strings.Join(parentPath[:depth-1], "."), Label: group.Label})
			groups[key] = true
			knownMetadata[key], knownMetadata[key+".label"] = true, true
		}
		primitive, err := metadataAt(&metadata, ui, path)
		if err != nil {
			return c, err
		}
		var m fieldMetadata
		if err := metadata.PrimitiveDecode(primitive, &m); err != nil {
			return c, err
		}
		if m.Label == "" {
			return c, fmt.Errorf("missing field label: %s", d.Key)
		}
		group := strings.Join(path[:len(path)-1], ".")
		if group == "" {
			group = d.Key
			c.UI.Groups = append(c.UI.Groups, Group{Key: group, Label: m.Label})
		}
		if m.Control == "" {
			m.Control = "input"
			if d.Kind == Boolean {
				m.Control = "toggle"
			}
		}
		if m.Minimum != nil {
			d.Minimum = *m.Minimum
		}
		d.Maximum, d.Step = m.Maximum, m.Step
		compatible := m.Control == "input" ||
			d.Kind == Boolean && m.Control == "toggle" ||
			d.Kind == String && (m.Control == "folder" || m.Control == "file") ||
			d.Kind == Number && (m.Control == "stepper" || m.Control == "stepper-progress")
		if !compatible {
			return c, fmt.Errorf("incompatible control: %s", d.Key)
		}
		if d.Kind != Number && (m.Minimum != nil || m.Maximum != nil || m.Step != 0) {
			return c, fmt.Errorf("numeric constraints on non-number: %s", d.Key)
		}
		if d.Kind == Number {
			if m.Minimum != nil && !finite(*m.Minimum) || m.Maximum != nil && (!finite(*m.Maximum) || *m.Maximum < d.Minimum) || !finite(m.Step) || m.Step < 0 {
				return c, fmt.Errorf("invalid numeric constraints: %s", d.Key)
			}
			if strings.HasPrefix(m.Control, "stepper") && d.Step <= 0 {
				return c, fmt.Errorf("stepper needs positive step: %s", d.Key)
			}
			if m.Control == "stepper-progress" && (m.Minimum == nil || m.Maximum == nil || *m.Maximum <= d.Minimum) {
				return c, fmt.Errorf("progress needs finite range: %s", d.Key)
			}
		}
		var check Values
		if err := check.Set(Catalogue{Settings: []Definition{d}}, d.Key, d.Default); err != nil {
			return c, err
		}
		c.Settings = append(c.Settings, d)
		c.UI.Fields = append(c.UI.Fields, Field{Key: d.Key, Group: group, Label: m.Label, Description: m.Description, Control: m.Control})
		knownMetadata[d.Key] = true
		for _, property := range []string{"label", "description", "control", "min", "max", "step"} {
			knownMetadata[d.Key+"."+property] = true
		}
	}
	if len(c.Settings) == 0 {
		return c, fmt.Errorf("no settings defined")
	}
	for _, path := range metadata.Keys() {
		if !knownMetadata[strings.Join(path, ".")] {
			return c, fmt.Errorf("unknown UI metadata: %s", path)
		}
	}
	return c, nil
}

func finite(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) }

func metadataAt(md *toml.MetaData, tree map[string]toml.Primitive, path []string) (toml.Primitive, error) {
	primitive, ok := tree[path[0]]
	if !ok {
		return primitive, fmt.Errorf("missing UI metadata: %s", strings.Join(path, "."))
	}
	if len(path) == 1 {
		return primitive, nil
	}
	var children map[string]toml.Primitive
	if err := md.PrimitiveDecode(primitive, &children); err != nil {
		return primitive, err
	}
	return metadataAt(md, children, path[1:])
}
