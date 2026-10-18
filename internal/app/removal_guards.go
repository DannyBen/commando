package app

import "github.com/dannyben/commando/internal/settings"

const (
	guardOff = iota
	guardAction
	guardCancel
)

// Guards express independent reasons to ask. A directory guard applies to any
// selection containing a directory; a single-file guard applies only to one file.
// The strongest matching guard also chooses the initial confirmation button.
func (v *view) removalGuard(permanent bool, count int, hasDirectory bool) int {
	group := "guards.recycle_confirmation."
	if permanent {
		group = "guards.delete_confirmation."
	}
	strength := guardOff
	consider := func(name string, matches bool) {
		if matches {
			value := guardOff
			switch v.settingsValues.Get(settings.CatalogueDefault, group+name).Text {
			case "ON":
				value = guardAction
			case "ON (Cancel default)":
				value = guardCancel
			}
			strength = max(strength, value)
		}
	}
	consider("single_files", count == 1 && !hasDirectory)
	consider("directories", hasDirectory)
	consider("multiple_entries", count > 1)
	return strength
}
