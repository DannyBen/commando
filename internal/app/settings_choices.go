package app

import (
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

// Choice rows retain stable targets across layout and live theme previews.
// Settings values, focus and transaction policy remain owned by the session.
type settingsChoiceRow struct{ buttons []clickTarget }

func (s *settingsSession) choiceRow(key string, count int) *settingsChoiceRow {
	if s.choices == nil {
		s.choices = make(map[string]*settingsChoiceRow)
	}
	row := s.choices[key]
	if row == nil || len(row.buttons) != count {
		row = &settingsChoiceRow{buttons: make([]clickTarget, count)}
		s.choices[key] = row
	}
	return row
}

func (v *view) updateSettingsChoices(gtx layout.Context, s *settingsSession) {
	if s.deleted {
		gtx = gtx.Disabled()
	}
	for fieldIndex, field := range s.fields {
		row := s.choices[field.Key]
		if row == nil {
			continue
		}
		for choice := range row.buttons {
			if !row.buttons[choice].Clicked(gtx) {
				continue
			}
			moved := s.selected != fieldIndex || s.actionsFocused
			if !v.focusSettingsField(gtx, s, fieldIndex) {
				return
			}
			d, _ := settings.CatalogueDefault.Definition(field.Key)
			value := settings.Value{}
			switch {
			case field.Control == "toggle":
				value.Bool = choice == 0
			case len(d.ChoiceValues) > 0:
				value.Number = d.ChoiceValues[choice]
			default:
				value.Text = d.Choices[choice]
			}
			if value != v.widgetValues().Get(settings.CatalogueDefault, field.Key) {
				v.previewSettingValue(field.Key, value)
			} else if moved {
				v.announceInteraction(eventSelectionChanged)
			}
			v.focusInteraction(gtx)
			return
		}
	}
}
