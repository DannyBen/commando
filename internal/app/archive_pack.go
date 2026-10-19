package app

import (
	"path/filepath"
	"strings"

	"github.com/dannyben/commando/internal/browser"
)

func packReady(v *view) bool { return selectedMutationReady(v) }

func (v *view) startPack() {
	if !v.interaction.idle() || !packReady(v) {
		return
	}
	source := v.active
	p := &v.panels[source]
	paths := p.selectedPaths()
	name := filepath.Base(p.path)
	if len(paths) == 1 {
		name = filepath.Base(paths[0])
		for _, item := range p.items {
			if item.file.Name == name && !item.file.Directory {
				if ext := filepath.Ext(name); ext != name {
					name = strings.TrimSuffix(name, ext)
				}
				break
			}
		}
	}
	if browser.ValidateName(name) != nil {
		name = "Archive"
	}
	name += ".zip"
	target := source + 1
	if target >= len(v.panels) {
		target = max(0, source-1)
	}
	p.prefix = ""
	v.choosePanelDestination(source, target, -1, "Pack here", func(target int) string {
		if !v.panels[target].nativeFolder() {
			return "Select a filesystem folder"
		}
		return v.panelTransferError(target, false)
	}, func(target int) {
		v.transferPending = true
		v.pendingNameDraft = &loadResult{request: loadRequest{panel: target, fileDraft: &fileDraftRequest{name: name, packPaths: paths}}}
	})
}
