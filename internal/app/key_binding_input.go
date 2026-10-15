package app

import (
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
)

func (v *view) browsingKeyFilters() []event.Filter {
	filters := []event.Filter{
		key.FocusFilter{Target: &v.focus},
		// Retain Tab in the ordered event stream and leave picker confirmation
		// independent of configurable browsing commands.
		key.Filter{Name: key.NameTab, Optional: key.ModShift},
		key.Filter{Focus: &v.focus, Name: key.NameReturn, Optional: key.ModCtrl},
		key.Filter{Focus: &v.focus, Name: key.NameEnter, Optional: key.ModCtrl},
		key.Filter{Focus: &v.focus, Name: key.NameUpArrow, Optional: key.ModShift},
		key.Filter{Focus: &v.focus, Name: key.NameDownArrow, Optional: key.ModShift},
	}
	for _, c := range v.shortcutCommands() {
		for slot := range c.bindings {
			s := c.effectiveShortcut(slot)
			if s.Name == "" || s.Modifiers == 0 && (s.Name == "/" || s.Name == "@") {
				continue // These existing triggers enter through text events.
			}
			filters = append(filters, key.Filter{Focus: &v.focus, Name: s.Name, Required: s.Modifiers})
			if s.Name == key.NameReturn {
				filters = append(filters, key.Filter{Focus: &v.focus, Name: key.NameEnter, Required: s.Modifiers})
			}
		}
	}
	return filters
}

func (v *view) dispatchTextShortcut(gtx layout.Context, text string) bool {
	for _, c := range v.shortcutCommands() {
		for slot := range c.bindings {
			trigger := c.effectiveShortcut(slot).textTrigger()
			if trigger == "" || text != trigger && (c.paletteQuery == "" || !strings.HasPrefix(text, trigger)) {
				continue
			}
			if c.paletteQuery != "" && !v.interaction.idle() {
				continue
			}
			// A palette query command may receive its prefix and initial search
			// together. The prefix remains @ even when its invocation key changes.
			v.executeCommand(gtx, c)
			if c.paletteQuery != "" && v.paletteInteraction() != nil {
				v.openPaletteQuery(gtx, c.paletteQuery+strings.TrimPrefix(text, trigger))
			}
			return true
		}
	}
	return false
}

func (v *view) updateNativeBindings() {
	if v.nativeKeys == nil {
		return
	}
	var names []rune
	for _, c := range bindingCommands(v.settingsValues) {
		c = v.resolveCommandBindings(c)
		for slot := range c.bindings {
			s := c.effectiveShortcut(slot)
			if name := []rune(s.Name); s.Modifiers&key.ModAlt != 0 && len(name) == 1 {
				names = append(names, name[0])
			}
		}
	}
	v.nativeKeys.setAltText(names)
}
