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

type nodeMetadata struct {
	Label, Palette, Description, Control string
	Title                                *string
	Visible                              *bool
	Minimum                              *float64 `toml:"min"`
	Maximum                              *float64 `toml:"max"`
	Step                                 float64
	Integer                              bool
}

// ParseCatalogue builds widgets from titled metadata nodes, independently of
// their number of fields. Hidden nodes retain their values and validation.
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
	// A single embedded array-table row is a collection's item template, not
	// an effective rule. User collections start empty.
	for _, path := range values.Keys() {
		raw, _ := lookup(tree, path)
		if rows, ok := raw.([]map[string]any); ok {
			if len(rows) != 1 {
				return c, fmt.Errorf("collection needs one template: %s", path)
			}
			c.Collections = append(c.Collections, strings.Join(path, "."))
			insert(tree, path, rows[0])
		}
	}
	groups := map[string]bool{}
	owners := map[string]string{}
	visible := map[string]bool{"": true}
	knownMetadata := map[string]bool{}
	for _, path := range orderedLeaves(tree, values.Keys(), metadata.Keys()) {
		scalar, _ := lookup(tree, path)
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
		case []any:
			d.Kind = StringList
		default:
			return c, fmt.Errorf("unsupported value type: %s", d.Key)
		}
		for depth := 1; depth < len(path); depth++ {
			parentPath := path[:depth]
			key := strings.Join(parentPath, ".")
			if groups[key] {
				continue
			}
			m, err := readMetadata(&metadata, ui, parentPath)
			if err != nil {
				return c, err
			}
			if m.Control != "" || m.Minimum != nil || m.Maximum != nil || m.Step != 0 {
				return c, fmt.Errorf("control on group: %s", key)
			}
			parent := strings.Join(parentPath[:depth-1], ".")
			visible[key] = visible[parent] && (m.Visible == nil || *m.Visible)
			owner, err := c.addWidget(key, parent, owners[parent], m, visible[key], false)
			if err != nil {
				return c, err
			}
			owners[key] = owner
			label := m.Label
			if label == "" && m.Title != nil {
				w, _ := c.Widget(key)
				label = w.Label
			}
			if label == "" && visible[key] {
				return c, fmt.Errorf("missing group label: %s", key)
			}
			c.UI.Groups = append(c.UI.Groups, Group{Key: key, Parent: parent, Label: label})
			groups[key] = true
			allowMetadata(knownMetadata, key, false)
		}
		m, err := readMetadata(&metadata, ui, path)
		if err != nil {
			return c, err
		}
		parent := strings.Join(path[:len(path)-1], ".")
		fieldVisible := visible[parent] && (m.Visible == nil || *m.Visible)
		owner, err := c.addWidget(d.Key, parent, owners[parent], m, fieldVisible, true)
		if err != nil {
			return c, err
		}
		if fieldVisible && (m.Label == "" || owner == "") {
			return c, fmt.Errorf("missing field label or widget: %s", d.Key)
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
		d.Integer = m.Integer
		compatible := m.Control == "input" ||
			d.Kind == Boolean && m.Control == "toggle" ||
			d.Kind == String && (m.Control == "folder" || m.Control == "file") ||
			d.Kind == Number && (m.Control == "stepper" || m.Control == "stepper-progress")
		if !compatible {
			return c, fmt.Errorf("incompatible control: %s", d.Key)
		}
		if d.Kind != Number && (m.Minimum != nil || m.Maximum != nil || m.Step != 0 || m.Integer) {
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
		c.UI.Fields = append(c.UI.Fields, Field{Key: d.Key, Widget: owner, Label: m.Label, Description: m.Description, Control: m.Control, Visible: fieldVisible})
		allowMetadata(knownMetadata, d.Key, true)
	}
	if len(c.Settings) == 0 {
		return c, fmt.Errorf("no settings defined")
	}
	for _, path := range metadata.Keys() {
		if !knownMetadata[strings.Join(path, ".")] {
			return c, fmt.Errorf("unknown UI metadata: %s", path)
		}
	}
	// A visible widget needs at least one visible field to be offered.
	for i := range c.UI.Widgets {
		w := &c.UI.Widgets[i]
		hasFields := false
		for _, field := range c.UI.Fields {
			if field.Widget == w.Key && field.Visible {
				hasFields = true
				break
			}
		}
		w.Visible = w.Visible && hasFields
	}
	return c, nil
}

// Metadata order determines discovery and control order, including when the
// defaults use a different order or the metadata uses dotted declarations.
func orderedLeaves(tree map[string]any, defaults, ui []toml.Key) []toml.Key {
	var result []toml.Key
	seen := map[string]bool{}
	visit := func(path toml.Key) {
		for depth := 1; depth <= len(path); depth++ {
			prefix := path[:depth]
			value, ok := lookup(tree, prefix)
			if !ok {
				break
			}
			if _, table := value.(map[string]any); table {
				continue
			}
			key := strings.Join(prefix, ".")
			if !seen[key] {
				result = append(result, prefix)
				seen[key] = true
			}
			break
		}
	}
	for _, path := range ui {
		visit(path)
	}
	// Missing UI definitions must still be rejected.
	for _, path := range defaults {
		visit(path)
	}
	return result
}

func readMetadata(md *toml.MetaData, ui map[string]toml.Primitive, path []string) (nodeMetadata, error) {
	var m nodeMetadata
	primitive, err := metadataAt(md, ui, path)
	if err == nil {
		err = md.PrimitiveDecode(primitive, &m)
	}
	return m, err
}

func allowMetadata(known map[string]bool, key string, leaf bool) {
	known[key] = true
	for _, property := range []string{"label", "palette", "title", "visible", "description"} {
		known[key+"."+property] = true
	}
	if leaf {
		for _, property := range []string{"control", "min", "max", "step", "integer"} {
			known[key+"."+property] = true
		}
	}
}

func (c *Catalogue) addWidget(key, parent, owner string, m nodeMetadata, visible, leaf bool) (string, error) {
	if m.Title == nil {
		if m.Palette != "" {
			return "", fmt.Errorf("palette name without widget title: %s", key)
		}
		return owner, nil
	}
	if *m.Title == "" {
		return "", fmt.Errorf("empty widget title: %s", key)
	}
	// A titled child starts its own widget; untitled descendants retain the
	// nearest titled ancestor as their field owner.
	label := m.Palette
	if label == "" && !leaf {
		label = m.Label
	}
	if label == "" {
		label = *m.Title
	}
	c.UI.Widgets = append(c.UI.Widgets, Widget{Key: key, Parent: parent, Label: label, Title: *m.Title, Visible: visible})
	return key, nil
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
