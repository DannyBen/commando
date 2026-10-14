package settings

import (
	"reflect"
	"strings"
	"testing"
)

func TestCatalogueDerivesGroupsFromParents(t *testing.T) {
	c, err := ParseCatalogue(`
audio.volume = 0.5
audio.notifications.enabled = true
`, `
[audio]
label = "Sound"
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
	if !reflect.DeepEqual(c.UI.Groups, want) || c.UI.Fields[0].Group != "audio" || c.UI.Fields[1].Group != "audio.notifications" {
		t.Fatalf("incorrect hierarchy: %#v", c.UI)
	}
}

func TestCatalogueControlsIndependentOfValues(t *testing.T) {
	for _, control := range []string{"input", "stepper", "stepper-progress"} {
		c, err := ParseCatalogue(defaultTOML, strings.Replace(uiTOML, `control = "stepper-progress"`, `control = "`+control+`"`, 1))
		if err != nil {
			t.Fatal(err)
		}
		d, _ := c.Definition("appearance.filenames.lightness")
		if d.Kind != Number || d.Default.Number != 0.36 || c.UI.Fields[0].Control != control {
			t.Fatal("control choice changed value type or default")
		}
	}
	// Omitted controls infer a toggle for a boolean and an input for a number.
	c, err := ParseCatalogue(defaultTOML, strings.ReplaceAll(strings.ReplaceAll(uiTOML, "control = \"toggle\"\n", ""), "control = \"stepper-progress\"\n", ""))
	if err != nil {
		t.Fatal(err)
	}
	if c.UI.Fields[0].Control != "input" || c.UI.Fields[3].Control != "toggle" {
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
		{"missing group label", defaultTOML, strings.Replace(uiTOML, "label = \"Colors\"\n", "", 1)},
		{"explicit parent", defaultTOML, strings.Replace(uiTOML, "label = \"Appearance\"", "label = \"Appearance\"\nparent = \"appearance.filenames\"", 1)},
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
	for key, number := range map[string]float64{"appearance.filenames.lightness": 0.5, "appearance.filenames.chroma": 0.2} {
		if err := values.Set(CatalogueDefault, key, Value{Number: number}); err != nil {
			t.Fatal(err)
		}
	}
	before := values.Clone()
	values.Reset("appearance.filenames.lightness")
	if err := values.Set(CatalogueDefault, "appearance.filenames.chroma", Value{Number: 0.3}); err != nil {
		t.Fatal(err)
	}
	if before.Get(CatalogueDefault, "appearance.filenames.lightness").Number != 0.5 || before.Get(CatalogueDefault, "appearance.filenames.chroma").Number != 0.2 {
		t.Fatal("editing nested overrides changed the snapshot")
	}
	values.Reset("appearance.filenames.chroma")
	if len(values.overrides) != 0 {
		t.Fatal("reset left empty tables")
	}
}
