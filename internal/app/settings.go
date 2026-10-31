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
	"gioui.org/unit"
	"gioui.org/widget"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
)

func (v *view) restoreSettings(values settings.Values, err error) {
	v.settingsValues = restoreLegacyBandOffsets(values)
	v.applySettings()
	if err == nil {
		return
	}
	message := "Cannot read settings"
	switch {
	case errors.Is(err, errInitialThemeSave):
		message = "Cannot save initial theme"
	case errors.Is(err, settings.ErrRepaired):
		message = "Settings error (auto-fixed)"
	case errors.Is(err, settings.ErrRepairFailed):
		message = "Settings repair failed"
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
	settingTraceMask             = "debug.trace.mask"
	settingProtectedSystem       = "view.show_protected_system_files"
	settingParentEntry           = "view.show_parent_entry"
	settingNotificationAnimation = "feedback.notifications.animation"
	settingLocationScroll        = "feedback.location_bar"
	settingLocationScrollEnabled = settingLocationScroll + ".enabled"
	settingLocationScrollModel   = settingLocationScroll + ".animation"
	settingLocationScrollSpeed   = settingLocationScroll + ".speed"
	settingTypeToSelectDelay     = "feedback.type_to_select.reset_delay_seconds"
	settingHistory               = "feedback.history"
	settingSaveHistory           = settingHistory + ".save_history"
	settingHistoryMatchDelay     = settingHistory + ".match_delay_ms"
	settingSoundEnabled          = "feedback.sounds.enabled"
	settingSoundProfile          = "feedback.sounds.profile"
	settingSoundVolume           = "feedback.sounds.master_volume"
	settingContrastGuard         = "appearance.contrast_guard.enabled"
	settingIconsEnabled          = "appearance.icons.enabled"
	settingIconOriginalColor     = "appearance.icons.original_color"
	settingAutoColorEnabled      = "appearance.autocolor.enabled"
	settingLightness             = "appearance.autocolor.lsh.lightness"
	settingSaturation            = "appearance.autocolor.lsh.saturation"
	settingSalt                  = "appearance.autocolor.salt"
	settingSelectionLightness    = "appearance.autocolor.selection.lightness_points"
	settingCutLightness          = "appearance.autocolor.selection.cut_lightness_points"
	settingHiddenLightness       = "appearance.autocolor.special_files.hidden_lightness_points"
	settingHiddenSaturation      = "appearance.autocolor.special_files.hidden_saturation_multiplier"
	settingDotfileLightness      = "appearance.autocolor.special_files.dotfile_lightness_points"
	settingDotfileSaturation     = "appearance.autocolor.special_files.dotfile_saturation_multiplier"
	settingDataFolder            = "application.data_folder.path"
	settingFontFamily            = "appearance.font.family"
	settingFontTextSize          = "appearance.font.text_size"
	settingFontSmallTextSize     = "appearance.font.small_text_size"
	settingFontWeight            = "appearance.font.weight"
	settingFontBoldWeight        = "appearance.font.bold_weight"
	settingPaddingRows           = "appearance.padding.vertical.rows"
	settingPaddingHeadersFooters = "appearance.padding.vertical.headers_footers"
	settingPaddingLocation       = "appearance.padding.vertical.location"
	settingPaddingSearch         = "appearance.padding.vertical.search"
	settingPaddingContent        = "appearance.padding.horizontal.content"
)

func settingsGroupLabel(key string) string {
	for _, group := range settings.CatalogueDefault.UI.Groups {
		if group.Key == key {
			parent := group.Parent
			switch group.Key {
			case "appearance.theme":
				parent = ""
			case "appearance.autocolor", "appearance.contrast_guard":
				parent = "appearance.theme"
			}
			if parent != "" {
				return settingsGroupLabel(parent) + " › " + group.Label
			}
			return group.Label
		}
	}
	return ""
}
func (v *view) settingsCatalogue() []command {
	var catalogue []command
	themeAdded := false
	widgets := append(slices.Clone(settings.CatalogueDefault.UI.Widgets), themeSelectWidget, themeSaveWidget)
	for _, widget := range widgets {
		if !widget.Visible {
			continue
		}
		key := widget.Key
		if key == "appearance.autocolor" || key == "appearance.contrast_guard" {
			widget.Parent = "appearance.theme"
		}
		if key == settingThemeSave {
			catalogue = append(catalogue, v.resolveCommandBindings(command{id: "settings." + key,
				group: settingsGroupLabel(widget.Parent), name: widget.Label,
				available: func(v *view) bool {
					return v.activeDataFolder != "" && v.loader != nil && v.canOfferInteraction(v.active, actionMutation)
				},
				soundEvent: eventSoundOwned, execute: func(v *view) { v.saveThemeFile() }}))
			continue
		}
		if isThemeColorWidget(key) {
			if themeAdded {
				continue
			}
			themeAdded = true
			key = settingThemeCustomize
			widget.Parent, widget.Label = "appearance.theme", "Customize colors"
		}
		if slices.Contains(settings.CatalogueDefault.Collections, key) {
			group := settingsGroupLabel(widget.Parent) + " › " + widget.Label
			rows := v.settingsValues.Collection(key)
			add := func(index int, name string) {
				// Collection rows use stable IDs independently of their labels and order.
				id := ""
				if index < 0 {
					id = "settings." + key + ".new"
				} else if field := settings.CatalogueDefault.CollectionIdentities[key]; field != "" {
					if value, _ := rows[index][field].(string); settings.ValidIdentity(value) {
						id = "settings." + key + ".rules." + value
					}
				}
				catalogue = append(catalogue, v.resolveCommandBindings(command{id: id, group: group, name: name,
					available:  func(v *view) bool { return v.canOfferInteraction(v.active, actionMutation) },
					soundEvent: eventSoundOwned, execute: func(v *view) { v.startCollectionWidget(key, index, name) }}))
			}
			add(-1, "New")
			for i, row := range rows {
				extensions, _ := row["ext"].([]string)
				add(i, strings.Join(extensions, " "))
			}
			continue
		}
		catalogue = append(catalogue, v.resolveCommandBindings(command{id: "settings." + key, group: settingsGroupLabel(widget.Parent), name: widget.Label,
			available:  func(v *view) bool { return v.canOfferInteraction(v.active, actionMutation) },
			execute:    func(v *view) { v.startSettingsWidget(key) },
			soundEvent: eventSoundOwned,
		}))
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
	preview            string
	palette            *paletteSession
	actions            actionBar
	choices            map[string]*settingsChoiceRow
	values             map[string]*valueTarget
	actionsFocused     bool
	action             int
	afterSave          func() // Optional navigation after a successful save.
	afterCancel        func() // Restore transient state changed by a live preview.
	pending            bool
	collection         string
	item               int
	draft              settings.Values
	commit             settings.Values
	deferred           bool
	deleted            bool
	dirty              bool
	completion         inlineCompletion
	themeName          string // Selected snapshot name, confined to this transaction.
	themes             []themeSnapshot
	themesLoading      bool        // Library reads must not lock the widget like a save.
	themeLibraryResult *loadResult // Choices arriving during text editing wait.
	bindingCommand     *command
	capturing          bool
	returnHeld         bool
}

type settingsSaveRequest struct {
	session  *settingsSession
	run      func() (string, error)
	complete func(string, error)
}

func (*settingsSession) openingEvent() interactionEvent { return eventWidgetOpened }

func (s *settingsSession) interactionPanel() int { return s.panel }
func (*settingsSession) blocksMutations() bool   { return true }
func (s *settingsSession) protectsListing() bool { return !s.selecting }
func (s *settingsSession) holdsOperations() bool { return s.pending }
func (s *settingsSession) blocksPanels() bool    { return s.pending }
func (s *settingsSession) canSuspendFor(next interactionSession) bool {
	if _, ok := next.(*confirmationSession); ok {
		return !s.pending && !s.selecting && !s.capturing && !s.editing
	}
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
	if s := v.newSettingsWidget(group); s != nil {
		if group == settingMetadataColumns {
			v.previewMetadataColumns(s)
		}
		if v.startInteraction(s) && group == settingThemeSelect {
			v.loadThemeLibrary(s)
		}
	}
}
func (v *view) newSettingsWidget(group string) *settingsSession {
	if group == settingThemeCustomize {
		group = themeSettingPrefix + "background"
	}
	spec, ok := settings.CatalogueDefault.Widget(group)
	if group == settingThemeSelect {
		spec, ok = themeSelectWidget, true
	}
	if !ok || !spec.Visible {
		return nil
	}

	if group == "appearance.autocolor" || group == "appearance.contrast_guard" {
		spec.Parent = "appearance.theme"
	}
	title := spec.Title
	if parent := settingsGroupLabel(spec.Parent); parent != "" {
		title = parent + " › " + title
	}
	s := &settingsSession{panel: v.active, title: title, before: v.settingsValues.Clone(), editor: widget.Editor{SingleLine: true, Submit: true}}
	s.preview = spec.Preview
	s.deferred = spec.Deferred
	if s.deferred {
		s.draft = v.settingsValues.Clone()
	}
	for _, field := range settings.CatalogueDefault.UI.Fields {
		if field.Widget == group && field.Visible {
			s.fields = append(s.fields, field)
		}
	}
	if group == settingThemeSelect {
		s.fields = []settings.Field{{Key: settingThemeSelect, Widget: settingThemeSelect, Label: "Theme", Control: "input"}}
		s.setThemeChoices(builtInThemes, v.settingsValues)
	}
	if isThemeColorWidget(group) {
		s.title = "Theme › Customize colors"
		s.fields = append([]settings.Field{{Key: settingThemeElement, Label: "Element", Control: "input"}}, s.fields...)
		s.completion.candidates = themeElementNames()
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
		for _, field := range s.fields {
			if field.Control == "autocomplete-stepper" {
				d, _ := settings.CatalogueDefault.Definition(field.Key)
				s.completion.candidates = slices.Clone(d.Choices)
			}
			if field.Key == settingFontFamily {
				s.completion.candidates = v.fontFamilyChoices()
				v.requestFontFamilies()
			}
		}
		return s
	}
	return nil
}
func (v *view) settingText(key string) string {
	if key == settingThemeSelect {
		if s := v.settingsInteraction(); s != nil && s.themeName != "" {
			return s.themeName
		}
		return "Current theme"
	}
	if key == settingThemeElement {
		return themeElementName(v.settingsWidget())
	}
	if s := v.settingsWidget(); s != nil && s.bindingCommand != nil {
		return v.shortcutSettingText(s, key)
	}
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
		if i := slices.Index(d.ChoiceValues, value.Number); i >= 0 {
			return d.Choices[i]
		}
		if strings.HasPrefix(key, "appearance.theme.") {
			return strconv.FormatFloat(math.Round(value.Number*10000)/10000, 'f', -1, 64)
		}
		return strconv.FormatFloat(value.Number, 'f', -1, 64)
	default:
		if key == settingDataFolder && value.Text == "" {
			return v.defaultDataFolder
		}
		return value.Text
	}
}
func (v *view) adjustSetting(key string, direction int) {
	d, _ := settings.CatalogueDefault.Definition(key)
	if key == settingFontFamily || key == settingThemeSelect || len(d.ChoiceValues) > 0 {
		v.cycleCompletedSetting(key, direction)
		return
	}
	c := settings.CatalogueDefault
	d, _ = c.Definition(key)
	control := ""
	for _, field := range c.UI.Fields {
		if field.Key == key {
			control = field.Control
			break
		}
	}
	if control != "toggle" && control != "choice" && control != "stepper" && control != "stepper-progress" {
		return
	}

	value := v.widgetValues().Get(c, key)
	switch d.Kind {
	case settings.String:
		if len(d.Choices) == 0 {
			return
		}
		index := slices.Index(d.Choices, value.Text)
		if direction == 0 {
			index = (index + 1) % len(d.Choices)
		} else {
			index = max(0, min(len(d.Choices)-1, index+direction))
		}
		value.Text = d.Choices[index]
	case settings.Boolean:
		if direction == 0 {
			value.Bool = !value.Bool
		} else {
			value.Bool = direction < 0
		}
	case settings.Number:
		scale := 1 / d.Step
		value.Number = math.Round(value.Number*scale+float64(direction)) / scale
		if d.Cycle > 0 {
			value.Number = math.Mod(value.Number+d.Cycle, d.Cycle)
		} else {
			value.Number = max(d.Minimum, value.Number)
			if d.Maximum != nil {
				value.Number = min(*d.Maximum, value.Number)
			}
		}
	default:
		return
	}
	v.previewSettingValue(key, value)
}

func (v *view) previewSettingValue(key string, value settings.Value) {
	c := settings.CatalogueDefault
	before := v.widgetValues().Get(c, key)
	if strings.HasPrefix(key, settingThemeGenerator+".") && value == before {
		return
	}
	if v.setWidgetValue(key, value) == nil {
		v.applySettings()
		if v.widgetValues().Get(c, key) != before {
			if s := v.settingsInteraction(); (s == nil || !s.deferred) && v.previewSoundSetting(key) {
				return
			}
			v.announceSettingChange(key, v.widgetValues().Get(c, key))
		}
	}
}
func (v *view) applySettings() {
	v.applyTraceSettings()
	clear(v.textContrast)
	v.updateNativeBindings()
	c := settings.CatalogueDefault
	v.appearance.Colors = themeColorsFromSettings(v.settingsValues)
	v.applyThemeEffects()
	v.theme.Fg, v.theme.Bg = v.appearance.Colors.Foreground, v.appearance.Colors.Surface
	v.theme.ContrastBg, v.theme.ContrastFg = v.appearance.Colors.Selected, v.appearance.Colors.SelectedName
	v.applyFontSettings()
	rows, bands, location := v.appearance.RowPaddingVertical, v.appearance.HeaderFooterPaddingVertical, v.appearance.LocationPaddingVertical
	v.appearance.RowPaddingVertical = unit.Dp(v.settingsValues.Get(c, settingPaddingRows).Number)
	v.appearance.HeaderFooterPaddingVertical = unit.Dp(v.settingsValues.Get(c, settingPaddingHeadersFooters).Number)
	v.appearance.LocationPaddingVertical = unit.Dp(v.settingsValues.Get(c, settingPaddingLocation).Number)
	v.appearance.SearchPaddingVertical = unit.Dp(v.settingsValues.Get(c, settingPaddingSearch).Number)
	v.appearance.ContentPaddingHorizontal = unit.Dp(v.settingsValues.Get(c, settingPaddingContent).Number)
	geometryChanged := rows != v.appearance.RowPaddingVertical || bands != v.appearance.HeaderFooterPaddingVertical || location != v.appearance.LocationPaddingVertical
	v.appearance.Feedback.Enabled = v.settingsValues.Get(c, settingNotificationAnimation).Bool
	v.applySoundSettings()
	colors := defaultFilenameColors
	colors.enabled = v.settingsValues.Get(c, settingAutoColorEnabled).Bool
	colors.filenameLightness = v.settingsValues.Get(c, settingLightness).Number / 100
	colors.filenameSaturation = v.settingsValues.Get(c, settingSaturation).Number / 100
	colors.salt = v.settingsValues.Get(c, settingSalt).Number
	colors.activeBackgroundLightnessOffset = v.settingsValues.Get(c, settingSelectionLightness).Number / 100
	colors.cutLightnessOffset = v.settingsValues.Get(c, settingCutLightness).Number / 100
	colors.hiddenLightnessOffset = v.settingsValues.Get(c, settingHiddenLightness).Number / 100
	colors.hiddenSaturationMultiplier = v.settingsValues.Get(c, settingHiddenSaturation).Number
	colors.dotfileLightnessOffset = v.settingsValues.Get(c, settingDotfileLightness).Number / 100
	colors.dotfileSaturationMultiplier = v.settingsValues.Get(c, settingDotfileSaturation).Number
	v.appearance.FilenameColors = &colors
	show := v.settingsValues.Get(c, settingProtectedSystem).Bool
	showParent := v.settingsValues.Get(c, settingParentEntry).Bool
	apply := func(p *panel) {
		if !v.metadataColumnEnabled(p.metadata) {
			p.setMetadataColumn(v.nextMetadataColumn(p.metadata, 1))
			if p.sortByMetadata {
				p.resort()
			}
		}
		if geometryChanged {
			p.reveal = true
		}
		if p.showProtectedSystem != show || p.showParentEntry != showParent {
			p.ensureEntries()
			p.showProtectedSystem = show
			p.showParentEntry = showParent
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
	if field.Control == "shortcut" {
		v.beginShortcutCapture(gtx)
		return
	}
	if field.Control == "toggle" || field.Control == "choice" {
		v.adjustSetting(field.Key, 0)
		return
	}
	s.editor.SetText(v.settingText(field.Key))
	s.editor.SetCaret(0, utf8.RuneCountInString(s.editor.Text()))
	s.completion.update(&s.editor)
	v.beginWidgetEdit(gtx, false)
}
func (v *view) applySettingDraft(gtx layout.Context) bool {
	s := v.settingsInteraction()
	c := settings.CatalogueDefault
	field := s.fields[s.selected]
	if field.Key == settingThemeElement {
		for _, spec := range themeElements() {
			if strings.EqualFold(strings.TrimSpace(s.editor.Text()), spec.Label) {
				s.editing = false
				v.switchThemeElement(gtx, spec.Key)
				return false // The replacement editor owns subsequent input.
			}
		}
		s.errorText = "Choose a theme element"
		return false
	}
	d, _ := c.Definition(field.Key)
	if field.Key == settingThemeSelect {
		d.Kind = settings.String
	}
	value := settings.Value{Text: s.editor.Text()}
	if len(d.ChoiceValues) > 0 {
		matched := false
		for i, label := range d.Choices {
			if strings.EqualFold(strings.TrimSpace(value.Text), label) {
				value = settings.Value{Number: d.ChoiceValues[i]}
				matched = true
				break
			}
		}
		if !matched {
			s.errorText = "Choose an available option"
			return false
		}
	} else if d.Kind == settings.Number {
		n, err := strconv.ParseFloat(strings.TrimSpace(s.editor.Text()), 64)
		if err != nil {
			s.errorText = "Enter a valid number"
			return false
		}
		value.Number = n
		if d.Integer && math.Trunc(n) != n {
			s.errorText = "Enter a whole number"
			return false
		}
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
		if field.Key == settingThemeSelect {
			s.errorText = "Choose an available theme"
		}
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
	defer v.beginSoundAction()()
	s := v.settingsInteraction()
	if s == nil || s.pending {
		return
	}
	if save && s.capturing {
		s.errorText = "Finish shortcut capture"
		return
	}
	if save && s.bindingCommand != nil {
		if conflict := findBindingConflict(s.draft); conflict != nil {
			if resolved, message, ok := reassignBindings(s.draft, s.bindingCommand.id, s.selected); ok {
				v.startConfirmation(gtx, &confirmationSession{panel: s.panel, message: message, acceptLabel: "Save and reassign", held: map[key.Name]bool{key.NameReturn: s.returnHeld},
					accept: func(gtx layout.Context) {
						s.draft = resolved
						v.finishSettingsWidget(gtx, true)
					}})
				return
			}
			s.errorText = shortcutConflictText(conflict, s.bindingCommand.id, s.selected)
			return
		}
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
		values := v.settingsValues.Clone()
		values.SetCollection(s.collection, rows)
		if err := settings.CatalogueDefault.NormalizeIdentities(&values); err != nil {
			s.errorText = "Cannot identify rule"
			return
		}
		pruneRuleBindings(&values)
		if s.deferred {
			s.commit = values
		} else {
			v.settingsValues = values
		}
	}
	if save {
		values := v.settingsValues
		if s.bindingCommand != nil || s.deferred && s.collection == "" {
			values = s.draft
		} else if s.deferred {
			values = s.commit
		}
		s.commit = values.Clone()
	}
	if save && v.prepareSettingsSave != nil {
		request := &settingsSaveRequest{session: s, run: v.prepareSettingsSave(s.commit.Clone())}
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
		v.setShortcutCapture(s, false)
		v.settingsValues = s.before
		v.applySettings()
		if s.afterCancel != nil {
			s.afterCancel()
		}
	} else if s.bindingCommand != nil || s.deferred {
		v.settingsValues = s.commit.Clone()
		v.applySettings()
	}
	v.finishInteraction(s)
	if save && s.afterSave != nil {
		s.afterSave()
		v.focusInteraction(gtx)
		return
	}
	v.restoreWidgetPalette(s, save)
	v.focusInteraction(gtx)
}

func (v *view) acceptSettingsSave(s *settingsSession, message string, err error) {
	defer v.beginSoundAction()()
	if v.settingsInteraction() != s {
		return
	}
	s.pending = false
	v.interaction.focusPending = true
	if err != nil {
		s.afterSave = nil
		s.errorText = settingsSaveError(err)
		return
	}
	if s.bindingCommand != nil || s.deferred {
		v.settingsValues = s.commit.Clone()
		v.applySettings()
	}
	v.finishInteraction(s)
	if s.afterSave != nil {
		s.afterSave()
		return
	}
	v.restoreWidgetPalette(s, true)
	if message != "" {
		v.showFeedback(s.panel, feedback.Info, message)
	}
}

func settingsSaveError(err error) string {
	if errors.Is(err, os.ErrPermission) {
		return "Settings save: access denied"
	}
	if errors.Is(err, errDataFolderPath) {
		return "Choose an absolute folder"
	}
	return "Could not save settings"
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
	if p.loading || !p.nativeFolder() || p.path == "" || p.selected < 0 || p.selected >= len(p.items) {
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
	v.confirmWidgetEdit(gtx)
	return true
}

func (v *view) updateSettingsWidget(gtx layout.Context) {
	s := v.settingsInteraction()
	if s.pending {
		return
	}
	v.offerThemeLibrary(s)
	defer v.offerThemeLibrary(s)
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
	if isThemeWidget(s) {
		v.updateThemeClipboard(gtx, s)
	}
	if s.capturing {
		v.updateShortcutCapture(gtx)
		return
	}
	v.updateSettingsChoices(gtx, s)
	if v.settingsInteraction() != s {
		return
	}
	v.updateSettingsValues(gtx, s)
	if v.settingsInteraction() != s {
		return
	}
	if s.capturing {
		v.updateShortcutCapture(gtx)
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
		key.Filter{Focus: focus, Name: key.NameUpArrow}, key.Filter{Focus: focus, Name: key.NameDownArrow},
	}
	if !s.editing {
		filters = append(filters, key.Filter{Focus: focus, Name: key.NameLeftArrow, Optional: key.ModCtrl}, key.Filter{Focus: focus, Name: key.NameRightArrow, Optional: key.ModCtrl}, key.Filter{Focus: focus, Name: key.NameSpace})
		field := s.fields[s.selected]
		if !s.actionsFocused {
			numeric := field.Control == "stepper" || field.Control == "stepper-progress"
			if numeric || field.Control == "choice" || s.completesField() {
				filters = append(filters, key.Filter{Focus: focus, Name: key.NameHome}, key.Filter{Focus: focus, Name: key.NameEnd})
			}
			if numeric {
				filters = append(filters, key.Filter{Focus: focus, Name: key.NamePageUp}, key.Filter{Focus: focus, Name: key.NamePageDown})
			}
		}
		if s.fields[s.selected].Control == "shortcut" {
			filters = append(filters, key.Filter{Focus: focus, Name: key.NameDeleteForward})
		}
	}
	var keys []key.Event
	for {
		e, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		if ke, ok := e.(key.Event); ok {
			if ke.Name == key.NameReturn || ke.Name == key.NameEnter {
				s.returnHeld = ke.State == key.Press
			}
			if ke.State == key.Press {
				keys = append(keys, ke)
			}
		}
	}
	if s.editing {
		v.updateEditorInput(gtx, &s.editor)
		for {
			e, ok := s.editor.Update(gtx)
			if !ok {
				break
			}
			if _, ok := e.(widget.ChangeEvent); ok {
				if s.fields[s.selected].Key != settingThemeElement && s.editor.Text() != v.settingText(s.fields[s.selected].Key) {
					s.dirty = true
				}
				s.errorText = ""
			}
		}
	}
	if s.editing && s.completesField() {
		s.completion.update(&s.editor)
	}
	for _, ke := range keys {
		if !s.editing && !s.actionsFocused && v.settingsStepperKey(gtx, s.fields[s.selected], ke) {
			continue
		}
		if ke.Modifiers == key.ModCtrl && (ke.Name == key.NameLeftArrow || ke.Name == key.NameRightArrow) {
			continue
		}
		if ke.Name == key.NameDeleteForward && !s.actionsFocused && s.fields[s.selected].Control == "shortcut" {
			before := v.shortcutSettingText(s, s.fields[s.selected].Key)
			if v.setShortcutDraft("none") && v.shortcutSettingText(s, s.fields[s.selected].Key) != before {
				v.announceInteraction(eventValueChanged)
			}
			continue
		}
		if s.editing && s.completesField() {
			if ke.Name == key.NameTab && ke.Modifiers == 0 && s.completion.accept(&s.editor) {
				if s.fields[s.selected].Key != settingThemeElement {
					s.dirty = true
				}
				s.errorText = ""
				continue
			}
			if ke.Name == key.NameUpArrow || ke.Name == key.NameDownArrow {
				direction := 1
				if ke.Name == key.NameUpArrow {
					direction = -1
				}
				if s.completion.cycle(direction) {
					v.announceInteraction(eventSelectionChanged)
				}
				continue
			}
		}
		if direction := widgetRowDirection(ke); direction != 0 {
			if s.deleted {
				continue
			}
			row := s.selected
			if s.actionsFocused {
				row = len(s.fields)
			}
			next := widgetRowNext(row, len(s.fields)+1, direction)
			if next == row {
				continue
			}
			if s.editing && !v.applySettingDraft(gtx) {
				return
			}
			s.actionsFocused = next == len(s.fields)
			if s.actionsFocused {
				s.action = 0
			} else {
				s.selected = next
			}
			s.errorText = ""
			v.announceInteraction(eventSelectionChanged)
			v.focusInteraction(gtx)
			continue
		}
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
				if v.applySettingDraft(gtx) {
					v.confirmWidgetEdit(gtx)
				}
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
				v.announceInteraction(eventSelectionChanged)
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
			v.announceInteraction(eventSelectionChanged)
			continue
		}
		if s.actionsFocused {
			previous := s.action
			switch ke.Name {
			case key.NameLeftArrow:
				s.action = max(0, s.action-1)
			case key.NameRightArrow:
				s.action = min(len(s.actionLabels())-1, s.action+1)
			case key.NameSpace:
				v.activateSettingsAction(gtx, s.action)
				return
			}
			if s.action != previous {
				v.announceInteraction(eventSelectionChanged)
			}
			continue
		}
		field := s.fields[s.selected]
		if field.Key == settingThemeElement && (ke.Name == key.NameLeftArrow || ke.Name == key.NameRightArrow) {
			direction := 1
			if ke.Name == key.NameLeftArrow {
				direction = -1
			}
			v.cycleThemeElement(gtx, direction)
			return
		}
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
		switch ke.Name {
		case key.NameLeftArrow:
			v.adjustSetting(s.fields[s.selected].Key, -1)
		case key.NameRightArrow:
			v.adjustSetting(s.fields[s.selected].Key, 1)
		case key.NameSpace:
			if field.Control == "toggle" || field.Control == "choice" {
				v.adjustSetting(s.fields[s.selected].Key, 0)
			}
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
		v.setShortcutCapture(s, false)
		s.selecting, s.editing = false, false
		s.dirty = false // Cancel explicitly discards the whole transaction.
		v.escape(gtx)
	case 2:
		s := v.settingsInteraction()
		v.setShortcutCapture(s, false)
		if s.collection != "" {
			if s.item < 0 {
				return
			}
			s.deleted, s.dirty = true, true
			s.editing, s.selecting = false, false
			s.errorText = ""
			s.action, s.actionsFocused = 0, true
			v.announceInteraction(eventDeleteStaged)
			v.focusInteraction(gtx)
			return
		}
		s.editing, s.selecting = false, false
		s.errorText = ""
		for _, field := range s.fields {
			if field.Key == settingThemeElement {
				continue
			}
			if v.widgetValues().Overridden(field.Key) {
				s.dirty = true
			}
			v.widgetValues().Reset(field.Key)
		}
		if s.fields[0].Widget == settingThemeGenerator {
			v.generateTheme()
			s.dirty = true
		}
		if s.bindingCommand != nil {
			if conflict := findBindingConflict(s.draft); conflict != nil {
				s.errorText = shortcutConflictText(conflict, s.bindingCommand.id, s.selected)
			}
		}
		v.applySettings()
		v.announceInteraction(eventWidgetReset)
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
	if s := v.settingsWidget(); s != nil && (s.collection != "" || s.bindingCommand != nil || s.deferred) {
		return &s.draft
	}
	return &v.settingsValues
}
func (s *settingsSession) actionLabels() []string {
	if len(s.fields) > 0 && s.fields[0].Key == settingThemeSelect {
		return []string{"Save", "Cancel"}
	}
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
	if key == settingThemeSelect {
		return v.selectTheme(value.Text)
	}
	values := v.widgetValues()
	c := settings.CatalogueDefault
	if s := v.settingsInteraction(); s != nil && s.bindingCommand != nil {
		c = bindingSettingsCatalogue()
	}
	before, explicit := values.Get(c, key), values.Overridden(key)
	if err := values.Set(c, key, value); err != nil {
		return err
	}
	if strings.HasPrefix(key, settingThemeGenerator+".") {
		v.generateTheme()
		if s := v.settingsInteraction(); s != nil {
			s.dirty = true
		}
	}
	if s := v.settingsInteraction(); s != nil && (before != values.Get(c, key) || !explicit) {
		s.dirty = true
	}
	return nil
}
