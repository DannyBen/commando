package app

import (
	"fmt"
	"image"
	"image/color"
	"time"

	"github.com/dannyben/commando/internal/browser"

	"gioui.org/io/event"
	"gioui.org/io/key"
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
	path     string // Last physical directory, retained while displaying drives.
	drives   bool
	items    []entry
	rows     map[int]*widget.Clickable
	list     widget.List
	selected int
	reveal   bool
	loading  bool
	err      error
	prefix   string
	prefixAt time.Time
}

type view struct {
	appearance Theme
	theme      *material.Theme
	panels     []panel
	active     int
	focus      bool
	started    bool
	loader     *directoryLoader
	openFile   func(string) error
	editing    bool
	location   widget.Editor
}

func newPreviewView() *view { return newView(samplePanels()) }

func newView(panels []panel) *view {
	appearance := defaultTheme
	v := &view{appearance: appearance, theme: appearance.materialTheme(), panels: panels,
		location: widget.Editor{SingleLine: true, Submit: true},
	}
	for i := range v.panels {
		p := &v.panels[i]
		p.rows = make(map[int]*widget.Clickable)
		p.list.Axis = layout.Vertical
	}
	return v
}

func (v *view) update(gtx layout.Context) {
	v.pollLoads()
	if !v.started {
		gtx.Execute(key.FocusCmd{Tag: &v.focus})
		v.started = gtx.Focused(&v.focus)
	}
	for pi := range v.panels {
		p := &v.panels[pi]
		for ri, row := range p.rows {
			for row.Clicked(gtx) {
				v.stopLocation(gtx)
				v.panels[v.active].prefix = ""
				v.active, p.selected = pi, ri
				p.prefix = ""
				p.reveal, p.err = true, nil
				gtx.Execute(key.FocusCmd{Tag: &v.focus})
			}
		}
	}
	v.expirePrefix(gtx)
	// Explicitly matching Tab prevents Gio from moving focus to a row widget.
	// Switch panels while browsing; keep focus in the editor while editing.
	for {
		e, ok := gtx.Event(key.Filter{Name: key.NameTab, Optional: key.ModShift})
		if !ok {
			break
		}
		if ke, ok := e.(key.Event); ok && ke.State == key.Press && !v.editing {
			direction := 1
			if ke.Modifiers&key.ModShift != 0 {
				direction = -1
			}
			v.switchPanel(direction)
		}
	}
	if v.editing {
		v.updateLocation(gtx)
		if v.editing {
			return
		}
	}
	for {
		e, ok := gtx.Event(
			key.FocusFilter{Target: &v.focus},
			key.Filter{Focus: &v.focus, Name: "D", Required: key.ModAlt},
			key.Filter{Focus: &v.focus, Name: key.NameEscape},
			key.Filter{Focus: &v.focus, Name: key.NameUpArrow},
			key.Filter{Focus: &v.focus, Name: key.NameDownArrow},
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
			v.typeToSelect(edit.Text, gtx.Now)
			continue
		}
		ke, ok := e.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		p := &v.panels[v.active]
		next := p.selected
		page := max(1, p.list.Position.Count-1)
		switch ke.Name {
		case "D":
			v.startLocation(gtx)
			return
		case key.NameEscape:
			p.prefix = ""
			continue
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
			continue
		case key.NameReturn, key.NameEnter:
			p.prefix = ""
			v.activateSelected()
			continue
		case key.NameDeleteBackward:
			p.prefix = ""
			v.parentFolder()
			continue
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
		}
		p.prefix = ""
		if len(p.items) == 0 {
			continue
		}
		p.selected = max(0, min(next, len(p.items)-1))
		p.reveal, p.err = true, nil
	}
	v.expirePrefix(gtx)
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
	return layout.Flex{}.Layout(gtx, children...)
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
					editor := material.Editor(v.theme, &v.location, "Path")
					editor.TextSize, editor.Color = v.appearance.TextSize, v.appearance.Colors.ActiveLocation
					editor.SelectionColor = v.appearance.Colors.TextSelection
					return editor.Layout(gtx)
				}
				return v.label(gtx, fmt.Sprintf("%d  %s", index+1, p.locationLabel()), v.appearance.TextSize, fg, text.Start)
			}, index == v.active)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.band(gtx, max(gtx.Dp(28), smallTextHeight+gtx.Dp(12)), v.appearance.Colors.Surface, func(gtx layout.Context) layout.Dimensions {
				return v.columns(gtx, entry{name: "NAME", size: "SIZE"}, v.appearance.Colors.Filename, v.appearance.Colors.Filesize, v.appearance.SmallTextSize)
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
				return p.rows[row].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					size := image.Pt(gtx.Constraints.Max.X, rowHeight)
					bg := v.appearance.Colors.Surface
					nameColor, sizeColor := v.appearance.Colors.Filename, v.appearance.Colors.Filesize
					if row == p.selected && index == v.active {
						bg = v.appearance.Colors.Selected
						nameColor, sizeColor = v.appearance.Colors.SelectedName, v.appearance.Colors.SelectedSize
					}
					paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
					if row == p.selected && index != v.active {
						// Keep the last selection visible without an active-row fill.
						paint.FillShape(gtx.Ops, v.appearance.Colors.Edge,
							clip.Rect{Max: image.Pt(gtx.Dp(3), size.Y)}.Op())
					}
					gtx.Constraints = layout.Exact(size)
					return v.rowContents(gtx, p.items[row], nameColor, sizeColor)
				})
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
				status, failed := p.status()
				if index == v.active && p.prefix != "" {
					status, failed = "> "+p.prefix, false
				}
				fg := v.appearance.Colors.Foreground
				if failed {
					fg = v.appearance.Colors.Error
				}
				return v.columns(gtx, entry{name: status,
					size: fmt.Sprintf("%d / %d", p.selected+1, len(p.items))}, fg, v.appearance.Colors.Muted, v.appearance.SmallTextSize)
			}, false)
		}),
	)
}

func (v *view) rowContents(gtx layout.Context, item entry, nameColor, sizeColor color.NRGBA) layout.Dimensions {
	padding := max(0, v.appearance.RowPaddingHorizontal)
	return layout.Inset{Left: padding, Right: padding}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return v.columns(gtx, item, nameColor, sizeColor, v.appearance.TextSize)
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

func (v *view) columns(gtx layout.Context, item entry, fg, secondary color.NRGBA, size unit.Sp) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return v.label(gtx, item.name, size, fg, text.Start)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width := gtx.Dp(84)
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			return v.label(gtx, item.size, size, secondary, text.End)
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
