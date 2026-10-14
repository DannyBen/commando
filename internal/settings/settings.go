// Package settings defines application values independently of their presentation.
package settings

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

var (
	ErrSyntax     = errors.New("invalid TOML syntax")
	ErrUnknownKey = errors.New("unknown setting")
	ErrType       = errors.New("invalid setting type")
	ErrRange      = errors.New("value outside range")
)

type Kind uint8

const (
	Boolean Kind = iota
	Number
	String
)

type Value struct {
	Bool   bool
	Number float64
	Text   string
}

type Definition struct {
	Key     string
	Kind    Kind
	Default Value
	Minimum float64
	Maximum *float64 // nil means unbounded.
	Step    float64
}

type Group struct{ Key, Parent, Label string }
type Widget struct {
	Key, Parent, Label, Title string
	Visible                   bool
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
	Settings []Definition
	UI       Presentation
}

func (c Catalogue) Definition(key string) (Definition, bool) {
	for _, d := range c.Settings {
		if d.Key == key {
			return d, true
		}
	}
	return Definition{}, false
}

// Values contains only explicit overrides, including values equal to a default.
// Persistence is deliberately separate from the catalogue and effective values.
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
	if d.Kind == Number && (math.IsNaN(value.Number) || math.IsInf(value.Number, 0) || value.Number < d.Minimum || d.Maximum != nil && value.Number > *d.Maximum) {
		return fmt.Errorf("%w: %s", ErrRange, key)
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
	case string:
		return Value{Text: value}
	}
	return Value{}
}

func (f Field) IsPath() bool { return f.Control == "folder" || f.Control == "file" }
