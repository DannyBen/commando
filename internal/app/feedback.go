package app

import (
	"errors"
	"image/color"
	"io/fs"
	"time"

	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func (v *view) showFeedback(index int, kind feedback.Kind, text string) {
	v.panels[index].feedback.Show(kind, text)
	if v.soundAction != nil {
		v.playFeedbackSound(feedback.Frame{Kind: kind, Started: true})
		v.panels[index].feedback.Acknowledge()
	}
}

func (v *view) operationProgress(index int, text string) {
	v.panels[index].feedback.QuietProgressAt(text, time.Now().Add(v.appearance.FeedbackProgressDelay))
}

// Capture a mailbox rather than a panel index: panels can move or close while
// background work finishes. Apply messages only on the UI thread.
func (v *view) backgroundFeedback(index int) func(feedback.Kind, string) {
	p := &v.panels[index]
	if p.pendingFeedback == nil {
		p.pendingFeedback = make(chan feedback.Message, 1)
	}
	pending, invalidate := p.pendingFeedback, v.invalidate
	return func(kind feedback.Kind, text string) {
		select {
		case pending <- feedback.Message{Kind: kind, Text: text}:
		default:
		}
		if invalidate != nil {
			invalidate()
		}
	}
}

func (v *view) pollFeedback() {
	for i := range v.panels {
		select {
		case message := <-v.panels[i].pendingFeedback:
			v.showFeedback(i, message.Kind, message.Text)
		default:
		}
	}
}

func (v *view) dismissFeedback() {
	if _, ok := v.interaction.current.(*sizeSession); ok {
		return // Ignored input must not erase active calculation progress.
	}
	dismissed := false
	for i := range v.panels {
		dismissed = dismissed || v.panels[i].feedback.Current() != nil
		v.panels[i].feedback.Dismiss()
	}
	// Repeated handling of one input must not stop the action's new cue.
	if dismissed && v.sounds != nil {
		v.sounds.Stop()
	}
}

// Observe clicks through a pass-through pointer handler, and notice Windows
// key presses through the native bridge without consuming editor input.
func (v *view) updateFeedbackInput(gtx layout.Context) {
	if v.nativeKeys.takeInteraction() {
		v.dismissFeedback()
	}
	for {
		_, ok := gtx.Event(pointer.Filter{Target: &v.feedbackPointer, Kinds: pointer.Press})
		if !ok {
			break
		}
		v.dismissFeedback()
	}
}

// Handle otherwise unused keys after widgets, preserving the widgets' normal
// key handling. Modifier keys and other unused keys still dismiss feedback.
func (v *view) dismissUnhandledFeedbackKeys(gtx layout.Context) {
	for {
		e, ok := gtx.Event(key.Filter{Optional: key.ModCtrl | key.ModShift | key.ModAlt | key.ModSuper | key.ModCommand})
		if !ok {
			return
		}
		if ke, ok := e.(key.Event); ok && ke.State == key.Press {
			v.dismissFeedback()
		}
	}
}

func (v *view) notificationColors(kind feedback.Kind) (background, text color.NRGBA) {
	var fg color.NRGBA
	switch kind {
	case feedback.Success:
		fg = v.appearance.Colors.Success
	case feedback.Warning:
		fg = v.appearance.Colors.Warning
	case feedback.Error:
		fg = v.appearance.Colors.Error
	default:
		fg = v.appearance.Colors.Info
	}
	return v.appearance.filenameSelectionColors(oklchFromRGB(fg))
}

// Notification text and the counter share the selected-row contrast treatment.
func (v *view) feedbackLabel(gtx layout.Context, content string, foreground color.NRGBA) layout.Dimensions {
	return v.labelStyle(content, v.appearance.SmallTextSize, v.resolvedTextFor(contrastPrimary, foreground, v.currentTextBackground()), text.Start, true).Layout(gtx)
}

func (v *view) panelFooter(gtx layout.Context, index, height int) layout.Dimensions {
	p := &v.panels[index]
	frame, ownsFooter := p.feedback.Frame(gtx.Now, v.appearance.Feedback)
	if !frame.Next.IsZero() {
		gtx.Execute(op.InvalidateCmd{At: frame.Next})
	}
	status, _ := p.statusAt(gtx.Now, v.dateDisplay())
	if !gtx.Now.IsZero() && !p.drives && (p.metadata == metadataModified || p.metadata == metadataCreated ||
		p.selected >= 0 && p.selected < len(p.items) && !p.items[p.selected].file.Modified.IsZero()) {
		gtx.Execute(op.InvalidateCmd{At: gtx.Now.Add(time.Minute)})
	}
	if index == v.active && p.prefix != "" {
		status = "> " + p.prefix
	}
	background, foreground, counter := v.appearance.Colors.Footer, v.appearance.Colors.Foreground, v.appearance.Colors.Muted
	counterRole := contrastSecondary
	if ownsFooter && !frame.Progress {
		background, foreground = v.notificationColors(frame.Kind)
		counter, counterRole = foreground, contrastPrimary
	}
	return v.band(gtx, height, background, func(gtx layout.Context) layout.Dimensions {
		return v.trailingLayout(gtx, func(gtx layout.Context) layout.Dimensions {
			if ownsFooter {
				v.playFeedbackSound(frame)
				return v.feedbackLabel(gtx, frame.Text, foreground)
			}
			return v.label(gtx, status, v.appearance.SmallTextSize, foreground, text.Start)
		}, func(gtx layout.Context) layout.Dimensions {
			return v.labelFor(counterRole, gtx, p.positionLabel(), v.appearance.SmallTextSize, counter, text.End)
		})
	}, false)
}

func (v *view) playFeedbackSound(frame feedback.Frame) {
	if v.sounds == nil || !frame.Started {
		return
	}
	event := eventMessageInfo
	switch frame.Kind {
	case feedback.Success:
		event = eventMessageSuccess
	case feedback.Warning:
		event = eventMessageWarning
	case feedback.Error:
		event = eventMessageError
	}
	v.announceInteraction(event)
}

func readError(err error) string {
	switch {
	case errors.Is(err, browser.ErrInvalidArchive):
		return "Invalid archive"
	case errors.Is(err, fs.ErrPermission):
		return "Access denied"
	case errors.Is(err, fs.ErrNotExist):
		return "Folder not found"
	default:
		return "Cannot read folder"
	}
}
