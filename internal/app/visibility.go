package app

import (
	"slices"
	"sort"
	"strings"

	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

type visibilityMode uint8

const (
	visibilityAll visibilityMode = iota
	visibilityHideHidden
	visibilityHideHiddenDotfiles
	visibilityModeCount
)

func visibilityFromState(value string) visibilityMode {
	switch value {
	case "hide-hidden":
		return visibilityHideHidden
	case "hide-hidden-dotfiles":
		return visibilityHideHiddenDotfiles
	default:
		return visibilityAll
	}
}

func (m visibilityMode) stateName() string {
	switch m {
	case visibilityHideHidden:
		return "hide-hidden"
	case visibilityHideHiddenDotfiles:
		return "hide-hidden-dotfiles"
	default:
		return "all"
	}
}

func (m visibilityMode) label() string {
	switch m {
	case visibilityHideHidden:
		return "Hide hidden"
	case visibilityHideHiddenDotfiles:
		return "Hide hidden and dotfiles"
	default:
		return "All files"
	}
}

func (m visibilityMode) shows(file browser.Entry) bool {
	if file.Parent || file.Path != "" {
		return true
	}
	if file.Hidden && file.System {
		return false
	}
	return !(m != visibilityAll && file.Hidden || m == visibilityHideHiddenDotfiles && strings.HasPrefix(file.Name, "."))
}

func (v *view) cycleVisibility() {
	p := &v.panels[v.active]
	if !v.interaction.idle() || p.loading || p.drives {
		return
	}
	p.ensureEntries()
	p.visibility = (p.visibility + 1) % visibilityModeCount
	p.refilter()
	v.showFeedback(v.active, feedback.Info, p.visibility.label())
}

func (p *panel) ensureEntries() {
	if p.entries == nil {
		p.entries = make([]browser.Entry, len(p.items))
		for i, item := range p.items {
			p.entries[i] = item.file
		}
	}
}

func (p *panel) visibleEntries() []browser.Entry {
	files := make([]browser.Entry, 0, len(p.entries))
	for _, file := range p.entries {
		if p.drives || p.shows(file) {
			files = append(files, file)
		}
	}
	if p.sortByMetadata && p.metadata != metadataNone && !p.drives {
		slices.SortFunc(files, p.compareEntries)
	}
	return files
}

func (p *panel) cacheEntry(file browser.Entry) {
	p.ensureEntries()
	for i, existing := range p.entries {
		if samePath(existing.Name, file.Name) {
			p.entries[i] = file
			slices.SortFunc(p.entries, browser.CompareEntries)
			return
		}
	}
	p.entries = append(p.entries, file)
	slices.SortFunc(p.entries, browser.CompareEntries)
}

// Rebuild only the visible projection. Selection/viewport anchors and surviving
// marks follow their entries; entries hidden by a mode change lose their marks.
func (p *panel) refilter() {
	selected, first := browser.Entry{}, browser.Entry{}
	if p.selected >= 0 && p.selected < len(p.items) {
		selected = p.items[p.selected].file
	}
	if row := p.list.Position.First; row >= 0 && row < len(p.items) {
		first = p.items[row].file
	}
	p.reconcileDirectoryListing(browser.Directory{Path: p.path, ArchiveFolder: p.archiveFolder, Drives: p.drives, Entries: p.entries})
	nearest := func(file browser.Entry) int {
		row := sort.Search(len(p.items), func(i int) bool { return p.compareEntries(p.items[i].file, file) >= 0 })
		return min(row, len(p.items)-1)
	}
	if selected.Name != "" && !p.shows(selected) {
		p.selected, p.reveal = nearest(selected), len(p.items) > 0
	}
	if first.Name != "" && !p.shows(first) {
		p.list.Position.First, p.list.Position.Offset = max(0, nearest(first)), 0
	}
}
