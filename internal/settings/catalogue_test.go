package settings

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWidgetNamesAndOwnershipIndependentOfFieldCount(t *testing.T) {
	ui := `[appearance]
label = "Appearance"
[appearance.autocolor]
label = "Color tools"
palette = "Auto color"
title = "Generated file colors"
[appearance.autocolor.lightness]
label = "Lightness"
`
	for _, defaults := range []string{"appearance.autocolor.lightness = 0.36", "appearance.autocolor.lightness = 0.36\nappearance.autocolor.chroma = 0.35"} {
		metadata := ui
		if strings.Contains(defaults, "chroma") {
			metadata += "\n[appearance.autocolor.chroma]\nlabel = 'Chroma'\n"
		}
		c, err := ParseCatalogue(defaults, metadata)
		if err != nil {
			t.Fatal(err)
		}
		w, ok := c.Widget("appearance.autocolor")
		if !ok || w.Label != "Auto color" || w.Title != "Generated file colors" || w.Parent != "appearance" || !w.Visible {
			t.Fatalf("widget names changed with field count: %#v", w)
		}
		for _, field := range c.UI.Fields {
			if field.Widget != w.Key || !field.Visible {
				t.Fatalf("field lost its widget: %#v", field)
			}
		}
	}
}

func TestHiddenBranchRetainsValuesAndValidation(t *testing.T) {
	c, err := ParseCatalogue(`
appearance.hue = 20.0
keymap.entry.rename = "F2"
`, `
[appearance]
label = "Appearance"
[appearance.hue]
label = "Initial hue"
palette = "Hue"
title = "Color rotation"
min = 0.0
max = 360.0
visible = false
[keymap]
visible = false
[keymap.entry]
[keymap.entry.rename]
`)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range c.UI.Widgets {
		if w.Visible {
			t.Fatal("hidden widget remained visible")
		}
	}
	for _, f := range c.UI.Fields {
		if f.Visible {
			t.Fatal("hidden field remained visible")
		}
	}
	var values Values
	if err := values.Set(c, "appearance.hue", Value{Number: 361}); !errors.Is(err, ErrRange) {
		t.Fatal("hidden value bypassed validation", err)
	}
	if err := values.Set(c, "keymap.entry.rename", Value{Text: "F3"}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := Save(dir, values); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(dir, c)
	if err != nil || loaded.Get(c, "keymap.entry.rename").Text != "F3" {
		t.Fatal("hidden setting did not round-trip", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.toml"), []byte("appearance.hue = 361"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir, c); !errors.Is(err, ErrRange) {
		t.Fatal("hidden loaded setting bypassed validation", err)
	}
}

func TestWidgetAndFieldOrderUsesMetadata(t *testing.T) {
	c, err := ParseCatalogue("first.a = 1.0\nfirst.b = 2.0\nsecond.c = true", `
[second]
label = "Second"
title = "Second widget"
[second.c]
label = "C"
[first]
label = "First"
title = "First widget"
[first.b]
label = "B"
[first.a]
label = "A"
`)
	if err != nil {
		t.Fatal(err)
	}
	if c.UI.Widgets[0].Key != "second" || c.UI.Fields[0].Key != "second.c" || c.UI.Fields[1].Key != "first.b" || c.UI.Fields[2].Key != "first.a" {
		t.Fatal("metadata order was lost")
	}
}

func TestCatalogueDerivesGroupsFromParents(t *testing.T) {
	c, err := ParseCatalogue(`
audio.volume = 0.5
audio.notifications.enabled = true
`, `
[audio]
label = "Sound"
title = "Audio options"
[audio.volume]
label = "Volume"
[audio.notifications]
label = "Notifications"
[audio.notifications.enabled]
label = "Enabled"
`)
	if err != nil {
		t.Fatal(err)
	}
	want := []Group{
		{Key: "audio", Label: "Sound"},
		{Key: "audio.notifications", Parent: "audio", Label: "Notifications"},
	}
	if !reflect.DeepEqual(c.UI.Groups, want) || c.UI.Fields[0].Widget != "audio" || c.UI.Fields[1].Widget != "audio" {
		t.Fatalf("incorrect hierarchy: %#v", c.UI)
	}
}

func TestNestedWidgetsOwnTheirNearestFields(t *testing.T) {
	c, err := ParseCatalogue(`tools.basic = true
tools.detail.value = 0.5
tools.detail.extra.value = 0.2
tools.other = true`, `
[tools]
label = "Tools"
title = "Tools"
[tools.basic]
label = "Basic"
[tools.detail]
label = "Detail"
title = "Detail"
[tools.detail.value]
label = "Value"
[tools.detail.extra]
label = "Extra"
[tools.detail.extra.value]
label = "Extra value"
[tools.other]
label = "Other"
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.UI.Widgets) != 2 || c.UI.Widgets[1].Parent != "tools" {
		t.Fatalf("child widget lost its parent: %#v", c.UI.Widgets)
	}
	for i, want := range []string{"tools", "tools.detail", "tools.detail", "tools"} {
		if c.UI.Fields[i].Widget != want {
			t.Fatalf("%s belongs to %s, want %s", c.UI.Fields[i].Key, c.UI.Fields[i].Widget, want)
		}
	}
}

func TestCatalogueControlsIndependentOfValues(t *testing.T) {
	for _, control := range []string{"input", "stepper", "stepper-progress"} {
		c, err := ParseCatalogue(defaultTOML, strings.Replace(uiTOML, `control = "stepper-progress"`, `control = "`+control+`"`, 1))
		if err != nil {
			t.Fatal(err)
		}
		d, _ := c.Definition("appearance.autocolor.lightness")
		if d.Kind != Number || d.Default.Number != 0.36 || c.UI.Fields[1].Control != control {
			t.Fatal("control choice changed value type or default")
		}
	}
	// Omitted controls infer a toggle for a boolean and an input for a number.
	c, err := ParseCatalogue(defaultTOML, strings.ReplaceAll(strings.ReplaceAll(uiTOML, "control = \"toggle\"\n", ""), "control = \"stepper-progress\"\n", ""))
	if err != nil {
		t.Fatal(err)
	}
	if c.UI.Fields[1].Control != "input" || c.UI.Fields[0].Control != "toggle" {
		t.Fatal("incorrect inferred controls")
	}
}

func TestCatalogueRejectsInvalidDefinitions(t *testing.T) {
	for _, tc := range []struct{ name, defaults, ui string }{
		{"syntax", "bad = [", uiTOML},
		{"missing metadata", defaultTOML + "\nfiles.extra = true\n", uiTOML},
		{"unknown metadata", defaultTOML, uiTOML + "\nmisspelled = true\n"},
		{"unknown leaf", defaultTOML, uiTOML + "\n[files.extra]\nlabel = 'Extra'\n"},
		{"wrong control type", defaultTOML, strings.Replace(uiTOML, `control = "toggle"`, `control = "folder"`, 1)},
		{"unknown control", defaultTOML, strings.Replace(uiTOML, `control = "toggle"`, `control = "toggel"`, 1)},
		{"invalid default", strings.Replace(defaultTOML, "lightness = 0.36", "lightness = 2.0", 1), uiTOML},
		{"reversed range", defaultTOML, strings.Replace(uiTOML, "min = 0.0", "min = 2.0", 1)},
		{"zero step", defaultTOML, strings.Replace(uiTOML, "step = 0.01", "step = 0.0", 1)},
		{"infinite range", defaultTOML, strings.Replace(uiTOML, "max = 1.0", "max = inf", 1)},
		{"progress without range", defaultTOML, strings.Replace(uiTOML, "max = 1.0\n", "", 1)},
		{"missing group label", defaultTOML, strings.Replace(uiTOML, "label = \"Appearance\"\n", "", 1)},
		{"explicit parent", defaultTOML, strings.Replace(uiTOML, "label = \"Appearance\"", "label = \"Appearance\"\nparent = \"appearance.autocolor\"", 1)},
		{"missing widget", defaultTOML, strings.Replace(uiTOML, "title = \"Auto color\"\n", "", 1)},
		{"empty title", defaultTOML, strings.Replace(uiTOML, "title = \"Auto color\"", "title = \"\"", 1)},
		{"control on category", defaultTOML, strings.Replace(uiTOML, "label = \"Appearance\"", "label = \"Appearance\"\ncontrol = \"input\"", 1)},
		{"invalid visibility", defaultTOML, strings.Replace(uiTOML, "label = \"Appearance\"", "label = \"Appearance\"\nvisible = \"false\"", 1)},
		{"ambiguous path", "\"files.extra\" = true", uiTOML},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseCatalogue(tc.defaults, tc.ui); err == nil {
				t.Fatal("accepted invalid catalogue")
			}
		})
	}
}

func TestNestedOverridesCloneAndReset(t *testing.T) {
	var values Values
	for key, number := range map[string]float64{"appearance.autocolor.lightness": 0.5, "appearance.autocolor.chroma": 0.2} {
		if err := values.Set(CatalogueDefault, key, Value{Number: number}); err != nil {
			t.Fatal(err)
		}
	}
	before := values.Clone()
	values.Reset("appearance.autocolor.lightness")
	if err := values.Set(CatalogueDefault, "appearance.autocolor.chroma", Value{Number: 0.3}); err != nil {
		t.Fatal(err)
	}
	if before.Get(CatalogueDefault, "appearance.autocolor.lightness").Number != 0.5 || before.Get(CatalogueDefault, "appearance.autocolor.chroma").Number != 0.2 {
		t.Fatal("editing nested overrides changed the snapshot")
	}
	values.Reset("appearance.autocolor.chroma")
	if len(values.overrides) != 0 {
		t.Fatal("reset left empty tables")
	}
}
