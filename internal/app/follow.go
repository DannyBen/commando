package app

import (
	"image/color"
	"path/filepath"
)

func (v *view) metadataHeaderColors(index int) (background, filename, metadata color.NRGBA) {
	theme := v.appearance
	if !v.followMe {
		return theme.Colors.Surface, theme.Colors.Filename, theme.Colors.Metadata
	}
	background, filename = v.activeBandColors(v.followColor(index), settingHeaderOffset)
	return background, filename, filename
}

// The indicator describes current rows, not a remembered navigation outcome.
func (v *view) followColor(index int) color.NRGBA {
	colors := v.appearance.Colors
	if index == v.active {
		return colors.Info
	}
	source, target := &v.panels[v.active], &v.panels[index]
	for _, p := range []*panel{source, target} {
		if !p.nativeFolder() || p.navigating || p.selected < 0 || p.selected >= len(p.items) {
			return colors.Error
		}
		file := p.items[p.selected].file
		if file.Parent || file.Favorite {
			return colors.Error
		}
	}
	a, b := source.items[source.selected].file, target.items[target.selected].file
	if a.Directory == b.Directory && target.entryKey(a.Name) == target.entryKey(b.Name) {
		return colors.Success
	}
	return colors.Error
}

// Only explicit user actions broadcast. Loads, refreshes and received actions
// never call these helpers, so following cannot generate a feedback loop.
func (v *view) followSelection() {
	source := &v.panels[v.active]
	if !v.followMe || !source.nativeFolder() || source.loading || !v.interaction.idle() || source.selected < 0 || source.selected >= len(source.items) {
		return
	}
	file := source.items[source.selected].file
	if file.Parent || file.Favorite {
		return
	}
	for i := range v.panels {
		if !v.followReady(i) {
			continue
		}
		p := &v.panels[i]
		for row, item := range p.items {
			if !item.file.Parent && !item.file.Favorite && item.file.Directory == file.Directory && p.entryKey(item.file.Name) == p.entryKey(file.Name) {
				p.selected, p.reveal, p.markRange = row, true, nil
				break
			}
		}
	}
}

func (v *view) followReady(index int) bool {
	p := &v.panels[index]
	return index != v.active && p.nativeFolder() && !p.loading && !p.refreshing &&
		!v.interaction.protectsListing(index) && v.pendingNameDraft == nil
}

func (v *view) followNavigation(name string, parent bool) {
	if !v.followMe || !v.panels[v.active].nativeFolder() || !v.interaction.idle() {
		return
	}
	for i := range v.panels {
		if !v.followReady(i) {
			continue
		}
		p := &v.panels[i]
		if parent {
			path := filepath.Dir(p.path)
			// "Leave X" applies only to panels currently inside a folder named X.
			if path != p.path && p.entryKey(filepath.Base(p.path)) == p.entryKey(name) {
				v.queueLoad(loadRequest{panel: i, path: path, selectName: filepath.Base(p.path), follow: true})
			}
			continue
		}
		// Use the complete cached listing, independently of each panel's filter.
		for _, file := range p.entries {
			if file.Directory && !file.Parent && !file.Favorite && file.Path == "" && p.entryKey(file.Name) == p.entryKey(name) {
				v.queueLoad(loadRequest{panel: i, path: filepath.Join(p.path, file.Name), follow: true})
				break
			}
		}
	}
}
