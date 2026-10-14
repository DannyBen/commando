package app

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
	"github.com/dannyben/commando/internal/sound"
)

func (v *view) restoreSettings(values settings.Values, err error) {
	v.settingsValues = values
	v.applySettings()
	if err == nil {
		return
	}
	message := "Cannot read settings"
	switch {
	case errors.Is(err, os.ErrPermission):
		message = "Settings: access denied"
	case errors.Is(err, settings.ErrSyntax):
		message = "Settings: invalid TOML"
	case errors.Is(err, settings.ErrUnknownKey):
		message = "Settings: unknown key"
	case errors.Is(err, settings.ErrType):
		message = "Settings: invalid value type"
	case errors.Is(err, settings.ErrValue):
		message = "Settings: invalid value"
	case errors.Is(err, settings.ErrRange):
		message = "Settings: value outside range"
	}
	v.showFeedback(v.active, feedback.Error, message)
}

// Application bindings identify leaves; defaults and UI definitions live in TOML.
const (
	settingProtectedSystem  = "view.show_protected_system_files"
	settingSoundsEnabled    = "feedback.sounds.enabled"
	settingAutoColorEnabled = "appearance.autocolor.enabled"
	settingLightness        = "appearance.autocolor.lightness"
	settingChroma           = "appearance.autocolor.chroma"
	settingHue              = "appearance.autocolor.hue"
	settingDataFolder       = "application.data_folder.path"
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
	for _, widget := range settings.CatalogueDefault.UI.Widgets {
		if !widget.Visible {
			continue
		}
		key := widget.Key
		if slices.Contains(settings.CatalogueDefault.Collections, key) {
			group := settingsGroupLabel(widget.Parent) + " › " + widget.Label
			add := func(index int, name string) {
				catalogue = append(catalogue, command{group: group, name: name,
					available: func(v *view) bool { return v.canOfferInteraction(v.active, actionMutation) },
					execute:   func(v *view) { v.startCollectionWidget(key, index, name) }})
			}
			add(-1, "New")
			for i, row := range v.settingsValues.Collection(key) {
				extensions, _ := row["ext"].([]string)
				add(i, strings.Join(extensions, " "))
			}
			continue
		}
		catalogue = append(catalogue, command{group: settingsGroupLabel(widget.Parent), name: widget.Label,
			available: func(v *view) bool { return v.canOfferInteraction(v.active, actionMutation) },
			execute:   func(v *view) { v.startSettingsWidget(key) },
		})
	}
	slices.SortStableFunc(catalogue, func(a, b command) int {
		return strings.Compare(a.label(), b.label())
	})
	return catalogue
}

// One widget owns a group transaction. File-panel browsing and native text
// editors temporarily share this owner rather than creating another UI flow.
type settingsSession struct {
	panel, selected    int
	fields             []settings.Field
	title              string
	before             settings.Values
	editor             widget.Editor
	editing, selecting bool
	errorText          string
	surface            dockedSurface
	palette            *paletteSession
	actions            actionBar
	actionsFocused     bool
	action             int
	pending            bool
	collection         string
	item               int
	draft              settings.Values
	deleted            bool
	dirty              bool
}

type settingsSaveRequest struct {
	session *settingsSession
	run     func() (string, error)
}

func (s *settingsSession) interactionPanel() int { return s.panel }
func (*settingsSession) blocksMutations() bool   { return true }
func (s *settingsSession) protectsListing() bool { return !s.selecting }
func (s *settingsSession) holdsOperations() bool { return s.pending }
func (s *settingsSession) blocksPanels() bool    { return s.pending }
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
	spec, ok := settings.CatalogueDefault.Widget(group)
	if !ok || !spec.Visible {
		return
	}

	title := spec.Title
	if parent := settingsGroupLabel(spec.Parent); parent != "" {
		title = parent + " › " + title
	}
	s := &settingsSession{panel: v.active, title: title, before: v.settingsValues.Clone(), editor: widget.Editor{SingleLine: true, Submit: true}}
	for _, field := range settings.CatalogueDefault.UI.Fields {
		if field.Widget == group && field.Visible {
			s.fields = append(s.fields, field)
		}
	}
	// Enabled switches lead their widget; other fields retain metadata order.
	slices.SortStableFunc(s.fields, func(a, b settings.Field) int {
		enabled := func(f settings.Field) bool { return f.Control == "toggle" && strings.HasSuffix(f.Key, ".enabled") }
		first, second := enabled(a), enabled(b)
		if first == second {
			return 0
		}
		if first {
			return -1
		}
		return 1
	})
	if len(s.fields) > 0 {
		if v.interaction.begin(s) && v.sounds != nil {
			v.playSound(sound.Settings)
		}
	}
}
func (v *view) settingText(key string) string {
	c := settings.CatalogueDefault
	d, _ := c.Definition(key)
	value := v.widgetValues().Get(c, key)
	switch d.Kind {
	case settings.Boolean:
		if value.Bool {
			return "true"
		}
		return "false"
	case settings.Number:
		return strconv.FormatFloat(value.Number, 'f', -1, 64)
	default:
		if key == settingDataFolder && value.Text == "" {
			return v.defaultDataFolder
		}
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

	value := v.widgetValues().Get(c, key)
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
	if v.setWidgetValue(key, value) == nil {
		v.applySettings()
		if value != before {
			v.playNavigationSound()
		}
	}
}
func (v *view) applySettings() {
	c := settings.CatalogueDefault
	if v.sounds != nil && !v.settingsValues.Get(c, settingSoundsEnabled).Bool {
		v.sounds.Stop()
	}
	colors := defaultFilenameColors
	colors.enabled = v.settingsValues.Get(c, settingAutoColorEnabled).Bool
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
	if s.deleted {
		return
	}
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
	if err := v.setWidgetValue(field.Key, value); err != nil {
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
	if s == nil || s.pending {
		return
	}
	if save && !s.deleted && s.editing && !v.applySettingDraft(gtx) {
		return
	}
	if save && s.selecting {
		v.updateSettingsSelection()
	}
	if save && s.collection != "" {
		rows := s.before.Collection(s.collection)
		if s.deleted {
			rows = append(rows[:s.item], rows[s.item+1:]...)
		} else {
			row := s.draft.Item(s.collection)
			if s.collection == settings.EditorCollection {
				if err := settings.NormalizeEditor(row); err != nil {
					s.errorText = err.Error()
					return
				}
			}
			if s.item < 0 {
				rows = append(rows, row)
			} else {
				rows[s.item] = row
			}
		}
		v.settingsValues.SetCollection(s.collection, rows)
	}
	if save && v.prepareSettingsSave != nil {
		request := &settingsSaveRequest{session: s, run: v.prepareSettingsSave(v.settingsValues.Clone())}
		s.pending = true
		s.selecting = false
		if v.loader != nil {
			if !v.loader.submit(loadRequest{panel: s.panel, settingsSave: request}) {
				v.acceptSettingsSave(s, "", errors.New("worker unavailable"))
			}
		} else {
			message, err := request.run()
			v.acceptSettingsSave(s, message, err)
		}
		return
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
		v.playSound(sound.Settings)
	}
	v.focusInteraction(gtx)
}

func (v *view) acceptSettingsSave(s *settingsSession, message string, err error) {
	if v.settingsInteraction() != s {
		return
	}
	s.pending = false
	v.interaction.focusPending = true
	if err != nil {
		s.errorText = "Could not save settings"
		if errors.Is(err, os.ErrPermission) {
			s.errorText = "Settings save: access denied"
		} else if errors.Is(err, errDataFolderPath) {
			s.errorText = "Choose an absolute folder"
		}
		return
	}
	v.interaction.finish(s)
	if message != "" {
		v.showFeedback(s.panel, feedback.Info, message)
	} else if v.sounds != nil {
		v.playSound(sound.Settings)
	}
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
		return
	}

	_ = v.setWidgetValue(field.Key, settings.Value{Text: path})
}

// A file picker changes its value only when the highlighted file is confirmed.
// Folders continue through the shared browsing navigation handler.
func (v *view) chooseSettingsFile(gtx layout.Context) bool {
	s := v.settingsInteraction()
	if s == nil || !s.selecting || s.fields[s.selected].Control != "file" {
		return false
	}
	p := &v.panels[v.active]
	if p.loading || p.drives || p.path == "" || p.selected < 0 || p.selected >= len(p.items) {
		return false
	}
	file := p.items[p.selected].file
	if file.Directory || file.Parent || file.Path != "" {
		return false
	}
	field := s.fields[s.selected]
	path := filepath.Join(p.path, file.Name)
	if err := v.setWidgetValue(field.Key, settings.Value{Text: path}); err != nil {
		return false
	}
	s.selecting = false
	s.errorText = ""
	v.focusInteraction(gtx)
	return true
}

func (v *view) updateSettingsWidget(gtx layout.Context) {
	s := v.settingsInteraction()
	if s.pending {
		return
	}
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
				if s.editor.Text() != v.settingText(s.fields[s.selected].Key) {
					s.dirty = true
				}
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
			if s.deleted {
				direction := 1
				if ke.Modifiers == key.ModShift {
					direction = -1
				}
				s.actionsFocused = true
				s.action = (s.action + direction + len(s.actionLabels())) % len(s.actionLabels())
				v.playNavigationSound()
				continue
			}
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
			next := (index + direction + len(s.fields) + len(s.actionLabels())) % (len(s.fields) + len(s.actionLabels()))
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
				s.action = min(len(s.actionLabels())-1, s.action+1)
			case key.NameUpArrow:
				if s.deleted {
					continue
				}
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
	if s := v.settingsInteraction(); s == nil || s.pending {
		return
	}
	switch action {
	case 0:
		v.finishSettingsWidget(gtx, true)
	case 1:
		s := v.settingsInteraction()
		s.selecting, s.editing = false, false
		s.dirty = false // Cancel explicitly discards the whole transaction.
		v.escape(gtx)
	case 2:
		s := v.settingsInteraction()
		if s.collection != "" {
			if s.item < 0 {
				return
			}
			s.deleted, s.dirty = true, true
			s.editing, s.selecting = false, false
			s.errorText = ""
			s.action, s.actionsFocused = 0, true
			v.focusInteraction(gtx)
			return
		}
		s.editing, s.selecting = false, false
		s.errorText = ""
		for _, field := range s.fields {
			if v.settingsValues.Overridden(field.Key) {
				s.dirty = true
			}
			v.settingsValues.Reset(field.Key)
		}
		v.applySettings()
		v.focusInteraction(gtx)
	}
}

func (v *view) startCollectionWidget(collection string, index int, label string) {
	rows := v.settingsValues.Collection(collection)
	if index >= len(rows) {
		return
	}
	v.startSettingsWidget(collection)
	s := v.settingsInteraction()
	if s == nil {
		return
	}
	s.collection, s.item = collection, index
	s.title += " › " + label
	row := map[string]any{}
	if index >= 0 {
		row = rows[index]
	}
	s.draft.SetItem(collection, row)
	for _, field := range s.fields {
		if !s.draft.Overridden(field.Key) {
			d, _ := settings.CatalogueDefault.Definition(field.Key)
			_ = s.draft.Set(settings.CatalogueDefault, field.Key, d.Default)
		}
	}
}
func (v *view) widgetValues() *settings.Values {
	if s := v.settingsWidget(); s != nil && s.collection != "" {
		return &s.draft
	}
	return &v.settingsValues
}
func (s *settingsSession) actionLabels() []string {
	if s.collection == "" {
		return []string{"Save", "Cancel", "Reset"}
	}
	if s.item < 0 || s.deleted {
		return []string{"Save", "Cancel"}
	}
	return []string{"Save", "Cancel", "Delete"}
}

// Dirty is sticky for this transaction: even a subsequently reverted edit
// requires an explicit Save or Cancel before the widget can be dismissed.
func (v *view) setWidgetValue(key string, value settings.Value) error {
	values := v.widgetValues()
	c := settings.CatalogueDefault
	before, explicit := values.Get(c, key), values.Overridden(key)
	if err := values.Set(c, key, value); err != nil {
		return err
	}
	if s := v.settingsInteraction(); s != nil && (before != values.Get(c, key) || !explicit) {
		s.dirty = true
	}
	return nil
}
