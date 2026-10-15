package app

import (
	"testing"

	"gioui.org/io/key"
)

func TestCommandCatalogueIdentitiesAndDefaultAliases(t *testing.T) {
	v := newPreviewView()
	ids := make(map[string]bool)
	bindings := make(map[shortcut]string)
	for _, c := range append(v.commandCatalogue(), v.settingsCatalogue()...) {
		if c.id == "" || ids[c.id] {
			t.Fatalf("missing or duplicate command ID: %q (%s)", c.id, c.label())
		}
		ids[c.id] = true
		for _, binding := range c.bindings {
			if binding.Default.Name == "" {
				continue
			}
			if previous := bindings[binding.Default]; previous != "" {
				t.Fatalf("default shortcut %s shared by %s and %s", binding.Default, previous, c.id)
			}
			bindings[binding.Default] = c.id
		}
	}
	for _, tc := range []struct {
		id      string
		display string
		keys    [2]key.Event
	}{
		{"entry.paste", "Ctrl+V / Shift+Insert", [2]key.Event{modified("V", key.ModCtrl), modified(insertKey, key.ModShift)}},
		{"entry.context_menu", "Alt+/ / Menu", [2]key.Event{modified("/", key.ModAlt), pressed(contextMenuKey)}},
		{"application.command_palette", "` / Ctrl+Shift+P", [2]key.Event{pressed("`"), modified("P", key.ModCtrl|key.ModShift)}},
	} {
		t.Run(tc.id, func(t *testing.T) {
			for _, c := range v.commandCatalogue() {
				if c.id == tc.id {
					if c.shortcut() != tc.display || !c.matches(tc.keys[0]) || !c.matches(tc.keys[1]) {
						t.Fatal("display and matching lost a default alias", c.shortcut())
					}
					return
				}
			}
			t.Fatal("command is missing")
		})
	}
}

func TestPaletteExecutesActionWithoutUsingItsBindings(t *testing.T) {
	v := newPreviewView()
	send, _ := panelInputFrames(v)
	send(pressed("`"))
	send(key.EditEvent{Text: "Navigation Next panel"})
	s := v.paletteInteraction()
	if len(s.results) == 0 || s.results[0].command.id != "navigation.next_panel" {
		t.Fatal("Next panel was not found")
	}
	// The action remains executable even if its shortcuts are removed or its
	// display label is changed. No synthetic Tab may be needed for dispatch.
	s.results[0].command.bindings = [2]commandBinding{}
	s.results[0].command.name = "Different label"
	send(pressed(key.NameReturn))
	if v.active != 1 || !v.interaction.idle() {
		t.Fatal("palette action depended on its binding or display label")
	}
}

func TestSpaceOnParentDoesNotBecomeFilenameInput(t *testing.T) {
	v := selectionView()
	send, _ := panelInputFrames(v)
	send(pressed(key.NameHome))
	send(pressed(key.NameSpace), key.EditEvent{Text: " "})
	p := &v.panels[0]
	if p.selected != 0 || p.prefix != "" || len(p.marks) != 0 {
		t.Fatal("Space on a parent row changed selection, marks or filename input")
	}
}
