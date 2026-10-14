package settings

import (
	"math"
	"testing"
)

func TestOverridesAndValidation(t *testing.T) {
	c := CatalogueDefault
	var v Values
	if got := v.Get(c, "appearance.filenames.lightness").Number; got != 0.36 {
		t.Fatalf("default = %v", got)
	}
	if err := v.Set(c, "appearance.filenames.lightness", Value{Number: 0.36}); err != nil {
		t.Fatal(err)
	}
	if !v.Overridden("appearance.filenames.lightness") {
		t.Fatal("explicit default was not retained")
	}
	for _, n := range []float64{-0.01, 1.01, math.NaN(), math.Inf(1)} {
		if err := v.Set(c, "appearance.filenames.lightness", Value{Number: n}); err == nil {
			t.Fatalf("accepted invalid value %v", n)
		}
	}
	if got := v.Get(c, "appearance.filenames.lightness").Number; got != 0.36 {
		t.Fatal("invalid edit changed value")
	}
	if err := v.Set(c, "missing", Value{}); err == nil {
		t.Fatal("accepted unknown key")
	}
	v.Reset("appearance.filenames.lightness")
	if v.Overridden("appearance.filenames.lightness") || v.Get(c, "appearance.filenames.lightness").Number != 0.36 {
		t.Fatal("reset did not restore inheritance")
	}
	if err := v.Set(c, "appearance.filenames.chroma", Value{Number: 0.5}); err != nil {
		t.Fatal("chroma upper boundary rejected", err)
	}
	if err := v.Set(c, "appearance.filenames.chroma", Value{Number: 0.51}); err == nil {
		t.Fatal("chroma above upper boundary accepted")
	}
}

func TestPresentationReferences(t *testing.T) {
	c := CatalogueDefault
	groups := map[string]bool{"": true}
	for _, g := range c.UI.Groups {
		if groups[g.Key] || g.Label == "" {
			t.Fatalf("invalid group %q", g.Key)
		}
		groups[g.Key] = true
	}
	for _, g := range c.UI.Groups {
		if !groups[g.Parent] {
			t.Fatalf("missing parent for %s", g.Key)
		}
	}
	fields := map[string]bool{}
	for _, f := range c.UI.Fields {
		if _, ok := c.Definition(f.Key); !ok || !groups[f.Group] || f.Label == "" || fields[f.Key] {
			t.Fatalf("invalid field %s", f.Key)
		}
		fields[f.Key] = true
	}
	for _, d := range c.Settings {
		if !fields[d.Key] {
			t.Fatalf("missing UI for %s", d.Key)
		}
	}
}
