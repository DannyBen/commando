package app

import (
	"math"
	"math/rand/v2"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/settings"
)

// Stepper commands run only while browsing values. Native editor keys and the
// action bar retain ownership while typing or choosing Save/Cancel/Reset.
func (v *view) settingsStepperKey(field settings.Field, ke key.Event) bool {
	if field.Control != "stepper" && field.Control != "stepper-progress" {
		return false
	}
	if ke.Modifiers == key.ModCtrl {
		switch ke.Name {
		case key.NameLeftArrow:
			v.adjustSetting(field.Key, -10)
		case key.NameRightArrow:
			v.adjustSetting(field.Key, 10)
		default:
			return false
		}
		return true
	}
	if ke.Modifiers != 0 {
		return false
	}
	d, _ := settings.CatalogueDefault.Definition(field.Key)
	value := v.widgetValues().Get(settings.CatalogueDefault, field.Key)
	switch ke.Name {
	case key.NamePageDown:
		v.adjustSetting(field.Key, 25)
		return true
	case key.NamePageUp:
		v.adjustSetting(field.Key, -25)
		return true
	case key.NameHome:
		if math.IsInf(d.Minimum, -1) {
			return true
		}
		value.Number = d.Minimum
	case key.NameEnd:
		switch {
		case field.Key == settingThemeGenerator+".seed":
			previous := value.Number
			for value.Number == previous {
				value.Number = float64(rand.IntN(999999) + 1)
			}
		case d.Cycle > 0:
			// The cycle endpoint aliases zero. Pick the last distinct step.
			value.Number = (math.Ceil(d.Cycle/d.Step) - 1) * d.Step
		case d.Maximum != nil:
			value.Number = *d.Maximum
		default:
			return true
		}
	default:
		return false
	}
	v.previewSettingValue(field.Key, value)
	return true
}
