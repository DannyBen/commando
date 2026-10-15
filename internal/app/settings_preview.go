package app

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/text"
	"gioui.org/unit"
)

// Preview samples are passive labelled color pairs, with an optional real row
// indicator. They have no editor state, event handlers or keyboard focus.
type previewSample struct {
	label                        string
	foreground, background, edge color.NRGBA
}
type previewSamples []previewSample

func (v *view) settingsPreviewItems(kind string) previewSamples {
	c := v.appearance.Colors
	sample := func(label string, fg, bg color.NRGBA) previewSamples {
		return previewSamples{{label: label, foreground: fg, background: bg}}
	}
	switch kind {
	case "mark":
		return previewSamples{{"File mark", c.Foreground, c.Surface, c.Marked}}
	case "edge":
		return previewSamples{{"Edge mark", c.Foreground, c.Surface, c.Edge}}
	case "secondary":
		return sample("Secondary text", c.Muted, c.Surface)
	case "selected_text":
		return sample("Selected text", c.SelectedName, c.Selected)
	case "text_selection":
		return sample("Selected text", c.EditorText, c.TextSelection)
	case "cut":
		return sample("Cut item", c.CutFilename, c.Surface)
	case "info":
		return sample("Info text", c.Info, c.Footer)
	case "success":
		return sample("Success text", c.Success, c.Footer)
	case "warning":
		return sample("Warning text", c.Warning, c.Footer)
	case "error":
		return sample("Error text", c.Error, c.Footer)
	case "notifications":
		return previewSamples{{label: "Info", foreground: c.Info, background: c.Footer},
			{label: "Success", foreground: c.Success, background: c.Footer},
			{label: "Warning", foreground: c.Warning, background: c.Footer},
			{label: "Error", foreground: c.Error, background: c.Footer}}
	}
	return nil
}

func (samples previewSamples) layout(v *view, gtx layout.Context, height int) layout.Dimensions {
	cells := make([]layout.FlexChild, 0, len(samples))
	for _, sample := range samples {
		cells = append(cells, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width := min(gtx.Constraints.Max.X, v.textWidth(gtx, sample.label, v.appearance.TextSize)+gtx.Dp(16))
			size := image.Pt(width, height)
			defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
			return v.fileRowSurface(gtx, size, sample.background, sample.edge, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: unit.Dp(8), Right: unit.Dp(8)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = 0
						return v.label(gtx, sample.label, v.appearance.TextSize, sample.foreground, text.Start)
					})
				})
			})
		}))
	}
	width := gtx.Constraints.Min.X
	gtx.Constraints.Min.X = 0
	dims := layout.Flex{Alignment: layout.Middle}.Layout(gtx, cells...)
	dims.Size.X = max(width, dims.Size.X)
	return dims
}
