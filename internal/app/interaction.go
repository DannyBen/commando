package app

import (
	"fmt"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/diagnostics"
)

// Foreground interaction has one owner. Background work and panel loading are
// independent: completing a read or transfer does not change this session.
// New interactions supply their own session data and join the routing below.
type interactionSession interface {
	interactionPanel() int
}

type interactionController struct {
	current      interactionSession // nil is ordinary browsing.
	suspended    interactionSession
	focusPending bool // Worker completions restore focus through the next frame's input source.
}

func (c *interactionController) operationsHeld() bool {
	owner, ok := c.current.(interface{ holdsOperations() bool })
	return ok && owner.holdsOperations()
}

func (c *interactionController) panelsBlocked() bool {
	owner, ok := c.current.(interface{ blocksPanels() bool })
	return ok && owner.blocksPanels()
}

func (c *interactionController) idle() bool { return c.current == nil }

func (c *interactionController) begin(session interactionSession) bool {
	if session == nil {
		return false
	}
	if !c.idle() {
		owner, ok := c.current.(interface{ canSuspendFor(interactionSession) bool })
		if !ok || c.suspended != nil || !owner.canSuspendFor(session) {
			return false
		}
		c.suspended = c.current
	}
	c.current = session
	diagnostics.Record(diagnostics.UI, 0, "InteractionStarted", map[string]any{"type": fmt.Sprintf("%T", session), "panel": session.interactionPanel()})
	return true
}

// startInteraction announces a fresh session only after it is accepted.
// Restoring an existing session uses the controller directly and stays quiet.
func (v *view) startInteraction(session interactionSession) bool {
	if !v.interaction.begin(session) {
		return false
	}
	if owner, ok := session.(interface{ openingEvent() interactionEvent }); ok {
		v.announceInteraction(owner.openingEvent())
	}
	return true
}

// Only the owning session can end an interaction. A stale completion cannot
// close an editor or decision opened after it.
func (c *interactionController) finish(session interactionSession) bool {
	if session == nil || c.current != session {
		return false
	}
	diagnostics.Record(diagnostics.UI, 0, "InteractionFinished", map[string]any{"type": fmt.Sprintf("%T", session), "panel": session.interactionPanel()})
	c.current, c.suspended = c.suspended, nil
	return true
}

func (c *interactionController) protectsListing(panel int) bool {
	if owner, ok := c.current.(interface{ protectsListing() bool }); ok && !owner.protectsListing() {
		return false
	}
	return c.current != nil && c.current.interactionPanel() == panel
}

type locationSession struct {
	panel                       int
	completion                  inlineCompletion
	completionPath, pendingPath string
	completionLoaded            bool
	completionEntries           []browser.Entry
}

func (s *locationSession) interactionPanel() int     { return s.panel }
func (s *nameSession) interactionPanel() int         { return s.panel }
func (s *inlineActionSession) interactionPanel() int { return s.panel }

func (v *view) locationEditing() bool {
	_, ok := v.interaction.current.(*locationSession)
	return ok
}

func (v *view) nameInteraction() *nameSession {
	s, _ := v.interaction.current.(*nameSession)
	return s
}

func (v *view) inlineInteraction() *inlineActionSession {
	s, _ := v.interaction.current.(*inlineActionSession)
	return s
}

func (v *view) activeEditor() *widget.Editor {
	if s := v.settingsInteraction(); s != nil {
		if s.editing {
			return &s.editor
		}
		return nil
	}
	switch v.interaction.current.(type) {
	case *locationSession:
		return &v.location
	case *nameSession:
		return &v.nameEditor
	case *filterSession:
		return &v.filterEditor
	case *paletteSession:
		return &v.commandSearch
	default:
		return nil
	}
}

func (v *view) focusInteraction(gtx layout.Context) {
	var target any = &v.focus
	if s := v.confirmationInteraction(); s != nil {
		target = s
	} else if editor := v.activeEditor(); editor != nil {
		target = editor
	}
	gtx.Execute(key.FocusCmd{Tag: target})
}

// Cancellation owns the transition; each session retains its semantic cleanup
// (restoring a draft viewport, cancelling a copy, returning to the source pane).
// A submitted name mutation must finish before its editor can be left.
func (v *view) cancelInteraction(gtx layout.Context) bool {
	switch s := v.interaction.current.(type) {
	case *confirmationSession:
		return false // Only its explicit buttons and Escape resolve a decision.
	case *settingsSession:
		return s.selecting
	case *filterSession:
		v.stopFilter(gtx)
	case *locationSession:
		v.stopLocation(gtx)
	case *nameSession:
		if s.pending {
			return false
		}
		v.stopName(gtx)
	case *inlineActionSession:
		v.cancelInlineActions()
		v.focusInteraction(gtx)
	case *paletteSession:
		v.stopPalette(gtx)
	default:
		return v.interaction.idle()
	}
	return true
}

// Escape uses the same session cancellation as other input routes, with one
// semantic cue. Submitted name mutations remain protected until reconciliation.
func (v *view) escape(gtx layout.Context) {
	if s := v.confirmationInteraction(); s != nil {
		v.finishConfirmation(gtx, s, false)
		return
	}
	if s := v.settingsInteraction(); s != nil {
		if s.pending {
			return
		}
		if s.dirty && !s.editing && !s.selecting && !s.capturing {
			s.actionsFocused, s.action = true, 1
			v.focusInteraction(gtx)
			if v.sounds != nil {
				v.announceInteraction(eventUnsavedChanges)
			}
			return
		}
	}
	if s := v.nameInteraction(); s != nil && s.pending {
		return
	}
	v.dismissFeedback()
	if s := v.settingsInteraction(); s != nil {
		if s.capturing {
			v.setShortcutCapture(s, false)
			s.errorText = ""
			v.focusInteraction(gtx)
		} else if s.editing {
			s.editing = false
			s.errorText = ""
			s.actionsFocused = false
			v.focusInteraction(gtx)
		} else if s.selecting {
			s.selecting = false
			v.focusInteraction(gtx)
		} else {
			v.finishSettingsWidget(gtx, false)
		}
	} else if v.interaction.idle() {
		p := &v.panels[v.active]
		if p.prefix != "" {
			p.prefix = ""
		} else if p.filter != "" {
			p.setFilter("")
		} else {
			p.clearMarks()
		}
	} else if s := v.filterInteraction(); s != nil {
		v.panels[s.panel].setFilter("")
		v.stopFilter(gtx)
	} else if !v.cancelInteraction(gtx) {
		return
	}
	if v.sounds != nil {
		v.announceInteraction(eventCancelled)
	}
	// Cancellation may produce a footer result, such as Copy cancelled. Its
	// animation stays intact; the Escape cue already announces the result.
	v.panels[v.active].feedback.Acknowledge()
}

// Return true when a foreground session owns this key, even when the session
// ignores it. Browsing shortcuts must not leak through an editor or choice.
func (v *view) interactionKey(gtx layout.Context, ke key.Event) bool {
	if s := v.confirmationInteraction(); s != nil {
		v.confirmationKey(gtx, s, ke)
		return true
	}
	if s := v.settingsInteraction(); s != nil && s.capturing {
		v.captureShortcutKey(gtx, ke)
		return true
	}
	if v.settingsSelecting() {
		return false
	}
	if editor := v.activeEditor(); editor != nil {
		pasteWithInsert(gtx, editor, ke)
		return true
	}
	if a := v.inlineInteraction(); a != nil && a.panel == v.active {
		v.inlineActionKey(gtx, ke)
		return true
	}
	return !v.interaction.idle()
}

// Editors consume their events before ordinary browsing, preserving event
// order and native text handling. True means editing is still active.
func (v *view) updateInteraction(gtx layout.Context) bool {
	if s := v.confirmationInteraction(); s != nil {
		v.updateConfirmation(gtx, s)
		return true
	}
	if s := v.settingsInteraction(); s != nil {
		v.updateSettingsWidget(gtx)
		return v.settingsInteraction() != s || !s.selecting
	}
	if v.filterInteraction() != nil {
		v.updateEditorInput(gtx, &v.filterEditor)
		v.updateFilter(gtx)
		return v.activeEditor() != nil
	}
	if v.activeEditor() == nil {
		return false
	}
	for !v.locationEditing() {
		e, ok := gtx.Event(key.Filter{Name: key.NameTab, Optional: key.ModShift})
		if !ok {
			break
		}
		if ke, ok := e.(key.Event); ok && ke.State == key.Press {
			v.dismissFeedback()
		}
	}
	switch v.interaction.current.(type) {
	case *nameSession:
		v.updateEditorInput(gtx, &v.nameEditor)
		v.updateName(gtx)
	case *locationSession:
		v.updateEditorInput(gtx, &v.location)
		v.updateLocation(gtx)
	case *paletteSession:
		v.updateEditorInput(gtx, &v.commandSearch)
		v.updatePalette(gtx)
	}
	return v.activeEditor() != nil
}

func (c *interactionController) mutationsBlocked() bool {
	for _, session := range []interactionSession{c.current, c.suspended} {
		if owner, ok := session.(interface{ blocksMutations() bool }); ok && owner.blocksMutations() {
			return true
		}
	}
	return false
}
