package app

import "github.com/dannyben/commando/internal/settings"

// The third row is a projection of the draft, not a second copy of its values.
// Its empty placeholder preserves the dialog footprint for Both actions.
func (s *settingsSession) projectActionFields() {
	if s.collection != settings.ActionCollection {
		return
	}
	key := ""
	switch s.draft.Get(settings.CatalogueDefault, settings.ActionCollection+".target").Text {
	case settings.ActionFiles:
		key = settings.ActionCollection + ".ext"
	case settings.ActionFolders:
		key = settings.ActionCollection + ".mode"
	}
	s.fields[2] = settings.Field{}
	for _, field := range settings.CatalogueDefault.UI.Fields {
		if field.Key == key {
			s.fields[2] = field
			break
		}
	}
}
