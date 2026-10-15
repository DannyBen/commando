package settings

import (
	"math"
	"testing"
)

func TestOverridesAndValidation(t *testing.T) {
	c := CatalogueDefault
	var v Values
	if got := v.Get(c, "appearance.autocolor.lsh.lightness").Number; got != 36 {
		t.Fatalf("default = %v", got)
	}
	if err := v.Set(c, "appearance.autocolor.lsh.lightness", Value{Number: 36}); err != nil {
		t.Fatal(err)
	}
	if !v.Overridden("appearance.autocolor.lsh.lightness") {
		t.Fatal("explicit default was not retained")
	}
	for _, n := range []float64{-1, 101, math.NaN(), math.Inf(1)} {
		if err := v.Set(c, "appearance.autocolor.lsh.lightness", Value{Number: n}); err == nil {
			t.Fatalf("accepted invalid value %v", n)
		}
	}
	if got := v.Get(c, "appearance.autocolor.lsh.lightness").Number; got != 36 {
		t.Fatal("invalid edit changed value")
	}
	if err := v.Set(c, "missing", Value{}); err == nil {
		t.Fatal("accepted unknown key")
	}
	v.Reset("appearance.autocolor.lsh.lightness")
	if v.Overridden("appearance.autocolor.lsh.lightness") || v.Get(c, "appearance.autocolor.lsh.lightness").Number != 36 {
		t.Fatal("reset did not restore inheritance")
	}
	if err := v.Set(c, "appearance.autocolor.lsh.saturation", Value{Number: 100}); err != nil {
		t.Fatal("chroma upper boundary rejected", err)
	}
	if err := v.Set(c, "appearance.autocolor.lsh.saturation", Value{Number: 101}); err == nil {
		t.Fatal("chroma above upper boundary accepted")
	}
}

func TestSaltValuesPersistBeyondHueRange(t *testing.T) {
	c := CatalogueDefault
	dir := t.TempDir()
	var values Values
	for _, n := range []float64{0, 1, 361, 1000000} {
		if err := values.Set(c, "appearance.autocolor.salt", Value{Number: n}); err != nil {
			t.Fatal(err)
		}
		if err := Save(dir, values); err != nil {
			t.Fatal(err)
		}
		reloaded, err := Load(dir, c)
		if err != nil || reloaded.Get(c, "appearance.autocolor.salt").Number != n {
			t.Fatalf("salt %v did not survive save/load: %v", n, err)
		}
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
		if _, ok := c.Definition(f.Key); !ok || f.Widget == "" || f.Label == "" || fields[f.Key] {
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
