package app

import (
	"slices"
	"strconv"
	"strings"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/settings"
)

func panelFocusCommand(number int) command {
	index := number - 1
	c := command{id: "panel.focus_" + strconv.Itoa(number), group: "Panel", name: "Focus " + strconv.Itoa(number),
		available: func(v *view) bool { return index < len(v.panels) },
		execute:   func(v *view) { v.switchPanelTo(index) }}
	if number <= 9 {
		c.bindings[0].Default = shortcut{Name: key.Name(strconv.Itoa(number)), Modifiers: key.ModAlt}
	}
	return c
}

func panelFocusNumber(id string) (int, bool) {
	number, err := strconv.Atoi(strings.TrimPrefix(id, "panel.focus_"))
	return number, err == nil && number > 0 && id == "panel.focus_"+strconv.Itoa(number)
}

// Runtime dispatch also includes settings widgets, while the palette keeps
// its ordinary and @ catalogues separate.
func (v *view) shortcutCommands() []command {
	return append(v.commandCatalogue(), v.settingsCatalogue()...)
}

// Conflict checks include absent numbered panels: their saved bindings retain
// ownership until cleared. Only live panels appear in the palette and dispatch.
func bindingCommands(values settings.Values) []command {
	catalogue := slices.Clone(commands)
	for _, c := range (&view{}).settingsCatalogue() {
		if c.id != "" {
			catalogue = append(catalogue, c)
		}
	}
	numbers := []int{1, 2, 3, 4, 5, 6, 7, 8, 9}
	for name := range values.Item("keybindings.panel") {
		if number, ok := panelFocusNumber("panel." + name); ok && number > 9 {
			numbers = append(numbers, number)
		}
	}
	slices.Sort(numbers)
	for _, number := range numbers {
		catalogue = append(catalogue, panelFocusCommand(number))
	}
	return catalogue
}

func dynamicBindingDefinition(field string) (settings.Definition, bool) {
	for slot := range 2 {
		id := strings.TrimPrefix(strings.TrimSuffix(field, ".shortcut_"+strconv.Itoa(slot+1)), "keybindings.")
		if number, ok := panelFocusNumber(id); ok && field == bindingSettingKey(id, slot) {
			return bindingDefinition(panelFocusCommand(number), slot), true
		}
	}
	return settings.Definition{}, false
}
