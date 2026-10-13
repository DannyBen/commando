package app

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (v *view) layoutPalette(gtx layout.Context) layout.Dimensions {
	s := v.paletteInteraction()
	textHeight := v.textHeight(gtx, v.appearance.TextSize)
	list := material.List(v.theme, &s.list)
	list.AnchorStrategy = material.Occupy
	list.Indicator.MinorWidth = 3
	list.Indicator.Color, list.Indicator.HoverColor = v.appearance.Colors.Scrollbar, v.appearance.Colors.ScrollbarHover
	list.Track.Color = v.appearance.Colors.ScrollbarTrack
	// Measure the full catalogue so filtering never changes the surface width.
	labelWidth, shortcutWidth := 0, 0
	catalogue := s.commands
	if s.settingsMode {
		catalogue = s.settingsCommands
	}
	for _, command := range catalogue {
		labelWidth = max(labelWidth, v.textWidth(gtx, command.label(), v.appearance.TextSize))
		shortcutWidth = max(shortcutWidth, v.textWidth(gtx, command.shortcut(), v.appearance.SmallTextSize))
	}
	shortcutWidth += gtx.Dp(12)
	// Row insets, the border and the list's occupied scrollbar track.
	width := labelWidth + shortcutWidth + gtx.Dp(28+2) + gtx.Dp(list.Width())
	width = max(gtx.Dp(v.appearance.OverlayMinWidth), min(gtx.Dp(v.appearance.OverlayMaxWidth), width))
	return s.surface.layout(gtx, locationBarHeight(gtx, textHeight), width, v.appearance.Colors, v.appearance.OverlayBackdropOpacity, func(gtx layout.Context) layout.Dimensions {
		rowHeight := max(1, textHeight+2*gtx.Dp(max(0, v.appearance.RowPaddingVertical)))
		searchHeight := max(gtx.Dp(40), textHeight+gtx.Dp(12))
		footerHeight := max(gtx.Dp(28), v.textHeight(gtx, v.appearance.SmallTextSize)+gtx.Dp(12))
		// Leave the panel footers visible. Tall text still gets one usable row
		// when space permits; the outer clip bounds tiny windows safely.
		available := max(0, gtx.Constraints.Max.Y-searchHeight-footerHeight-gtx.Dp(32))
		listHeight := min(max(1, len(s.results))*rowHeight, max(rowHeight, available*3/4))
		listHeight = min(listHeight, max(0, gtx.Constraints.Max.Y-searchHeight-footerHeight))
		shortcutWidth := min(shortcutWidth, gtx.Constraints.Max.X/2)
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return v.band(gtx, searchHeight, v.appearance.Colors.Surface, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return v.label(gtx, "> ", v.appearance.TextSize, v.appearance.Colors.EditorText, text.Start)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return v.textEditor(&v.commandSearch, "Search commands").Layout(gtx)
						}),
					)
				}, true)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = listHeight, listHeight
				if len(s.results) == 0 {
					return layout.Inset{Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							message := "No commands found"
							if s.settingsMode {
								message = "No settings found"
							}
							return v.label(gtx, message, v.appearance.TextSize, v.appearance.Colors.Foreground, text.Start)
						})
					})
				}
				if s.reveal {
					top := s.list.Position.First*rowHeight + s.list.Position.Offset
					if y := s.selected * rowHeight; y < top {
						s.list.ScrollTo(s.selected)
					} else if bottom := (s.selected + 1) * rowHeight; bottom > top+listHeight {
						top = max(0, bottom-listHeight)
						s.list.Position.First, s.list.Position.Offset = top/rowHeight, top%rowHeight
					}
					s.reveal = false
				}
				return list.Layout(gtx, len(s.results), func(gtx layout.Context, row int) layout.Dimensions {
					if s.rows[row] == nil {
						s.rows[row] = new(widget.Clickable)
					}
					return s.rows[row].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						size := image.Pt(gtx.Constraints.Max.X, rowHeight)
						bg, fg := v.appearance.Colors.Surface, v.appearance.Colors.Foreground
						if row == s.selected {
							bg, fg = v.appearance.Colors.Selected, v.appearance.Colors.SelectedName
						}
						if row != s.selected && !s.results[row].command.allowed(v) {
							fg = v.appearance.Colors.Muted
						}
						paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
						gtx.Constraints = layout.Exact(size)
						return layout.Inset{Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								gtx.Constraints.Min.Y = 0
								result := s.results[row]
								return v.columnLayout(gtx, shortcutWidth, func(gtx layout.Context) layout.Dimensions {
									return v.highlightedLabel(gtx, result.command.label(), result.match.Indices, v.appearance.TextSize, fg)
								}, func(gtx layout.Context) layout.Dimensions {
									return v.label(gtx, result.command.shortcut(), v.appearance.SmallTextSize, fg, text.End)
								})
							})
						})
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return v.band(gtx, footerHeight, v.appearance.Colors.Footer, func(gtx layout.Context) layout.Dimensions {
					hint := "Enter Run   Esc Close"
					if s.settingsMode {
						hint = "Enter Edit   Esc Close"
					}
					if len(s.results) > 0 && !s.results[s.selected].command.allowed(v) {
						hint = "Unavailable here   Esc Close"
					}
					return v.label(gtx, hint, v.appearance.SmallTextSize, v.appearance.Colors.Foreground, text.Start)
				}, false)
			}),
		)
	})
}
