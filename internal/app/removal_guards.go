package app

import "github.com/dannyben/commando/internal/settings"

const (
	guardOff = iota
	guardAction
	guardCancel
)

func (p *panel) removalSelection() (names []string, hasDirectory bool) {
	for row, item := range p.items {
		selected := p.marks[row]
		if len(p.marks) == 0 {
			selected = row == p.selected
		}
		if selected && p.markable(row) {
			names = append(names, item.file.Name)
			hasDirectory = hasDirectory || item.file.Directory
		}
	}
	return
}

func (v *view) removalPresentation(permanent bool) commandPresentation {
	p := &v.panels[v.active]
	presentation := commandPresentation{name: "Recycle", hidden: permanent && p.archiveFolder != ""}
	if permanent {
		presentation.name = "Delete permanently"
	} else if p.archiveFolder != "" {
		presentation.name = "Delete from archive"
	}
	names, hasDirectory := p.removalSelection()
	if len(names) > 0 && v.removalGuard(permanent || p.archiveFolder != "", len(names), hasDirectory) != guardOff {
		presentation.name += "…"
	}
	return presentation
}

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
