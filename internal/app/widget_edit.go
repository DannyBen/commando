package app

import "gioui.org/layout"

// Text fields and shortcut capture share editing lifecycle feedback. Drafts
// applied while tabbing or saving stay quiet; their enclosing action announces
// navigation or successful save instead of playing two cues.
func (v *view) beginWidgetEdit(gtx layout.Context, capture bool) {
	s := v.settingsInteraction()
	s.editing, s.errorText = !capture, ""
	v.setShortcutCapture(s, capture)
	v.announceInteraction(eventEditStarted)
	v.focusInteraction(gtx)
}

func (v *view) confirmWidgetEdit(gtx layout.Context) {
	s := v.settingsInteraction()
	s.editing, s.errorText = false, ""
	v.setShortcutCapture(s, false)
	v.announceInteraction(eventEditConfirmed)
	v.focusInteraction(gtx)
}
