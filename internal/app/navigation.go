package app

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func newLocationView(locations []panelState, active int, invalidate func()) *view {
	panels := make([]panel, len(locations))
	for i, location := range locations {
		panels[i].path, panels[i].drives, panels[i].selected = location.Path, location.Drives, -1
	}
	v := newView(panels)
	if active >= 0 && active < len(panels) {
		v.active = active
	}
	v.openFile = browser.OpenFile
	v.loader = newDirectoryLoader(browser.ReadLocation, invalidate, len(panels))
	for i, location := range locations {
		v.panels[i].metadata = metadataFromState(location.Metadata)
		v.panels[i].sortByMetadata = location.SortByMetadata
		v.panels[i].visibility = visibilityFromState(location.Visibility)
		if location.Drives {
			v.load(i, "", location.Path)
		} else {
			v.queueLoad(loadRequest{panel: i, path: location.Path})
		}
	}
	return v
}

func (v *view) load(index int, path, selectName string) {
	v.queueLoad(loadRequest{panel: index, path: path, selectName: selectName})
}

func (v *view) refreshPanel() {
	if v.activeEditor() != nil {
		return
	}
	p := &v.panels[v.active]
	path := p.path
	if p.drives {
		path = ""
	}
	v.queueLoad(loadRequest{panel: v.active, path: path, refresh: true})
}

func (v *view) queueLoad(request loadRequest) {
	request.action = actionBrowse
	if v.interaction.protectsListing(request.panel) || v.pendingNameDraft != nil && v.pendingNameDraft.request.panel == request.panel {
		return
	}
	p := &v.panels[request.panel]
	if v.loader == nil || p.loading {
		return
	}
	if !v.loader.submit(request) {
		return
	}
	p.loading, p.err = true, nil
	p.navigating = !request.refresh
	progress := "Reading folder"
	if request.shortcut {
		progress = "Opening shortcut"
	}
	p.feedback.ProgressAt(progress, time.Now().Add(v.appearance.FeedbackProgressDelay))
	if !request.refresh && !request.shortcut && v.sounds != nil {
		v.announceInteraction(eventDirectoryNavigation)
	}
}

func (v *view) pollLoads() {
	if v.loader == nil {
		return
	}
	for {
		select {
		case result := <-v.loader.results:
			v.acceptLoad(result)
		default:
			return
		}
	}
}

func (v *view) acceptLoad(result loadResult) {
	if v.loader != nil {
		defer v.loader.complete(result.request)
	}
	if result.request.fontFamilies {
		v.fontFamiliesLoading = false
		v.fontFamilies = append([]string{}, result.fontFamilies...)
		if s := v.settingsInteraction(); s != nil {
			s.completion.candidates = v.fontFamilyChoices()
			s.completion.update(&s.editor)
		}
		return
	}
	if result.request.editor != nil {
		v.acceptEditorLaunch(result)
		return
	}
	if result.request.settingsSave != nil {
		v.acceptSettingsSave(result.request.settingsSave.session, result.settingsMessage, result.err)
		return
	}
	if result.request.autoRefresh {
		v.acceptAutoRefresh(result)
		return
	}
	if result.request.clipboard != nil || result.request.transfer != nil {
		v.acceptFileOperation(result)
		return
	}
	if result.request.recycle != nil {
		v.acceptRecycle(result)
		return
	}
	if result.request.create != nil || result.request.rename != nil || result.request.duplicate != nil {
		v.acceptName(result)
		return
	}
	p := &v.panels[result.request.panel]
	if result.request.shortcut {
		if result.externalOpen {
			p.loading, p.navigating = false, false
			p.feedback.Complete()
			v.openPath(result.request.path)
			return
		}
		v.announceInteraction(eventDirectoryNavigation)
	}
	p.loading, p.err = false, result.err
	p.navigating = false
	if result.err != nil {
		v.showFeedback(result.request.panel, feedback.Error, readError(result.err))
		return
	}
	p.feedback.Complete()
	refresh := result.request.refresh && p.drives == result.directory.Drives &&
		(p.drives || samePath(filepath.Clean(p.path), filepath.Clean(result.directory.Path)))
	if p.drives != result.directory.Drives || !samePath(p.path, result.directory.Path) {
		p.refreshNeeded = false
	}
	if result.request.refresh && v.monitor != nil && !result.directory.Drives {
		v.monitor.retry(result.directory.Path)
	}
	previousRow := p.selected
	var previousItem browser.Entry
	if previousRow >= 0 && previousRow < len(p.items) {
		previousItem = p.items[previousRow].file
	}
	if !refresh {
		p.filter = ""
	}
	p.clearMarks()
	p.prefix = ""
	p.drives = result.directory.Drives
	if !p.drives {
		p.path = result.directory.Path
	}
	p.entries = slices.Clone(result.directory.Entries)
	files := p.visibleEntries()
	p.items = make([]entry, len(files))
	p.selected = -1
	if len(p.items) > 0 {
		p.selected = 0
		if refresh {
			p.selected = max(0, min(previousRow, len(p.items)-1))
		}
	}
	for i, file := range files {
		name, size := file.Name, "?"
		if file.Directory {
			size = "—"
		} else if file.SizeKnown {
			size = formatSize(file.Bytes)
		}
		p.items[i] = entry{name: name, size: size, file: file}
		if refresh && previousRow >= 0 {
			sameItem := file.Name == previousItem.Name && file.Parent == previousItem.Parent
			if p.drives {
				sameItem = previousItem.Path != "" && samePath(file.Path, previousItem.Path)
			}
			if sameItem {
				p.selected = i
			}
		}
		if (file.Directory || result.request.explicitPath) && (file.Name == result.request.selectName ||
			result.request.explicitPath && samePath(file.Name, result.request.selectName) ||
			file.Path != "" && strings.EqualFold(file.Path, result.request.selectName)) {
			p.selected = i
		}
	}
	p.rows = make(map[int]*fileRow)
	if refresh {
		// Keep the viewport unless the current item moved or disappeared.
		p.reveal = p.reveal || p.selected != previousRow
	} else {
		p.list = widget.List{List: layout.List{Axis: layout.Vertical}}
		p.reveal = p.selected >= 0
	}
	if result.pathMissing {
		v.showFeedback(result.request.panel, feedback.Warning, "Path not found")
	}
}

func (v *view) activateSelected() {
	p := &v.panels[v.active]
	if p.loading || p.selected < 0 || p.selected >= len(p.items) {
		return
	}
	file := p.items[p.selected].file
	if !file.Directory {
		if v.settingsSelecting() {
			return
		}
		if file.Name != "" {
			path := filepath.Join(p.path, file.Name)
			if strings.EqualFold(filepath.Ext(path), ".lnk") && v.loader != nil {
				v.queueLoad(loadRequest{panel: v.active, path: path, shortcut: true})
			} else {
				v.openPath(path)
			}
		}
		return
	}
	if file.Path != "" {
		v.load(v.active, file.Path, "")
	} else if file.Parent {
		v.parentFolder()
	} else {
		v.load(v.active, filepath.Join(p.path, file.Name), "")
	}
}

func (v *view) parentFolder() {
	p := &v.panels[v.active]
	if p.loading || p.drives {
		return
	}
	parent := filepath.Dir(p.path)
	if parent == p.path {
		v.load(v.active, "", p.path)
		return
	}
	v.load(v.active, parent, filepath.Base(p.path))
}

func (v *view) openAdjacent(direction int) {
	target := v.active + direction
	if v.loader == nil || target >= 0 && target < len(v.panels) && v.panels[target].loading {
		return
	}
	p := &v.panels[v.active]
	if p.loading || p.selected < 0 || p.selected >= len(p.items) {
		return
	}
	file := p.items[p.selected].file
	if !file.Directory {
		return
	}
	path := file.Path
	if path == "" {
		path = filepath.Join(p.path, file.Name)
	}
	if target < 0 || target >= len(v.panels) {
		if !v.addPanel(direction) {
			return
		}
	} else {
		v.switchPanel(direction)
	}
	v.load(v.active, path, "")
	v.panels[v.active].prefix = ""
}

func (p *panel) itemCount() int {
	count := len(p.items)
	if count > 0 && p.items[0].file.Parent {
		count--
	}
	return count
}

func (p *panel) status() (string, bool) {
	if message := p.feedback.Current(); message != nil {
		return message.Text, message.Kind == feedback.Error
	}
	if p.drives {
		return fmt.Sprintf("%d drives", len(p.items)), false
	}
	if len(p.marks) > 0 {
		return fmt.Sprintf("%d marked", len(p.marks)), false
	}
	return fmt.Sprintf("%d items", p.itemCount()), false
}

func formatSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	size := float64(bytes)
	units := [...]string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}
	unit := 0
	for size >= 1024 && unit < len(units)-1 {
		size /= 1024
		unit++
	}
	return fmt.Sprintf("%.1f %s", size, units[unit])
}

func (p *panel) locationLabel() string {
	if p.drives {
		return "Drives"
	}
	return p.path
}

func (p *panel) locationPath() string {
	if p.drives && p.selected >= 0 && p.selected < len(p.items) {
		return p.items[p.selected].file.Path
	}
	return p.path
}

func (v *view) openPath(path string) {
	if open := v.openFile; open != nil {
		go func() { _ = open(path) }()
	}
}
