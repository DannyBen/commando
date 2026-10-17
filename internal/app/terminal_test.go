package app

import (
	"errors"
	"maps"
	"os"
	"slices"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
	"github.com/dannyben/commando/internal/settings"
)

func TestTerminalCommandsCaptureOpenFolderThroughWorker(t *testing.T) {
	for _, admin := range []bool{false, true} {
		v := newPreviewView()
		v.active, v.ownerWindow = 1, 123
		p := &v.panels[1]
		path, selected, position := p.path, p.selected, p.list.Position
		items, marks := slices.Clone(p.items), maps.Clone(p.marks)
		v.loader = newDirectoryLoader(browser.ReadLocation, nil)
		calls := make(chan terminalRequest, 1)
		v.loader.launchTerminal = func(directory string, elevated bool, owner uintptr) error {
			if directory != path {
				t.Errorf("terminal used selected entry instead of open folder: %q", directory)
			}
			calls <- terminalRequest{elevated, owner}
			return nil
		}
		var command command
		id := "programs.terminal_here"
		wantShortcut := "Ctrl+T"
		if admin {
			id, wantShortcut = "programs.terminal_here_admin", ""
		}
		for _, candidate := range v.commandCatalogue() {
			if candidate.id == id {
				command = candidate
			}
		}
		if command.id == "" || command.shortcut() != wantShortcut || !command.allowed(v) {
			t.Fatal("terminal command missing, unavailable or incorrectly bound", id)
		}
		v.openTerminal(admin)
		select {
		case call := <-calls:
			if call.admin != admin || call.owner != 123 {
				t.Fatal("worker lost elevation or owner")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("terminal did not run on the worker")
		}
		v.active = 2 // Completion must preserve subsequent panel focus.
		v.acceptLoad(<-v.loader.results)
		v.finishOperations()
		if v.active != 2 || p.loading || v.mutationsBusy() || p.path != path || p.selected != selected || p.list.Position != position || !slices.Equal(items, p.items) || !maps.Equal(marks, p.marks) || p.feedback.Current() != nil {
			t.Fatal("terminal completion changed browsing or produced success feedback")
		}
	}
}

func TestTerminalKeyPaletteAndBusyPolicy(t *testing.T) {
	v := newPreviewView()
	v.loader = &directoryLoader{requests: make(chan loadRequest, 2), done: make(chan struct{})}
	send, _ := panelInputFrames(v)
	send(modified("T", key.ModCtrl))
	request := <-v.loader.requests
	if request.path != v.panels[0].path || request.terminal == nil || request.terminal.admin {
		t.Fatal("Ctrl+T did not capture the open folder")
	}
	v.acceptLoad(loadResult{request: request})
	send(pressed("`"))
	send(key.EditEvent{Text: "Programs Terminal here Admin"}, pressed(key.NameReturn))
	request = <-v.loader.requests
	if request.terminal == nil || !request.terminal.admin || !v.interaction.idle() {
		t.Fatal("palette did not launch the admin command")
	}
	v.acceptLoad(loadResult{request: request})
	v.panels[0].drives = true
	send(modified("T", key.ModCtrl))
	v.panels[0].drives = false
	send(modified("D", key.ModAlt), modified("T", key.ModCtrl))
	if !v.locationEditing() {
		t.Fatal("terminal interrupted a native editor")
	}
	send(pressed(key.NameEscape))
	v.loader.busy.Store(1)
	send(modified("T", key.ModCtrl))
	v.loader.busy.Store(0)
	select {
	case <-v.loader.requests:
		t.Fatal("terminal ignored drives, editor or busy policy")
	default:
	}
}

func TestTerminalLaunchFailureAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		err     error
		message string
	}{
		{nil, ""}, {browser.ErrTerminalCancelled, ""},
		{os.ErrPermission, "Terminal access denied"},
		{os.ErrNotExist, "Terminal path not found"},
		{browser.ErrTerminalUnavailable, "Terminal requires Windows"},
		{browser.ErrTerminalPathExpansion, "Terminal path contains %"},
		{errors.New("native details"), "Cannot open terminal"},
	} {
		v := newPreviewView()
		v.panels[0].loading = true
		v.acceptLoad(loadResult{request: loadRequest{panel: 0, terminal: &terminalRequest{}}, err: tc.err})
		message := v.panels[0].feedback.Current()
		if v.panels[0].loading || tc.message == "" && message != nil || tc.message != "" && (message == nil || message.Text != tc.message || message.Kind != feedback.Error) {
			t.Fatal("terminal failure/cancellation feedback was incorrect", tc.err, message)
		}
	}
}

func TestTerminalDefaultYieldsToExistingUserBinding(t *testing.T) {
	values, err := settings.Parse([]byte("[keybindings.view.next_metadata_column]\nshortcut_1 = 'Ctrl+T'\n"), bindingSettingsCatalogue())
	if err != nil {
		t.Fatal(err)
	}
	v := newPreviewView()
	v.restoreSettings(values, nil)
	send, _ := panelInputFrames(v)
	before := v.panels[0].metadata
	send(modified("T", key.ModCtrl))
	if v.panels[0].metadata != before.cycle(1) {
		t.Fatal("terminal default took over the user's Ctrl+T")
	}
	for _, c := range v.commandCatalogue() {
		if c.id == "programs.terminal_here" && c.shortcut() != "" {
			t.Fatal("conflicting terminal default remained bound")
		}
	}
}
