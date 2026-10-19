package app

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/dannyben/commando/internal/settings"
)

// Add an area here when its catalogue is complete. All future additions in that
// area then need their own help to pass ordinary tests and CI. Until then, use
// op help-audit [AREA] as the red/green writing queue; fallbacks never count.
var completedHelpAreas = []string{"foundation", "settings", "palette.settings", "palette.commands"}

type helpRequirement struct{ area, id string }

func helpRequirements(c settings.Catalogue, paletteCommands, paletteSettings []command) []helpRequirement {
	byID := map[string]helpRequirement{}
	add := func(area, id string) { byID[id] = helpRequirement{area, id} }
	for _, id := range []string{"help", "browsing.introduction", "palette", "palette.settings", "settings"} {
		add("foundation", id)
	}
	for _, command := range paletteCommands {
		add("palette.commands", paletteHelpID(command.id))
	}
	for _, command := range paletteSettings {
		add("palette.settings", paletteHelpID(command.id))
	}
	// Existing-rule editing is supported even when no editor rules are saved.
	add("palette.settings", "palette.settings.programs.editors.rules")
	widgets := map[string]bool{}
	for _, widget := range c.UI.Widgets {
		if widget.Visible {
			widgets[widget.Key] = true
			add("settings."+widget.Key, "settings."+widget.Key)
		}
	}
	for _, field := range c.UI.Fields {
		if field.Visible && widgets[field.Widget] {
			add("settings."+field.Widget, "settings."+field.Key)
		}
	}
	// These controls are supplied by the app rather than settings-ui.toml.
	add("settings."+settingThemeSelect, "settings."+settingThemeSelect)
	add("settings."+settingThemeCustomize, "settings."+settingThemeElement)
	for _, id := range []string{
		"settings.actions.save", "settings.actions.cancel", "settings.actions.reset", "settings.actions.delete",
		"settings.saving", "settings.selecting",
		"settings.shortcuts", "settings.shortcuts.capture", "settings.shortcuts.shortcut_1", "settings.shortcuts.shortcut_2",
		"editing.location", "editing.name", "editing.filter", "actions", "confirmation", "native",
	} {
		add("interactions", id)
	}
	var requirements []helpRequirement
	for _, requirement := range byID {
		requirements = append(requirements, requirement)
	}
	slices.SortFunc(requirements, func(a, b helpRequirement) int {
		if order := strings.Compare(a.area, b.area); order != 0 {
			return order
		}
		return strings.Compare(a.id, b.id)
	})
	return requirements
}

func helpAreaMatches(area, scope string) bool {
	return scope == "" || area == scope || strings.HasPrefix(area, scope+".")
}

func missingHelp(requirements []helpRequirement, catalogue helpCatalogue, scope string) (missing []helpRequirement, total int) {
	for _, requirement := range requirements {
		if !helpAreaMatches(requirement.area, scope) {
			continue
		}
		total++
		if _, ok := catalogue[requirement.id]; !ok {
			missing = append(missing, requirement)
		}
	}
	return missing, total
}

func TestHelpCoverage(t *testing.T) {
	v := newPreviewView()
	requirements := helpRequirements(settings.CatalogueDefault, v.commandCatalogue(), v.settingsCatalogue())
	scopes := completedHelpAreas
	if os.Getenv("COMMANDO_HELP_AUDIT") == "1" {
		scopes = []string{os.Getenv("COMMANDO_HELP_AREA")}
	}
	for _, scope := range scopes {
		missing, total := missingHelp(requirements, defaultHelp, scope)
		if total == 0 {
			t.Errorf("unknown or empty help area: %q", scope)
			continue
		}
		for _, requirement := range missing {
			t.Errorf("missing help: %s (area: %s)", requirement.id, requirement.area)
		}
		t.Logf("area %q: %d/%d entries present; %d missing", scope, total-len(missing), total, len(missing))
	}
}

func TestHelpCoverageDetectsCatalogueAdditionsWithoutFallbacks(t *testing.T) {
	c := settings.CatalogueDefault
	c.UI.Fields = slices.Clone(c.UI.Fields)
	c.UI.Fields = append(c.UI.Fields, settings.Field{
		Key: settingThemeGenerator + ".new_control", Widget: settingThemeGenerator, Visible: true,
	})
	commands := []command{{id: "new.command"}, panelFocusCommand(1), panelFocusCommand(20)}
	requirements := helpRequirements(c, commands, nil)
	area := "settings." + settingThemeGenerator
	complete := helpCatalogue{}
	for _, requirement := range helpRequirements(settings.CatalogueDefault, nil, nil) {
		if helpAreaMatches(requirement.area, area) {
			complete[requirement.id] = helpTip{}
		}
	}
	missing, total := missingHelp(requirements, complete, area)
	newID := "settings." + settingThemeGenerator + ".new_control"
	if total != len(complete)+1 || len(missing) != 1 || missing[0].id != newID {
		t.Fatalf("new control did not turn a complete area red: %v (%d requirements)", missing, total)
	}
	complete[newID] = helpTip{}
	if missing, _ := missingHelp(requirements, complete, area); len(missing) != 0 {
		t.Fatal("adding the missing entry did not turn coverage green", missing)
	}
	missing, total = missingHelp(requirements, defaultHelp, "palette.commands")
	if total != 2 || len(missing) != 1 || missing[0].id != "palette.new.command" {
		t.Fatal("new command was missed or numbered panels required duplicate help", missing, total)
	}
	for _, area := range []string{"found", "settings.appearance.theme.generato"} {
		if _, total := missingHelp(requirements, defaultHelp, area); total != 0 {
			t.Fatal("partial area name matched an unrelated area", area)
		}
	}
	if _, total := missingHelp(requirements, defaultHelp, "settings"); total == 0 {
		t.Fatal("parent area did not include child areas")
	}
}

func TestPaletteHelpFamiliesMatchRuntimeRouting(t *testing.T) {
	for _, id := range []string{panelFocusCommand(20).id, editorRuleCommandPrefix + "example"} {
		v := newPreviewView()
		v.interaction.current = &paletteSession{results: []commandMatch{{command: command{id: id}}}}
		family := paletteHelpID(id)
		if got, _ := (helpCatalogue{family: helpTip{}}).resolve(v.helpContextIDs()...); got != family {
			t.Fatal("audit required an unreachable help family", family, v.helpContextIDs())
		}
	}
	for slot := range 2 {
		v := newPreviewView()
		v.interaction.current = &settingsSession{bindingCommand: &command{id: "entry.rename"}, selected: slot,
			fields: []settings.Field{{Key: "keybindings.entry.rename.shortcut_1"}, {Key: "keybindings.entry.rename.shortcut_2"}}}
		ids := v.helpContextIDs()
		want := []string{"settings.shortcuts.shortcut_1", "settings.shortcuts.shortcut_2"}[slot]
		if !slices.Contains(ids, want) || !slices.Contains(ids, "settings.shortcuts") {
			t.Fatal("audit required unreachable shortcut help", ids)
		}
	}
}
