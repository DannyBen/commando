package app

import (
	"image"
	"maps"
	"slices"
	"strings"
	"testing"

	"gioui.org/io/key"
	"gioui.org/unit"
	"github.com/dannyben/commando/internal/settings"
)

func TestActionsPaletteSharesCatalogueAndOriginatingPanel(t *testing.T) {
	for _, route := range []string{"shortcut", "prefix", "menu"} {
		t.Run(route, func(t *testing.T) {
			v := newPreviewView()
			v.applySettings() // Match the settings restored when cancelling the shortcut editor.
			v.active = 1
			v.settingsValues.SetCollection(settings.ActionCollection, []map[string]any{
				customActionRow("send", "Send test", settings.ActionFiles, settings.EntriesToken),
			})
			p := &v.panels[1]
			p.toggleMark(1)
			send, router := paletteFrames(v, image.Pt(480, 480), unit.Metric{PxPerDp: 1, PxPerSp: 1})
			marks, selected := maps.Clone(p.marks), p.selected
			if route == "shortcut" {
				send(pressed(";"), key.EditEvent{Text: ";"})
			} else {
				send(pressed("`"))
				query := ";"
				if route == "menu" {
					query = "Menu Actions"
				}
				send(key.EditEvent{Text: query})
				if route == "menu" {
					c := v.paletteInteraction().results[0].command
					if c.id != "application.actions" || c.label() != "Menu › Actions" || c.shortcut() != ";" {
						t.Fatal("Actions entry point is missing or has lost its shortcut", c)
					}
					send(pressed(key.NameReturn))
				}
			}
			s := v.paletteInteraction()
			if s == nil || !s.actionsMode || s.settingsMode || s.query != ";" || s.panel != 1 || !router.Source().Focused(&v.commandSearch) {
				t.Fatal("Actions did not open in its originating panel with a single prefix")
			}
			want := map[string]command{}
			for _, c := range s.commands {
				if c.group == "Actions" && !c.mask.hidden {
					want[c.id] = c
				}
			}
			if len(s.results) != len(want) {
				t.Fatal("Actions scope lost entries or included another group")
			}
			for _, result := range s.results {
				c := result.command
				original, found := want[c.id]
				if !found || c.label() != strings.TrimPrefix(original.label(), "Actions › ") || c.originalLabel() != original.originalLabel() ||
					c.shortcut() != original.shortcut() || c.allowed(v) != original.allowed(v) {
					t.Fatal("scope changed command identity, binding, availability or original label", c)
				}
				delete(want, c.id)
			}
			send(replacePalette(v, ";cpy"))
			if s.results[0].command.id != "entry.copy" || !slices.Equal(s.results[0].match.Indices, []int{0, 2, 3}) {
				t.Fatal("scope did not search and highlight the displayed label")
			}
			send(replacePalette(v, ";no-such-action"), pressed(key.NameReturn))
			if len(s.results) != 0 || v.paletteInteraction() != s || !slices.Contains(v.helpContextIDs(), "palette.actions") {
				t.Fatal("empty Actions search lost its session or scoped help")
			}
			send(replacePalette(v, ""))
			if s.actionsMode || s.settingsMode || len(s.results) != len(v.commandCatalogue()) || s.results[0].command.label() != "Actions › Edit" {
				t.Fatal("clearing ; did not restore the complete catalogue and roots")
			}
			send(replacePalette(v, "@font"))
			if !s.settingsMode || s.actionsMode || s.results[0].command.label() != "Appearance › Font" {
				t.Fatal("switching from Actions to Settings failed")
			}
			send(replacePalette(v, ";Send test"))
			if len(s.results) != 1 || s.results[0].command.id != customActionCommandPrefix+"send" {
				t.Fatal("scope mixed the custom action with its settings entry")
			}
			send(modified("E", key.ModCtrl))
			widget := v.settingsInteraction()
			if widget == nil || widget.palette != s || widget.title != "Actions › Send test › Shortcuts" || widget.bindingCommand.id != customActionCommandPrefix+"send" {
				t.Fatal("scoped shortcut editor lost the global command identity")
			}
			send(pressed(key.NameEscape))
			if v.paletteInteraction() != s || !s.actionsMode || s.query != ";Send test" || s.results[0].command.label() != "Send test" {
				t.Fatal("shortcut cancellation did not restore the Actions view")
			}
			send(pressed(key.NameEscape))
			if !router.Source().Focused(&v.focus) || v.active != 1 || p.selected != selected || !maps.Equal(marks, p.marks) {
				t.Fatalf("Actions changed panel state or lost browsing focus: focused=%v active=%d selected=%d/%d marks=%v/%v", router.Source().Focused(&v.focus), v.active, p.selected, selected, p.marks, marks)
			}
		})
	}
}

func TestActionsPaletteKeepsArchivePresentation(t *testing.T) {
	v, _ := archiveView(t)
	selectName(t, &v.panels[0], "a2.txt")
	send, _ := panelInputFrames(v)
	send(key.EditEvent{Text: ";delete"})
	s := v.paletteInteraction()
	if s == nil || len(s.results) != 1 || s.results[0].command.id != "entry.recycle" || s.results[0].command.label() != "Delete from archive…" {
		t.Fatal("Actions lost the archive mask or exposed permanent deletion")
	}
	send(modified("E", key.ModCtrl))
	if widget := v.settingsInteraction(); widget == nil || widget.title != "Actions › Recycle › Shortcuts" {
		t.Fatal("archive mask replaced the global shortcut identity")
	}
}
