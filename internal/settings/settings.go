// Package settings defines application values independently of their presentation.
package settings

import (
	"fmt"
	"math"
)

type Kind uint8

const (
	Boolean Kind = iota
	Number
	Folder
	File
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
type Field struct{ Key, Group, Label, Description, Control string }
type Presentation struct {
	Groups []Group
	Fields []Field
}
type Catalogue struct {
	Settings []Definition
	UI       Presentation
}

const (
	ProtectedSystem = "files.show_protected_system"
	Lightness       = "appearance.filenames.lightness"
	Chroma          = "appearance.filenames.chroma"
	Hue             = "appearance.filenames.hue"
	DataFolder      = "storage.data_folder"
)

var CatalogueDefault = Catalogue{
	Settings: []Definition{
		{Key: ProtectedSystem, Kind: Boolean, Default: Value{Bool: false}},
		{Key: Lightness, Kind: Number, Default: Value{Number: 0.36}, Minimum: 0, Maximum: maximum(1), Step: 0.01},
		{Key: Chroma, Kind: Number, Default: Value{Number: 0.35}, Minimum: 0, Maximum: maximum(0.5), Step: 0.01},
		{Key: Hue, Kind: Number, Default: Value{Number: 20}, Minimum: 0, Maximum: maximum(360), Step: 1},
		{Key: DataFolder, Kind: Folder, Default: Value{Text: ""}},
	},
	UI: Presentation{
		Groups: []Group{
			{Key: "appearance", Label: "Appearance"},
			{Key: "appearance.colors", Parent: "appearance", Label: "Colors"},

			{Key: "files", Label: "Files"},
			{Key: "files.visibility", Parent: "files", Label: "Visibility"},
			{Key: "storage", Label: "Storage"},
			{Key: "storage.folder", Parent: "storage", Label: "Data folder"},
		},
		Fields: []Field{
			{Key: ProtectedSystem, Group: "files.visibility", Label: "Show protected system entries", Description: "Include entries with both hidden and system attributes.", Control: "toggle"},
			{Key: Lightness, Group: "appearance.colors", Label: "Lightness", Description: "Adjust the lightness of filename colors.", Control: "stepper"},
			{Key: Chroma, Group: "appearance.colors", Label: "Chroma", Description: "Adjust the color intensity of filenames.", Control: "stepper"},
			{Key: Hue, Group: "appearance.colors", Label: "Initial hue", Description: "Rotate the filename color palette.", Control: "stepper"},
			{Key: DataFolder, Group: "storage.folder", Label: "Data folder", Description: "Browse another panel to choose a folder. Preview only; application data stays in its current location.", Control: "path"},
		},
	},
}

func maximum(n float64) *float64 { return &n }
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
type Values struct{ overrides map[string]Value }

func (v *Values) Get(c Catalogue, key string) Value {
	if value, ok := v.overrides[key]; ok {
		return value
	}
	d, _ := c.Definition(key)
	return d.Default
}
func (v *Values) Overridden(key string) bool { _, ok := v.overrides[key]; return ok }
func (v *Values) Reset(key string)           { delete(v.overrides, key) }
func (v *Values) Set(c Catalogue, key string, value Value) error {
	d, ok := c.Definition(key)
	if !ok {
		return fmt.Errorf("unknown setting: %s", key)
	}
	if d.Kind == Number && (math.IsNaN(value.Number) || math.IsInf(value.Number, 0) || value.Number < d.Minimum || d.Maximum != nil && value.Number > *d.Maximum) {
		return fmt.Errorf("value outside range: %s", key)
	}
	if v.overrides == nil {
		v.overrides = make(map[string]Value)
	}
	v.overrides[key] = value
	return nil
}

// Clone retains override presence as well as values, so cancelled previews do
// not pin inherited defaults.
func (v Values) Clone() Values {
	copy := Values{overrides: make(map[string]Value, len(v.overrides))}
	for key, value := range v.overrides {
		copy.overrides[key] = value
	}
	return copy
}
