package app

import (
	"image"
	"strings"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
)

func (v *view) layoutHelpFooter(gtx layout.Context) layout.Dimensions {
	if v.helpHidden {
		v.helpScrollID = ""
		v.helpScroll = [2]textMarquee{}
		return layout.Dimensions{}
	}
	padding := max(0, v.appearance.HeaderFooterPaddingVertical)
	border := gtx.Dp(1)
	height := 2*v.textHeight(gtx, v.appearance.TextSize) + 2*gtx.Dp(padding) + border
	size := gtx.Constraints.Constrain(image.Pt(gtx.Constraints.Max.X, height))
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	background := v.appearance.Colors.Footer
	paint.FillShape(gtx.Ops, background, clip.Rect{Max: size}.Op())
	paint.FillShape(gtx.Ops, v.appearance.Colors.Border, clip.Rect{Max: image.Pt(size.X, border)}.Op())
	gtx.Constraints = layout.Exact(size)
	inset := v.contentInset()
	inset.Top, inset.Bottom = padding+1, padding
	id, tip := defaultHelp.resolve(v.helpContextIDs()...)
	if v.helpScrollID != id {
		v.helpScrollID, v.helpScroll = id, [2]textMarquee{}
	}
	inset.Layout(gtx, v.textOn(background, func(gtx layout.Context) layout.Dimensions {
		var children []layout.FlexChild
		for i, line := range tip.lines {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return v.layoutHelpLine(gtx, line, &v.helpScroll[i])
			}))
		}
		dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		// Use only spare space on the bottom line; help retains its full width.
		lineWidth := 0
		if len(tip.lines) > 1 {
			lineWidth = v.helpScroll[1].fullWidth
		}
		label := "v" + version
		if lineWidth+gtx.Dp(8)+v.textWidth(gtx, label, v.appearance.SmallTextSize) <= gtx.Constraints.Max.X {
			layout.SE.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return v.labelFor(contrastSecondary, gtx, label, v.appearance.SmallTextSize, v.appearance.Colors.Muted, text.End)
			})
		}
		return dims
	}))
	return layout.Dimensions{Size: size}
}

func (v *view) layoutHelpLine(gtx layout.Context, line []helpSpan, scroll *textMarquee) layout.Dimensions {
	var children []layout.FlexChild
	var content strings.Builder
	for _, span := range v.resolveHelpLine(line) {
		content.WriteString(span.text)
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if span.command != "" || span.settingsQuery != "" {
				return v.labelFor(contrastSecondary, gtx, span.text, v.appearance.TextSize, v.appearance.Colors.Muted, text.Start)
			}
			return v.label(gtx, span.text, v.appearance.TextSize, v.appearance.Colors.Foreground, text.Start)
		}))
	}
	// Shape every span in full, including styled keys, before clipping the line.
	full := gtx
	full.Constraints = layout.Constraints{Max: image.Pt(1_000_000, gtx.Constraints.Max.Y)}
	recording := op.Record(gtx.Ops)
	dims := layout.Flex{Alignment: layout.Baseline}.Layout(full, children...)
	drawing := recording.Stop()
	width := gtx.Constraints.Max.X
	offset := scroll.offset(gtx, content.String(), width, dims.Size.X)
	defer clip.Rect{Max: image.Pt(width, dims.Size.Y)}.Push(gtx.Ops).Pop()
	defer op.Affine(f32.Affine2D{}.Offset(f32.Pt(-offset, 0))).Push(gtx.Ops).Pop()
	drawing.Add(gtx.Ops)
	dims.Size.X = width
	return dims
}
