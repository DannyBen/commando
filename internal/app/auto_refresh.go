package app

import (
	"gioui.org/layout"
	"gioui.org/op"
	"github.com/dannyben/commando/internal/feedback"
)

func (v *view) updateAutoRefresh(gtx layout.Context) {
	if v.monitor == nil || v.loader == nil {
		return
	}
	v.monitor.sync(v.fileOperationLocations())
	paths, next := v.monitor.take(gtx.Now)
	if !next.IsZero() {
		gtx.Execute(op.InvalidateCmd{At: next})
	}
	for pi := range v.panels {
		p := &v.panels[pi]
		for _, path := range paths {
			if !p.drives && samePath(p.path, path) {
				p.refreshNeeded = true
			}
		}
		if !p.refreshNeeded || p.drives || p.loading || p.refreshing ||
			v.interaction.protectsListing(pi) || !v.actionAllowed(actionMutation) {
			continue
		}
		// This is a background read, not a mutation. Use a nonblocking enqueue so
		// automatic work never holds the UI while the worker's queue is full.
		select {
		case v.loader.requests <- loadRequest{panel: pi, path: p.path, autoRefresh: true, action: actionBrowse}:
			p.refreshing, p.refreshNeeded = true, false
		default:
		}
	}
}

func (v *view) acceptAutoRefresh(result loadResult) {
	p := &v.panels[result.request.panel]
	p.refreshing = false
	if p.drives || !samePath(p.path, result.request.path) {
		return // Navigation or a confirmed folder move changed the location.
	}
	if p.loading || v.interaction.protectsListing(result.request.panel) || !v.actionAllowed(actionMutation) {
		p.refreshNeeded = true // Reread when safe; the captured listing may be stale.
		return
	}
	if result.err != nil {
		p.err = result.err
		v.showFeedback(result.request.panel, feedback.Error, readError(result.err))
		return
	}
	// Reuse the preservation rules used after transfers. Automatic refresh must
	// not clear operation feedback or interrupt a filename prefix being typed.
	prefix, prefixAt := p.prefix, p.prefixAt
	p.reconcileTransferListing(result.directory)
	p.prefix, p.prefixAt = prefix, prefixAt
}
