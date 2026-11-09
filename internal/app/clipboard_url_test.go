package app

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
)

func TestURLPasteUsesSharedDraftAndCapturedURL(t *testing.T) {
	for _, palette := range []bool{false, true} {
		v, root, _ := clipboardView(t)
		const target = "https://example.com/some/page?q=1&x=two%20words#section"
		current := target
		v.loader.readClipboard = func() (browser.FileClipboard, error) { return browser.FileClipboard{Text: current}, nil }
		router, send := clipboardFrames(v)
		if palette {
			send(pressed("`"), key.EditEvent{Text: "Entry Paste"})
			send(pressed(key.NameReturn))
		} else {
			send(modified("V", key.ModCtrl))
		}
		waitLoaded(t, v, 0)
		send()
		if n := v.nameInteraction(); n == nil || n.kind != nameCreateFile || v.nameEditor.Text() != "example.com.url" || v.nameEditor.SelectedText() != "example.com" || !router.Source().Focused(&v.nameEditor) {
			t.Fatal("URL did not open focused hostname draft")
		}
		if _, err := os.Stat(filepath.Join(root, "example.com.url")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("shortcut saved before confirmation")
		}
		current = "https://newer.example/"
		send(replaceCreate(v, "my link.url"), pressed(key.NameReturn))
		waitCreated(t, v)
		send()
		data, err := os.ReadFile(filepath.Join(root, "my link.url"))
		if err != nil || string(data) != "[InternetShortcut]\r\nURL="+target+"\r\n" {
			t.Fatal("saved shortcut lost captured URL", string(data), err)
		}
		if current != "https://newer.example/" || v.nameInteraction() != nil || v.transferPending || v.mutationsBusy() {
			t.Fatal("paste changed clipboard or retained ownership")
		}
		for _, index := range []int{0, 2} {
			if !slices.ContainsFunc(v.panels[index].items, func(e entry) bool { return e.name == "my link.url" }) {
				t.Fatal("shortcut missing from destination or alias panel")
			}
		}
	}
}

func TestURLPasteCancelAndCollisionRetry(t *testing.T) {
	for _, retry := range []bool{false, true} {
		v, root, _ := clipboardView(t)
		v.loader.readClipboard = func() (browser.FileClipboard, error) {
			return browser.FileClipboard{Text: "https://example.com/path#fragment"}, nil
		}
		send, _ := panelInputFrames(v)
		send(modified("V", key.ModCtrl))
		waitLoaded(t, v, 0)
		send()
		if !retry {
			send(pressed(key.NameEscape))
			if _, err := os.Stat(filepath.Join(root, "example.com.url")); !errors.Is(err, os.ErrNotExist) || v.nameInteraction() != nil || v.transferPending {
				t.Fatal("cancel created shortcut or retained draft")
			}
			continue
		}
		if err := os.WriteFile(filepath.Join(root, "example.com.url"), []byte("existing"), 0600); err != nil {
			t.Fatal(err)
		}
		send(pressed(key.NameReturn))
		waitCreated(t, v)
		send()
		if n := v.nameInteraction(); n == nil || n.pending || !errors.Is(n.err, os.ErrExist) {
			t.Fatal("collision discarded shortcut draft")
		}
		if data, _ := os.ReadFile(filepath.Join(root, "example.com.url")); string(data) != "existing" {
			t.Fatal("collision replaced existing file")
		}
		send(replaceCreate(v, "retry.url"), pressed(key.NameReturn))
		waitCreated(t, v)
		send()
		if data, err := os.ReadFile(filepath.Join(root, "retry.url")); err != nil || string(data) != "[InternetShortcut]\r\nURL=https://example.com/path#fragment\r\n" {
			t.Fatal("retry lost URL", err)
		}
	}
}

func TestURLPasteIgnoresUnrelatedTextAndPasteShortcut(t *testing.T) {
	for _, tc := range []struct {
		text string
		mods key.Modifiers
	}{{"just text", key.ModCtrl}, {"https://example.com", key.ModCtrl | key.ModShift}} {
		v, _, _ := clipboardView(t)
		v.loader.readClipboard = func() (browser.FileClipboard, error) { return browser.FileClipboard{Text: tc.text}, nil }
		send, _ := panelInputFrames(v)
		send(modified("V", tc.mods))
		waitLoaded(t, v, 0)
		send()
		if v.pendingNameDraft != nil || v.nameInteraction() != nil || v.transferPending || v.panels[0].feedback.Current().Text != "Nothing to paste" {
			t.Fatal("unrelated text or Paste shortcut created URL draft")
		}
	}
}

func TestURLPasteArrivalDefersBehindEditorAndCancelsAfterLeavingPanel(t *testing.T) {
	for _, leave := range []bool{false, true} {
		v := newPreviewView()
		v.loader = &directoryLoader{requests: make(chan loadRequest, 3), results: make(chan loadResult, 3),
			readClipboard: func() (browser.FileClipboard, error) {
				return browser.FileClipboard{Text: "https://example.com/path"}, nil
			}}
		send, _ := panelInputFrames(v)
		send(modified("V", key.ModCtrl))
		request := <-v.loader.requests
		if leave {
			send(pressed(key.NameRightArrow))
		}
		send(modified("D", key.ModAlt))
		v.location.SetText("draft path")
		v.location.SetCaret(1, 4)
		v.acceptLoad(v.loader.fileOperation(request))
		send()
		start, end := v.location.Selection()
		if !v.locationEditing() || v.location.Text() != "draft path" || start != 1 || end != 4 {
			t.Fatal("URL arrival disturbed location editor")
		}
		send(pressed(key.NameEscape))
		send()
		if leave {
			if v.nameInteraction() != nil || v.pendingNameDraft != nil || v.transferPending {
				t.Fatal("URL followed focus to another panel")
			}
		} else if v.nameInteraction() == nil || v.nameEditor.Text() != "example.com.url" {
			t.Fatal("URL draft did not follow editor dismissal")
		}
	}
}
