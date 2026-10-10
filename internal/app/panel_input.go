package app

import (
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
)

const prefixDelay = time.Second

func (v *view) expirePrefix(gtx layout.Context) {
	p := &v.panels[v.active]
	if p.prefix == "" {
		return
	}
	until := p.prefixAt.Add(prefixDelay)
	if !gtx.Now.Before(until) {
		p.prefix = ""
	} else {
		gtx.Execute(op.InvalidateCmd{At: until})
	}
}

func (v *view) typeToSelect(text string, now time.Time) {
	p := &v.panels[v.active]
	text = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, text)
	if text == "" {
		return
	}
	if now.Sub(p.prefixAt) >= prefixDelay {
		p.prefix = ""
	}
	fresh := p.prefix == ""
	p.prefix += text
	p.prefixAt = now
	v.selectPrefix(fresh)
}

func (v *view) erasePrefix(now time.Time) {
	p := &v.panels[v.active]
	_, size := utf8.DecodeLastRuneInString(p.prefix)
	p.prefix = p.prefix[:len(p.prefix)-size]
	p.prefixAt = now
	if p.prefix != "" {
		v.selectPrefix(false)
	}
}

func (v *view) selectPrefix(fresh bool) {
	p := &v.panels[v.active]
	start := max(0, p.selected)
	if fresh {
		start++
	}
	prefix := strings.ToLower(p.prefix)
	for offset := 0; offset < len(p.items); offset++ {
		index := (start + offset) % len(p.items)
		item := p.items[index]
		name := item.file.Name
		if name == "" {
			name = strings.TrimSuffix(item.name, "/")
		}
		if !item.file.Parent && name != ".." && strings.HasPrefix(strings.ToLower(name), prefix) {
			p.selected, p.reveal, p.err = index, true, nil
			return
		}
	}
}

func (v *view) startLocation(gtx layout.Context) {
	p := &v.panels[v.active]
	p.prefix = ""
	v.location.SetText(p.path)
	v.location.SetCaret(0, utf8.RuneCountInString(p.path))
	v.editing = true
	gtx.Execute(key.FocusCmd{Tag: &v.location})
}

func (v *view) stopLocation(gtx layout.Context) {
	if !v.editing {
		return
	}
	v.editing = false
	gtx.Execute(key.FocusCmd{Tag: &v.focus})
}

func (v *view) updateLocation(gtx layout.Context) {
	for {
		e, ok := gtx.Event(
			key.Filter{Focus: &v.location, Name: key.NameEscape},
			key.Filter{Focus: &v.location, Name: "D", Required: key.ModAlt},
		)
		if !ok {
			break
		}
		if ke, ok := e.(key.Event); ok && ke.State == key.Press {
			if ke.Name == key.NameEscape {
				v.stopLocation(gtx)
				return
			}
			v.location.SetCaret(0, utf8.RuneCountInString(v.location.Text()))
		}
	}
	for {
		e, ok := v.location.Update(gtx)
		if !ok {
			return
		}
		if submit, ok := e.(widget.SubmitEvent); ok {
			path := submit.Text
			if path == "" || v.panels[v.active].loading {
				return
			}
			if !filepath.IsAbs(path) && filepath.VolumeName(path) == "" {
				base := v.panels[v.active].path
				if os.IsPathSeparator(path[0]) {
					path = filepath.VolumeName(base) + path
				} else {
					path = filepath.Join(base, path)
				}
			}
			v.stopLocation(gtx)
			v.load(v.active, path, "")
			return
		}
	}
}
