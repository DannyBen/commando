package app

import (
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
)

func TestFooterFileMetadata(t *testing.T) {
	file := entry{file: browser.Entry{Name: "file.txt", Bytes: 2048, SizeKnown: true,
		Modified: time.Date(2026, 10, 21, 12, 0, 0, 0, time.Local)}}
	for _, tc := range []struct {
		column metadataColumn
		want   string
	}{
		{metadataNone, "2.0 KB · 2026-10-21"},
		{metadataExtension, "2.0 KB · 2026-10-21"},
		{metadataSize, "2026-10-21"},
		{metadataModified, "2.0 KB"},
		{metadataCreated, "2.0 KB · 2026-10-21"},
	} {
		p := panel{items: []entry{file}, metadata: tc.column}
		if got, failed := p.status(); got != tc.want || failed {
			t.Errorf("column %v: status=%q, failed=%v", tc.column, got, failed)
		}
	}
}

func TestFooterNoInventedMetadata(t *testing.T) {
	for _, file := range []browser.Entry{
		{Name: "folder", Directory: true},
		{Name: "..", Parent: true, Directory: true},
		{Name: "drive", Path: `C:\`},
		{Name: "unavailable"},
	} {
		p := panel{items: []entry{{file: file}}}
		if got, _ := p.status(); got != "" {
			t.Errorf("%s: status=%q", file.Name, got)
		}
	}
	p := panel{selected: -1}
	if got, _ := p.status(); got != "" || p.positionLabel() != "0 / 0" {
		t.Fatal("empty panel has metadata or a nonzero count")
	}
}

func TestFooterMarkingInput(t *testing.T) {
	v := selectionView()
	p := &v.panels[0]
	p.metadata = metadataSize
	for i := 2; i < len(p.items); i++ {
		p.items[i].file.Bytes = 1024
		p.items[i].file.SizeKnown = true
	}
	send, _ := panelInputFrames(v)
	assertFooter := func(metadata, counter string) {
		t.Helper()
		if got, _ := p.status(); got != metadata || p.positionLabel() != counter {
			t.Fatalf("footer=%q %q, want %q %q", got, p.positionLabel(), metadata, counter)
		}
	}
	assertFooter("", "1 / 5")
	send(pressed(key.NameHome))
	assertFooter("", "0 / 5")
	send(pressed(insertKey)) // Parent is never marked.
	assertFooter("", "1 / 5")
	send(pressed(key.NameDownArrow), pressed(insertKey))
	assertFooter("1.0 KB", "1 / 5")
	send(pressed(key.NameSpace), key.EditEvent{Text: " "})
	assertFooter("2.0 KB", "2 / 5")
	send(pressed(key.NameUpArrow), pressed(key.NameUpArrow), pressed(key.NameSpace), key.EditEvent{Text: " "})
	assertFooter("", "3 / 5") // A marked folder makes the size incomplete.
	send(pressed(key.NameEscape))
	assertFooter("", "1 / 5")
}

func TestFooterUnknownMarkedSize(t *testing.T) {
	p := panel{items: []entry{
		{file: browser.Entry{Bytes: 1024, SizeKnown: true}},
		{file: browser.Entry{}},
	}, marks: map[int]bool{0: true, 1: true}}
	if got, _ := p.status(); got != "" {
		t.Fatalf("incomplete total shown: %q", got)
	}
	p.items = nil // Pending navigation can retain marks until reconciliation.
	if got, _ := p.status(); got != "" {
		t.Fatalf("pending listing showed a size: %q", got)
	}
}
