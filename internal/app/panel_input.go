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
	"github.com/dannyben/commando/internal/settings"
)

func (v *view) prefixResetDelay() time.Duration {
	seconds := v.settingsValues.Get(settings.CatalogueDefault, settingTypeToSelectDelay).Number
	return time.Duration(seconds * float64(time.Second))
}

func (v *view) expirePrefix(gtx layout.Context) {
	p := &v.panels[v.active]
	if p.prefix == "" {
		return
	}
	until := p.prefixAt.Add(v.prefixResetDelay())
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
	if now.Sub(p.prefixAt) >= v.prefixResetDelay() {
		p.prefix = ""
	}
	fresh := p.prefix == ""
	p.prefix += text
	p.prefixAt = now
	v.selectPrefix(fresh, 1)
}

func (v *view) selectPrefix(skipCurrent bool, direction int) {
	p := &v.panels[v.active]
	start := max(0, p.selected)
	if skipCurrent {
		start += direction
	}
	prefix := strings.ToLower(p.prefix)
	for offset := 0; offset < len(p.items); offset++ {
		index := (start + offset*direction + len(p.items)) % len(p.items)
		item := p.items[index]
		name := item.file.Name
		if name == "" {
			name = item.name
		}
		if !item.file.Parent && name != ".." && strings.HasPrefix(strings.ToLower(name), prefix) {
			p.selected, p.reveal, p.err = index, true, nil
			v.followSelection()
			return
		}
	}
}

func (v *view) startLocation(gtx layout.Context) {
	if !v.startInteraction(&locationSession{panel: v.active}) {
		return
	}
	p := &v.panels[v.active]
	p.prefix = ""
	path := p.locationPath()
	v.location.SetText(path)
	v.location.SetCaret(0, utf8.RuneCountInString(path))
	v.focusInteraction(gtx)
}

func (v *view) openLocationQuery(gtx layout.Context, query string) {
	if !v.locationEditing() {
		v.startLocation(gtx)
	}
	if !v.locationEditing() {
		return
	}
	v.location.SetText(query)
	v.location.SetCaret(utf8.RuneCountInString(query), utf8.RuneCountInString(query))
	v.focusInteraction(gtx)
}

func (v *view) stopLocation(gtx layout.Context) {
	if !v.locationEditing() {
		return
	}
	v.finishInteraction(v.interaction.current)
	v.focusInteraction(gtx)
}

func (v *view) updateLocation(gtx layout.Context) {
	var keys []key.Event
	for {
		e, ok := gtx.Event(
			key.Filter{Focus: &v.location, Name: key.NameTab, Optional: key.ModShift},
			key.Filter{Focus: &v.location, Name: key.NameUpArrow},
			key.Filter{Focus: &v.location, Name: key.NameDownArrow},
			key.Filter{Focus: &v.location, Name: key.NameReturn},
			key.Filter{Focus: &v.location, Name: key.NameEnter},
			key.Filter{Focus: &v.location, Name: key.NameEscape},
			key.Filter{Focus: &v.location, Name: "D", Required: key.ModAlt},
		)
		if !ok {
			break
		}
		if ke, ok := e.(key.Event); ok && ke.State == key.Press {
			if ke.Name == key.NameEscape {
				// Release ownership before the editor can consume subsequent browsing
				// keys from the same input batch.
				v.escape(gtx)
				return
			}
			keys = append(keys, ke)
			// Leave later events available for browsing after dismissal/navigation.
			if ke.Name == key.NameReturn || ke.Name == key.NameEnter {
				break
			}
		}
	}
	submitted := false
	for {
		e, ok := v.location.Update(gtx)
		if !ok {
			break
		}
		if _, ok := e.(widget.ChangeEvent); ok {
			v.dismissFeedback()
		}
		if _, ok := e.(widget.SubmitEvent); ok {
			submitted = true
		}
	}
	s := v.interaction.current.(*locationSession)
	searching := v.updateHistorySearch(gtx, s)
	if !searching {
		v.updateLocationCompletion()
	}
	for _, ke := range keys {
		v.dismissFeedback()
		switch ke.Name {
		case key.NameEscape:
			v.escape(gtx)
			return
		case key.NameReturn, key.NameEnter:
			if searching {
				v.submitHistorySearch(gtx, s)
			} else {
				v.submitLocation(gtx)
			}
			return
		case "D":
			v.location.SetCaret(0, utf8.RuneCountInString(v.location.Text()))
			v.updateLocationCompletion()
		case key.NameTab:
			if searching && ke.Modifiers == 0 {
				v.cycleHistorySearch(gtx, s)
			} else if !searching && ke.Modifiers == 0 && v.acceptLocationSuggestion() {
				v.updateLocationCompletion()
			}
		case key.NameUpArrow, key.NameDownArrow:
			if searching {
				continue
			}
			direction := 1
			if ke.Name == key.NameUpArrow {
				direction = -1
			}
			if s.completion.cycle(direction) {
				v.announceInteraction(eventSelectionChanged)
			}
		}
	}
	if submitted && searching {
		v.submitHistorySearch(gtx, s)
	} else if submitted {
		v.submitLocation(gtx)
	}
}

func (v *view) submitLocation(gtx layout.Context) {
	v.dismissFeedback()
	base := v.panels[v.active].locationPath()
	path := resolveLocationPath(base, v.location.Text())
	if path == "" || v.panels[v.active].loading {
		return
	}
	suggestedPath := ""
	favoriteName := ""
	if s, ok := v.interaction.current.(*locationSession); ok {
		s.completion.update(&v.location)
		if favoriteCompletionQuery(v.location.Text()) {
			favoriteName = v.location.Text()
		}
		if s.completion.suffix() != "" {
			favoriteName = ""
			candidate := s.completion.matches[s.completion.selected]
			suggestedPath = s.selectedFavoriteTarget()
			if suggestedPath == "" {
				suggestedPath = resolveLocationPath(base, candidate)
			}
		}
	}
	// Submission hands audio ownership to navigation; cancellation still uses
	// stopLocation and the shared close cue.
	v.clearDeepSearch(v.active)
	v.interaction.finish(v.interaction.current)
	v.focusInteraction(gtx)
	v.queueLoad(loadRequest{panel: v.active, path: path, suggestedPath: suggestedPath, favoriteName: favoriteName, favoritesDirectory: v.favoritesDirectory(), explicitPath: true})
}

func resolveLocationPath(base, text string) string {
	path := strings.TrimSpace(text)
	if len(path) >= 2 && path[0] == '"' && path[len(path)-1] == '"' {
		path = path[1 : len(path)-1]
	}
	path = expandLocationEnvironment(path)
	if path == "" {
		return ""
	}
	if runtime.GOOS == "windows" {
		path = normalizeDriveShortcut(path)
	}
	if !filepath.IsAbs(path) && filepath.VolumeName(path) == "" {
		if os.IsPathSeparator(path[0]) {
			path = filepath.VolumeName(base) + path
		} else {
			path = filepath.Join(base, path)
		}
	}
	return path
}

// Expand %NAME% once, preserving unknown variables and literal percent signs.
func expandLocationEnvironment(path string) string {
	var expanded strings.Builder
	for {
		start := strings.IndexByte(path, '%')
		if start < 0 {
			expanded.WriteString(path)
			return expanded.String()
		}
		end := strings.IndexByte(path[start+1:], '%')
		if end < 0 {
			expanded.WriteString(path)
			return expanded.String()
		}
		end += start + 1
		expanded.WriteString(path[:start])
		if value, ok := os.LookupEnv(path[start+1 : end]); ok {
			expanded.WriteString(value)
		} else {
			expanded.WriteString(path[start : end+1])
		}
		path = path[end+1:]
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
