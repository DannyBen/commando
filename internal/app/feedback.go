package app

import (
	"errors"
	"image/color"
	"io/fs"

	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"github.com/dannyben/commando/internal/feedback"
)

func (v *view) showFeedback(index int, kind feedback.Kind, text string) {
	v.panels[index].feedback.Show(kind, text)
}

func (v *view) dismissFeedback() {
	for i := range v.panels {
		v.panels[i].feedback.Dismiss()
	}
	if v.sounds != nil {
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

func (v *view) feedbackColor(kind feedback.Kind) color.NRGBA {
	switch kind {
	case feedback.Success:
		return v.appearance.Colors.Success
	case feedback.Warning:
		return v.appearance.Colors.Warning
	case feedback.Error:
		return v.appearance.Colors.Error
	default:
		return v.appearance.Colors.Info
	}
}

func (v *view) footerContents(gtx layout.Context, index int) layout.Dimensions {
	p := &v.panels[index]
	frame, ownsFooter := p.feedback.Frame(gtx.Now, v.appearance.Feedback)
	if !frame.Next.IsZero() {
		gtx.Execute(op.InvalidateCmd{At: frame.Next})
	}
	if ownsFooter {
		v.playFeedbackSound(frame)
		return v.label(gtx, frame.Text, v.appearance.SmallTextSize, v.feedbackColor(frame.Kind), text.Start)
	}
	status, _ := p.status()
	if index == v.active && p.prefix != "" {
		status = "> " + p.prefix
	}
	return v.columnLayout(gtx, gtx.Dp(84), func(gtx layout.Context) layout.Dimensions {
		return v.label(gtx, status, v.appearance.SmallTextSize, v.appearance.Colors.Foreground, text.Start)
	}, func(gtx layout.Context) layout.Dimensions {
		return v.labelFor(contrastSecondary, gtx, p.positionLabel(), v.appearance.SmallTextSize, v.appearance.Colors.Muted, text.End)
	})
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
	case errors.Is(err, fs.ErrPermission):
		return "Access denied"
	case errors.Is(err, fs.ErrNotExist):
		return "Folder not found"
	default:
		return "Cannot read folder"
	}
}
