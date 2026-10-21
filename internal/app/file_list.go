package app

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/widget/material"
	"github.com/dannyben/commando/internal/browser"
)

func (v *view) panel(gtx layout.Context, index int) layout.Dimensions {
	footerSource := gtx.Source
	if v.interaction.panelsBlocked() {
		gtx = gtx.Disabled()
	}
	p := &v.panels[index]
	preview := v.settingsPanelPreview(index)
	textHeight := v.textHeight(gtx, v.appearance.TextSize)
	smallTextHeight := v.textHeight(gtx, v.appearance.SmallTextSize)
	rowHeight := max(1, textHeight+2*gtx.Dp(max(0, v.appearance.RowPaddingVertical)))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.layoutLocationBar(gtx, index, v.locationBarHeight(gtx, textHeight))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			background, filename, metadata := v.metadataHeaderColors(index)
			metadataRole := contrastSecondary
			if v.followMe {
				metadataRole = contrastPrimary
			}
			return v.band(gtx, smallTextHeight+gtx.Dp(2*max(0, v.appearance.HeaderFooterPaddingVertical)), background, func(gtx layout.Context) layout.Dimensions {
				if preview.textSelection {
					return v.themePreviewSelection(gtx)
				}
				if p.filter != "" || v.filterInteraction() != nil && v.filterInteraction().panel == index {
					return v.filterContents(gtx, index)
				}
				heading := p.metadata.heading()
				if s := v.multiRenameInteraction(); s != nil && s.panel == index {
					heading = ""
				}
				if p.drives {
					heading = "FREE / TOTAL"
				}
				return v.columns(gtx, entry{name: "NAME", size: heading}, filename, metadata, v.appearance.SmallTextSize, metadataRole, false)
			}, false)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(preview.rows) > 0 {
				return preview.layoutRows(v, gtx, p.metadata, rowHeight)
			}
			transient, extra := v.transientRows(index)
			count := len(p.items) + extra
			if count == 0 {
				v.emptyFolderContents(gtx, index, rowHeight)
				p.layoutEmptySpace(gtx, 0, gtx.Constraints.Max.X)
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
					active, cut := false, false
					if naming {
						bg = v.appearance.Colors.NameRow
					} else {
						active = storedRow == p.selected && index == v.active && transient < 0 && !p.navigating
						state := p.rows[row].state(gtx, active)
						active = state.selectedAppearance()
						if s := v.multiRenameInteraction(); s != nil && s.panel == index {
							active = false
						}
						cut = v.cutEntry(index, storedRow)
						bg, nameColor, sizeColor = v.appearance.entryColors(p.items[storedRow].file, active, cut)
						bg = state.background(bg, v.appearance.HoverStrength)
					}
					edge := color.NRGBA{}
					if naming && v.nameInteraction().draft() {
						edge = v.appearance.Colors.InlineEdge
					} else if p.marks[storedRow] || (storedRow == p.selected && index != v.active) {
						edge = v.appearance.Colors.Edge
					}
					return v.fileRowSurface(gtx, size, bg, edge, func(gtx layout.Context) layout.Dimensions {
						if naming {
							metadata := ""
							item := entry{file: browser.Entry{Name: v.nameEditor.Text(), Directory: v.nameInteraction().noun() == "folder"}}
							if storedRow >= 0 {
								metadata = p.metadata.value(p.displayEntry(p.items[storedRow]))
								item = p.items[storedRow]
							}
							return v.withRowIcon(gtx, p, item, false, storedRow < 0, func(gtx layout.Context) layout.Dimensions {
								return v.nameContents(gtx, metadata, sizeColor)
							})
						}
						return v.withFileIcon(gtx, p, p.items[storedRow], cut, func(gtx layout.Context) layout.Dimensions {
							return v.multiRenameRow(gtx, index, p.items[storedRow], nameColor, sizeColor, v.appearance.entryTextRoles(p.items[storedRow].file, active, cut))
						})
					})
				}
				if naming {
					return contents(gtx)
				}
				return p.rows[row].Layout(gtx, contents)
			})
			bottom := max(0, (count-p.list.Position.First)*rowHeight-p.list.Position.Offset)
			p.layoutEmptySpace(gtx, bottom, max(0, dims.Size.X-gtx.Dp(list.Width())))
			// Keyboard menus anchor beside the revealed current row in client pixels.
			headerHeight := v.locationBarHeight(gtx, textHeight) + smallTextHeight + gtx.Dp(2*max(0, v.appearance.HeaderFooterPaddingVertical))
			y := (p.selected-p.list.Position.First)*rowHeight - p.list.Position.Offset
			p.selectionAnchor = image.Pt(gtx.Dp(max(0, v.appearance.ContentPaddingHorizontal)), headerHeight+max(0, min(y+rowHeight, dims.Size.Y)))
			// Keep row widgets only around the visible viewport.
			for row := range p.rows {
				if row < p.list.Position.First-1 || row > p.list.Position.First+p.list.Position.Count {
					delete(p.rows, row)
				}
			}
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// Footer redraw deadlines remain active while panel input is blocked.
			gtx.Source = footerSource
			if len(preview.messages) > 0 {
				return preview.footer(v, gtx, smallTextHeight+gtx.Dp(2*max(0, v.appearance.HeaderFooterPaddingVertical)))
			}
			dims := v.band(gtx, smallTextHeight+gtx.Dp(2*max(0, v.appearance.HeaderFooterPaddingVertical)), v.appearance.Colors.Footer, func(gtx layout.Context) layout.Dimensions {
				return v.footerContents(gtx, index)
			}, false)
			if message := p.feedback.Current(); message != nil {
				paint.FillShape(gtx.Ops, v.feedbackColor(message.Kind), clip.Rect{Max: image.Pt(dims.Size.X, gtx.Dp(2))}.Op())
			}
			return dims
		}),
	)
}

// A passive focus cue, never a file entry or an actionable row.
func (v *view) emptyFolderContents(gtx layout.Context, index, rowHeight int) {
	p := &v.panels[index]
	label := p.emptyFolderLabel()
	if label == "" {
		return
	}
	background := v.appearance.Colors.Surface
	if index == v.active {
		background = v.appearance.Colors.Selected
	}
	v.fileRowSurface(gtx, image.Pt(gtx.Constraints.Max.X, min(rowHeight, gtx.Constraints.Max.Y)), background, color.NRGBA{}, func(gtx layout.Context) layout.Dimensions {
		return v.contentInset().Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				return v.rawLabel(gtx, label, v.appearance.TextSize, v.hintColor(), text.Start)
			})
		})
	})
}

func (p *panel) emptyFolderLabel() string {
	if p.showParentEntry || p.drives || p.loading || p.err != nil || p.path == "" || len(p.items) != 0 {
		return ""
	}
	for _, file := range p.entries {
		if !file.Parent {
			if p.filter != "" {
				return "No matching entries"
			}
			return "No visible entries"
		}
	}
	return "Empty folder"
}

// Only the unused list area is a focus target. Rows, editors, transient actions,
// headings, footers and the scrollbar retain their own input handling.
func (p *panel) layoutEmptySpace(gtx layout.Context, top, width int) {
	if top >= gtx.Constraints.Max.Y || width <= 0 {
		return
	}
	if p.emptySpace == nil {
		p.emptySpace = new(fileRow)
	}
	defer op.Offset(image.Pt(0, top)).Push(gtx.Ops).Pop()
	gtx.Constraints = layout.Exact(image.Pt(width, gtx.Constraints.Max.Y-top))
	p.emptySpace.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Max}
	})
}

func (p *panel) positionLabel() string {
	position := p.selected + 1
	if len(p.items) > 0 && p.items[0].file.Parent {
		position = max(0, position-1)
	}
	if len(p.marks) > 0 {
		position = len(p.marks)
	}
	return fmt.Sprintf("%d / %d", position, p.itemCount())
}

// Shared by real listings and passive theme previews, including mark and
// inactive-selection bars. Input handling belongs to the surrounding file row.
func (v *view) fileRowSurface(gtx layout.Context, size image.Point, background, edge color.NRGBA, content layout.Widget) layout.Dimensions {
	paint.FillShape(gtx.Ops, background, clip.Rect{Max: size}.Op())
	if edge.A != 0 {
		paint.FillShape(gtx.Ops, edge, clip.Rect{Max: image.Pt(gtx.Dp(3), size.Y)}.Op())
	}
	gtx.Constraints = layout.Exact(size)
	return v.textOn(background, content)(gtx)
}

func (v *view) rowContents(gtx layout.Context, item entry, nameColor, sizeColor color.NRGBA, mode metadataColumn, roles fileTextRoles, drives bool) layout.Dimensions {
	return v.textAs(roles.filename, func(gtx layout.Context) layout.Dimensions {
		return v.contentInset().Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				metadata := mode.value(item)
				if drives {
					metadata = item.size
				}
				return v.columns(gtx, entry{name: mode.displayName(item), size: metadata}, nameColor, sizeColor, v.appearance.TextSize, roles.metadata, item.file.Directory && !item.file.Parent && !drives)
			})
		})
	})(gtx)
}
