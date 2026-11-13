package app

import (
	"slices"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/settings"
)

func TestPaletteShortcutSearchNormalizesCombinationsAndDispatches(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	s := v.paletteInteraction()
	for _, query := range []string{"Ctrl+M", "ctrl m", "  CTRL + m  "} {
		send(replacePalette(v, query))
		if len(s.results) == 0 || s.results[0].command.id != "view.next_metadata_column" || !s.results[0].shortcutMatch || !slices.Equal(s.results[0].match.Indices, []int{0, 1, 2, 3, 4, 5}) {
			t.Fatalf("shortcut search %q failed or lost its binding highlights: %+v", query, s.results)
		}
	}
	before := v.panels[v.active].metadata
	send(pressed(key.NameReturn))
	if !v.interaction.idle() || v.panels[v.active].metadata != before.cycle(1) {
		t.Fatal("shortcut search did not use normal command dispatch")
	}
}

func TestPaletteShortcutSearchUsesCurrentBindingsAndScopes(t *testing.T) {
	v := newPreviewView()
	for slot, value := range []string{"none", "Ctrl+Y"} {
		if err := v.settingsValues.Set(bindingSettingsCatalogue(), bindingSettingKey("view.next_metadata_column", slot), settings.Value{Text: value}); err != nil {
			t.Fatal(err)
		}
	}
	v.applySettings()
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	s := v.paletteInteraction()
	for _, query := range []string{"ctrl m", "ctrl y", ";f5", "@f5"} {
		send(replacePalette(v, query))
		found := false
		for _, result := range s.results {
			found = found || result.command.id == "view.next_metadata_column"
			if query == ";f5" && (result.command.group != "Actions" || !result.shortcutMatch) {
				t.Fatal("shortcut search escaped the Actions scope")
			}
		}
		if found != (query == "ctrl y") {
			t.Fatalf("search %q used a stale default or missed an alternative", query)
		}
		if query == ";f5" && len(s.results) == 0 || query == "@f5" && len(s.results) != 0 {
			t.Fatal("shortcut search lost palette scope rules")
		}
	}
	send(replacePalette(v, "ctrl y"))
	if len(s.results) != 1 || s.results[0].command.shortcut() != "Ctrl+Y" || !slices.Equal(s.results[0].match.Indices, []int{0, 1, 2, 3, 4, 5}) {
		t.Fatal("alternative binding offset includes an unassigned primary slot")
	}
}

func TestPaletteShortcutSearchMatchesWholeKeysAndPreservesNameRanking(t *testing.T) {
	c := cmd("test", "Test", "Example", "F5", 0, nil)
	c.bindings[1].Default = shortcut{Name: "F5", Modifiers: key.ModShift}
	for _, tc := range []struct {
		query string
		want  []int
	}{
		{"f5", []int{0, 1, 11, 12}},
		{"shift f5", []int{5, 6, 7, 8, 9, 10, 11, 12}},
		{"shift+f5", []int{5, 6, 7, 8, 9, 10, 11, 12}},
		{"shf5", nil},
		{"f1", nil},
		{"ctrl f5", nil},
	} {
		binding, valid := paletteShortcutQuery(tc.query)
		var indices []int
		if valid {
			match, _ := c.matchShortcut(binding)
			indices = match.Indices
		}
		if !slices.Equal(indices, tc.want) {
			t.Fatalf("search %q highlights %v; want %v", tc.query, indices, tc.want)
		}
	}
	s := &paletteSession{commands: []command{c, cmd("f11", "Test", "Other", "F11", 0, nil), cmd("named", "Test", "F5 notes", "", 0, nil)}}
	s.filter("f5")
	if len(s.results) != 2 || s.results[0].command.id != "named" || s.results[0].shortcutMatch || s.results[1].command.id != "test" {
		t.Fatal("shortcut search changed name ranking or matched a different function key")
	}
	s.filter("f1")
	if len(s.results) != 0 {
		t.Fatal("F1 matched the F11 binding by prefix")
	}
	plus := cmd("plus", "Test", "Increase", "+", 0, nil)
	binding, valid := paletteShortcutQuery("+")
	match, found := plus.matchShortcut(binding)
	if !valid || !found || !slices.Equal(match.Indices, []int{0}) {
		t.Fatal("literal plus key was consumed as a separator")
	}
}
