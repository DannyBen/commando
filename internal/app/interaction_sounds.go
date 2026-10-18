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
