package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
)

type editorRequest struct {
	launches  []settings.EditorLaunch
	unmatched int
}

func (p *panel) selectedEditorFiles() []string {
	if !p.nativeFolder() {
		return nil
	}
	var files []string
	marked := len(p.marks) > 0
	for i, item := range p.items {
		if marked && !p.marks[i] || !marked && i != p.selected {
			continue
		}
		if item.file.Parent || item.file.Directory || item.file.Path != "" {
			continue
		}
		files = append(files, filepath.Join(p.path, item.file.Name))
	}
	return files
}
func editorReady(v *view) bool {
	return folderReady(v) && v.loader != nil && v.canOfferInteraction(v.active, actionMutation) && len(v.panels[v.active].selectedEditorFiles()) > 0
}
func (v *view) editSelectedFiles() {
	if !v.interaction.idle() || !editorReady(v) {
		return
	}
	p := &v.panels[v.active]
	launches, unmatched, err := settings.EditorLaunches(v.settingsValues.EditorRules(), p.selectedEditorFiles())
	if err != nil {
		v.showFeedback(v.active, feedback.Error, "Invalid editor arguments")
		return
	}
	if len(launches) == 0 {
		v.showFeedback(v.active, feedback.Warning, "No matching editor")
		return
	}
	request := loadRequest{panel: v.active, path: p.path, editor: &editorRequest{launches, unmatched}}
	if v.loader.submit(request) {
		p.loading = true
	}
}
func (v *view) acceptEditorLaunch(result loadResult) {
	p := &v.panels[result.request.panel]
	p.loading = false
	switch {
	case result.err != nil:
		message := "Cannot start editor"
		if result.editorsStarted > 0 {
			message = "Some editors failed to start"
		} else if errors.Is(result.err, os.ErrPermission) {
			message = "Editor access denied"
		}
		v.showFeedback(result.request.panel, feedback.Error, message)
	case result.request.editor.unmatched > 0:
		v.showFeedback(result.request.panel, feedback.Warning, fmt.Sprintf("%d files lack editor", result.request.editor.unmatched))
	default:
		v.showFeedback(result.request.panel, feedback.Success, "Files sent to editor")
	}
}
