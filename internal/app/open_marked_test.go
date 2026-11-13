package app

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/feedback"
)

func TestOpenMarkedFilesUsesSeparateRequestsAndContinuesAfterFailure(t *testing.T) {
	for _, route := range []string{"Return", "Enter", "palette"} {
		t.Run(route, func(t *testing.T) {
			v, root, _ := clipboardView(t)
			p := &v.panels[0]
			for _, name := range []string{"folder", "a.txt", "z.txt"} {
				selectName(t, p, name)
				p.toggleMark(p.selected)
			}
			selectName(t, p, "a.txt")
			before := maps.Clone(p.marks)
			opened := make(chan string, 3)
			release := make(chan struct{})
			t.Cleanup(func() {
				select {
				case <-release:
				default:
					close(release)
				}
			})
			v.openFile = func(path string) error {
				opened <- path
				if filepath.Base(path) == "a.txt" {
					<-release
					return errors.New("no associated application")
				}
				return nil
			}
			send, _ := panelInputFrames(v)
			switch route {
			case "Return":
				send(pressed(key.NameReturn))
			case "Enter":
				send(pressed(key.NameEnter))
			default:
				send(pressed("`"), key.EditEvent{Text: "Actions Open"})
				send(pressed(key.NameReturn))
			}
			next := func(want string) {
				t.Helper()
				select {
				case path := <-opened:
					if path != filepath.Join(root, want) {
						t.Fatalf("opened %q, want %q", path, want)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("marked file was not opened")
				}
			}
			next("a.txt")
			if !maps.Equal(p.marks, before) || p.items[p.selected].name != "a.txt" || p.path != root || p.loading {
				t.Fatal("opening changed marks, current row, or location")
			}
			p.clearMarks() // The submitted batch must retain its original targets.
			send(pressed(key.NameRightArrow))
			if v.active != 1 {
				t.Fatal("opening blocked panel navigation")
			}
			close(release)
			next("z.txt")
			send()
			message := p.feedback.Current()
			if message == nil || message.Kind != feedback.Error || message.Text != "Failed to open" || v.active != 1 {
				t.Fatal("failure feedback lost its origin or changed focus", message)
			}
			select {
			case extra := <-opened:
				t.Fatal("opened a folder or duplicate", extra)
			default:
			}
		})
	}
}

func TestOpenFolderWithMarksNavigatesAcrossInputRoutes(t *testing.T) {
	for _, location := range []string{"native", "archive"} {
		for _, current := range []string{"marked folder", "unmarked folder", "parent"} {
			for _, route := range []string{"Return", "Enter", "palette"} {
				t.Run(location+"/"+current+"/"+route, func(t *testing.T) {
					var v *view
					var root, folder, file, wantPath, wantArchive string
					if location == "archive" {
						v, root = archiveView(t)
						folder, file, wantPath, wantArchive = "docs", "a2.txt", root, "docs"
					} else {
						v, root, _ = clipboardView(t)
						folder, file, wantPath = "folder", "a.txt", filepath.Join(root, "folder")
					}
					p := &v.panels[0]
					selectName(t, p, file)
					p.toggleMark(p.selected)
					selectName(t, p, folder)
					if current != "unmarked folder" {
						p.toggleMark(p.selected)
					}
					if current == "parent" {
						selectName(t, p, "..")
						wantPath, wantArchive = filepath.Dir(root), ""
					}
					opened := make(chan string, 3)
					v.openFile = func(path string) error { opened <- path; return nil }
					send, _ := panelInputFrames(v)
					switch route {
					case "Return":
						send(pressed(key.NameReturn))
					case "Enter":
						send(pressed(key.NameEnter))
					default:
						send(pressed("`"), key.EditEvent{Text: "Actions Open"})
						send(pressed(key.NameReturn))
					}
					waitLoaded(t, v, 0)
					if p.path != wantPath || p.archiveFolder != wantArchive || len(p.marks) != 0 {
						t.Fatal("folder did not navigate and clear listing marks", p.path, p.archiveFolder, p.marks)
					}
					select {
					case path := <-opened:
						t.Fatal("folder navigation also opened marked file", path)
					default:
					}
				})
			}
		}
	}
}

func TestOpenMarkedFilesFromUnmarkedFileSkipsCurrentFileAndFolders(t *testing.T) {
	v, root, _ := clipboardView(t)
	p := &v.panels[0]
	markClipboardItems(t, p) // Mark folder and a.txt; leave the current z.txt unmarked.
	before := maps.Clone(p.marks)
	opened := make(chan string, 3)
	v.openFile = func(path string) error { opened <- path; return nil }
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	select {
	case path := <-opened:
		if path != filepath.Join(root, "a.txt") {
			t.Fatal("opened unmarked current file or marked folder", path)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("marked file was not opened")
	}
	if !maps.Equal(p.marks, before) || p.path != root || p.items[p.selected].name != "z.txt" || p.loading {
		t.Fatal("opening changed marks, current row, or location")
	}
}

func TestOpenMarkedFoldersDoesNotActivateUnmarkedFile(t *testing.T) {
	v, root, _ := clipboardView(t)
	p := &v.panels[0]
	selectName(t, p, "folder")
	p.toggleMark(p.selected)
	selectName(t, p, "a.txt")
	opened := make(chan string, 1)
	v.openFile = func(path string) error { opened <- path; return nil }
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	if p.path != root || p.loading || len(p.marks) != 1 {
		t.Fatal("folder-only marks changed browsing")
	}
	select {
	case path := <-opened:
		t.Fatal("opened unmarked file", path)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestOpenMarkedArchiveFilesSkipsFoldersAndContinuesAfterFailure(t *testing.T) {
	v, archive := archiveView(t)
	p := &v.panels[0]
	for _, name := range []string{"docs", "a2.txt", "a10.txt"} {
		selectName(t, p, name)
		p.toggleMark(p.selected)
	}
	selectName(t, p, "a2.txt")
	marks, selected := maps.Clone(p.marks), p.selected
	before, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	type openedFile struct{ path, data string }
	opened := make(chan openedFile, 3)
	v.openFile = func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		opened <- openedFile{path, string(data)}
		if string(data) == "a2.txt" {
			return errors.New("cannot open first file")
		}
		return nil
	}
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send()
	var names []string
	for len(opened) > 0 {
		file := <-opened
		names = append(names, file.data)
		if file.data == "a10.txt" {
			t.Cleanup(func() { os.Chmod(file.path, 0600); os.RemoveAll(filepath.Dir(file.path)) })
			info, err := os.Stat(file.path)
			if err != nil || info.Mode().Perm()&0222 != 0 {
				t.Fatal("successful copy was not retained read-only", err)
			}
		}
	}
	if !slices.Equal(names, []string{"a2.txt", "a10.txt"}) {
		t.Fatal("archive entries did not receive separate open requests", names)
	}
	after, err := os.ReadFile(archive)
	if err != nil || !slices.Equal(before, after) || !maps.Equal(marks, p.marks) || selected != p.selected || p.archiveFolder != "." {
		t.Fatal("opening changed archive or browsing state")
	}
	if message := p.feedback.Current(); message == nil || message.Text != "Failed to open" {
		t.Fatal("partial opening failure was not reported", message)
	}
}

func TestMarkedFolderShortcutUsesDefaultApplication(t *testing.T) {
	v, link, _ := shortcutView(t)
	p := &v.panels[0]
	p.toggleMark(p.selected)
	opened := make(chan string, 1)
	v.openFile = func(path string) error { opened <- path; return nil }
	send, _ := panelInputFrames(v)
	send(pressed(key.NameReturn))
	select {
	case path := <-opened:
		if path != link {
			t.Fatal("marked shortcut did not use its original path", path)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("marked shortcut did not open externally")
	}
	if p.loading || p.path != filepath.Dir(link) || len(p.marks) != 1 {
		t.Fatal("marked shortcut navigated or changed marks")
	}
}
