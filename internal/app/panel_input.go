package app

import (
	"os"
	"path/filepath"
	"runtime"
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
	p.markRange = nil
	if now.Sub(p.prefixAt) >= prefixDelay {
		p.prefix = ""
	}
	fresh := p.prefix == ""
	p.prefix += text
	p.prefixAt = now
	v.selectPrefix(fresh)
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
			name = item.name
		}
		if !item.file.Parent && name != ".." && strings.HasPrefix(strings.ToLower(name), prefix) {
			p.selected, p.reveal, p.err = index, true, nil
			return
		}
	}
}

func (v *view) startLocation(gtx layout.Context) {
	if !v.interaction.begin(&locationSession{panel: v.active}) {
		return
	}
	p := &v.panels[v.active]
	p.prefix = ""
	path := p.locationPath()
	v.location.SetText(path)
	v.location.SetCaret(0, utf8.RuneCountInString(path))
	v.focusInteraction(gtx)
}

func (v *view) stopLocation(gtx layout.Context) {
	if !v.locationEditing() {
		return
	}
	v.interaction.finish(v.interaction.current)
	v.focusInteraction(gtx)
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
			v.dismissFeedback()
			if ke.Name == key.NameEscape {
				v.escape(gtx)
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
		if _, ok := e.(widget.ChangeEvent); ok {
			v.dismissFeedback()
		}
		if submit, ok := e.(widget.SubmitEvent); ok {
			v.dismissFeedback()
			path := submit.Text
			if path == "" || v.panels[v.active].loading {
				return
			}
			if runtime.GOOS == "windows" {
				path = normalizeDriveShortcut(path)
			}
			if !filepath.IsAbs(path) && filepath.VolumeName(path) == "" {
				base := v.panels[v.active].locationPath()
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

// Bare drive letters are explicit root shortcuts in the location editor.
// Other paths, including .\d and drive-relative D:folder, keep their meaning.
func normalizeDriveShortcut(path string) string {
	if len(path) != 1 && !(len(path) == 2 && path[1] == ':') {
		return path
	}
	letter := path[0]
	if letter >= 'a' && letter <= 'z' {
		letter -= 'a' - 'A'
	}
	if letter < 'A' || letter > 'Z' {
		return path
	}
	return string(letter) + `:\`
}
