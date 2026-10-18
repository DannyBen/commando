// Package settings defines application values independently of their presentation.
package settings

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
)

var (
	ErrSyntax     = errors.New("invalid TOML syntax")
	ErrUnknownKey = errors.New("unknown setting")
	ErrType       = errors.New("invalid setting type")
	ErrValue      = errors.New("invalid setting value")
	ErrRange      = errors.New("value outside range")
)

type Kind uint8

const (
	Boolean Kind = iota
	Number
	String
	StringList
)

type Value struct {
	Bool   bool
	Number float64
	Text   string
}

type Definition struct {
	DecimalPlaces int // Positive values limit committed numeric precision; zero retains full precision.
	Key           string
	Kind          Kind
	Default       Value
	Minimum       float64
	Maximum       *float64 // nil means unbounded.
	Step          float64
	Cycle         float64
	Integer       bool
	Choices       []string
	ChoiceValues  []float64 // Optional numeric values paired with display labels.
	Validate      func(Value) error
}

type Group struct{ Key, Parent, Label string }
type Widget struct {
	Key, Parent, Label, Title string
	Preview                   string
	Visible                   bool
	Deferred                  bool // Edits apply only after a successful Save.
}
type Field struct {
	Key, Widget, Label, Description, Control string
	Visible                                  bool
}
type Presentation struct {
	Groups  []Group
	Widgets []Widget
	Fields  []Field
}

func (c Catalogue) Widget(key string) (Widget, bool) {
	for _, w := range c.UI.Widgets {
		if w.Key == key {
			return w, true
		}
	}
	return Widget{}, false
}

type Catalogue struct {
	// CollectionIdentities declares each collection's hidden stable identity field.
	CollectionIdentities map[string]string
	Collections          []string
	Settings             []Definition
	UI                   Presentation
	// Dynamic definitions support indexed settings without bounding their count.
	DynamicDefinition func(string) (Definition, bool)
	DynamicGroup      func(string) bool
	// ValidateOverrides checks related values after loading all fields. It may
	// remove invalid overrides so recovery can preserve unrelated settings.
	ValidateOverrides func(*Values) error
}

func (c Catalogue) Definition(key string) (Definition, bool) {
	for _, d := range c.Settings {
		if d.Key == key {
			return d, true
		}
	}
	if c.DynamicDefinition != nil {
		return c.DynamicDefinition(key)
	}
	return Definition{}, false
}

// Values contains only explicit overrides, including values equal to a default.
// Persistence filters defaults from a copy, independently of transaction state.
type Values struct{ overrides map[string]any }

func (v *Values) Get(c Catalogue, key string) Value {
	if value, ok := lookup(v.overrides, strings.Split(key, ".")); ok {
		return valueFromScalar(value)
	}
	d, _ := c.Definition(key)
	return d.Default
}
func (v *Values) Overridden(key string) bool {
	_, ok := lookup(v.overrides, strings.Split(key, "."))
	return ok
}
func (v *Values) Reset(key string) { remove(v.overrides, strings.Split(key, ".")) }
func (v *Values) Set(c Catalogue, key string, value Value) error {
	d, ok := c.Definition(key)
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	if d.Kind == Number && d.Cycle > 0 && !math.IsNaN(value.Number) && !math.IsInf(value.Number, 0) {
		value.Number = math.Mod(value.Number, d.Cycle)
		if value.Number < 0 {
			value.Number += d.Cycle
		}
	}
	if d.Kind == Number && (math.IsNaN(value.Number) || math.IsInf(value.Number, 0) || value.Number < d.Minimum || d.Maximum != nil && value.Number > *d.Maximum) {
		return fmt.Errorf("%w: %s", ErrRange, key)
	}
	if d.Kind == Number && d.Integer {
		if math.Trunc(value.Number) != value.Number {
			return fmt.Errorf("%w: %s needs a whole number", ErrValue, key)
		}
		// Keep integer step increments exact in the numeric value store.
		if math.Abs(value.Number) > 1<<53-1 {
			return fmt.Errorf("%w: %s", ErrRange, key)
		}
	}
	if d.Kind == Number && d.DecimalPlaces > 0 {
		scale := math.Pow10(d.DecimalPlaces)
		value.Number = math.Round(value.Number*scale) / scale
		if d.Cycle > 0 && value.Number >= d.Cycle {
			value.Number = 0
		}
	}
	if d.Validate != nil {
		if err := d.Validate(value); err != nil {
			return fmt.Errorf("%w: %s: %v", ErrValue, key, err)
		}
	}
	if len(d.ChoiceValues) > 0 && !slices.Contains(d.ChoiceValues, value.Number) || len(d.Choices) > 0 && len(d.ChoiceValues) == 0 && !slices.Contains(d.Choices, value.Text) {
		return fmt.Errorf("%w: %s", ErrValue, key)
	}
	if v.overrides == nil {
		v.overrides = make(map[string]any)
	}
	var scalar any
	switch d.Kind {
	case Boolean:
		scalar = value.Bool
	case Number:
		scalar = value.Number
	case String:
		scalar = value.Text
	case StringList:
		scalar = strings.Fields(value.Text)
	default:
		return fmt.Errorf("unsupported setting kind: %s", key)
	}
	insert(v.overrides, strings.Split(key, "."), scalar)
	return nil
}

// Clone retains override presence as well as values, so cancelled previews do
// not pin inherited defaults.
func (v Values) Clone() Values {
	return Values{overrides: clone(v.overrides)}
}

func lookup(tree map[string]any, path []string) (any, bool) {
	for _, part := range path[:len(path)-1] {
		next, ok := tree[part].(map[string]any)
		if !ok {
			return nil, false
		}
		tree = next
	}
	value, ok := tree[path[len(path)-1]]
	return value, ok
}

func insert(tree map[string]any, path []string, value any) {
	for _, part := range path[:len(path)-1] {
		next, ok := tree[part].(map[string]any)
		if !ok {
			next = make(map[string]any)
			tree[part] = next
		}
		tree = next
	}
	tree[path[len(path)-1]] = value
}

func remove(tree map[string]any, path []string) {
	if len(path) == 1 {
		delete(tree, path[0])
		return
	}
	next, ok := tree[path[0]].(map[string]any)
	if !ok {
		return
	}
	remove(next, path[1:])
	if len(next) == 0 {
		delete(tree, path[0])
	}
}

func clone(tree map[string]any) map[string]any {
	result := make(map[string]any, len(tree))
	for key, value := range tree {
		if nested, ok := value.(map[string]any); ok {
			value = clone(nested)
		} else if rows, ok := value.([]map[string]any); ok {
			copyRows := make([]map[string]any, len(rows))
			for i, row := range rows {
				copyRows[i] = clone(row)
			}
			value = copyRows
		} else if list, ok := value.([]string); ok {
			value = append([]string{}, list...)
		}
		result[key] = value
	}
	return result
}

func valueFromScalar(scalar any) Value {
	switch value := scalar.(type) {
	case bool:
		return Value{Bool: value}
	case float64:
		return Value{Number: value}
	case int64:
		return Value{Number: float64(value)}
	case string:
		return Value{Text: value}
	case []string:
		return Value{Text: strings.Join(value, " ")}
	case []any:
		var parts []string
		for _, part := range value {
			if text, ok := part.(string); ok {
				parts = append(parts, text)
			}
		}
		return Value{Text: strings.Join(parts, " ")}
	}
	return Value{}
}

func (f Field) IsPath() bool { return f.Control == "folder" || f.Control == "file" }

// Collection items use the same field definitions as ordinary settings widgets.
func (v Values) Collection(key string) []map[string]any {
	raw, _ := lookup(v.overrides, strings.Split(key, "."))
	rows, _ := raw.([]map[string]any)
	result := make([]map[string]any, len(rows))
	for i, row := range rows {
		result[i] = clone(row)
	}
	return result
}
func (v *Values) SetCollection(key string, rows []map[string]any) {
	if v.overrides == nil {
		v.overrides = make(map[string]any)
	}
	copyRows := make([]map[string]any, len(rows))
	for i, row := range rows {
		copyRows[i] = clone(row)
	}
	insert(v.overrides, strings.Split(key, "."), copyRows)
}
func (v *Values) SetItem(key string, row map[string]any) {
	if v.overrides == nil {
		v.overrides = make(map[string]any)
	}
	insert(v.overrides, strings.Split(key, "."), clone(row))
}
func (v Values) Item(key string) map[string]any {
	row, _ := lookup(v.overrides, strings.Split(key, "."))
	table, _ := row.(map[string]any)
	return clone(table)
}
