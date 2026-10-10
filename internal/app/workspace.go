package app

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

const rowHeight unit.Dp = 28

var colors = struct {
	surface, header, border, foreground, muted, accent, selected, filename, filesize color.NRGBA
}{
	surface: rgb(0xffffff), header: rgb(0xffffff), border: rgb(0x8c8c8c),
	foreground: rgb(0x111111), muted: rgb(0x333333), accent: rgb(0x0057d9),
	selected: rgb(0x0057d9),
	filename: rgb(0x0057d9), filesize: rgb(0x9d7300),
}

type entry struct {
	name, size string
}

type panel struct {
	path     string
	items    []entry
	rows     []widget.Clickable
	list     widget.List
	selected int
	reveal   bool
}

type view struct {
	theme   *material.Theme
	panels  []panel
	active  int
	focus   bool
	started bool
}

func newView() *view {
	theme := material.NewTheme()
	faces := bundledFont()
	theme.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(faces))
	theme.Face = faces[0].Font.Typeface
	theme.Fg, theme.Bg = colors.foreground, colors.surface
	theme.ContrastBg, theme.ContrastFg = colors.accent, colors.surface
	v := &view{theme: theme, panels: samplePanels()}
	for i := range v.panels {
		p := &v.panels[i]
		p.rows = make([]widget.Clickable, len(p.items))
		p.list.Axis = layout.Vertical
	}
	return v
}

func (v *view) update(gtx layout.Context) {
	if !v.started {
		gtx.Execute(key.FocusCmd{Tag: &v.focus})
		v.started = gtx.Focused(&v.focus)
	}
	for pi := range v.panels {
		p := &v.panels[pi]
		for ri := range p.rows {
			for p.rows[ri].Clicked(gtx) {
				v.active, p.selected = pi, ri
				p.reveal = true
				gtx.Execute(key.FocusCmd{Tag: &v.focus})
			}
		}
	}
	for {
		e, ok := gtx.Event(
			key.FocusFilter{Target: &v.focus},
			key.Filter{Focus: &v.focus, Name: key.NameUpArrow},
			key.Filter{Focus: &v.focus, Name: key.NameDownArrow},
			key.Filter{Focus: &v.focus, Name: key.NameLeftArrow},
			key.Filter{Focus: &v.focus, Name: key.NameRightArrow},
			key.Filter{Focus: &v.focus, Name: key.NameHome},
			key.Filter{Focus: &v.focus, Name: key.NameEnd},
			key.Filter{Focus: &v.focus, Name: key.NamePageUp},
			key.Filter{Focus: &v.focus, Name: key.NamePageDown},
		)
		if !ok {
			break
		}
		ke, ok := e.(key.Event)
		if !ok || ke.State != key.Press {
			continue
		}
		p := &v.panels[v.active]
		next := p.selected
		page := max(1, p.list.Position.Count-1)
		switch ke.Name {
		case key.NameLeftArrow:
			v.active = max(0, v.active-1)
			continue
		case key.NameRightArrow:
			v.active = min(len(v.panels)-1, v.active+1)
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
		p.selected = max(0, min(next, len(p.items)-1))
		p.reveal = true
	}
}

func (v *view) layout(gtx layout.Context) layout.Dimensions {
	v.update(gtx)
	paint.Fill(gtx.Ops, colors.surface)
	event.Op(gtx.Ops, &v.focus)
	children := make([]layout.FlexChild, 0, len(v.panels)*2-1)
	for i := range v.panels {
		if i > 0 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				size := image.Pt(gtx.Dp(1), gtx.Constraints.Max.Y)
				paint.FillShape(gtx.Ops, colors.border, clip.Rect{Max: size}.Op())
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
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.band(gtx, 40, colors.header, func(gtx layout.Context) layout.Dimensions {
				fg := colors.foreground
				if index == v.active {
					fg = colors.accent
				}
				return v.label(gtx, fmt.Sprintf("%d  %s", index+1, p.path), 13, fg, text.Start)
			}, index == v.active)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.band(gtx, 28, colors.surface, func(gtx layout.Context) layout.Dimensions {
				return v.columns(gtx, entry{"NAME", "SIZE"}, colors.filename, colors.filesize, 12)
			}, false)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if p.reveal {
				h := gtx.Dp(rowHeight)
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
			list.Indicator.Color = colors.muted
			list.Indicator.HoverColor = colors.muted
			return list.Layout(gtx, len(p.items), func(gtx layout.Context, row int) layout.Dimensions {
				return p.rows[row].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(rowHeight))
					bg := colors.surface
					nameColor, sizeColor := colors.filename, colors.filesize
					if row == p.selected && index == v.active {
						bg = colors.selected
						nameColor, sizeColor = colors.surface, rgb(0xffdf00)
					}
					paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
					if row == p.selected && index != v.active {
						// Keep the last selection visible without an active-row fill.
						paint.FillShape(gtx.Ops, colors.accent,
							clip.Rect{Max: image.Pt(gtx.Dp(3), size.Y)}.Op())
					}
					gtx.Constraints = layout.Exact(size)
					return layout.Inset{Top: 6, Bottom: 6, Left: 14, Right: 14}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return v.columns(gtx, p.items[row], nameColor, sizeColor, 13)
						})
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.band(gtx, 30, colors.header, func(gtx layout.Context) layout.Dimensions {
				return v.columns(gtx, entry{fmt.Sprintf("%d items", len(p.items)),
					fmt.Sprintf("%d / %d", p.selected+1, len(p.items))}, colors.foreground, colors.muted, 12)
			}, false)
		}),
	)
}

func (v *view) band(gtx layout.Context, height unit.Dp, bg color.NRGBA, content layout.Widget, active bool) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(height))
	paint.FillShape(gtx.Ops, bg, clip.Rect{Max: size}.Op())
	paint.FillShape(gtx.Ops, colors.border,
		clip.Rect{Min: image.Pt(0, size.Y-gtx.Dp(1)), Max: size}.Op())
	if active {
		paint.FillShape(gtx.Ops, colors.accent, clip.Rect{Max: image.Pt(size.X, gtx.Dp(2))}.Op())
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

func rgb(value uint32) color.NRGBA {
	return color.NRGBA{R: uint8(value >> 16), G: uint8(value >> 8), B: uint8(value), A: 255}
}
