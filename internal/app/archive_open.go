package app

import "github.com/dannyben/commando/internal/feedback"

type archiveOpenRequest struct {
	names []string
	open  func(string) error
}

func (v *view) openArchiveSelected() {
	if !openReady(v) || !v.canStartInteraction(v.active, actionMutation) || v.openFile == nil {
		return
	}
	p := &v.panels[v.active]
	if p.archiveFolder == "" || p.items[p.selected].file.Directory || v.settingsSelecting() {
		return
	}
	v.openArchiveFiles([]string{p.items[p.selected].file.Name})
}

func (v *view) openArchiveFiles(names []string) {
	if len(names) == 0 || !openReady(v) || !v.canStartInteraction(v.active, actionMutation) || v.openFile == nil || v.settingsSelecting() {
		return
	}
	p := &v.panels[v.active]
	request := loadRequest{panel: v.active, path: p.path, archiveFolder: p.archiveFolder,
		archiveOpen: &archiveOpenRequest{names: names, open: v.openFile}}
	if v.loader.submit(request) {
		p.loading = true
		v.operationProgress(v.active, "Opening archive file")
	}
}

func (v *view) acceptArchiveOpen(result loadResult) {
	p := &v.panels[result.request.panel]
	p.loading = false
	p.feedback.Complete()
	if result.err != nil {
		v.showFeedback(result.request.panel, feedback.Error, "Failed to open")
	}
}
