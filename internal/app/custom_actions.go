package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
)

const customActionCommandPrefix = "programs.actions.run."

type customActionTarget struct {
	path      string
	directory bool
}

type customActionLaunch struct {
	arguments []string
	directory string
}

type customActionRequest struct {
	program  string
	terminal bool
	launches []customActionLaunch
}

func customActionIdentity(commandID string) (string, bool) {
	id, ok := strings.CutPrefix(commandID, customActionCommandPrefix)
	return id, ok && settings.ValidIdentity(id)
}

func (v *view) customActionCommands() []command {
	var catalogue []command
	for _, rule := range v.settingsValues.ActionRules() {
		if !settings.ValidIdentity(rule.ID) {
			continue
		}
		id := rule.ID
		catalogue = append(catalogue, v.resolveCommandBindings(command{
			id: customActionCommandPrefix + id, group: "Actions", name: rule.Name,
			available: func(v *view) bool {
				rule, ok := v.customActionRule(id)
				return ok && terminalReady(v) && len(v.panels[v.active].customActionTargets(rule)) > 0
			},
			execute: func(v *view) { v.runCustomAction(id) },
		}))
	}
	return catalogue
}

func (v *view) customActionRule(id string) (settings.ActionRule, bool) {
	for _, rule := range v.settingsValues.ActionRules() {
		if rule.ID == id {
			return rule, true
		}
	}
	return settings.ActionRule{}, false
}

// Marks take precedence over the current row, using only this panel. Folder
// actions ignore files; file actions ignore folders. Both accepts every kind.
func (p *panel) customActionTargets(rule settings.ActionRule) []customActionTarget {
	if rule.Target == settings.ActionFolders && rule.Mode == settings.ActionHere {
		return []customActionTarget{{p.path, true}}
	}
	var targets []customActionTarget
	for row, item := range p.items {
		if len(p.marks) > 0 && !p.marks[row] || len(p.marks) == 0 && row != p.selected {
			continue
		}
		file := item.file
		if file.Parent || rule.Target == settings.ActionFolders && !file.Directory || rule.Target == settings.ActionFiles && file.Directory {
			continue
		}
		if file.Path != "" || !rule.Matches(file.Name, file.Directory) {
			return nil
		}
		targets = append(targets, customActionTarget{filepath.Join(p.path, file.Name), file.Directory})
	}
	if len(targets) == 0 && rule.Target == settings.ActionFolders && rule.Mode == settings.ActionAuto {
		return []customActionTarget{{p.path, true}}
	}
	return targets
}

func (v *view) runCustomAction(id string) {
	rule, ok := v.customActionRule(id)
	if !ok || !v.interaction.idle() || !terminalReady(v) {
		return
	}
	p := &v.panels[v.active]
	targets := p.customActionTargets(rule)
	if len(targets) == 0 {
		return
	}
	paths := make([]string, len(targets))
	for i, target := range targets {
		paths[i] = target.path
	}
	arguments, err := settings.ActionArguments(rule.Arguments, paths)
	if err != nil {
		v.showFeedback(v.active, feedback.Error, "Invalid action arguments")
		return
	}
	request := &customActionRequest{program: rule.Program, terminal: rule.Terminal}
	for i, args := range arguments {
		directory := p.path
		// One-folder launches start inside that folder. Batch launches use
		// the panel's folder when they receive more than one target.
		if len(targets) == 1 && targets[0].directory {
			directory = targets[0].path
		} else if strings.Contains(rule.Arguments, settings.EntryToken) && targets[i].directory {
			directory = targets[i].path
		}
		request.launches = append(request.launches, customActionLaunch{args, directory})
	}
	if v.loader.submit(loadRequest{panel: v.active, path: p.path, customAction: request}) {
		p.loading = true
	}
}

func (v *view) acceptCustomAction(result loadResult) {
	v.panels[result.request.panel].loading = false
	if result.err == nil {
		return
	}
	message := "Cannot start action"
	switch {
	case result.actionsStarted > 0:
		message = "Some actions failed to start"
	case errors.Is(result.err, browser.ErrTerminalUnavailable):
		message = "Terminal requires Windows"
	case errors.Is(result.err, os.ErrPermission):
		message = "Action access denied"
	case errors.Is(result.err, os.ErrNotExist):
		message = "Action program not found"
	}
	v.showFeedback(result.request.panel, feedback.Error, message)
}
