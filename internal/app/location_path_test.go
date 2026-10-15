package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func TestLocationSubmissionResolvesFilesFoldersAndMissingAncestors(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "folder with spaces")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(folder, "report.txt")
	if err := os.WriteFile(file, []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMMANDO_PASTED_PATH", file)
	for _, tc := range []struct {
		name, path, want, selected string
		warning                    bool
	}{
		{"folder", folder, folder, "", false},
		{"quoted folder", `"` + folder + `"`, folder, "", false},
		{"quoted file", `"` + file + `"`, folder, "report.txt", false},
		{"file", file, folder, "report.txt", false},
		{"relative file", filepath.Join("folder with spaces", "report.txt"), folder, "report.txt", false},
		{"environment file", `"%COMMANDO_PASTED_PATH%"`, folder, "report.txt", false},
		{"missing file", filepath.Join(folder, "absent.txt"), folder, "", true},
		{"missing folder", filepath.Join(folder, "absent"), folder, "", true},
		{"several missing parents", filepath.Join(folder, "one", "two", "three.exe"), folder, "", true},
		{"file used as folder", filepath.Join(file, "child"), folder, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newBrowserView([]string{root, root}, nil)
			t.Cleanup(v.finishOperations)
			waitLoaded(t, v, 0)
			waitLoaded(t, v, 1)
			send, _ := panelInputFrames(v)
			send(modified("D", key.ModAlt))
			send(key.EditEvent{Range: key.Range{End: utf8.RuneCountInString(v.location.Text())}, Text: tc.path})
			send(pressed(key.NameReturn))
			waitLoaded(t, v, 0)
			send()
			p := &v.panels[0]
			if p.path != tc.want || p.err != nil || v.active != 0 || v.locationEditing() || v.panels[1].path != root {
				t.Fatalf("wrong destination or focus: path=%q err=%v", p.path, p.err)
			}
			if tc.selected != "" && (p.selected < 0 || p.items[p.selected].file.Name != tc.selected) {
				t.Fatal("existing file was not selected")
			}
			message := p.feedback.Current()
			if tc.warning {
				if message == nil || message.Kind != feedback.Warning || message.Text != "Path not found" {
					t.Fatal("missing path did not warn", message)
				}
			} else if message != nil {
				t.Fatal("existing path produced warning", message)
			}
		})
	}
}

func TestExplicitPathStopsAtReadErrorsAndDoesNotUseFallbackRoot(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "missing", "file.txt")
	for _, failure := range []error{os.ErrPermission, errors.New("read failure"), os.ErrNotExist} {
		t.Run(failure.Error(), func(t *testing.T) {
			var reads []string
			l := &directoryLoader{readDirectory: func(path string) (browser.Directory, error) {
				reads = append(reads, path)
				if path == missing || path == filepath.Dir(missing) {
					return browser.Directory{}, os.ErrNotExist
				}
				return browser.Directory{}, failure
			}}
			_, selected, recovered, err := l.readExplicitPath(missing)
			if !errors.Is(err, failure) || selected != "" || recovered {
				t.Fatal("read failure was hidden", err)
			}
			if !errors.Is(failure, os.ErrNotExist) && reads[len(reads)-1] != root {
				t.Fatal("climbed beyond a non-missing read error", reads)
			}
			if errors.Is(failure, os.ErrNotExist) && filepath.Dir(reads[len(reads)-1]) != reads[len(reads)-1] {
				t.Fatal("missing ancestry did not stop at its root", reads)
			}
			for i := 1; i < len(reads); i++ {
				if reads[i] != filepath.Dir(reads[i-1]) {
					t.Fatal("explicit path escaped its ancestry", reads)
				}
			}
		})
	}
}

func TestExplicitPathReadFailurePreservesPanelState(t *testing.T) {
	root := t.TempDir()
	v := newBrowserView([]string{root}, nil)
	t.Cleanup(v.finishOperations)
	waitLoaded(t, v, 0)
	v.loader.close()
	v.loader = newDirectoryLoader(func(string) (browser.Directory, error) { return browser.Directory{}, os.ErrPermission }, nil)
	p := &v.panels[0]
	p.entries = []browser.Entry{{Name: "keep.txt"}}
	p.refilter()
	selectName(t, p, "keep.txt")
	p.toggleMark(p.selected)
	selected, count := p.selected, len(p.items)
	send, _ := panelInputFrames(v)
	send(modified("D", key.ModAlt))
	send(key.EditEvent{Range: key.Range{End: utf8.RuneCountInString(v.location.Text())}, Text: filepath.Join(root, "denied")})
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	if p.path != root || !errors.Is(p.err, os.ErrPermission) || len(p.items) != count || p.selected != selected || !p.marks[selected] || p.feedback.Current() == nil || p.feedback.Current().Text != "Access denied" {
		t.Fatal("read error changed listing or lost feedback")
	}
}
