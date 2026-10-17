package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/dannyben/commando/internal/browser"
)

func waitLocationCompletion(t *testing.T, v *view) {
	t.Helper()
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		s, ok := v.interaction.current.(*locationSession)
		if !ok || s.pendingPath == "" {
			return
		}
		select {
		case result := <-v.loader.results:
			v.acceptLoad(result)
		case <-timer.C:
			t.Fatal("completion lookup did not finish")
		}
	}
}

func TestLocationCompletionTypingCyclingAcceptanceUndoAndNavigation(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"report2", "report10", ".hidden"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "report.txt"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	v := newBrowserView([]string{root}, nil)
	t.Cleanup(v.finishOperations)
	waitLoaded(t, v, 0)
	p := &v.panels[0]
	selectName(t, p, "report.txt")
	p.toggleMark(p.selected)
	send, _ := panelInputFrames(v)
	send(modified("D", key.ModAlt))
	selected, position := p.selected, p.list.Position
	replace := func(query string) {
		n := utf8.RuneCountInString(query)
		send(key.EditEvent{Range: key.Range{End: utf8.RuneCountInString(v.location.Text())}, Text: query}, key.SelectionEvent{Start: n, End: n})
		waitLocationCompletion(t, v)
		send()
	}
	replace("re")
	s := v.interaction.current.(*locationSession)
	if s.completion.suffix() != "port2"+string(filepath.Separator) || v.location.Text() != "re" || p.loading || p.path != root || p.selected != selected || !p.marks[selected] || p.list.Position != position {
		t.Fatalf("typing changed browsing state or failed directories-first completion: suffix=%q text=%q loading=%v path=%q selected=%d want=%d marks=%v pos=%+v want=%+v", s.completion.suffix(), v.location.Text(), p.loading, p.path, p.selected, selected, p.marks, p.list.Position, position)
	}
	send(pressed(key.NameDownArrow))
	if s.completion.suffix() != "port10"+string(filepath.Separator) {
		t.Fatal("directories were not naturally sorted")
	}
	send(pressed(key.NameDownArrow))
	if s.completion.suffix() != "port.txt" {
		t.Fatal("files did not follow directories")
	}
	send(pressed(key.NameTab))
	if !v.locationEditing() || v.location.Text() != "report.txt" || p.path != root {
		t.Fatal("Tab navigated instead of accepting")
	}
	send(modified("Z", key.ModCtrl))
	if v.location.Text() != "re" {
		t.Fatal("completion broke native Undo")
	}
	replace(".")
	if s.completion.suffix() != "hidden"+string(filepath.Separator) {
		t.Fatal("hidden entries were excluded")
	}
	replace("re")
	send(pressed(key.NameReturn)) // Missing literal paths use the visible completion.
	waitLoaded(t, v, 0)
	if p.path != filepath.Join(root, "report2") || v.locationEditing() {
		t.Fatal("Enter did not use the visible completion for the missing path")
	}
	send(modified("D", key.ModAlt))
	replace(filepath.Join(root, "report.t"))
	send(pressed(key.NameTab), pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	if p.path != root || p.items[p.selected].file.Name != "report.txt" {
		t.Fatal("accepted file was not selected after Enter")
	}
}

func TestLocationCompletionPathsQuotesAndQuietFailures(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "folder with spaces")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMMANDO_COMPLETION_ROOT", root)
	sep := string(filepath.Separator)
	for _, query := range []string{"fol", "." + sep + "fol", filepath.Join(root, "fol"), "%COMMANDO_COMPLETION_ROOT%" + sep + "fol", `"` + filepath.Join(root, "fol"), `"` + filepath.Join(root, "fol") + `"`} {
		t.Run(query, func(t *testing.T) {
			v := newBrowserView([]string{root}, nil)
			t.Cleanup(v.finishOperations)
			waitLoaded(t, v, 0)
			send, _ := panelInputFrames(v)
			send(modified("D", key.ModAlt))
			n := utf8.RuneCountInString(query)
			if strings.HasSuffix(query, `"`) {
				n--
			}
			send(key.EditEvent{Range: key.Range{End: utf8.RuneCountInString(v.location.Text())}, Text: query}, key.SelectionEvent{Start: n, End: n})
			waitLocationCompletion(t, v)
			send()
			if s := v.interaction.current.(*locationSession); !strings.HasPrefix(s.completion.suffix(), "der with spaces"+sep) {
				t.Fatalf("no path suggestion: %q", s.completion.suffix())
			}
			send(pressed(key.NameTab))
			if strings.HasPrefix(query, `"`) && (!strings.HasPrefix(v.location.Text(), `"`) || !strings.HasSuffix(v.location.Text(), `"`)) {
				t.Fatal("completion lost quotes")
			}
			send(pressed(key.NameReturn))
			waitLoaded(t, v, 0)
			if v.panels[0].path != folder {
				t.Fatalf("completed path navigated to %q", v.panels[0].path)
			}
		})
	}
	for _, query := range []string{`\\server\share\fol`, "//server/share/fol"} {
		if _, ok := completionLocation(root, query); ok {
			t.Fatal("network path offered completion")
		}
	}
	v := newPreviewView()
	v.panels[0].path = root
	v.loader = newDirectoryLoader(func(string) (browser.Directory, error) { return browser.Directory{}, errors.New("denied") }, nil)
	t.Cleanup(v.finishOperations)
	v.startLocation(layout.Context{})
	v.location.SetText("fol")
	v.location.SetCaret(3, 3)
	v.updateLocationCompletion()
	waitLocationCompletion(t, v)
	if v.location.Text() != "fol" || v.panels[0].feedback.Current() != nil || v.panels[0].loading {
		t.Fatal("failed lookup changed editor or browsing feedback")
	}
}

func TestLocationCompletionDiscardsOldDirectoryAndSession(t *testing.T) {
	root := t.TempDir()
	v := newPreviewView()
	v.panels[0].path = root
	v.startLocation(layout.Context{})
	s := v.interaction.current.(*locationSession)
	v.location.SetText("old/x")
	v.location.SetCaret(5, 5)
	v.updateLocationCompletion()
	oldPath := s.completionPath
	s.pendingPath = oldPath
	v.location.SetText("new/x")
	v.location.SetCaret(5, 5)
	v.updateLocationCompletion()
	v.acceptLoad(loadResult{request: loadRequest{path: oldPath, completion: s}, directory: browser.Directory{Entries: []browser.Entry{{Name: "x-stale"}}}})
	if s.completion.suffix() != "" || v.location.Text() != "new/x" {
		t.Fatal("old directory restored stale suggestion or text")
	}
	s.pendingPath = s.completionPath
	request := loadRequest{path: s.pendingPath, completion: s}
	v.stopLocation(layout.Context{})
	v.startLocation(layout.Context{})
	v.acceptLoad(loadResult{request: request, directory: browser.Directory{Entries: []browser.Entry{{Name: "x-stale"}}}})
	fresh := v.interaction.current.(*locationSession)
	if fresh.completion.suffix() != "" || fresh == s {
		t.Fatal("old editing session populated a newer one")
	}
}

func TestLocationCompletionPreview(t *testing.T) {
	dir := os.Getenv("COMMANDO_PREVIEW_SETTINGS")
	if dir == "" {
		t.Skip("optional location completion preview")
	}
	v := newPreviewView()
	v.startLocation(layout.Context{})
	s := v.interaction.current.(*locationSession)
	s.completionPath = filepath.Clean(v.panels[0].locationPath())
	s.completionLoaded = true
	s.completionEntries = []browser.Entry{{Name: "documents", Directory: true}, {Name: "document.txt"}}
	v.location.SetText("doc")
	v.location.SetCaret(3, 3)
	if err := renderViewPNG(filepath.Join(dir, "location-completion.png"), v); err != nil {
		t.Fatal(err)
	}
	v.location.SetText(`"doc"`)
	v.location.SetCaret(4, 4)
	if err := renderViewPNG(filepath.Join(dir, "location-completion-quoted.png"), v); err != nil {
		t.Fatal(err)
	}
}

func TestLocationEnterUsesCompletionOnlyForMissingLiteral(t *testing.T) {
	for _, scenario := range []string{"selected suggestion", "existing folder", "new existing file", "completed file", "vanished suggestion", "no suggestion"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range []string{"Bashly", "Batch"} {
				if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, "data.txt"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			query, want, selected, warning := "ba", filepath.Join(root, "Batch"), "", false
			switch scenario {
			case "existing folder":
				if err := os.Mkdir(filepath.Join(root, "ba"), 0700); err != nil {
					t.Fatal(err)
				}
				want = filepath.Join(root, "ba")
			case "new existing file":
				want, selected = root, "ba"
			case "completed file":
				query, want, selected = "da", root, "data.txt"
			case "vanished suggestion":
				want, warning = root, true
			case "no suggestion":
				query, want, warning = "absent", root, true
			}
			v := newBrowserView([]string{root}, nil)
			t.Cleanup(v.finishOperations)
			waitLoaded(t, v, 0)
			send, _ := panelInputFrames(v)
			send(modified("D", key.ModAlt))
			n := utf8.RuneCountInString(query)
			send(key.EditEvent{Range: key.Range{End: utf8.RuneCountInString(v.location.Text())}, Text: query}, key.SelectionEvent{Start: n, End: n})
			waitLocationCompletion(t, v)
			send()
			s := v.interaction.current.(*locationSession)
			switch scenario {
			case "selected suggestion":
				send(pressed(key.NameDownArrow))
			case "existing folder":
				// Deliberately show a different directory; the literal still wins.
				send(pressed(key.NameDownArrow))
			case "new existing file":
				// The file arrives after the completion snapshot was read.
				if err := os.WriteFile(filepath.Join(root, "ba"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "vanished suggestion":
				if err := os.Remove(filepath.Join(root, "Bashly")); err != nil {
					t.Fatal(err)
				}
			}
			if scenario != "no suggestion" && s.completion.suffix() == "" {
				t.Fatal("test requires a visible suggestion")
			}
			send(pressed(key.NameReturn))
			waitLoaded(t, v, 0)
			p := &v.panels[0]
			if p.path != want || p.err != nil || v.locationEditing() {
				t.Fatalf("Enter: path=%q want=%q err=%v", p.path, want, p.err)
			}
			if selected != "" && p.items[p.selected].file.Name != selected {
				t.Fatalf("selected %q instead of %q", p.items[p.selected].file.Name, selected)
			}
			message := p.feedback.Current()
			if warning {
				if message == nil || message.Text != "Path not found" {
					t.Fatal("missing path lost its warning")
				}
			} else if message != nil {
				t.Fatalf("successful completion showed feedback: %v", message)
			}
		})
	}
}

func TestLocationEnterCompletionPreservesReadErrors(t *testing.T) {
	root := t.TempDir()
	literal, suggestion := filepath.Join(root, "missing"), filepath.Join(root, "suggestion")
	if err := os.Mkdir(suggestion, 0700); err != nil {
		t.Fatal(err)
	}
	for _, failure := range []error{os.ErrPermission, errors.New("read failure")} {
		for _, failingPath := range []string{literal, suggestion} {
			t.Run(failure.Error()+failingPath, func(t *testing.T) {
				l := &directoryLoader{readDirectory: func(path string) (browser.Directory, error) {
					if path == failingPath {
						return browser.Directory{}, failure
					}
					return browser.ReadDirectory(path)
				}}
				_, _, _, err := l.readExplicitPath(literal, suggestion)
				if !errors.Is(err, failure) {
					t.Fatalf("read failure hidden by completion or recovery: %v", err)
				}
			})
		}
	}
}
