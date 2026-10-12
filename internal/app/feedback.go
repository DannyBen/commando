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
	"github.com/dannyben/commando/internal/sound"
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
	if frame, ownsFooter := p.feedback.Frame(gtx.Now, v.appearance.Feedback); ownsFooter {
		v.playFeedbackSound(frame)
		if !frame.Next.IsZero() {
			gtx.Execute(op.InvalidateCmd{At: frame.Next})
		}
		return v.label(gtx, frame.Text, v.appearance.SmallTextSize, v.feedbackColor(frame.Kind), text.Start)
	}
	status, _ := p.status()
	if index == v.active && p.prefix != "" {
		status = "> " + p.prefix
	}
	return v.columns(gtx, entry{name: status, size: p.positionLabel()}, v.appearance.Colors.Foreground,
		v.appearance.Colors.Muted, v.appearance.SmallTextSize, gtx.Dp(84))
}

func (v *view) playFeedbackSound(frame feedback.Frame) {
	if v.sounds == nil {
		return
	}
	if frame.Finished {
		var cue sound.Name
		switch frame.Kind {
		case feedback.Success:
			cue = sound.MessageSuccess
		case feedback.Warning:
			cue = sound.MessageWarning
		case feedback.Error:
			cue = sound.MessageError
		}
		if cue != "" {
			v.sounds.Play(cue)
			return
		}
	}
	if frame.Typed {
		v.sounds.Play(sound.MessageTyping)
	}
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
