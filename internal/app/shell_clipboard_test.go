package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
)

func TestShellClipboardPasteUsesSharedWorkerAndPreservesClipboard(t *testing.T) {
	for _, route := range []string{"Ctrl+V", "Shift+Insert", "palette"} {
		t.Run(route, func(t *testing.T) {
			v, _, target := clipboardView(t)
			v.active = 1
			snapshot := browser.FileClipboard{Shell: true, Sequence: 42}
			v.loader.readClipboard = func() (browser.FileClipboard, error) { return snapshot, nil }
			v.loader.completeClipboard = func(browser.FileClipboard, []string) (browser.FileClipboard, error) {
				t.Error("Shell copy must not change the clipboard")
				return snapshot, nil
			}
			called := false
			v.loader.transfer = func(request browser.TransferRequest) (browser.TransferResult, error) {
				called = true
				if request.ShellClipboard == nil || *request.ShellClipboard != 42 || len(request.Paths) != 0 || request.Move || request.Directory != target {
					t.Errorf("incorrect Shell transfer: %+v", request)
				}
				if err := os.WriteFile(filepath.Join(target, "from-archive.txt"), []byte("contents"), 0600); err != nil {
					t.Error(err)
				}
				return browser.TransferResult{Directory: target, Attempted: true, Summary: browser.TransferSummary{Requested: 1, Completed: 1},
					Items: []browser.TransferredItem{{Entry: browser.Entry{Name: "from-archive.txt"}}}}, nil
			}
			send, _ := panelInputFrames(v)
			switch route {
			case "Ctrl+V":
				send(modified("V", key.ModCtrl))
			case "Shift+Insert":
				send(modified(insertKey, key.ModShift))
			default:
				send(pressed("`"), key.EditEvent{Text: "Entry Paste"})
				send(pressed(key.NameReturn))
			}
			waitLoaded(t, v, 1)
			send()
			if !called || v.mutationsBusy() || v.transferPending || v.active != 1 {
				t.Fatal("Shell paste failed to release its session")
			}
			found := false
			for _, item := range v.panels[1].items {
				found = found || item.file.Name == "from-archive.txt"
			}
			if !found {
				t.Fatal("destination panel was not reconciled")
			}
		})
	}
}

func TestShellClipboardRejectsUnsupportedDestinationsAndReportsFailures(t *testing.T) {
	for _, kind := range []string{"archive", "shortcut", "changed", "cancelled"} {
		t.Run(kind, func(t *testing.T) {
			v, _, target := clipboardView(t)
			v.loader.readClipboard = func() (browser.FileClipboard, error) {
				return browser.FileClipboard{Shell: true, Sequence: 7}, nil
			}
			called := false
			v.loader.transfer = func(browser.TransferRequest) (browser.TransferResult, error) {
				called = true
				if kind == "changed" {
					return browser.TransferResult{}, browser.ErrClipboardChanged
				}
				return browser.TransferResult{Directory: target, Attempted: true, Aborted: true,
					Summary: browser.TransferSummary{Requested: 1, Unfinished: 1}}, nil
			}
			request := loadRequest{panel: 1, path: target, transfer: &transferRequest{locations: v.fileOperationLocations()}}
			if kind == "archive" {
				request.archiveFolder = "."
			}
			request.transfer.shortcuts = kind == "shortcut"
			result := v.loader.fileOperation(request)
			switch kind {
			case "archive", "shortcut":
				if called || !errors.Is(result.err, errors.ErrUnsupported) {
					t.Fatal("unsupported Shell paste reached the engine", result.err)
				}
			case "changed":
				if !called || fileTransferError(result.err) != "Clipboard changed; paste again" || result.transferred.Attempted {
					t.Fatal("changed clipboard was not reported without a transfer")
				}
			case "cancelled":
				if !called || !result.transferred.Aborted || !result.panelUpdates[1].needed {
					t.Fatal("cancelled transfer did not retain its result and refresh")
				}
			}
		})
	}
}
