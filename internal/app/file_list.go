package app

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget/material"
)

func (v *view) panel(gtx layout.Context, index int) layout.Dimensions {
	p := &v.panels[index]
	textHeight := v.textHeight(gtx, v.appearance.TextSize)
	smallTextHeight := v.textHeight(gtx, v.appearance.SmallTextSize)
	rowHeight := max(1, textHeight+2*gtx.Dp(max(0, v.appearance.RowPaddingVertical)))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.headerBand(gtx, v.locationBarHeight(gtx, textHeight), v.appearance.Colors.Header, func(gtx layout.Context) layout.Dimensions {
				fg := v.appearance.Colors.Foreground
				if index == v.active {
					fg = v.appearance.Colors.ActiveLocation
				}
				if v.locationEditing() && index == v.active {
					return v.textEditor(&v.location, "Path").Layout(gtx)
				}
				return v.label(gtx, fmt.Sprintf("%d  %s", index+1, p.locationLabel()), v.appearance.TextSize, fg, text.Start)
			}, index == v.active)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.band(gtx, max(gtx.Dp(28), smallTextHeight+gtx.Dp(2*max(0, v.appearance.HeaderFooterPaddingVertical))), v.appearance.Colors.Surface, func(gtx layout.Context) layout.Dimensions {
				if p.filter != "" || v.filterInteraction() != nil && v.filterInteraction().panel == index {
					return v.filterContents(gtx, index)
				}
				return v.columns(gtx, entry{name: "NAME", size: p.metadata.heading()}, v.appearance.Colors.Filename, v.appearance.Colors.Metadata, v.appearance.SmallTextSize)
			}, false)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			transient, extra := v.transientRows(index)
			count := len(p.items) + extra
			if count == 0 {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}
			if p.reveal || v.inlineInteraction() != nil && v.inlineInteraction().panel == index {
				selected := p.selected
				end := selected + 1
				if transient >= 0 {
					selected, end = transient, transient+extra
					if a := v.inlineInteraction(); a != nil && a.panel == index {
						if a.subjectRow >= 0 {
							selected = a.subjectRow
						}
						if (end-selected)*rowHeight > gtx.Constraints.Max.Y {
							selected = transient + len(a.headers) + a.selected
							end = selected + 1
						}
					}
				}
				h := rowHeight
				top := p.list.Position.First*h + p.list.Position.Offset
				if y := selected * h; y < top {
					p.list.ScrollTo(selected)
				} else if bottom := end * h; bottom > top+gtx.Constraints.Max.Y {
					top = max(0, bottom-gtx.Constraints.Max.Y)
					p.list.Position.First, p.list.Position.Offset = top/h, top%h
				}
				p.reveal = false
			}
			list := material.List(v.theme, &p.list)
			list.AnchorStrategy = material.Occupy
			list.Indicator.MinorWidth = 3
			list.Indicator.Color = v.appearance.Colors.Scrollbar
			list.Indicator.HoverColor = v.appearance.Colors.ScrollbarHover
			list.Track.Color = v.appearance.Colors.ScrollbarTrack
			dims := list.Layout(gtx, count, func(gtx layout.Context, row int) layout.Dimensions {
				storedRow := v.storedRow(index, row)
				naming := v.nameInteraction() != nil && !v.nameInteraction().finished && v.nameInteraction().panel == index && v.nameInteraction().row == row
				if p.rows[row] == nil {
					p.rows[row] = new(fileRow)
				}
				if v.inlineActionRow(index, row) {
					return p.rows[row].Layout(gtx, func(gtx layout.Context) layout.Dimensions { return v.inlineActionContents(gtx, row, rowHeight) })
				}
				contents := func(gtx layout.Context) layout.Dimensions {
					size := image.Pt(gtx.Constraints.Max.X, rowHeight)
					bg := v.appearance.Colors.Surface
					nameColor, sizeColor := v.appearance.Colors.Filename, v.appearance.Colors.Metadata
					if naming {
						bg = v.appearance.Colors.NameRow
					} else {
						active := storedRow == p.selected && index == v.active && transient < 0 && !p.navigating
						bg, nameColor, sizeColor = v.appearance.entryColors(p.items[storedRow].file, active, v.cutEntry(index, storedRow))
					}
					paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
					if naming && v.nameInteraction().draft() {
						paint.FillShape(gtx.Ops, v.appearance.Colors.InlineEdge, clip.Rect{Max: image.Pt(gtx.Dp(3), size.Y)}.Op())
					} else if p.marks[storedRow] {
						paint.FillShape(gtx.Ops, v.appearance.Colors.Marked,
							clip.Rect{Max: image.Pt(gtx.Dp(3), size.Y)}.Op())
					} else if storedRow == p.selected && index != v.active {
						// Keep the last selection visible without an active-row fill.
						paint.FillShape(gtx.Ops, v.appearance.Colors.Edge,
							clip.Rect{Max: image.Pt(gtx.Dp(3), size.Y)}.Op())
					}
					gtx.Constraints = layout.Exact(size)
					if naming {
						metadata := ""
						if storedRow >= 0 {
							metadata = p.metadata.value(p.items[storedRow])
						}
						return v.nameContents(gtx, metadata, sizeColor)
					}
					return v.rowContents(gtx, p.items[storedRow], nameColor, sizeColor, p.metadata)
				}
				if naming {
					return contents(gtx)
				}
				return p.rows[row].Layout(gtx, contents)
			})
			// Keyboard menus anchor beside the revealed current row in client pixels.
			headerHeight := v.locationBarHeight(gtx, textHeight) + max(gtx.Dp(28), smallTextHeight+gtx.Dp(2*max(0, v.appearance.HeaderFooterPaddingVertical)))
			panelX := (v.frameSize.X + gtx.Dp(1)) * index / len(v.panels)
			y := (p.selected-p.list.Position.First)*rowHeight - p.list.Position.Offset
			p.selectionAnchor = image.Pt(panelX+gtx.Dp(max(0, v.appearance.ContentPaddingHorizontal)), headerHeight+max(0, min(y+rowHeight, dims.Size.Y)))
			// Keep row widgets only around the visible viewport.
			for row := range p.rows {
				if row < p.list.Position.First-1 || row > p.list.Position.First+p.list.Position.Count {
					delete(p.rows, row)
				}
			}
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := v.band(gtx, max(gtx.Dp(30), smallTextHeight+gtx.Dp(2*max(0, v.appearance.HeaderFooterPaddingVertical))), v.appearance.Colors.Footer, func(gtx layout.Context) layout.Dimensions {
				return v.footerContents(gtx, index)
			}, false)
			if message := p.feedback.Current(); message != nil {
				paint.FillShape(gtx.Ops, v.feedbackColor(message.Kind), clip.Rect{Max: image.Pt(dims.Size.X, gtx.Dp(2))}.Op())
			}
			return dims
		}),
	)
}

func (p *panel) positionLabel() string { return fmt.Sprintf("%d / %d", p.selected+1, len(p.items)) }

func (v *view) rowContents(gtx layout.Context, item entry, nameColor, sizeColor color.NRGBA, mode metadataColumn) layout.Dimensions {
	return v.contentInset().Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return v.columns(gtx, entry{name: mode.displayName(item), size: mode.value(item)}, nameColor, sizeColor, v.appearance.TextSize)
		})
	})
}
