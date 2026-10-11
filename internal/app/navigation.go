package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func newBrowserView(paths []string, invalidate func()) *view {
	locations := make([]panelState, len(paths))
	for i, path := range paths {
		locations[i].Path = path
	}
	return newRestoredView(locations, 0, invalidate)
}

func newRestoredView(locations []panelState, active int, invalidate func()) *view {
	panels := make([]panel, len(locations))
	for i, location := range locations {
		panels[i].path, panels[i].drives, panels[i].selected = location.Path, location.Drives, -1
	}
	v := newView(panels)
	if active >= 0 && active < len(panels) {
		v.active = active
	}
	v.openFile = browser.OpenFile
	v.loader = newDirectoryLoader(browser.ReadLocation, invalidate)
	for i, location := range locations {
		v.panels[i].metadata = metadataFromState(location.Metadata)
		if location.Drives {
			v.load(i, "", location.Path)
		} else {
			v.load(i, location.Path, "")
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
	p := &v.panels[request.panel]
	if v.loader == nil || p.loading {
		return
	}
	p.loading, p.err = true, nil
	p.feedback.Progress("Reading folder")
	v.loader.requests <- request
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
	if result.request.create != nil || result.request.rename != nil {
		v.acceptName(result)
		return
	}
	p := &v.panels[result.request.panel]
	p.loading, p.err = false, result.err
	if result.err != nil {
		v.showFeedback(result.request.panel, feedback.Error, readError(result.err))
		return
	}
	p.feedback.Complete()
	previousRow := p.selected
	var previousItem browser.Entry
	if previousRow >= 0 && previousRow < len(p.items) {
		previousItem = p.items[previousRow].file
	}
	p.clearMarks()
	p.prefix = ""
	p.drives = result.directory.Drives
	if !p.drives {
		p.path = result.directory.Path
	}
	p.items = make([]entry, len(result.directory.Entries))
	p.selected = -1
	if len(p.items) > 0 {
		p.selected = 0
		if result.request.refresh {
			p.selected = max(0, min(previousRow, len(p.items)-1))
		}
	}
	for i, file := range result.directory.Entries {
		name, size := file.Name, "?"
		if file.Directory {
			size = "—"
		} else if file.SizeKnown {
			size = formatSize(file.Bytes)
		}
		p.items[i] = entry{name: name, size: size, file: file}
		if result.request.refresh && previousRow >= 0 {
			sameItem := file.Name == previousItem.Name && file.Parent == previousItem.Parent
			if p.drives {
				sameItem = previousItem.Path != "" && samePath(file.Path, previousItem.Path)
			}
			if sameItem {
				p.selected = i
			}
		}
		if file.Directory && (file.Name == result.request.selectName ||
			file.Path != "" && strings.EqualFold(file.Path, result.request.selectName)) {
			p.selected = i
		}
	}
	p.rows = make(map[int]*widget.Clickable)
	if result.request.refresh {
		// Keep the viewport unless the current item moved or disappeared.
		p.reveal = p.reveal || p.selected != previousRow
	} else {
		p.list = widget.List{List: layout.List{Axis: layout.Vertical}}
		p.reveal = p.selected >= 0
	}
}

func (v *view) activateSelected() {
	p := &v.panels[v.active]
	if p.loading || p.selected < 0 || p.selected >= len(p.items) {
		return
	}
	file := p.items[p.selected].file
	if !file.Directory {
		if open := v.openFile; open != nil && file.Name != "" {
			path := filepath.Join(p.path, file.Name)
			go func() { _ = open(path) }()
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
	if target < 0 || target >= len(v.panels) || v.loader == nil || v.panels[target].loading {
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
	v.load(target, path, "")
	p.prefix, v.panels[target].prefix = "", ""
	v.active = target
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
