package app

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"gioui.org/io/clipboard"
	"gioui.org/io/key"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/settings"
)

const (
	themeSettingPrefix    = "appearance.theme.colors."
	settingThemeCustomize = "appearance.theme.customize"
	settingThemeElement   = themeSettingPrefix + "element"
)

func themeElements() []settings.Widget {
	var elements []settings.Widget
	for _, spec := range settings.CatalogueDefault.UI.Widgets {
		if isThemeColorWidget(spec.Key) {
			elements = append(elements, spec)
		}
	}
	return elements
}

func isThemeColorWidget(key string) bool {
	return strings.HasPrefix(key, themeSettingPrefix) && key != settingThemeGenerator
}

func themeElementNames() []string {
	var names []string
	for _, spec := range themeElements() {
		names = append(names, spec.Label)
	}
	return names
}

func isThemeWidget(s *settingsSession) bool {
	return s != nil && len(s.fields) > 0 && s.fields[0].Key == settingThemeElement
}

func themeElementKey(s *settingsSession) string {
	if !isThemeWidget(s) {
		return ""
	}
	return strings.TrimSuffix(s.fields[1].Key, ".lightness")
}

func themeElementName(s *settingsSession) string {
	spec, _ := settings.CatalogueDefault.Widget(themeElementKey(s))
	return spec.Label
}

func (s *settingsSession) completesField() bool {
	return s.fields[s.selected].Key == settingFontFamily || s.fields[s.selected].Key == settingThemeElement || s.fields[s.selected].Key == settingThemeSelect
}

func (v *view) cycleThemeElement(gtx layout.Context, direction int) {
	s := v.settingsInteraction()
	elements := themeElements()
	for i, spec := range elements {
		if spec.Key == themeElementKey(s) {
			v.switchThemeElement(gtx, elements[(i+direction+len(elements))%len(elements)].Key)
			return
		}
	}
}

// Save through the normal transaction, then replace the editor before the next
// frame. A failed save keeps the original editor and its unsaved changes.
func (v *view) switchThemeElement(gtx layout.Context, element string) {
	s := v.settingsInteraction()
	if s == nil || s.pending {
		return
	}
	if themeElementKey(s) == element {
		s.editing = false
		s.errorText = ""
		v.focusInteraction(gtx)
		return
	}
	s.afterSave = func() {
		next := v.newSettingsWidget(element)
		next.palette = s.palette
		if v.interaction.begin(next) {
			v.announceInteraction(eventSelectionChanged)
		}
	}
	v.finishSettingsWidget(gtx, true)
}

func (v *view) themeClipboardText(s *settingsSession) string {
	prefix := themeElementKey(s)
	n := func(component string) float64 {
		return v.settingsValues.Get(settings.CatalogueDefault, prefix+"."+component).Number
	}
	return fmt.Sprintf("%g %g %g", n("lightness"), n("saturation"), n("hue"))
}

func (v *view) pasteThemeColor(s *settingsSession, text string) {
	parts := strings.Fields(text)
	if len(parts) != 3 {
		s.errorText = "Copy three LSH values"
		return
	}
	values := v.settingsValues.Clone()
	for i, component := range []string{"lightness", "saturation", "hue"} {
		n, err := strconv.ParseFloat(parts[i], 64)
		if err != nil || values.Set(settings.CatalogueDefault, themeElementKey(s)+"."+component, settings.Value{Number: n}) != nil {
			s.errorText = "Invalid LSH values"
			return
		}
	}
	v.settingsValues = values
	s.dirty, s.errorText = true, ""
	v.applySettings()
	v.announceInteraction(eventValueChanged)
}

func (v *view) updateThemeClipboard(gtx layout.Context, s *settingsSession) {
	for {
		e, ok := gtx.Event(transfer.TargetFilter{Target: s, Type: "application/text"})
		if !ok {
			break
		}
		if data, ok := e.(transfer.DataEvent); ok {
			r := data.Open()
			text, err := io.ReadAll(io.LimitReader(r, 1024))
			r.Close()
			if err != nil {
				s.errorText = "Could not read clipboard"
				continue
			}
			v.pasteThemeColor(s, string(text))
		}
	}
	if s.editing {
		return
	} // Text editors retain ordinary clipboard behavior.
	for {
		e, ok := gtx.Event(key.Filter{Focus: &v.focus, Name: "C", Required: key.ModCtrl}, key.Filter{Focus: &v.focus, Name: "V", Required: key.ModCtrl})
		if !ok {
			return
		}
		ke, ok := e.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		if ke.Name == "C" {
			gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(v.themeClipboardText(s)))})
		} else {
			gtx.Execute(clipboard.ReadCmd{Tag: s})
		}
	}
}
