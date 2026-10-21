package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/diagnostics"
	"github.com/dannyben/commando/internal/feedback"
)

func newLocationView(locations []panelState, active int, invalidate func(), dataFolder ...string) *view {
	panels := make([]panel, len(locations))
	for i, location := range locations {
		panels[i].path, panels[i].drives, panels[i].selected = location.Path, location.Drives, -1
		if !location.Drives {
			panels[i].archiveFolder = location.ArchiveFolder
		}
	}
	v := newView(panels)
	if len(dataFolder) > 0 {
		v.activeDataFolder = dataFolder[0]
	}
	if active >= 0 && active < len(panels) {
		v.active = active
	}
	v.openFile = browser.OpenFile
	v.invalidate = invalidate
	v.loader = newDirectoryLoader(browser.ReadLocation, invalidate, len(panels))
	v.loader.readFileIcon = browser.NewFileIconReader()
	v.loader.readFileTypeIcon = browser.NewFileTypeIconReader()
	if v.activeDataFolder != "" {
		v.loader.submit(loadRequest{path: v.activeDataFolder, folderCacheLoad: true, action: actionBrowse})
	}
	for i, location := range locations {
		v.panels[i].metadata = metadataFromState(location.Metadata)
		v.panels[i].lastMetadata = metadataFromState(location.LastMetadata)
		v.panels[i].sortByMetadata = location.SortByMetadata
		v.panels[i].sizeRatio = panelSizeRatio(location.SizeRatio)
		v.panels[i].visibility = visibilityFromState(location.Visibility)
		if location.Drives {
			v.load(i, "", location.Path)
		} else {
			v.queueLoad(loadRequest{panel: i, path: location.Path, archiveFolder: location.ArchiveFolder})
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
	v.queueLoad(loadRequest{panel: v.active, path: path, archiveFolder: p.archiveFolder, refresh: true})
}

func (v *view) queueLoad(request loadRequest) bool {
	request.action = actionBrowse
	if request.path == "" {
		request.homeDirectory = v.favoritesDirectory()
	}
	if v.interaction.protectsListing(request.panel) || v.pendingNameDraft != nil && v.pendingNameDraft.request.panel == request.panel {
		return false
	}
	p := &v.panels[request.panel]
	if v.loader == nil || p.loading {
		return false
	}
	if !v.loader.submit(request) {
		return false
	}
	p.loading, p.err = true, nil
	p.navigating = !request.refresh
	progress := "Reading folder"
	if request.shortcut {
		progress = "Opening shortcut"
	}
	p.feedback.ProgressAt(progress, time.Now().Add(v.appearance.FeedbackProgressDelay))
	if !request.refresh && !request.shortcut && !request.follow && v.sounds != nil {
		v.announceInteraction(eventDirectoryNavigation)
	}
	return true
}

func (v *view) pollLoads() {
	defer v.beginSoundAction()() // Simultaneous completions announce the strongest result.
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
	defer v.beginSoundAction()()
	if result.request.driveSpace != nil {
		v.acceptDriveSpace(result)
		return
	}
	if result.request.folderCacheLoad {
		v.applyFolderSizes(result.folderSizes)
		return
	}
	if result.request.size != nil {
		v.acceptCalculateSize(result)
		return
	}
	if result.progress != "" {
		v.operationProgress(result.request.panel, result.progress)
		return
	}
	if v.loader != nil {
		defer v.loader.complete(result.request)
	}
	if result.request.multiRename != nil || result.request.rename != nil || result.request.removal != nil || result.request.transfer != nil || result.request.duplicate != nil {
		v.refreshHomePanels()
	}
	if result.request.homeDirectory != "" && result.err == nil {
		v.rememberFavorites(result.directory)
	}
	if result.request.favoriteRead != nil {
		v.acceptFavoriteRead(result)
		return
	}
	if result.request.favoriteWrite != nil {
		v.acceptFavoriteWrite(result)
		return
	}
	if len(result.request.icons) > 0 {
		v.acceptFileIcons(result)
		return
	}
	if result.request.completion != nil {
		v.acceptLocationCompletion(result)
		return
	}
	if s := result.request.themeLibrary; s != nil {
		v.acceptThemeLibrary(s, result.themes, result.err)
		return
	}
	if result.request.fontFamilies {
		v.fontFamiliesLoading = false
		v.fontFamilies = append([]string{}, result.fontFamilies...)
		if s := v.settingsInteraction(); s != nil && len(s.fields) > 0 && s.fields[0].Key == settingFontFamily {
			s.completion.candidates = v.fontFamilyChoices()
			s.completion.update(&s.editor)
		}
		return
	}
	if result.request.archiveOpen != nil {
		v.acceptArchiveOpen(result)
		return
	}
	if result.request.unpack != nil {
		v.acceptUnpack(result)
		return
	}
	if result.request.terminal != nil {
		v.acceptTerminalLaunch(result)
		return
	}
	if result.request.editor != nil {
		v.acceptEditorLaunch(result)
		return
	}
	if result.request.settingsSave != nil {
		request := result.request.settingsSave
		if request.complete != nil {
			request.complete(result.settingsMessage, result.err)
		} else {
			v.acceptSettingsSave(request.session, result.settingsMessage, result.err)
		}
		return
	}
	if result.request.autoRefresh {
		v.acceptAutoRefresh(result)
		return
	}
	if result.request.multiRename != nil {
		v.acceptMultiRename(result)
		return
	}
	if result.request.clipboard != nil || result.request.transfer != nil {
		v.acceptFileOperation(result)
		return
	}
	if result.request.removal != nil {
		v.acceptRemoval(result)
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
			v.openPath(result.request.panel, result.request.path)
			return
		}
		v.announceInteraction(eventDirectoryNavigation)
	}
	p.loading, p.err = false, result.err
	p.navigating = false
	if result.request.follow && os.IsNotExist(result.err) {
		p.err = nil
		p.feedback.Complete()
		return
	}
	if result.err != nil {
		v.showFeedback(result.request.panel, feedback.Error, readError(result.err))
		return
	}
	p.feedback.Complete()
	refresh := result.request.refresh && p.sameLocation(result.directory)
	if !p.sameLocation(result.directory) {
		p.refreshNeeded = false
	}
	if result.request.refresh && v.monitor != nil && result.directory.ArchiveFolder == "" {
		path := result.directory.Path
		if result.directory.Drives {
			path = result.request.homeDirectory
		}
		if path != "" {
			v.monitor.retry(path)
		}
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
	p.archiveFolder = result.directory.ArchiveFolder
	if !p.drives {
		p.path = result.directory.Path
	}
	p.entries = slices.Clone(result.directory.Entries)
	p.prepareDriveSpaces()
	files := p.visibleEntries()
	p.items = make([]entry, len(files))
	p.selected = -1
	if len(p.items) > 0 {
		p.selected = 0
		if !refresh && len(files) > 1 && files[0].Parent {
			p.selected = 1
		}
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
		if file.Name == result.request.selectName ||
			result.request.explicitPath && samePath(file.Name, result.request.selectName) ||
			file.Path != "" && strings.EqualFold(file.Path, result.request.selectName) {
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
	} else if result.request.successMessage != "" {
		v.showFeedback(result.request.panel, feedback.Info, result.request.successMessage)
	}
	if result.request.fileDraft != nil {
		v.pendingNameDraft = &result
	}
}

func (v *view) activateSelected() {
	p := &v.panels[v.active]
	if p.loading || p.selected < 0 || p.selected >= len(p.items) {
		return
	}
	file := p.items[p.selected].file
	if p.archiveFolder != "" {
		if file.Directory {
			location, selected := p.selectedLocation()
			v.loadLocation(v.active, location, selected)
		} else {
			v.openArchiveSelected()
		}
		return
	}
	if !file.Directory {
		if v.settingsSelecting() {
			return
		}
		if file.Name != "" {
			path := filepath.Join(p.path, file.Name)
			if file.Favorite {
				path = file.Path
			}
			if browser.ArchiveFormat(path) != nil {
				v.loadLocation(v.active, browser.Location{Path: path, Folder: "."}, "")
			} else if strings.EqualFold(filepath.Ext(path), ".lnk") && v.loader != nil {
				v.queueLoad(loadRequest{panel: v.active, path: path, shortcut: true})
			} else {
				v.openPath(v.active, path)
			}
		}
		return
	}
	if file.Path != "" {
		v.load(v.active, file.Path, "")
	} else if file.Parent {
		v.parentFolder()
	} else {
		if v.queueLoad(loadRequest{panel: v.active, path: filepath.Join(p.path, file.Name)}) {
			v.followNavigation(file.Name, false)
		}
	}
}

func (v *view) parentFolder() {
	p := &v.panels[v.active]
	if p.loading || p.drives {
		return
	}
	if p.archiveFolder != "" {
		parent, selected := p.location().Parent()
		v.loadLocation(v.active, parent, selected)
		return
	}
	parent := filepath.Dir(p.path)
	if parent == p.path {
		if v.followMe {
			return
		}
		v.showDrives()
		return
	}
	if v.queueLoad(loadRequest{panel: v.active, path: parent, selectName: filepath.Base(p.path)}) {
		v.followNavigation(filepath.Base(p.path), true)
	}
}

func (v *view) showDrives() {
	if !folderReady(v) {
		return
	}
	root := filepath.VolumeName(v.panels[v.active].path) + string(filepath.Separator)
	v.load(v.active, "", root)
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
	location, selectName := p.selectedLocation()
	if target < 0 || target >= len(v.panels) {
		if !v.addPanel(direction) {
			return
		}
	} else {
		v.switchPanel(direction)
	}
	v.loadLocation(v.active, location, selectName)
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
	now := time.Now()
	if message := p.feedback.Current(); message != nil {
		return message.Text, message.Kind == feedback.Error
	}
	if p.drives {
		if p.selected >= 0 && p.selected < len(p.items) {
			if space, ok := p.driveSpaceFor(p.items[p.selected].file); ok {
				return formatSize(space.Total-space.Free) + " used", false
			}
		}
		return "", false
	}
	if len(p.marks) > 0 {
		var total int64
		var files int64
		approximate := false
		for row := range p.marks {
			if row < 0 || row >= len(p.items) {
				return "", false
			}
			file := p.items[row].file
			size, known := p.entrySize(file, now)
			if !known {
				return "", false
			}
			total += size
			if file.Directory {
				record, _ := p.cachedFolderSize(file, now)
				files += record.Files
				approximate = true
			} else {
				files++
			}
		}
		if approximate {
			return "≈" + sizeResultLabel(browser.SizeResult{Bytes: total, Files: files}), false
		}
		return formatSize(total), false
	}
	if p.selected < 0 || p.selected >= len(p.items) {
		return "", false
	}
	item := p.items[p.selected]
	if record, ok := p.cachedFolderSize(item.file, now); ok {
		if p.metadata == metadataSize {
			return "≈" + fileCountLabel(record.Files), false
		}
		return "≈" + sizeResultLabel(browser.SizeResult{Bytes: record.Bytes, Files: record.Files}), false
	}
	if item.file.Directory || item.file.Parent || item.file.Path != "" {
		return "", false
	}
	size, date := "", ""
	if p.metadata != metadataSize && item.file.SizeKnown {
		size = formatSize(item.file.Bytes)
	}
	if p.metadata != metadataModified && !item.file.Modified.IsZero() {
		date = metadataModified.value(item)
	}
	if size != "" && date != "" {
		return size + " · " + date, false
	}
	return size + date, false
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
		return "Home"
	}
	return p.location().String()
}

func (p *panel) locationPath() string {
	if p.drives && p.selected >= 0 && p.selected < len(p.items) {
		return p.items[p.selected].file.Path
	}
	return p.location().String()
}

func (v *view) openPath(index int, path string) {
	if open := v.openFile; open != nil {
		post := v.backgroundFeedback(index)
		go func() {
			id := diagnostics.NextID()
			diagnostics.Record(diagnostics.Shell, id, "OpenRequested", map[string]any{"path": path})
			err := open(path)
			diagnostics.Record(diagnostics.Shell, id, "OpenCompleted", map[string]any{"error": diagnostics.Error(err)})
			if err != nil {
				post(feedback.Error, "Failed to open")
			}
		}()
	}
}
