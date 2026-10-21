package app

import "github.com/dannyben/commando/internal/sound"

// Interaction events identify accepted UI outcomes, independently of physical
// keys, commands and audio cues. Stable names can support future overrides.
type interactionEvent string

const (
	eventSelectionChanged    interactionEvent = "selection_changed"
	eventValueChanged        interactionEvent = "value_changed"
	eventPaletteOpened       interactionEvent = "palette_opened"
	eventWidgetOpened        interactionEvent = "widget_opened"
	eventQuestionOpened      interactionEvent = "question_opened"
	eventHelpOpened          interactionEvent = "help_opened"
	eventHelpClosed          interactionEvent = "help_closed"
	eventFollowEnabled       interactionEvent = "follow_enabled"
	eventFollowDisabled      interactionEvent = "follow_disabled"
	eventEditStarted         interactionEvent = "edit_started"
	eventEditConfirmed       interactionEvent = "edit_confirmed"
	eventSettingsSaved       interactionEvent = "settings_saved"
	eventWidgetReset         interactionEvent = "widget_reset"
	eventDeleteStaged        interactionEvent = "delete_staged"
	eventCancelled           interactionEvent = "cancelled"
	eventUnsavedChanges      interactionEvent = "unsaved_changes"
	eventDirectoryNavigation interactionEvent = "directory_navigation"
	eventMessageInfo         interactionEvent = "message_info"
	eventMessageSuccess      interactionEvent = "message_success"
	eventMessageWarning      interactionEvent = "message_warning"
	eventMessageError        interactionEvent = "message_error"
)

// This is the application sound policy. An empty or unmapped cue is silent.
// Shared interaction handlers report events; features do not select audio.
var interactionSounds = map[interactionEvent]sound.Name{
	eventSelectionChanged:    sound.Navigation,
	eventValueChanged:        sound.Navigation,
	eventPaletteOpened:       sound.Palette,
	eventWidgetOpened:        sound.Settings,
	eventQuestionOpened:      sound.Question,
	eventHelpOpened:          sound.HelpOpened,
	eventHelpClosed:          sound.HelpClosed,
	eventFollowEnabled:       sound.FollowEnabled,
	eventFollowDisabled:      sound.FollowDisabled,
	eventEditStarted:         sound.Settings,
	eventEditConfirmed:       sound.Settings,
	eventSettingsSaved:       sound.Settings,
	eventWidgetReset:         sound.Settings,
	eventDeleteStaged:        sound.Settings,
	eventCancelled:           sound.Cancel,
	eventUnsavedChanges:      sound.MessageWarning,
	eventDirectoryNavigation: sound.DirectoryNavigation,
	eventMessageInfo:         sound.MessageInfo,
	eventMessageSuccess:      sound.MessageSuccess,
	eventMessageWarning:      sound.MessageWarning,
	eventMessageError:        sound.MessageError,
}

func (v *view) announceInteraction(event interactionEvent) {
	if cue := interactionSounds[event]; cue != "" {
		v.playSound(cue)
	}
}

// Resize cues describe the resulting size, independently of the command's
// direction. Resetting to normal has the same pitch as reaching it by steps.
var panelSizeSounds = map[float64]sound.Name{
	0.6: sound.PanelSize06,
	0.8: sound.PanelSize08,
	1:   sound.PanelSize10,
	1.2: sound.PanelSize12,
	1.5: sound.PanelSize15,
	2:   sound.PanelSize20,
}

func (v *view) announcePanelResize(ratio float64) {
	if cue := panelSizeSounds[ratio]; cue != "" {
		v.playSound(cue)
	}
}
