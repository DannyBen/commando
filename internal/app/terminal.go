package app

import (
	"errors"
	"os"

	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

type terminalRequest struct {
	admin bool
	owner uintptr
}

func terminalReady(v *view) bool {
	return folderReady(v) && v.panels[v.active].nativeFolder() && v.loader != nil && v.canOfferInteraction(v.active, actionMutation)
}

func (v *view) openTerminal(admin bool) {
	if !v.interaction.idle() || !terminalReady(v) {
		return
	}
	p := &v.panels[v.active]
	request := loadRequest{panel: v.active, path: p.path, terminal: &terminalRequest{admin, v.ownerWindow}}
	if v.loader.submit(request) {
		p.loading = true
	}
}

func (v *view) acceptTerminalLaunch(result loadResult) {
	v.panels[result.request.panel].loading = false
	if result.err == nil || errors.Is(result.err, browser.ErrTerminalCancelled) {
		return
	}
	message := "Cannot open terminal"
	if errors.Is(result.err, os.ErrPermission) {
		message = "Terminal access denied"
	} else if errors.Is(result.err, browser.ErrTerminalUnavailable) {
		message = "Terminal requires Windows"
	} else if errors.Is(result.err, browser.ErrTerminalPathExpansion) {
		message = "Terminal path contains %"
	} else if errors.Is(result.err, os.ErrNotExist) {
		message = "Terminal path not found"
	}
	v.showFeedback(result.request.panel, feedback.Error, message)
}
