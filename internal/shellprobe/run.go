// Package shellprobe provides a standalone Gio file list for desktop tests of
// native Windows dragging and shell menus, without changing Commando's UI.
package shellprobe

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"time"

	gioapp "gioui.org/app"
	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/gpu/headless"
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
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/windowshell"
)

type request struct {
	kind     string
	paths    []string
	extended bool
}

type outcome struct {
	request      request
	result       windowshell.Result
	directory    browser.Directory
	err, readErr error
}

type fileRow struct {
	pressed bool
	button  pointer.Buttons
	origin  f32.Point
	id      pointer.ID
}

type view struct {
	theme         *material.Theme
	root          string
	files         fileList
	rows          map[string]*fileRow
	list          widget.List
	open, refresh widget.Clickable
	focus         bool
	busy          bool
	status        string
	pending       *request
	logger        *log.Logger
}

func newView(root string, entries []browser.Entry, logger *log.Logger) *view {
	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(gofont.Collection()))
	v := &view{root: root, theme: theme, rows: make(map[string]*fileRow), logger: logger,
		status: "Ready. Drag a row out, or right-click it."}
	v.list.Axis = layout.Vertical
	v.files.replace(entries)
	return v
}

func Run() error {
	preview := flag.String("png", "", "render the demo to a PNG and exit")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("this demo creates its own sample folder; no arguments required")
	}
	if *preview != "" {
		return renderPreview(*preview)
	}
	root, err := sampleFiles()
	if err != nil {
		return err
	}
	dir, err := browser.ReadDirectory(filepath.Join(root, "source"))
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(root, "session.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	logger := log.New(logFile, "", log.Ldate|log.Ltime|log.Lmicroseconds)
	logger.Printf("START sample folder=%q; desktop outcomes require manual inspection", root)
	go func() {
		w := new(gioapp.Window)
		w.Option(gioapp.Title("Commando shell probe"), gioapp.Size(unit.Dp(760), unit.Dp(540)),
			gioapp.MinSize(unit.Dp(600), unit.Dp(420)))
		v := newView(root, dir.Entries, logger)
		var ops op.Ops
		var native nativeBridge
		var hwnd uintptr
		results := make(chan outcome, 1)
		done := make(chan struct{})
		for {
			e := w.Event()
			if handle := nativeHandle(e); handle != 0 {
				hwnd = handle
			}
			switch e := e.(type) {
			case gioapp.DestroyEvent:
				close(done)
				logger.Printf("CLOSE error=%v", e.Err)
				logFile.Close()
				os.Exit(0)
			case gioapp.FrameEvent:
				select {
				case result := <-results:
					v.complete(result)
				default:
				}
				v.layout(gioapp.NewContext(&ops, e))
				e.Frame(&ops)
				if v.pending != nil {
					req := *v.pending
					v.pending = nil
					windowHandle := hwnd
					go func() {
						out := outcome{request: req}
						switch req.kind {
						case "open":
							out.err = browser.OpenFile(root)
						case "refresh":
						default:
							out.result, out.err = native.perform(w, windowHandle, req)
						}
						out.directory, out.readErr = browser.ReadDirectory(filepath.Join(root, "source"))
						select {
						case results <- out:
							w.Invalidate()
						case <-done:
						}
					}()
				}
			}
		}
	}()
	gioapp.Main()
	return nil
}

func (v *view) start(kind string, extended bool) {
	if v.busy {
		return
	}
	paths := v.files.paths(filepath.Join(v.root, "source"))
	if (kind == "drag" || kind == "menu") && len(paths) == 0 {
		return
	}
	v.pending = &request{kind: kind, paths: paths, extended: extended}
	v.busy = true
	v.status = fmt.Sprintf("%s: %d selected item(s)", kind, len(paths))
	v.logger.Printf("BEGIN kind=%s paths=%q extended=%t", kind, paths, extended)
}

func (v *view) complete(out outcome) {
	v.busy = false
	if out.readErr == nil {
		v.files.replace(out.directory.Entries)
	}
	for _, row := range v.rows {
		row.pressed = false
	}
	r := out.result
	v.logger.Printf("END kind=%s action=%q cancelled=%t effect=0x%X commands=%d menuVersion=%d menuMessages=%d nativeThread=%d error=%v refreshError=%v",
		out.request.kind, r.Action, r.Cancelled, r.Effect, r.Commands, r.MenuVersion, r.MenuMessages, r.ThreadID, out.err, out.readErr)
	switch {
	case out.err != nil:
		v.status = "Failed: " + out.err.Error()
	case out.readErr != nil:
		v.status = "Refresh failed: " + out.readErr.Error()
	case r.Cancelled:
		v.status = "Cancelled. No command applied."
	case out.request.kind == "drag":
		v.status = fmt.Sprintf("Drag returned effect 0x%X. Check the destination.", r.Effect)
	case out.request.kind == "menu":
		v.status = fmt.Sprintf("%s returned. %d forwarded menu messages.", r.Action, r.MenuMessages)
	default:
		v.status = "Ready. Drag a row out, or right-click it."
	}
}

func (v *view) update(gtx layout.Context) {
	if !v.focus {
		gtx.Execute(key.FocusCmd{Tag: &v.focus})
		v.focus = gtx.Focused(&v.focus)
	}
	for v.open.Clicked(gtx) {
		v.start("open", false)
		gtx.Execute(key.FocusCmd{Tag: &v.focus})
	}
	for v.refresh.Clicked(gtx) {
		v.start("refresh", false)
		gtx.Execute(key.FocusCmd{Tag: &v.focus})
	}
	for {
		e, ok := gtx.Event(
			key.FocusFilter{Target: &v.focus},
			key.Filter{Focus: &v.focus, Name: key.NameUpArrow},
			key.Filter{Focus: &v.focus, Name: key.NameDownArrow},
			key.Filter{Focus: &v.focus, Name: key.NameSpace},
			key.Filter{Focus: &v.focus, Name: key.NameEscape},
			key.Filter{Focus: &v.focus, Name: "A", Required: key.ModCtrl},
			key.Filter{Focus: &v.focus, Name: "R", Required: key.ModCtrl},
			key.Filter{Focus: &v.focus, Name: "F10", Required: key.ModShift},
		)
		if !ok {
			break
		}
		ke, ok := e.(key.Event)
		if !ok || ke.State != key.Press || v.busy {
			continue
		}
		switch ke.Name {
		case key.NameUpArrow, key.NameDownArrow:
			if len(v.files.items) > 0 {
				delta := 1
				if ke.Name == key.NameUpArrow {
					delta = -1
				}
				index := min(max(v.files.index()+delta, 0), len(v.files.items)-1)
				v.files.current = v.files.items[index].Name
			}
		case key.NameSpace:
			v.files.selectItem(v.files.current, true)
		case key.NameEscape:
			clear(v.files.marks)
		case "A":
			v.files.marks = make(map[string]bool)
			for _, item := range v.files.items {
				v.files.marks[item.Name] = true
			}
		case "R":
			v.start("refresh", false)
		case "F10":
			v.start("menu", false)
		}
	}
	for name, row := range v.rows {
		for {
			e, ok := gtx.Event(pointer.Filter{Target: row, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel})
			if !ok {
				break
			}
			pe := e.(pointer.Event)
			if v.busy {
				row.pressed = false
				continue
			}
			switch pe.Kind {
			case pointer.Press:
				row.pressed, row.button, row.origin, row.id = true, pe.Buttons, pe.Position, pe.PointerID
				v.files.selectItem(name, pe.Modifiers&key.ModCtrl != 0 && pe.Buttons.Contain(pointer.ButtonPrimary))
				gtx.Execute(key.FocusCmd{Tag: &v.focus})
			case pointer.Drag:
				delta := pe.Position.Sub(row.origin)
				threshold := float32(gtx.Dp(6))
				if row.pressed && row.id == pe.PointerID && row.button.Contain(pointer.ButtonPrimary) && pe.Buttons.Contain(pointer.ButtonPrimary) &&
					delta.X*delta.X+delta.Y*delta.Y >= threshold*threshold {
					row.pressed = false
					v.start("drag", false)
				}
			case pointer.Release:
				if row.pressed && row.id == pe.PointerID && row.button.Contain(pointer.ButtonSecondary) {
					v.start("menu", pe.Modifiers&key.ModShift != 0)
				}
				row.pressed = false
			case pointer.Cancel:
				row.pressed = false
			}
		}
	}
}

func (v *view) layout(gtx layout.Context) layout.Dimensions {
	v.update(gtx)
	event.Op(gtx.Ops, &v.focus)
	paint.Fill(gtx.Ops, color.NRGBA{R: 250, G: 250, B: 250, A: 255})
	return layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(material.H6(v.theme, "Windows shell integration test").Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: 8, Bottom: 8}.Layout(gtx, material.Body2(v.theme,
					"Drag files to Explorer or another app. Right-click for the native shell menu.\nCtrl+click or Space marks items; Ctrl+A marks all; Shift+F10 opens the menu.").Layout)
			}),
			layout.Rigid(material.Body2(v.theme, "Test folder: "+v.root).Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: 8, Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					if v.busy {
						gtx = gtx.Disabled()
					}
					return layout.Flex{}.Layout(gtx,
						layout.Rigid(material.Button(v.theme, &v.open, "Open test folder").Layout),
						layout.Rigid(layout.Spacer{Width: 8}.Layout),
						layout.Rigid(material.Button(v.theme, &v.refresh, "Refresh").Layout),
					)
				})
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return material.List(v.theme, &v.list).Layout(gtx, len(v.files.items), func(gtx layout.Context, i int) layout.Dimensions {
					item := v.files.items[i]
					row := v.rows[item.Name]
					if row == nil {
						row = new(fileRow)
						v.rows[item.Name] = row
					}
					surface, fg := color.NRGBA{R: 255, G: 255, B: 255, A: 255}, color.NRGBA{A: 255}
					if item.Name == v.files.current {
						surface = color.NRGBA{R: 0, G: 85, B: 255, A: 255}
						fg = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
					}
					width := gtx.Constraints.Max.X
					gtx.Constraints.Min.X = width
					macro := op.Record(gtx.Ops)
					label := item.Name
					if item.Directory {
						label += "  (folder)"
					}
					style := material.Body1(v.theme, label)
					style.Color = fg
					dims := layout.Inset{Top: 10, Bottom: 10, Left: 14, Right: 10}.Layout(gtx, style.Layout)
					call := macro.Stop()
					area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
					paint.Fill(gtx.Ops, surface)
					if v.files.marks[item.Name] {
						paint.FillShape(gtx.Ops, color.NRGBA{R: 255, G: 160, B: 0, A: 255}, clip.Rect{Max: image.Pt(4, dims.Size.Y)}.Op())
					}
					call.Add(gtx.Ops)
					event.Op(gtx.Ops, row)
					area.Pop()
					return dims
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: 8}.Layout(gtx, material.Body2(v.theme, v.status+"\nResults: session.log in the test folder. Samples remain after closing.").Layout)
			}),
		)
	})
}

func renderPreview(path string) error {
	v := newView("TEMP / commando-shell-probe /", []browser.Entry{
		{Name: "Folder sample", Directory: true}, {Name: "alpha.txt"}, {Name: "café-日本語.txt"}, {Name: "image.png"}, {Name: "two words.txt"},
	}, log.New(os.Stderr, "", 0))
	v.files.marks = map[string]bool{"alpha.txt": true, "image.png": true}
	w, err := headless.NewWindow(760, 540)
	if err != nil {
		return err
	}
	defer w.Release()
	var ops op.Ops
	v.layout(layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(760, 540))})
	if err := w.Frame(&ops); err != nil {
		return err
	}
	img := image.NewRGBA(image.Rect(0, 0, 760, 540))
	if err := w.Screenshot(img); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = png.Encode(f, img)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
