package app

import (
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func (v *view) layout(gtx layout.Context) layout.Dimensions {
	v.frameSize = gtx.Constraints.Max
	v.update(gtx)
	v.fileIconWanted = v.fileIconWanted[:0]
	paint.Fill(gtx.Ops, v.appearance.Colors.Surface)
	event.Op(gtx.Ops, &v.focus)
	children := make([]layout.FlexChild, 0, len(v.panels)*2-1)
	for i := range v.panels {
		if i > 0 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)
				paint.FillShape(gtx.Ops, v.appearance.Colors.Border, clip.Rect{Max: size}.Op())
				return layout.Dimensions{Size: size}
			}))
		}
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if v.interaction.panelsBlocked() {
				gtx.Source = gtx.Source.Disabled()
			}
			return v.panel(gtx, i)
		}))
	}
	dims := layout.Flex{}.Layout(gtx, children...)
	v.submitFileIcons()
	area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	event.Op(gtx.Ops, &v.feedbackPointer)
	pass.Pop()
	area.Pop()
	if v.paletteInteraction() != nil {
		v.layoutPalette(gtx)
	}
	if v.settingsWidget() != nil {
		settingsContext := gtx
		if v.confirmationInteraction() != nil {
			settingsContext.Source = settingsContext.Source.Disabled()
		}
		v.layoutSettingsWidget(settingsContext)
	}
	if v.confirmationInteraction() != nil {
		v.layoutConfirmation(gtx)
	}
	return dims
}

func (v *view) band(gtx layout.Context, height int, bg color.NRGBA, content layout.Widget, active bool) layout.Dimensions {
	return v.bandWithInset(gtx, height, bg, v.contentInset(), content, active)
}

func (v *view) bandWithInset(gtx layout.Context, height int, bg color.NRGBA, inset layout.Inset, content layout.Widget, active bool) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, height)
	paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
	paint.FillShape(gtx.Ops, v.appearance.Colors.Border,
		clip.Rect{Min: image.Pt(0, size.Y-gtx.Dp(1)), Max: size}.Op())
	if active {
		paint.FillShape(gtx.Ops, v.appearance.Colors.Edge, clip.Rect{Max: image.Pt(size.X, gtx.Dp(2))}.Op())
	}
	gtx.Constraints = layout.Exact(size)
	// Content can fill its row, but must not paint over the separating border
	// or outside the band. Keep layout metrics unchanged for text alignment.
	contentArea := clip.Rect{Max: image.Pt(size.X, max(0, size.Y-gtx.Dp(1)))}.Push(gtx.Ops)
	inset.Layout(gtx, v.textOn(bg, func(gtx layout.Context) layout.Dimensions {
		return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return content(gtx)
		})
	}))
	contentArea.Pop()
	return layout.Dimensions{Size: size}
}

func (v *view) locationBarHeight(gtx layout.Context, textHeight int) int {
	return textHeight + gtx.Dp(2*max(0, v.appearance.LocationPaddingVertical)) + gtx.Dp(2)
}

func (v *view) contentInset() layout.Inset {
	padding := max(0, v.appearance.ContentPaddingHorizontal)
	return layout.Inset{Left: padding, Right: padding}
}

// Headers reserve the top edge separately from their centered content. Inactive
// headers reserve the same strip so neighboring panels retain aligned geometry.
func (v *view) headerBand(gtx layout.Context, height int, bg color.NRGBA, content layout.Widget, active bool) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, height)
	edgeHeight := min(height, gtx.Dp(2))
	edge := bg
	if active {
		edge = v.appearance.Colors.Edge
	}
	paint.FillShape(gtx.Ops, edge, clip.Rect{Max: image.Pt(size.X, edgeHeight)}.Op())
	defer op.Offset(image.Pt(0, edgeHeight)).Push(gtx.Ops).Pop()
	gtx.Constraints = layout.Exact(image.Pt(size.X, max(0, height-edgeHeight)))
	v.band(gtx, gtx.Constraints.Max.Y, bg, content, false)
	return layout.Dimensions{Size: size}
}

func (v *view) columns(gtx layout.Context, item entry, fg, secondary color.NRGBA, size unit.Sp, metadataRole contrastRole, boldName bool) layout.Dimensions {
	var metadata layout.Widget
	if item.size != "" {
		metadata = func(gtx layout.Context) layout.Dimensions {
			return v.labelFor(metadataRole, gtx, item.size, size, secondary, text.End)
		}
	}
	return v.trailingLayout(gtx, func(gtx layout.Context) layout.Dimensions {
		return v.labelStyle(item.name, size, v.resolvedText(fg), text.Start, boldName).Layout(gtx)
	}, metadata)
}

func (v *view) columnLayout(gtx layout.Context, width int, name, metadata layout.Widget) layout.Dimensions {
	if width == 0 {
		return name(gtx)
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, name),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, metadata)
		}),
	)
}

// Each row gives its trailing content only the space it needs. Empty metadata
// leaves the full width available to the filename or editor.
func (v *view) trailingLayout(gtx layout.Context, name, metadata layout.Widget) layout.Dimensions {
	if metadata == nil {
		return name(gtx)
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, name),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: unit.Dp(8)}.Layout(gtx, metadata)
		}),
	)
}

func (v *view) label(gtx layout.Context, value string, size unit.Sp, fg color.NRGBA, alignment text.Alignment) layout.Dimensions {
	return v.rawLabel(gtx, value, size, v.resolvedText(fg), alignment)
}

func (v *view) labelFor(role contrastRole, gtx layout.Context, value string, size unit.Sp, fg color.NRGBA, alignment text.Alignment) layout.Dimensions {
	return v.rawLabel(gtx, value, size, v.resolvedTextFor(role, fg, v.currentTextBackground()), alignment)
}

// Raw labels are used for measurement and selection samples whose foreground
// was already resolved against the ordinary editor surface.
func (v *view) rawLabel(gtx layout.Context, value string, size unit.Sp, fg color.NRGBA, alignment text.Alignment) layout.Dimensions {
	return v.labelStyle(value, size, fg, alignment, false).Layout(gtx)
}

func (v *view) labelStyle(value string, size unit.Sp, fg color.NRGBA, alignment text.Alignment, bold bool) material.LabelStyle {
	label := material.Label(v.theme, size, value)
	label.Font.Weight = v.appearance.FontWeight
	if bold {
		label.Font.Weight = v.appearance.BoldWeight
	}
	label.Color, label.Alignment, label.MaxLines = fg, alignment, 1
	return label
}

// Measure an unconstrained line using the same font and layout as the rows.
// Discard its drawing operations; only the height is needed.
func (v *view) textHeight(gtx layout.Context, size unit.Sp) int {
	gtx.Constraints = layout.Constraints{Max: image.Pt(1_000_000, 1_000_000)}
	macro := op.Record(gtx.Ops)
	dims := v.rawLabel(gtx, "Mg", size, v.appearance.Colors.Foreground, text.Start)
	macro.Stop()
	return dims.Size.Y
}
