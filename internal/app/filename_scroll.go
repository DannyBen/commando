package app

import (
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

const (
	settingFilenameScroll        = "feedback.filenames"
	settingFilenameScrollEnabled = settingFilenameScroll + ".enabled"
	settingFilenameScrollModel   = settingFilenameScroll + ".animation"
	settingFilenameScrollSpeed   = settingFilenameScroll + ".speed"
)

// Only the current filename with browsing focus may animate. Clearing both
// state and identity makes every return from another interaction start fresh.
func (v *view) prepareFilenameScroll(gtx layout.Context, index int) bool {
	p := &v.panels[index]
	if index != v.active || !v.interaction.idle() || !gtx.Focused(&v.focus) || p.loading || p.navigating ||
		p.drives || p.selected < 0 || p.selected >= len(p.items) || p.items[p.selected].file.Parent ||
		!v.settingsValues.Get(settings.CatalogueDefault, settingFilenameScrollEnabled).Bool {
		p.filenameScroll, p.filenameScrollID = textMarquee{}, ""
		return false
	}
	id := p.path + "\x00" + p.archiveFolder + "\x00" + p.items[p.selected].file.Name + "\x00" + p.metadata.stateName()
	if p.filenameScrollID != id {
		p.filenameScroll, p.filenameScrollID = textMarquee{}, id
	}
	return true
}
