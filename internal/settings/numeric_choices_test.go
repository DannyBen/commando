package settings

import (
	"strings"
	"testing"
)

func TestNumericChoicesKeepValuesIndependentOfLabels(t *testing.T) {
	const defaults = "[sample]\nprofile = -1"
	const ui = `[sample]
label = "Sample"
title = "Sample"
[sample.profile]
label = "Profile"
control = "autocomplete-stepper"
integer = true
choices = ["Off", "Quiet", "All"]
choice_values = [0, 1, -1]
`
	c, err := ParseCatalogue(defaults, ui)
	if err != nil {
		t.Fatal(err)
	}
	var values Values
	if err := values.Set(c, "sample.profile", Value{Number: 1}); err != nil {
		t.Fatal(err)
	}
	// Renaming and inserting UI choices does not reinterpret the stored level.
	renamed, err := ParseCatalogue(defaults, strings.Replace(strings.Replace(ui, `"Quiet"`, `"Alerts", "More alerts"`, 1), "[0, 1, -1]", "[0, 1, 2, -1]", 1))
	if err != nil || values.Get(renamed, "sample.profile").Number != 1 {
		t.Fatal("labels changed the saved profile", err)
	}
	if err := values.Set(c, "sample.profile", Value{Number: 2}); err == nil {
		t.Fatal("unlisted profile accepted")
	}
	for _, bad := range []string{
		strings.Replace(ui, "[0, 1, -1]", "[0, -1]", 1),
		strings.Replace(ui, "[0, 1, -1]", "[0, 0, -1]", 1),
		strings.Replace(ui, "[0, 1, -1]", "[0, 1.5, -1]", 1),
		strings.Replace(ui, "[0, 1, -1]", "[0, inf, -1]", 1),
		strings.Replace(ui, "integer = true", "integer = true\nmax = 0", 1),
	} {
		if _, err := ParseCatalogue(defaults, bad); err == nil {
			t.Fatal("invalid numeric choice mapping accepted")
		}
	}
}
