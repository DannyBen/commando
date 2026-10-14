package app

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

// Application bindings identify leaves; defaults and UI definitions live in TOML.
const (
	settingProtectedSystem = "files.show_protected_system"
	settingLightness       = "appearance.filenames.lightness"
	settingChroma          = "appearance.filenames.chroma"
	settingHue             = "appearance.filenames.hue"
	settingDataFolder      = "storage.data_folder"
)

func settingsGroupLabel(key string) string {
	for _, group := range settings.CatalogueDefault.UI.Groups {
		if group.Key == key {
			if group.Parent != "" {
				return settingsGroupLabel(group.Parent) + " › " + group.Label
			}
			return group.Label
		}
	}
	return ""
}
func (v *view) settingsCatalogue() []command {
	var catalogue []command
	for _, group := range settings.CatalogueDefault.UI.Groups {
		fields := false
		for _, field := range settings.CatalogueDefault.UI.Fields {
			if field.Group == group.Key {
				fields = true
				break
			}
		}
		if !fields {
			continue
		}
		key := group.Key
		catalogue = append(catalogue, command{group: settingsGroupLabel(group.Parent), name: group.Label,
			available: func(v *view) bool { return v.canOfferInteraction(v.active, actionMutation) },
			execute:   func(v *view) { v.startSettingsWidget(key) },
		})
	}
	return catalogue
}

// One widget owns a group transaction. File-panel browsing and native text
// editors temporarily share this owner rather than creating another UI flow.
type settingsSession struct {
	panel, selected    int
	fields             []settings.Field
	before             settings.Values
	editor             widget.Editor
	editing, selecting bool
	errorText          string
	surface            dockedSurface
	palette            *paletteSession
	actions            actionBar
	actionsFocused     bool
	action             int
}

func (s *settingsSession) interactionPanel() int { return s.panel }
func (*settingsSession) blocksMutations() bool   { return true }
func (s *settingsSession) protectsListing() bool { return !s.selecting }
func (s *settingsSession) canSuspendFor(next interactionSession) bool {
	if !s.selecting {
		return false
	}
	switch next.(type) {
	case *locationSession, *filterSession:
		return true
	}
	return false
}
func (v *view) settingsInteraction() *settingsSession {
	s, _ := v.interaction.current.(*settingsSession)
	return s
}
func (v *view) settingsSelecting() bool { s := v.settingsInteraction(); return s != nil && s.selecting }
func (v *view) startSettingsWidget(group string) {
	if !v.interaction.idle() || !v.actionAllowed(actionMutation) {
		return
	}
	s := &settingsSession{panel: v.active, before: v.settingsValues.Clone(), editor: widget.Editor{SingleLine: true, Submit: true}}
	for _, field := range settings.CatalogueDefault.UI.Fields {
		if field.Group == group {
			s.fields = append(s.fields, field)
		}
	}
	if len(s.fields) > 0 {
		if v.interaction.begin(s) && v.sounds != nil {
			v.sounds.Play(sound.Settings)
		}
	}
}
func (v *view) settingText(key string) string {
	c := settings.CatalogueDefault
	d, _ := c.Definition(key)
	value := v.settingsValues.Get(c, key)
	switch d.Kind {
	case settings.Boolean:
		if value.Bool {
			return "true"
		}
		return "false"
	case settings.Number:
		return strconv.FormatFloat(value.Number, 'f', -1, 64)
	default:
		return value.Text
	}
}
func (v *view) adjustSetting(key string, direction int) {
	c := settings.CatalogueDefault
	d, _ := c.Definition(key)
	control := ""
	for _, field := range c.UI.Fields {
		if field.Key == key {
			control = field.Control
			break
		}
	}
	if control != "toggle" && control != "stepper" && control != "stepper-progress" {
		return
	}

	value := v.settingsValues.Get(c, key)
	before := value
	switch d.Kind {
	case settings.Boolean:
		if direction == 0 {
			value.Bool = !value.Bool
		} else {
			value.Bool = direction < 0
		}
	case settings.Number:
		scale := 1 / d.Step
		value.Number = max(d.Minimum, math.Round(value.Number*scale+float64(direction))/scale)
		if d.Maximum != nil {
			value.Number = min(*d.Maximum, value.Number)
		}
	default:
		return
	}
	if v.settingsValues.Set(c, key, value) == nil {
		v.applySettings()
		if value != before {
			v.playNavigationSound()
		}
	}
}
func (v *view) applySettings() {
	c := settings.CatalogueDefault
	colors := defaultFilenameColors
	colors.filenameLightness = v.settingsValues.Get(c, settingLightness).Number
	colors.filenameChroma = v.settingsValues.Get(c, settingChroma).Number
	colors.hueSeed = v.settingsValues.Get(c, settingHue).Number
	v.appearance.FilenameColors = &colors
	show := v.settingsValues.Get(c, settingProtectedSystem).Bool
	apply := func(p *panel) {
		if p.showProtectedSystem != show {
			p.ensureEntries()
			p.showProtectedSystem = show
			p.refilter()
		}
	}
	for i := range v.panels {
		apply(&v.panels[i])
	}
	for i := range v.closedPanels {
		apply(&v.closedPanels[i].panel)
	}
}
func (v *view) beginSettingEdit(gtx layout.Context) {
	s := v.settingsInteraction()
	field := s.fields[s.selected]
	if field.Control == "toggle" {
		v.adjustSetting(field.Key, 0)
		return
	}
	s.editing = true
	s.errorText = ""
	s.editor.SetText(v.settingText(field.Key))
	s.editor.SetCaret(0, utf8.RuneCountInString(s.editor.Text()))
	v.focusInteraction(gtx)
}
func (v *view) applySettingDraft(gtx layout.Context) bool {
	s := v.settingsInteraction()
	c := settings.CatalogueDefault
	field := s.fields[s.selected]
	d, _ := c.Definition(field.Key)
	value := settings.Value{Text: s.editor.Text()}
	if d.Kind == settings.Number {
		n, err := strconv.ParseFloat(strings.TrimSpace(s.editor.Text()), 64)
		if err != nil {
			s.errorText = "Enter a valid number"
			return false
		}
		value.Number = n
	} else if d.Kind == settings.Boolean {
		b, err := strconv.ParseBool(strings.TrimSpace(s.editor.Text()))
		if err != nil {
			s.errorText = "Enter true or false"
			return false
		}
		value.Bool = b
	}
	if err := v.settingsValues.Set(c, field.Key, value); err != nil {
		s.errorText = "Value outside allowed range"
		if d.Maximum != nil {
			s.errorText = fmt.Sprintf("Use %g to %g", d.Minimum, *d.Maximum)
		}
		return false
	}
	s.editing = false
	s.errorText = ""
	v.applySettings()
	v.focusInteraction(gtx)
	return true
}
func (v *view) finishSettingsWidget(gtx layout.Context, save bool) {
	s := v.settingsInteraction()
	if s == nil {
		return
	}
	if save && s.editing && !v.applySettingDraft(gtx) {
		return
	}
	if save && s.selecting {
		v.updateSettingsSelection()
	}
	if save && v.saveSettings != nil {
		if err := v.saveSettings(v.settingsValues); err != nil {
			s.errorText = "Could not save settings"
			if errors.Is(err, os.ErrPermission) {
				s.errorText = "Settings save: access denied"
			}
			v.focusInteraction(gtx)
			return
		}
	}
	if !save {
		v.settingsValues = s.before
		v.applySettings()
	}
	v.interaction.finish(s)
	if !save && s.palette != nil {
		v.active = s.palette.panel
		v.interaction.begin(s.palette)
	}
	if save && v.sounds != nil {
		v.sounds.Play(sound.Settings)
	}
	v.focusInteraction(gtx)
}
func (v *view) updateSettingsSelection() {
	s := v.settingsInteraction()
	if s == nil || !s.selecting {
		return
	}
	p := &v.panels[v.active]
	if p.loading || p.drives || p.path == "" {
		return
	}
	field := s.fields[s.selected]
	path := p.path
	if field.Control == "file" {
		if p.selected < 0 || p.selected >= len(p.items) {
			return
		}
		file := p.items[p.selected].file
		if file.Directory || file.Parent || file.Path != "" {
			return
		}
		path = filepath.Join(p.path, file.Name)
	}
	_ = v.settingsValues.Set(settings.CatalogueDefault, field.Key, settings.Value{Text: path})
}

func (v *view) updateSettingsWidget(gtx layout.Context) {
	s := v.settingsInteraction()
	if action, ok := s.actions.update(gtx); ok {
		s.action, s.actionsFocused = action, true
		v.activateSettingsAction(gtx, action)
		if v.settingsInteraction() != s {
			return
		}
		if s.editing {
			s.actionsFocused = false
		}
		v.focusInteraction(gtx)
	}
	if s.selecting {
		return
	}

	focus := any(&v.focus)
	if s.editing {
		focus = &s.editor
	}
	filters := []event.Filter{
		key.FocusFilter{Target: &v.focus}, key.Filter{Focus: focus, Name: key.NameEscape},
		key.Filter{Focus: focus, Name: key.NameReturn, Optional: key.ModCtrl}, key.Filter{Focus: focus, Name: key.NameEnter, Optional: key.ModCtrl},
		key.Filter{Focus: focus, Name: key.NameTab, Optional: key.ModShift},
	}
	if !s.editing {
		filters = append(filters, key.Filter{Focus: focus, Name: key.NameUpArrow}, key.Filter{Focus: focus, Name: key.NameDownArrow}, key.Filter{Focus: focus, Name: key.NameLeftArrow}, key.Filter{Focus: focus, Name: key.NameRightArrow}, key.Filter{Focus: focus, Name: key.NameSpace})
	}
	var keys []key.Event
	for {
		e, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		if ke, ok := e.(key.Event); ok && ke.State == key.Press {
			keys = append(keys, ke)
		}
	}
	if s.editing {
		v.updateEditorPaste(gtx, &s.editor)
		for {
			e, ok := s.editor.Update(gtx)
			if !ok {
				break
			}
			if _, ok := e.(widget.ChangeEvent); ok {
				s.errorText = ""
			}
		}
	}
	for _, ke := range keys {
		if ke.Name == key.NameEscape {
			v.escape(gtx)
			return
		}
		if ke.Name == key.NameReturn || ke.Name == key.NameEnter {
			if ke.Modifiers == key.ModCtrl {
				v.finishSettingsWidget(gtx, true)
			} else if s.actionsFocused {
				v.activateSettingsAction(gtx, s.action)
			} else if s.editing {
				v.applySettingDraft(gtx)
			} else {
				v.beginSettingEdit(gtx)
			}
			return
		}
		if ke.Name == key.NameTab {
			if s.editing && !v.applySettingDraft(gtx) {
				return
			}
			index := s.selected
			if s.actionsFocused {
				index = len(s.fields) + s.action
			}
			direction := 1
			if ke.Modifiers == key.ModShift {
				direction = -1
			}
			next := (index + direction + len(s.fields) + 3) % (len(s.fields) + 3)
			s.actionsFocused = next >= len(s.fields)
			if s.actionsFocused {
				s.action = next - len(s.fields)
			} else {
				s.selected = next
			}
			s.errorText = ""
			v.playNavigationSound()
			continue
		}
		if s.actionsFocused {
			previous := s.action
			switch ke.Name {
			case key.NameLeftArrow:
				s.action = max(0, s.action-1)
			case key.NameRightArrow:
				s.action = min(2, s.action+1)
			case key.NameUpArrow:
				s.actionsFocused = false
				v.playNavigationSound()
			case key.NameSpace:
				v.activateSettingsAction(gtx, s.action)
				return
			}
			if s.action != previous {
				v.playNavigationSound()
			}
			continue
		}
		field := s.fields[s.selected]
		if field.IsPath() && (ke.Name == key.NameLeftArrow || ke.Name == key.NameRightArrow) {
			s.selecting = true
			if ke.Name == key.NameLeftArrow {
				v.switchPanel(-1)
			}
			if ke.Name == key.NameRightArrow {
				v.switchPanel(1)
			}
			v.updateSettingsSelection()
			v.focusInteraction(gtx)
			return
		}
		old := s.selected
		switch ke.Name {
		case key.NameUpArrow:
			s.selected = max(0, s.selected-1)
		case key.NameDownArrow:
			if s.selected == len(s.fields)-1 {
				s.actionsFocused = true
				s.action = 0
				v.playNavigationSound()
			} else {
				s.selected++
			}
		case key.NameLeftArrow:
			v.adjustSetting(s.fields[s.selected].Key, -1)
		case key.NameRightArrow:
			v.adjustSetting(s.fields[s.selected].Key, 1)
		case key.NameSpace:
			if field.Control == "toggle" {
				v.adjustSetting(s.fields[s.selected].Key, 0)
			}
		}
		if old != s.selected {
			s.errorText = ""
			v.playNavigationSound()
		}
	}
}

func (v *view) settingsWidget() *settingsSession {
	if s := v.settingsInteraction(); s != nil {
		return s
	}
	s, _ := v.interaction.suspended.(*settingsSession)
	return s
}

func (v *view) activateSettingsAction(gtx layout.Context, action int) {
	switch action {
	case 0:
		v.finishSettingsWidget(gtx, true)
	case 1:
		v.settingsInteraction().selecting = false
		v.escape(gtx)
	case 2:
		s := v.settingsInteraction()
		s.editing, s.selecting = false, false
		s.errorText = ""
		for _, field := range s.fields {
			v.settingsValues.Reset(field.Key)
		}
		v.applySettings()
		v.focusInteraction(gtx)
	}
}
