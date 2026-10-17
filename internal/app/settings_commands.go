package app

import (
	"errors"

	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
)

// Direct settings commands share live application and worker persistence without
// opening a widget. The worker's mutation policy protects the saved snapshot.
func (v *view) toggleBooleanSetting(field string) {
	if !v.canStartInteraction(v.active, actionMutation) {
		return
	}
	before := v.settingsValues.Clone()
	enabled := !before.Get(settings.CatalogueDefault, field).Bool
	if err := v.settingsValues.Set(settings.CatalogueDefault, field, settings.Value{Bool: enabled}); err != nil {
		return
	}
	v.applySettings()
	if !v.previewSoundSetting(field) {
		v.announceInteraction(eventValueChanged)
	}
	if v.prepareSettingsSave == nil {
		return // Previews do not persist settings.
	}
	panel := v.active
	request := &settingsSaveRequest{run: v.prepareSettingsSave(v.settingsValues.Clone()),
		complete: func(message string, err error) {
			v.directSettingsSave = nil
			if err != nil {
				v.settingsValues = before
				v.applySettings()
				v.showFeedback(panel, feedback.Error, settingsSaveError(err))
			} else if message != "" {
				v.showFeedback(panel, feedback.Info, message)
			}
		}}
	v.directSettingsSave = request
	if v.loader != nil {
		if !v.loader.submit(loadRequest{panel: panel, settingsSave: request}) {
			request.complete("", errors.New("worker unavailable"))
		}
		return
	}
	message, err := request.run()
	request.complete(message, err)
}
