package app

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type entry struct {
	name, size string
	file       browser.Entry
}

type panel struct {
	path      string // Last physical directory, retained while displaying drives.
	drives    bool
	items     []entry
	rows      map[int]*widget.Clickable
	list      widget.List
	selected  int
	marks     map[int]bool
	markRange *selectionRange
	metadata  metadataColumn
	reveal    bool
	loading   bool
	err       error
	prefix    string
	prefixAt  time.Time
	feedback  feedback.Channel
}

type view struct {
	appearance        Theme
	theme             *material.Theme
	panels            []panel
	active            int
	focus             bool
	started           bool
	loader            *directoryLoader
	openFile          func(string) error
	editing           bool
	location          widget.Editor
	renaming          *renameSession
	renameEditor      widget.Editor
	nativeKeys        *nativeKeyInput
	suppressSpaceText bool
	feedbackPointer   bool
}

func newPreviewView() *view {
	v := newView(samplePanels())
	for _, row := range []int{1, 3, 4} {
		v.panels[0].toggleMark(row)
	}
	for _, row := range []int{2, 5} {
		v.panels[1].toggleMark(row)
	}
	return v
}

func newView(panels []panel) *view {
	appearance := defaultTheme
	v := &view{appearance: appearance, theme: appearance.materialTheme(), panels: panels,
		location:     widget.Editor{SingleLine: true, Submit: true},
		renameEditor: widget.Editor{SingleLine: true, Submit: true},
	}
	for i := range v.panels {
		p := &v.panels[i]
		p.rows = make(map[int]*widget.Clickable)
		p.list.Axis = layout.Vertical
		p.metadata = metadataSize
	}
	return v
}

func (v *view) update(gtx layout.Context) {
	v.updateFeedbackInput(gtx)
	defer v.dismissUnhandledFeedbackKeys(gtx)
	v.pollLoads()
	if !v.started {
		gtx.Execute(key.FocusCmd{Tag: &v.focus})
		v.started = gtx.Focused(&v.focus)
	}
	for pi := range v.panels {
		p := &v.panels[pi]
		for ri, row := range p.rows {
			for {
				click, ok := row.Update(gtx)
				if !ok {
					break
				}
				v.dismissFeedback()
				if v.renaming != nil {
					if v.renaming.pending {
						continue
					}
					v.stopRename(gtx)
				}
				v.stopLocation(gtx)
				v.panels[v.active].prefix = ""
				v.active = pi
				p.clickRow(ri, click.Modifiers&key.ModShift != 0, click.Modifiers&key.ModCtrl != 0)
				p.prefix = ""
				p.reveal, p.err = true, nil
				gtx.Execute(key.FocusCmd{Tag: &v.focus})
			}
		}
	}
	v.expirePrefix(gtx)
	// Windows Insert uses the native bridge; route Shift+Insert to editor paste.
	for _, ke := range v.nativeKeys.drain() {
		if ke.State == key.Press {
			v.dismissFeedback()
		}
		if editor := v.activeEditor(); editor != nil {
			pasteWithInsert(gtx, editor, ke)
		} else {
			v.browseKey(gtx, ke)
		}
	}
	if v.editing || v.renaming != nil {
		// Consume Tab in the editor without changing its text or focus.
		for {
			e, ok := gtx.Event(key.Filter{Name: key.NameTab, Optional: key.ModShift})
			if !ok {
				break
			}
			if ke, ok := e.(key.Event); ok && ke.State == key.Press {
				v.dismissFeedback()
			}
		}
		if v.renaming != nil {
			v.updateEditorPaste(gtx, &v.renameEditor)
			v.updateRename(gtx)
			if v.renaming != nil {
				return
			}
		} else {
			v.updateEditorPaste(gtx, &v.location)
			v.updateLocation(gtx)
		}
		if v.editing {
			return
		}
	}
	for {
		e, ok := gtx.Event(
			key.FocusFilter{Target: &v.focus},
			// Consume Tab alongside browsing keys to retain their event order.
			key.Filter{Name: key.NameTab, Optional: key.ModShift},
			key.Filter{Focus: &v.focus, Name: "D", Required: key.ModAlt},
			key.Filter{Focus: &v.focus, Name: key.NameF2},
			key.Filter{Focus: &v.focus, Name: key.NameEscape},
			key.Filter{Focus: &v.focus, Name: key.NameUpArrow, Optional: key.ModShift},
			key.Filter{Focus: &v.focus, Name: key.NameDownArrow, Optional: key.ModShift},
			key.Filter{Focus: &v.focus, Name: insertKey},
			key.Filter{Focus: &v.focus, Name: key.NameSpace},
			key.Filter{Focus: &v.focus, Name: "A", Required: key.ModCtrl},
			key.Filter{Focus: &v.focus, Name: "M", Required: key.ModCtrl, Optional: key.ModShift},
			key.Filter{Focus: &v.focus, Name: "R", Required: key.ModCtrl},
			key.Filter{Focus: &v.focus, Name: key.NameLeftArrow},
			key.Filter{Focus: &v.focus, Name: key.NameRightArrow},
			key.Filter{Focus: &v.focus, Name: key.NameLeftArrow, Required: key.ModCtrl},
			key.Filter{Focus: &v.focus, Name: key.NameRightArrow, Required: key.ModCtrl},
			key.Filter{Focus: &v.focus, Name: key.NameHome},
			key.Filter{Focus: &v.focus, Name: key.NameEnd},
			key.Filter{Focus: &v.focus, Name: key.NamePageUp},
			key.Filter{Focus: &v.focus, Name: key.NamePageDown},
			key.Filter{Focus: &v.focus, Name: key.NameReturn},
			key.Filter{Focus: &v.focus, Name: key.NameEnter},
			key.Filter{Focus: &v.focus, Name: key.NameDeleteBackward},
		)
		if !ok {
			break
		}
		if edit, ok := e.(key.EditEvent); ok {
			v.dismissFeedback()
			if v.suppressSpaceText {
				v.suppressSpaceText = false
				if edit.Text == " " {
					continue
				}
			}
			v.typeToSelect(edit.Text, gtx.Now)
			continue
		}
		ke, ok := e.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		v.browseKey(gtx, ke)
		if v.editing || v.renaming != nil {
			return
		}
	}
	v.expirePrefix(gtx)
}

func (v *view) browseKey(gtx layout.Context, ke key.Event) {
	if ke.State != key.Press {
		return
	}
	v.dismissFeedback()
	v.suppressSpaceText = false
	p := &v.panels[v.active]
	next := p.selected
	page := max(1, p.list.Position.Count-1)
	switch ke.Name {
	case key.NameF2:
		v.startRename(gtx)
		return
	case key.NameTab:
		direction := 1
		if ke.Modifiers&key.ModShift != 0 {
			direction = -1
		}
		v.switchPanel(direction)
		return
	case "M":
		direction := 1
		if ke.Modifiers&key.ModShift != 0 {
			direction = -1
		}
		p.metadata = p.metadata.cycle(direction)
		return
	case "D":
		v.startLocation(gtx)
		return
	case "R":
		v.refreshPanel()
		return
	case key.NameEscape:
		if p.prefix != "" {
			p.prefix = ""
		} else {
			p.clearMarks()
		}
		return
	case "A":
		p.prefix = ""
		p.markAll()
		return
	case key.NameSpace:
		if p.prefix == "" {
			p.toggleMark(p.selected)
			v.suppressSpaceText = true
		}
		return
	case insertKey:
		if ke.Modifiers != 0 || p.loading {
			return
		}
		p.toggleMark(p.selected)
		next++
	case key.NameLeftArrow, key.NameRightArrow:
		direction := 1
		if ke.Name == key.NameLeftArrow {
			direction = -1
		}
		if ke.Modifiers&key.ModCtrl != 0 {
			v.openAdjacent(direction)
		} else {
			v.switchPanel(direction)
		}
		return
	case key.NameReturn, key.NameEnter:
		p.prefix = ""
		v.activateSelected()
		return
	case key.NameDeleteBackward:
		p.prefix = ""
		v.parentFolder()
		return
	case key.NameUpArrow:
		next--
	case key.NameDownArrow:
		next++
	case key.NameHome:
		next = 0
	case key.NameEnd:
		next = len(p.items) - 1
	case key.NamePageUp:
		next -= page
	case key.NamePageDown:
		next += page
	default:
		return
	}
	p.prefix = ""
	if len(p.items) == 0 {
		return
	}
	next = max(0, min(next, len(p.items)-1))
	if ke.Modifiers&key.ModShift != 0 {
		p.markTo(next)
	} else {
		p.markRange = nil
	}
	p.selected = next
	p.reveal, p.err = true, nil
}

func (v *view) switchPanel(direction int) {
	v.panels[v.active].prefix = ""
	v.active = max(0, min(len(v.panels)-1, v.active+direction))
}

func (v *view) layout(gtx layout.Context) layout.Dimensions {
	v.update(gtx)
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
			return v.panel(gtx, i)
		}))
	}
	dims := layout.Flex{}.Layout(gtx, children...)
	area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	pass := pointer.PassOp{}.Push(gtx.Ops)
	event.Op(gtx.Ops, &v.feedbackPointer)
	pass.Pop()
	area.Pop()
	return dims
}

func (v *view) panel(gtx layout.Context, index int) layout.Dimensions {
	p := &v.panels[index]
	textHeight := v.textHeight(gtx, v.appearance.TextSize)
	smallTextHeight := v.textHeight(gtx, v.appearance.SmallTextSize)
	rowHeight := max(1, textHeight+2*gtx.Dp(max(0, v.appearance.RowPaddingVertical)))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.band(gtx, max(gtx.Dp(40), textHeight+gtx.Dp(12)), v.appearance.Colors.Header, func(gtx layout.Context) layout.Dimensions {
				fg := v.appearance.Colors.Foreground
				if index == v.active {
					fg = v.appearance.Colors.ActiveLocation
				}
				if v.editing && index == v.active {
					return v.textEditor(&v.location, "Path").Layout(gtx)
				}
				return v.label(gtx, fmt.Sprintf("%d  %s", index+1, p.locationLabel()), v.appearance.TextSize, fg, text.Start)
			}, index == v.active)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.band(gtx, max(gtx.Dp(28), smallTextHeight+gtx.Dp(12)), v.appearance.Colors.Surface, func(gtx layout.Context) layout.Dimensions {
				return v.columns(gtx, entry{name: "NAME", size: p.metadata.heading()}, v.appearance.Colors.Filename, v.appearance.Colors.Metadata, v.appearance.SmallTextSize, v.metadataWidth(gtx, p.metadata))
			}, false)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(p.items) == 0 {
				return layout.Dimensions{Size: gtx.Constraints.Max}
			}
			if p.reveal {
				h := rowHeight
				top := p.list.Position.First*h + p.list.Position.Offset
				if y := p.selected * h; y < top {
					p.list.ScrollTo(p.selected)
				} else if bottom := (p.selected + 1) * h; bottom > top+gtx.Constraints.Max.Y {
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
			dims := list.Layout(gtx, len(p.items), func(gtx layout.Context, row int) layout.Dimensions {
				if p.rows[row] == nil {
					p.rows[row] = new(widget.Clickable)
				}
				contents := func(gtx layout.Context) layout.Dimensions {
					size := image.Pt(gtx.Constraints.Max.X, rowHeight)
					bg := v.appearance.Colors.Surface
					nameColor, sizeColor := v.appearance.Colors.Filename, v.appearance.Colors.Metadata
					renaming := v.renaming != nil && !v.renaming.finished && v.renaming.panel == index && v.renaming.row == row
					if renaming {
						bg = v.appearance.Colors.RenameRow
					} else if row == p.selected && index == v.active {
						bg = v.appearance.Colors.Selected
						nameColor, sizeColor = v.appearance.Colors.SelectedName, v.appearance.Colors.SelectedMetadata
					}
					paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
					if p.marks[row] {
						paint.FillShape(gtx.Ops, v.appearance.Colors.Marked,
							clip.Rect{Max: image.Pt(gtx.Dp(3), size.Y)}.Op())
					} else if row == p.selected && index != v.active {
						// Keep the last selection visible without an active-row fill.
						paint.FillShape(gtx.Ops, v.appearance.Colors.Edge,
							clip.Rect{Max: image.Pt(gtx.Dp(3), size.Y)}.Op())
					}
					gtx.Constraints = layout.Exact(size)
					if renaming {
						return v.renameContents(gtx, p.items[row], sizeColor, p.metadata)
					}
					return v.rowContents(gtx, p.items[row], nameColor, sizeColor, p.metadata)
				}
				if v.renaming != nil && !v.renaming.finished && v.renaming.panel == index && v.renaming.row == row {
					return contents(gtx)
				}
				return p.rows[row].Layout(gtx, contents)
			})
			// Keep row widgets only around the visible viewport.
			for row := range p.rows {
				if row < p.list.Position.First-1 || row > p.list.Position.First+p.list.Position.Count {
					delete(p.rows, row)
				}
			}
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.band(gtx, max(gtx.Dp(30), smallTextHeight+gtx.Dp(12)), v.appearance.Colors.Footer, func(gtx layout.Context) layout.Dimensions {
				return v.footerContents(gtx, index)
			}, false)
		}),
	)
}

func (p *panel) positionLabel() string { return fmt.Sprintf("%d / %d", p.selected+1, len(p.items)) }

func (v *view) rowContents(gtx layout.Context, item entry, nameColor, sizeColor color.NRGBA, mode metadataColumn) layout.Dimensions {
	padding := max(0, v.appearance.RowPaddingHorizontal)
	return layout.Inset{Left: padding, Right: padding}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return v.columns(gtx, entry{name: item.name, size: mode.value(item)}, nameColor, sizeColor, v.appearance.TextSize, v.metadataWidth(gtx, mode))
		})
	})
}

func (v *view) band(gtx layout.Context, height int, bg color.NRGBA, content layout.Widget, active bool) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, height)
	paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
	paint.FillShape(gtx.Ops, v.appearance.Colors.Border,
		clip.Rect{Min: image.Pt(0, size.Y-gtx.Dp(1)), Max: size}.Op())
	if active {
		paint.FillShape(gtx.Ops, v.appearance.Colors.Edge, clip.Rect{Max: image.Pt(size.X, gtx.Dp(2))}.Op())
	}
	gtx.Constraints = layout.Exact(size)
	layout.Inset{Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return content(gtx)
		})
	})
	return layout.Dimensions{Size: size}
}

func (v *view) columns(gtx layout.Context, item entry, fg, secondary color.NRGBA, size unit.Sp, width int) layout.Dimensions {
	return v.columnLayout(gtx, width, func(gtx layout.Context) layout.Dimensions {
		return v.label(gtx, item.name, size, fg, text.Start)
	}, func(gtx layout.Context) layout.Dimensions {
		return v.label(gtx, item.size, size, secondary, text.End)
	})
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

func (v *view) label(gtx layout.Context, value string, size unit.Sp, fg color.NRGBA, alignment text.Alignment) layout.Dimensions {
	label := material.Label(v.theme, size, value)
	label.Color, label.Alignment, label.MaxLines = fg, alignment, 1
	return label.Layout(gtx)
}

// Measure an unconstrained line using the same font and layout as the rows.
// Discard its drawing operations; only the height is needed.
func (v *view) textHeight(gtx layout.Context, size unit.Sp) int {
	gtx.Constraints = layout.Constraints{Max: image.Pt(1_000_000, 1_000_000)}
	macro := op.Record(gtx.Ops)
	dims := v.label(gtx, "Mg", size, v.appearance.Colors.Foreground, text.Start)
	macro.Stop()
	return dims.Size.Y
}
