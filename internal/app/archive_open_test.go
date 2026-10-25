package app

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func TestArchiveOpenUsesCurrentFileAcrossInputRoutes(t *testing.T) {
	for _, folder := range []string{".", "docs/sub"} {
		for _, route := range []string{"enter", "palette", "double-click"} {
			t.Run(folder+"/"+route, func(t *testing.T) {
				v, filename := archiveView(t)
				if folder != "." {
					v.loadLocation(0, browser.Location{Path: filename, Folder: folder}, "report.txt")
					waitLoaded(t, v, 0)
				}
				p := &v.panels[0]
				name := "a2.txt"
				if folder != "." {
					name = "report.txt"
				}
				selectName(t, p, name)
				if folder == "." && route == "double-click" {
					p.marks = map[int]bool{1: true, len(p.items) - 1: true}
				}
				before, _ := os.ReadFile(filename)
				marks, selected := maps.Clone(p.marks), p.selected
				opened := make(chan string, 1)
				release := make(chan struct{})
				t.Cleanup(func() {
					select {
					case <-release:
					default:
						close(release)
					}
				})
				v.openFile = func(path string) error { opened <- path; <-release; return nil }
				send, _ := panelInputFrames(v)
				position := p.list.Position
				switch route {
				case "enter":
					send(pressed(key.NameReturn))
				case "palette":
					send(pressed("`"))
					send(key.EditEvent{Text: "Entry Open"}, pressed(key.NameReturn))
				case "double-click":
					height := p.rows[p.selected].size.Y
					point := f32.Pt(20, float32(68+p.selected*height+height/2))
					rowClick(send, point, time.Second, 0)
					rowClick(send, point, time.Second+100*time.Millisecond, 0)
				}
				if !p.loading || !v.mutationsBusy() {
					t.Fatal("opening bypassed worker busy policy")
				}
				close(release)
				v.active = 1
				waitLoaded(t, v, 0)
				var path string
				select {
				case path = <-opened:
				default:
					t.Fatal("archive file was not opened")
				}
				defer func() { os.Chmod(path, 0600); os.RemoveAll(filepath.Dir(path)) }()
				data, err := os.ReadFile(path)
				if err != nil || string(data) != func() string {
					if folder == "." {
						return name
					}
					return "docs/sub/" + name
				}() {
					t.Fatal("wrong extracted content", string(data), err)
				}
				info, err := os.Stat(path)
				if err != nil || info.Mode().Perm()&0222 != 0 {
					t.Fatal("copy is writable", err)
				}
				after, _ := os.ReadFile(filename)
				if !bytes.Equal(before, after) || v.active != 1 || p.selected != selected || !maps.Equal(p.marks, marks) || p.list.Position != position || p.archiveFolder != folder || v.mutationsBusy() || p.feedback.Current() != nil {
					t.Fatalf("open changed browsing: bytes=%v active=%d selected=%d/%d marks=%v/%v position=%+v/%+v folder=%q busy=%v feedback=%+v", bytes.Equal(before, after), v.active, p.selected, selected, p.marks, marks, p.list.Position, position, p.archiveFolder, v.mutationsBusy(), p.feedback.Current())
				}
				v.finishOperations()
				if _, err := os.Stat(path); err != nil {
					t.Fatal("closing Commando removed external application's file", err)
				}
			})
		}
	}
}

func TestArchiveOpenFailureAndBusyPolicy(t *testing.T) {
	v, _ := archiveView(t)
	p := &v.panels[0]
	selectName(t, p, "a2.txt")
	v.openFile = func(string) error { return errors.New("native details") }
	v.loader.busy.Store(1)
	if openReady(v) {
		t.Fatal("archive open offered during mutation")
	}
	v.activateSelected()
	if p.loading {
		t.Fatal("direct route bypassed busy policy")
	}
	v.loader.busy.Store(0)
	v.activateSelected()
	waitLoaded(t, v, 0)
	message := p.feedback.Current()
	if v.mutationsBusy() || message == nil || message.Kind != feedback.Error || message.Text != "Failed to open" {
		t.Fatal("launch failure not reconciled", message)
	}
}
