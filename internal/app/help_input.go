package app

import (
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
)

// Function-key bindings work across foreground sessions. Other bindings retain
// normal browsing dispatch, leaving text and native editor shortcuts alone.
// Shortcut capture always owns its input, including F1.
func (v *view) updateHelpInput(gtx layout.Context) {
	if s := v.settingsInteraction(); s != nil && s.capturing {
		clear(v.helpHeld)
		return
	}
	focus := any(&v.focus)
	if s := v.confirmationInteraction(); s != nil {
		focus = s
	} else if editor := v.activeEditor(); editor != nil {
		focus = editor
	}
	if !gtx.Focused(focus) {
		clear(v.helpHeld)
		return
	}
	c := v.helpCommand()
	var filters []event.Filter
	for slot := range c.bindings {
		s := c.effectiveShortcut(slot)
		if len(s.Name) > 1 && s.Name[0] == 'F' {
			filters = append(filters, key.Filter{Focus: focus, Name: s.Name, Required: s.Modifiers})
		}
	}
	if len(filters) == 0 {
		return
	}
	for {
		e, ok := gtx.Event(filters...)
		if !ok {
			return
		}
		ke, ok := e.(key.Event)
		if !ok {
			continue
		}
		if ke.State == key.Release {
			delete(v.helpHeld, ke.Name)
			continue
		}
		if v.helpHeld[ke.Name] {
			continue
		}
		if v.helpHeld == nil {
			v.helpHeld = make(map[key.Name]bool)
		}
		v.helpHeld[ke.Name] = true
		v.dismissFeedback()
		v.executeCommand(gtx, c)
	}
}
