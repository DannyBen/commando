package app

import (
	"errors"
	"io/fs"
	"time"

	"gioui.org/layout"
	"github.com/dannyben/commando/internal/diagnostics"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/windowshell"
)

// Native sessions hold the shared foreground and operation policies until the
// native loop returns. Their completion cannot finish a newer interaction.
type nativeSession struct {
	panel      int
	request    shellRequest
	dispatched bool
	traceID    uint64
}

func (s *nativeSession) interactionPanel() int { return s.panel }
func (*nativeSession) holdsOperations() bool   { return true }
func (*nativeSession) blocksPanels() bool      { return true }

type shellAction uint8

const (
	shellMenu shellAction = iota
	shellDrag
	shellProperties
)

type shellRequest struct {
	action  shellAction
	armed   bool
	paths   []string
	options windowshell.MenuOptions
	queued  time.Time
}

type shellCompletion struct {
	session *nativeSession
	result  windowshell.Result
	queue   time.Duration
	err     error
}

// Dispatch after submitting the frame. Window.Run can enter a nested shell
// message loop; the Gio event consumer must remain available throughout it.
func (v *view) dispatchNative(invalidate func()) {
	s, ok := v.interaction.current.(*nativeSession)
	if !ok || s.dispatched {
		return
	}
	s.dispatched = true
	s.traceID = diagnostics.NextID()
	diagnostics.Record(diagnostics.Shell, s.traceID, "NativeRequested", map[string]any{"action": s.request.action, "paths": s.request.paths, "armed": s.request.armed})
	request := s.request
	if request.options.ClientPoint != nil {
		anchor := v.panels[s.panel].selectionAnchor
		request.options.ClientPoint = &anchor
	}
	execute, results := v.shellExecutor, v.shellResults
	go func() {
		completion := execute(request)
		completion.session = s
		results <- completion
		invalidate()
	}()
}

func (v *view) completeNative(completion shellCompletion) bool {
	if !v.interaction.finish(completion.session) {
		return false
	}
	traceShellCompletion(completion)
	if completion.err != nil {
		message := "Cannot open context menu"
		switch completion.session.request.action {
		case shellDrag:
			message = "Cannot drag files"
		case shellProperties:
			message = "Cannot open properties"
		}
		switch {
		case errors.Is(completion.err, fs.ErrPermission):
			message = "Access denied"
		case errors.Is(completion.err, fs.ErrNotExist):
			message = "Item not found"
		}
		v.showFeedback(completion.session.panel, feedback.Error, message)
	}
	if completion.err == nil && completion.session.request.action == shellDrag {
		panel := completion.session.panel
		switch {
		case completion.result.Cancelled:
			v.showFeedback(panel, feedback.Info, "Drag cancelled")
			if v.sounds != nil {
				v.announceInteraction(eventCancelled)
			}
			v.panels[panel].feedback.Acknowledge()
		case completion.result.Effect&1 != 0:
			v.showFeedback(panel, feedback.Success, "Drop accepted")
		default:
			v.showFeedback(panel, feedback.Info, "Drop not accepted")
		}
	}
	if !completion.result.Cancelled {
		// Shell verbs and extensions can change any open folder. Quiet reads
		// retain marks, identity, filtering and viewport through normal refresh.
		for i := range v.panels {
			v.panels[i].refreshNeeded = true
		}
	}
	return true
}

func (v *view) pollNative(gtx layout.Context) {
	select {
	case result := <-v.shellResults:
		if v.completeNative(result) {
			v.focusInteraction(gtx)
		}
	default:
	}
}

func (v *view) finishNative() {
	if s, ok := v.interaction.current.(*nativeSession); ok && s.dispatched {
		v.completeNative(<-v.shellResults)
	}
}

func (v *view) nativeItemsAvailable(index, row int, mouse bool, action shellAction) bool {
	if v.shellExecutor == nil || !v.canOfferInteraction(index, actionMutation) {
		return false
	}
	p := &v.panels[index]
	if p.drives {
		if mouse && p.markable(row) || !mouse && len(p.selectedPaths()) > 0 {
			return true
		}
		return action == shellProperties && row >= 0 && row < len(p.items) && p.items[row].file.Path != ""
	}
	if p.path == "" || !p.nativeFolder() {
		return false
	}
	if mouse {
		return p.markable(row)
	}
	return len(p.selectedPaths()) > 0
}
