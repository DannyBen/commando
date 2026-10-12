package app

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/dannyben/commando/internal/sound"
)

// Foreground interaction has one owner. Background work and panel loading are
// independent: completing a read or transfer does not change this session.
// New interactions supply their own session data and join the routing below.
type interactionSession interface {
	interactionPanel() int
}

type interactionController struct {
	current interactionSession // nil is ordinary browsing.
}

func (c *interactionController) idle() bool { return c.current == nil }

func (c *interactionController) begin(session interactionSession) bool {
	if !c.idle() || session == nil {
		return false
	}
	c.current = session
	return true
}

// Only the owning session can end an interaction. A stale completion cannot
// close an editor or decision opened after it.
func (c *interactionController) finish(session interactionSession) bool {
	if session == nil || c.current != session {
		return false
	}
	c.current = nil
	return true
}

func (c *interactionController) protectsListing(panel int) bool {
	return c.current != nil && c.current.interactionPanel() == panel
}

type locationSession struct{ panel int }

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
	switch v.interaction.current.(type) {
	case *locationSession:
		return &v.location
	case *nameSession:
		return &v.nameEditor
	case *paletteSession:
		return &v.commandSearch
	default:
		return nil
	}
}

func (v *view) focusInteraction(gtx layout.Context) {
	var target any = &v.focus
	if editor := v.activeEditor(); editor != nil {
		target = editor
	}
	gtx.Execute(key.FocusCmd{Tag: target})
}

// Cancellation owns the transition; each session retains its semantic cleanup
// (restoring a draft viewport, cancelling a copy, returning to the source pane).
// A submitted name mutation must finish before its editor can be left.
func (v *view) cancelInteraction(gtx layout.Context) bool {
	switch s := v.interaction.current.(type) {
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
	}
	return true
}

// Escape uses the same session cancellation as other input routes, with one
// semantic cue. Submitted name mutations remain protected until reconciliation.
func (v *view) escape(gtx layout.Context) {
	if s := v.nameInteraction(); s != nil && s.pending {
		return
	}
	v.dismissFeedback()
	if v.interaction.idle() {
		p := &v.panels[v.active]
		if p.prefix != "" {
			p.prefix = ""
		} else {
			p.clearMarks()
		}
	} else if !v.cancelInteraction(gtx) {
		return
	}
	if v.sounds != nil {
		v.sounds.Play(sound.Cancel)
	}
	// Cancellation may produce a footer result, such as Copy cancelled. Its
	// animation stays intact; the Escape cue already announces the result.
	v.panels[v.active].feedback.Acknowledge()
}

// Return true when a foreground session owns this key, even when the session
// ignores it. Browsing shortcuts must not leak through an editor or choice.
func (v *view) interactionKey(gtx layout.Context, ke key.Event) bool {
	if editor := v.activeEditor(); editor != nil {
		pasteWithInsert(gtx, editor, ke)
		return true
	}
	if a := v.inlineInteraction(); a != nil && a.panel == v.active {
		v.inlineActionKey(gtx, ke)
		return true
	}
	return false
}

// Editors consume their events before ordinary browsing, preserving event
// order and native text handling. True means editing is still active.
func (v *view) updateInteraction(gtx layout.Context) bool {
	if v.activeEditor() == nil {
		return false
	}
	for {
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
		v.updateEditorPaste(gtx, &v.nameEditor)
		v.updateName(gtx)
	case *locationSession:
		v.updateEditorPaste(gtx, &v.location)
		v.updateLocation(gtx)
	case *paletteSession:
		v.updateEditorPaste(gtx, &v.commandSearch)
		v.updatePalette(gtx)
	}
	return v.activeEditor() != nil
}
