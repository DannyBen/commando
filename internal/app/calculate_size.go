package app

import (
	"context"
	"errors"
	"io/fs"
	"strconv"

	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

type sizeSession struct {
	panel  int
	paths  []string
	ctx    context.Context
	cancel context.CancelFunc
}

func (s *sizeSession) interactionPanel() int { return s.panel }
func (*sizeSession) holdsOperations() bool   { return true }
func (*sizeSession) blocksPanels() bool      { return true }

func calculateSizeReady(v *view) bool {
	p := &v.panels[v.active]
	return v.loader != nil && p.nativeFolder() && !p.refreshing &&
		v.canOfferInteraction(v.active, actionMutation) && len(p.selectedPaths()) > 0
}

func (v *view) startCalculateSize(gtx layout.Context) {
	if !v.interaction.idle() || !calculateSizeReady(v) {
		return
	}
	p := &v.panels[v.active]
	ctx, cancel := context.WithCancel(context.Background())
	s := &sizeSession{panel: v.active, paths: p.selectedPaths(), ctx: ctx, cancel: cancel}
	if !v.startInteraction(s) {
		cancel()
		return
	}
	if !v.loader.submit(loadRequest{panel: s.panel, path: p.path, size: s, action: actionBrowse}) {
		v.interaction.finish(s)
		cancel()
		return
	}
	p.prefix = ""
	v.operationProgress(s.panel, "Calculating size…")
	v.focusInteraction(gtx)
}

func (v *view) acceptCalculateSize(result loadResult) {
	s := result.request.size
	defer s.cancel()
	if !v.interaction.finish(s) {
		return
	}
	p := &v.panels[s.panel]
	p.feedback.Complete()
	v.interaction.focusPending = true
	if errors.Is(result.err, context.Canceled) || s.ctx.Err() != nil {
		v.showFeedback(s.panel, feedback.Info, "Calculation cancelled")
		p.feedback.Acknowledge()
		v.announceInteraction(eventCancelled)
	} else if result.err != nil {
		message := "Cannot calculate size"
		switch {
		case errors.Is(result.err, fs.ErrPermission):
			message = "Access denied"
		case errors.Is(result.err, fs.ErrNotExist):
			message = "Entry not found"
		}
		v.showFeedback(s.panel, feedback.Error, message)
	} else {
		v.showFeedback(s.panel, feedback.Success, sizeResultLabel(result.size))
	}
}

func sizeResultLabel(result browser.SizeResult) string {
	count := strconv.FormatInt(result.Files, 10)
	for i := len(count) - 3; i > 0; i -= 3 {
		count = count[:i] + "," + count[i:]
	}
	word := " files · "
	if result.Files == 1 {
		word = " file · "
	}
	return count + word + formatSize(result.Bytes)
}
