package app

import (
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

func (s *settingsSession) valueTarget(key string) *valueTarget {
	if s.values == nil {
		s.values = make(map[string]*valueTarget)
	}
	if s.values[key] == nil {
		s.values[key] = new(valueTarget)
	}
	return s.values[key]
}

func settingsFieldSteps(field settings.Field) bool {
	return field.Control == "stepper" || field.Control == "stepper-progress" || field.Control == "autocomplete-stepper" ||
		field.Key == settingFontFamily || field.Key == settingThemeSelect || field.Key == settingThemeElement
}

// Moving between value controls uses the same draft validation as Tab.
func (v *view) focusSettingsField(gtx layout.Context, s *settingsSession, index int) bool {
	if s.editing && !v.applySettingDraft(gtx) {
		v.focusInteraction(gtx)
		return false
	}
	if v.settingsInteraction() != s {
		return false
	}
	s.selected, s.actionsFocused, s.errorText = index, false, ""
	v.focusInteraction(gtx)
	return true
}

func (v *view) updateSettingsValues(gtx layout.Context, s *settingsSession) {
	for i, field := range s.fields {
		target := s.values[field.Key]
		if target == nil {
			continue
		}
		input := gtx
		if s.deleted || s.editing && s.selected == i {
			input = gtx.Disabled()
		}
		if target.Clicked(input) {
			if !v.focusSettingsField(gtx, s, i) {
				return
			}
			v.beginSettingEdit(gtx)
			return
		}
		for {
			direction, ok := target.scroll(input)
			if !ok {
				break
			}
			if direction == 0 || s.editing || !settingsFieldSteps(field) {
				continue
			}
			if !v.focusSettingsField(gtx, s, i) {
				return
			}
			if field.Key == settingThemeElement {
				v.cycleThemeElement(gtx, direction)
				return // Switching elements can submit a save and replace the widget.
			} else {
				v.adjustSetting(field.Key, direction)
			}
			if v.settingsInteraction() != s {
				return
			}
		}
	}
}
