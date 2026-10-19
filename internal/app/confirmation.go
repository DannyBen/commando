package app

import (
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/dannyben/commando/internal/diagnostics"
)

// A decision temporarily owns input while its originating interaction stays
// intact underneath. Callbacks run only after ownership returns to that caller.
type confirmationSession struct {
	panel                int
	message, acceptLabel string
	selected             int // 0 accepts, 1 cancels. Callers choose the initial focus.
	accept               func(layout.Context)
	surface              overlay
	actions              actionBar
	messageList          widget.List
	held                 map[key.Name]bool
}

func (s *confirmationSession) interactionPanel() int        { return s.panel }
func (*confirmationSession) blocksPanels() bool             { return true }
func (*confirmationSession) blocksMutations() bool          { return true }
func (*confirmationSession) openingEvent() interactionEvent { return eventQuestionOpened }

func (s *confirmationSession) holdKey(name key.Name) {
	if name == key.NameEnter {
		name = key.NameReturn
	}
	if s.held == nil {
		s.held = make(map[key.Name]bool)
	}
	s.held[name] = true
}
func (v *view) confirmationInteraction() *confirmationSession {
	s, _ := v.interaction.current.(*confirmationSession)
	return s
}
func (v *view) startConfirmation(gtx layout.Context, s *confirmationSession) bool {
	var previousFocus any = &v.focus
	if editor := v.activeEditor(); editor != nil {
		previousFocus = editor
	}
	s.messageList.Axis = layout.Vertical
	if !v.startInteraction(s) {
		return false
	}
	s.releaseInvocationKeys(gtx, previousFocus)
	v.focusInteraction(gtx)
	return true
}

// A quick invocation can queue its release at the old focus before the dialog
// opens. Consume that tail before focus moves, so the next press is not mistaken
// for a repeat. Opening-key repeats remain suppressed until an actual release.
func (s *confirmationSession) releaseInvocationKeys(gtx layout.Context, previousFocus any) {
	for name, held := range s.held {
		if !held {
			continue
		}
		filter := func(name key.Name) event.Filter {
			return key.Filter{Focus: previousFocus, Name: name, Optional: key.ModCtrl | key.ModShift | key.ModAlt | key.ModSuper | key.ModCommand}
		}
		filters := []event.Filter{filter(name)}
		if name == key.NameReturn {
			filters = append(filters, filter(key.NameEnter))
		}
		for {
			e, ok := gtx.Event(filters...)
			if !ok {
				break
			}
			if ke, ok := e.(key.Event); ok && ke.State == key.Release {
				delete(s.held, name)
			}
		}
	}
}
func (v *view) finishConfirmation(gtx layout.Context, s *confirmationSession, accept bool) {
	if !v.interaction.finish(s) {
		return
	}
	diagnostics.Record(diagnostics.UI, 0, "ConfirmationResolved", map[string]any{"action": s.acceptLabel, "accepted": accept})
	if accept {
		if s.accept != nil {
			s.accept(gtx)
		}
	} else {
		v.announceInteraction(eventCancelled)
	}
	v.interaction.focusPending = true
	v.focusInteraction(gtx)
}
func (v *view) confirmationKey(gtx layout.Context, s *confirmationSession, ke key.Event) {
	name := ke.Name
	if name == key.NameEnter {
		name = key.NameReturn
	}
	if ke.State == key.Release {
		delete(s.held, name)
		return
	}
	if s.held[name] {
		return
	}
	if s.held == nil {
		s.held = make(map[key.Name]bool)
	}
	s.held[name] = true
	if ke.Modifiers != 0 && !(name == key.NameTab && ke.Modifiers == key.ModShift) {
		return
	}
	previous := s.selected
	switch name {
	case key.NameEscape:
		v.finishConfirmation(gtx, s, false)
	case key.NameReturn, key.NameSpace:
		v.finishConfirmation(gtx, s, s.selected == 0)
	case key.NameLeftArrow, key.NameHome:
		s.selected = 0
	case key.NameRightArrow, key.NameEnd:
		s.selected = 1
	case key.NameTab:
		s.selected = 1 - s.selected
	}
	if v.confirmationInteraction() == s && previous != s.selected {
		v.announceInteraction(eventSelectionChanged)
	}
}
func (v *view) updateConfirmation(gtx layout.Context, s *confirmationSession) {
	if !gtx.Focused(s) {
		v.focusInteraction(gtx)
	}
	// Outside presses are absorbed by the shared overlay and never dismiss it.
	s.surface.outsidePress(gtx)
	if action, ok := s.actions.update(gtx); ok {
		v.finishConfirmation(gtx, s, action == 0)
		return
	}
	for {
		e, ok := gtx.Event(
			key.FocusFilter{Target: s},
			// Gio reserves native Tab presses for focus navigation unless an
			// explicit named filter claims their system-event delivery.
			key.Filter{Focus: s, Name: key.NameTab, Optional: key.ModShift},
			key.Filter{Focus: s, Optional: key.ModCtrl | key.ModShift | key.ModAlt | key.ModSuper | key.ModCommand},
		)
		if !ok {
			return
		}
		if ke, ok := e.(key.Event); ok {
			v.confirmationKey(gtx, s, ke)
		}
		if v.confirmationInteraction() != s {
			return
		}
	}
}
