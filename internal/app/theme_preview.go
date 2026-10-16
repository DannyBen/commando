package app

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"github.com/dannyben/commando/internal/feedback"
)

// A passive presentation of otherwise transient theme states. It never owns
// input or writes to panel selection, marks, filters, feedback or the clipboard.
type themePanelPreview struct {
	selection, marks bool
	messages         []feedback.Kind
}

func (v *view) themePreview(index int) themePanelPreview {
	s := v.settingsWidget()
	if s == nil || len(s.fields) == 0 || s.fields[0].Widget != settingThemeGenerator {
		return themePanelPreview{}
	}
	if index == 0 {
		return themePanelPreview{selection: true, marks: true, messages: []feedback.Kind{feedback.Info, feedback.Success, feedback.Warning, feedback.Error}}
	}
	return themePanelPreview{}
}

func (preview themePanelPreview) currentRow(p *panel) int {
	if !preview.selection {
		return p.selected
	}
	first := max(0, p.list.Position.First)
	count := len(p.items) - first
	if p.list.Position.Count > 0 {
		count = min(count, p.list.Position.Count)
	}
	if count <= 0 {
		return -1
	}
	return first + min(2, count-1)
}

func (preview themePanelPreview) marked(p *panel, row int) bool {
	if !preview.marks || !p.markable(row) {
		return false
	}
	// Pick from the viewport, without scrolling to or changing real marks.
	remaining := 3
	for i := max(0, p.list.Position.First); i <= row; i++ {
		if p.markable(i) {
			remaining--
		}
	}
	return row >= p.list.Position.First && remaining >= 0
}

func (v *view) themePreviewSelection(gtx layout.Context) layout.Dimensions {
	c := v.appearance.Colors
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.label(gtx, "/ ", v.appearance.SmallTextSize, c.EditorText, text.Start)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			const sample = "Example selection"
			size := image.Pt(min(gtx.Constraints.Max.X, v.textWidth(gtx, sample, v.appearance.SmallTextSize)), v.textHeight(gtx, v.appearance.SmallTextSize))
			return v.fileRowSurface(gtx, size, c.TextSelection, color.NRGBA{}, func(gtx layout.Context) layout.Dimensions {
				return v.rawLabel(gtx, sample, v.appearance.SmallTextSize, v.resolvedTextOn(c.EditorText, c.Surface), text.Start)
			})
		}),
	)
}

func (preview themePanelPreview) footer(v *view, gtx layout.Context, height int) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(preview.messages))
	for i, kind := range preview.messages {
		children = append(children, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			label := map[feedback.Kind]string{feedback.Error: "Error", feedback.Success: "Success", feedback.Warning: "Warning", feedback.Info: "Info"}[kind]
			fg := v.feedbackColor(kind)
			// Keep normal outer margins, with only a small gap between samples.
			inset := layout.Inset{Left: 2, Right: 2}
			if i == 0 {
				inset.Left = max(0, v.appearance.ContentPaddingHorizontal)
			}
			if i == len(preview.messages)-1 {
				inset.Right = max(0, v.appearance.ContentPaddingHorizontal)
			}
			dims := v.bandWithInset(gtx, height, v.appearance.Colors.Footer, inset, func(gtx layout.Context) layout.Dimensions {
				return v.label(gtx, label, v.appearance.SmallTextSize, fg, text.Start)
			}, false)
			// Match the steady top edge of a real footer notification.
			paint.FillShape(gtx.Ops, fg, clip.Rect{Max: image.Pt(dims.Size.X, gtx.Dp(2))}.Op())
			return dims
		}))
	}
	return layout.Flex{}.Layout(gtx, children...)
}
