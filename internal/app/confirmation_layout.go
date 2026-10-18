package app

import (
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/widget/material"
)

func (v *view) confirmationMessage(gtx layout.Context, s *confirmationSession) layout.Dimensions {
	label := v.labelStyle(s.message, v.appearance.TextSize, v.resolvedTextOn(v.appearance.Colors.Foreground, v.appearance.Colors.Header), text.Start, false)
	label.MaxLines = 0
	return label.Layout(gtx)
}

func (v *view) layoutConfirmation(gtx layout.Context) layout.Dimensions {
	s := v.confirmationInteraction()
	event.Op(gtx.Ops, s)
	textHeight := v.textHeight(gtx, v.appearance.TextSize)
	rowPadding := gtx.Dp(2 * max(0, v.appearance.RowPaddingVertical))
	rowHeight := max(1, textHeight+rowPadding)
	footerHeight := v.textHeight(gtx, v.appearance.SmallTextSize) + gtx.Dp(2*max(0, v.appearance.HeaderFooterPaddingVertical))
	insetWidth := gtx.Dp(2*max(0, v.appearance.ContentPaddingHorizontal) + 2)
	width := v.textWidth(gtx, s.message, v.appearance.TextSize) + insetWidth
	width = min(gtx.Constraints.Max.X, max(gtx.Dp(v.appearance.OverlayMinWidth), min(gtx.Dp(v.appearance.OverlayMaxWidth), width)))
	measure := gtx
	measure.Constraints = layout.Constraints{Max: image.Pt(max(0, width-insetWidth), 1_000_000)}
	recording := op.Record(gtx.Ops)
	messageHeight := max(textHeight, v.confirmationMessage(measure, s).Size.Y) + rowPadding + gtx.Dp(2)
	recording.Stop()
	height := min(gtx.Constraints.Max.Y, messageHeight+rowHeight+footerHeight+gtx.Dp(2))
	top := max(0, (gtx.Constraints.Max.Y-height)/2)
	backdropTop := min(top, v.locationBarHeight(gtx, v.textHeight(gtx, v.appearance.TextSize)))
	return s.surface.layoutAt(gtx, top, backdropTop, width, v.appearance.Colors, v.appearance.OverlayBackdropOpacity, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = max(0, height-gtx.Dp(2))
		gtx.Constraints.Max.Y = gtx.Constraints.Min.Y
		// Reserve the controls before allocating the scrollable explanation.
		actionHeight := min(rowHeight, gtx.Constraints.Max.Y/2)
		hintHeight := min(footerHeight, max(0, gtx.Constraints.Max.Y-actionHeight))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return v.headerBand(gtx, gtx.Constraints.Max.Y, v.appearance.Colors.Header, func(gtx layout.Context) layout.Dimensions {
					return material.List(v.theme, &s.messageList).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
						return v.confirmationMessage(gtx, s)
					})
				}, true)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return s.actions.layout(gtx, []string{s.acceptLabel, "Cancel"}, s.selected, actionHeight, v.appearance.Colors, func(gtx layout.Context, label string, fg, bg color.NRGBA) layout.Dimensions {
					return v.rawLabel(gtx, label, v.appearance.TextSize, v.resolvedTextOn(fg, bg), text.Middle)
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				action := s.acceptLabel
				if s.selected == 1 {
					action = "Cancel"
				}
				return v.band(gtx, hintHeight, v.appearance.Colors.Footer, func(gtx layout.Context) layout.Dimensions {
					return v.label(gtx, "Enter "+action+" · Esc Cancel", v.appearance.SmallTextSize, v.appearance.Colors.Foreground, text.Start)
				}, false)
			}),
		)
	})
}
