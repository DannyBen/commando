package app

import (
	"strconv"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

func (v *view) canEditShortcuts(c command) bool {
	if !v.canOfferInteraction(v.active, actionMutation) {
		return false
	}
	for _, builtin := range commands {
		if builtin.id == c.id {
			for slot, binding := range builtin.bindings {
				if bindingConfigurable(builtin, slot) && !binding.Protected {
					return true
				}
			}
		}
	}
	return false
}

func (v *view) startShortcutWidget(gtx layout.Context) {
	p := v.paletteInteraction()
	if p == nil || p.selected < 0 || p.selected >= len(p.results) {
		return
	}
	c := p.results[p.selected].command
	if !v.canEditShortcuts(c) {
		return
	}
	s := &settingsSession{panel: p.panel, title: c.label() + " › Shortcuts", palette: p,
		before: v.settingsValues.Clone(), draft: v.settingsValues.Clone(), bindingCommand: &c}
	for slot := range c.bindings {
		s.fields = append(s.fields, settings.Field{Key: bindingSettingKey(c.id, slot),
			Label: "Shortcut " + strconv.Itoa(slot+1), Control: "shortcut"})
	}
	if !shortcutFieldEditable(s, 0) {
		s.selected = 1
	}
	v.interaction.finish(p)
	v.interaction.begin(s)
	v.playSound(sound.Settings)
	v.focusInteraction(gtx)
}

func shortcutFieldEditable(s *settingsSession, slot int) bool {
	return bindingConfigurable(*s.bindingCommand, slot) && !s.bindingCommand.bindings[slot].Protected
}

func (v *view) shortcutSettingText(s *settingsSession, field string) string {
	draft := view{settingsValues: s.draft}
	c := draft.resolveCommandBindings(*s.bindingCommand)
	for slot, f := range s.fields {
		if f.Key != field {
			continue
		}
		text := c.effectiveShortcut(slot).String()
		if text == "" {
			text = "Unassigned"
		}
		if !shortcutFieldEditable(s, slot) {
			text += " (protected)"
		}
		return text
	}
	return ""
}

func (v *view) setShortcutCapture(s *settingsSession, capturing bool) {
	s.capturing = capturing
	v.nativeKeys.setShortcutCapture(capturing)
}

func (v *view) beginShortcutCapture(gtx layout.Context) {
	s := v.settingsInteraction()
	if !shortcutFieldEditable(s, s.selected) {
		s.errorText = "Shortcut is protected"
		return
	}
	s.errorText = ""
	v.setShortcutCapture(s, true)
	v.playSound(sound.Settings)
	v.focusInteraction(gtx)
}

func (v *view) updateShortcutCapture(gtx layout.Context) {
	for {
		e, ok := gtx.Event(key.FocusFilter{Target: &v.focus}, key.Filter{Focus: &v.focus, Optional: key.ModCtrl | key.ModShift | key.ModAlt | key.ModSuper | key.ModCommand})
		if !ok {
			return
		}
		if ke, ok := e.(key.Event); ok {
			v.captureShortcutKey(gtx, ke)
			if s := v.settingsInteraction(); s == nil || !s.capturing {
				return
			}
		}
	}
}

func (v *view) captureShortcutKey(gtx layout.Context, ke key.Event) {
	if ke.State != key.Press {
		return
	}
	s := v.settingsInteraction()
	v.dismissFeedback()
	if ke.Name == key.NameEscape && ke.Modifiers == 0 {
		v.escape(gtx)
		return
	}
	switch ke.Name {
	case key.NameCtrl, key.NameShift, key.NameAlt, key.NameSuper, key.NameCommand:
		return
	}
	name := ke.Name
	if name == key.NameEnter {
		name = key.NameReturn
	}
	if v.setShortcutDraft((shortcut{Name: name, Modifiers: ke.Modifiers}).String()) {
		v.setShortcutCapture(s, false)
		v.playSound(sound.Settings)
		v.focusInteraction(gtx)
	}
}

func (v *view) setShortcutDraft(text string) bool {
	s := v.settingsInteraction()
	if !shortcutFieldEditable(s, s.selected) {
		s.errorText = "Shortcut is protected"
		return false
	}
	catalogue := bindingSettingsCatalogue()
	field := s.fields[s.selected].Key
	candidate := s.draft.Clone()
	if err := candidate.Set(catalogue, field, settings.Value{Text: text}); err != nil {
		s.errorText = "Unsupported shortcut"
		return false
	}
	if conflict := findBindingConflict(candidate); conflict != nil {
		s.errorText = shortcutConflictText(conflict, s.bindingCommand.id, s.selected)
		return false
	}
	if !s.draft.Overridden(field) || s.draft.Get(catalogue, field).Text != text {
		s.dirty = true
	}
	s.draft, s.errorText = candidate, ""
	return true
}

func shortcutConflictText(conflict *bindingConflict, id string, slot int) string {
	other := conflict.first
	if other.id == id && (other.slot == slot || conflict.second.id != id) {
		other = conflict.second
	}
	if other.id == id {
		return "Conflict: Shortcut " + strconv.Itoa(other.slot+1)
	}
	return "Conflict: " + other.name
}

func (v *view) restoreWidgetPalette(s *settingsSession, saved bool) {
	if s.palette == nil || saved && s.bindingCommand == nil {
		return
	}
	p := s.palette
	if s.bindingCommand != nil {
		p.commands = v.commandCatalogue()
		for i := range p.results {
			p.results[i].command = v.resolveCommandBindings(p.results[i].command)
		}
	}
	v.active = p.panel
	v.interaction.begin(p)
}
