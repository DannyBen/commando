package app

import (
	"errors"
	"image"
	"io/fs"
	"path/filepath"
	"time"

	"gioui.org/layout"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/windowshell"
)

// Native sessions hold the shared foreground and operation policies until the
// native loop returns. Their completion cannot finish a newer interaction.
type nativeSession struct {
	panel      int
	request    menuRequest
	dispatched bool
}

func (s *nativeSession) interactionPanel() int { return s.panel }
func (*nativeSession) holdsOperations() bool   { return true }
func (*nativeSession) blocksPanels() bool      { return true }

type menuRequest struct {
	paths   []string
	options windowshell.MenuOptions
	queued  time.Time
}

type menuCompletion struct {
	session *nativeSession
	result  windowshell.Result
	queue   time.Duration
	err     error
}

func (v *view) contextMenuAvailable(index, row int, mouse bool) bool {
	if v.menuExecutor == nil || !v.canOfferInteraction(index, actionMutation) {
		return false
	}
	p := &v.panels[index]
	if p.path == "" || p.drives {
		return false
	}
	if mouse {
		return p.markable(row)
	}
	return len(p.selectedPaths()) > 0
}

// Mouse, keyboard and command search join here. Mouse invocation targets only
// the clicked item; keyboard invocation uses the shared marked selection.
func (v *view) startContextMenu(gtx layout.Context, index, row int, mouse bool) {
	if !v.canStartInteraction(index, actionMutation) || !v.contextMenuAvailable(index, row, mouse) {
		return
	}
	p := &v.panels[index]
	paths := p.selectedPaths()
	if mouse {
		paths = []string{filepath.Join(p.path, p.items[row].file.Name)}
	}
	if len(paths) == 0 {
		return
	}
	s := &nativeSession{panel: index, request: menuRequest{paths: paths, queued: time.Now()}}
	if !mouse {
		s.request.options.ClientPoint = new(image.Point)
	}
	if !v.interaction.begin(s) {
		return
	}
	v.dismissFeedback()
	if mouse {
		v.panels[v.active].prefix = ""
		v.active = index
		p.selected, p.markRange = row, nil
	}
	p.prefix, p.reveal = "", true
	v.focusInteraction(gtx)
}

// Dispatch after submitting the frame. Window.Run can enter a nested shell
// message loop; the Gio event consumer must remain available throughout it.
func (v *view) dispatchNative(invalidate func()) {
	s, ok := v.interaction.current.(*nativeSession)
	if !ok || s.dispatched {
		return
	}
	s.dispatched = true
	request := s.request
	if request.options.ClientPoint != nil {
		anchor := v.panels[s.panel].selectionAnchor
		request.options.ClientPoint = &anchor
	}
	execute, results := v.menuExecutor, v.menuResults
	go func() {
		completion := execute(request)
		completion.session = s
		results <- completion
		invalidate()
	}()
}

func (v *view) completeNative(completion menuCompletion) bool {
	if !v.interaction.finish(completion.session) {
		return false
	}
	if v.menuTrace != nil {
		v.menuTrace(completion)
	}
	if completion.err != nil {
		message := "Cannot open context menu"
		switch {
		case errors.Is(completion.err, fs.ErrPermission):
			message = "Access denied"
		case errors.Is(completion.err, fs.ErrNotExist):
			message = "Item not found"
		}
		v.showFeedback(completion.session.panel, feedback.Error, message)
	}
	if !completion.result.Cancelled {
		// Shell verbs and extensions can change any open folder. Quiet reads
		// retain marks, identity, filtering and viewport through normal refresh.
		for i := range v.panels {
			if !v.panels[i].drives {
				v.panels[i].refreshNeeded = true
			}
		}
	}
	return true
}

func (v *view) pollNative(gtx layout.Context) {
	select {
	case result := <-v.menuResults:
		if v.completeNative(result) {
			v.focusInteraction(gtx)
		}
	default:
	}
}

func (v *view) finishNative() {
	if s, ok := v.interaction.current.(*nativeSession); ok && s.dispatched {
		v.completeNative(<-v.menuResults)
	}
}
