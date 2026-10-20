package app

import (
	"path/filepath"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func (v *view) updateAutoRefresh(gtx layout.Context) {
	if v.loader == nil {
		return
	}
	select {
	case <-v.loader.done:
		return
	default:
	}
	var paths []string
	if v.monitor != nil {
		watched := v.fileOperationLocations()
		for i, p := range v.panels {
			if p.drives {
				watched[i] = v.favoritesDirectory()
			}
			if p.archiveFolder != "" {
				watched[i] = filepath.Dir(p.path)
			}
		}
		v.monitor.sync(watched)
		var next time.Time
		paths, next = v.monitor.take(gtx.Now)
		if !next.IsZero() {
			gtx.Execute(op.InvalidateCmd{At: next})
		}
	}
	for pi := range v.panels {
		p := &v.panels[pi]
		for _, path := range paths {
			watched := v.entryDirectory(pi)
			if p.archiveFolder != "" {
				watched = filepath.Dir(p.path)
			}
			if samePath(watched, path) {
				p.refreshNeeded = true
			}
		}
		if !p.refreshNeeded || p.drives && v.favoritesDirectory() == "" || p.loading || p.refreshing ||
			v.pendingNameDraft != nil && v.pendingNameDraft.request.panel == pi ||
			v.interaction.protectsListing(pi) || !v.actionAllowed(actionMutation) {
			continue
		}
		// This is a background read, not a mutation. Use a nonblocking enqueue so
		// automatic work never holds the UI while the worker's queue is full.
		request := loadRequest{panel: pi, path: p.path, archiveFolder: p.archiveFolder, autoRefresh: true, action: actionBrowse}
		if p.drives {
			request.path, request.homeDirectory = "", v.favoritesDirectory()
		}
		select {
		case v.loader.requests <- request:
			p.refreshing, p.refreshNeeded = true, false
		default:
		}
	}
}

func (v *view) acceptAutoRefresh(result loadResult) {
	p := &v.panels[result.request.panel]
	p.refreshing = false
	home := result.request.homeDirectory != ""
	if p.drives != home || !home && (!samePath(p.path, result.request.path) || p.archiveFolder != result.request.archiveFolder) {
		return // Navigation or a confirmed folder move changed the location.
	}
	if p.loading || v.interaction.protectsListing(result.request.panel) || !v.actionAllowed(actionMutation) {
		p.refreshNeeded = true // Reread when safe; the captured listing may be stale.
		return
	}
	if result.err != nil {
		if browser.IsMissingDirectory(result.err) {
			p.refreshNeeded = false
			if v.monitor != nil {
				v.monitor.suspend(p.path)
			}
		}
		previous := p.err
		p.err = result.err
		message := readError(result.err)
		current := p.feedback.Current()
		if previous == nil || readError(previous) != message || current == nil || current.Kind != feedback.Error || current.Text != message {
			v.showFeedback(result.request.panel, feedback.Error, message)
		}
		return
	}
	// Reuse the preservation rules used after transfers. Automatic refresh must
	// not clear operation feedback or interrupt a filename prefix being typed.
	prefix, prefixAt := p.prefix, p.prefixAt
	unchanged := p.sameLocation(result.directory)
	p.reconcileDirectoryListing(result.directory)
	if unchanged {
		p.prefix, p.prefixAt = prefix, prefixAt
	}
}
