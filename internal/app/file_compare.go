package app

import (
	"path/filepath"

	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
)

func compareReady(v *view) bool {
	p := &v.panels[v.active]
	return v.loader != nil && v.canOfferInteraction(v.active, actionMutation) && p.nativeFilePath(p.selected) != ""
}

func (v *view) compareSelectedFile() {
	if !v.interaction.idle() || !compareReady(v) {
		return
	}
	panel := v.active
	source := v.panels[panel].nativeFilePath(v.panels[panel].selected)
	rules := v.settingsValues.ProgramRules(settings.CompareCollection)
	match := settings.MatchProgram(rules, source)
	if match < 0 {
		v.showFeedback(panel, feedback.Warning, "No matching compare program")
		return
	}
	rule := rules[match]
	v.chooseFileTarget("Compare", func(target, row int) {
		args, err := settings.CompareArguments(rule.Arguments, source, v.panels[target].nativeFilePath(row))
		if err != nil {
			v.showFeedback(panel, feedback.Error, "Invalid compare arguments")
			return
		}
		launch := settings.EditorLaunch{Program: rule.Program, Arguments: args}
		if v.loader.submit(loadRequest{panel: panel, path: filepath.Dir(source), editor: &editorRequest{launches: []settings.EditorLaunch{launch}, compare: true}}) {
			v.panels[panel].loading = true
		}
	})
}
