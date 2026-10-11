package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"gioui.org/io/key"
)

func TestStateStoreRoundTrip(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "chosen-data-folder")
	if state, err := loadState(dataDir); err != nil || !reflect.DeepEqual(state, sessionState{}) {
		t.Fatalf("first run: state=%+v err=%v", state, err)
	}
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Fatalf("loading state created the data folder: %v", err)
	}
	want := sessionState{
		Window:      windowState{X: -1200, Y: 42, Width: 960, Height: 540, Positioned: true, Maximized: true},
		Panels:      []panelState{{Path: filepath.Join(t.TempDir(), "Å folder")}, {Path: "/", Drives: true}},
		ActivePanel: 1,
	}
	for i := 0; i < 2; i++ {
		if err := saveState(dataDir, want); err != nil {
			t.Fatal(err)
		}
		got, err := loadState(dataDir)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("round trip %d: got=%+v want=%+v err=%v", i, got, want, err)
		}
		want.Window.Maximized = false
		want.ActivePanel = 0
	}
	entries, err := os.ReadDir(dataDir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Fatalf("unexpected storage files: entries=%v err=%v", entries, err)
	}
}

func TestStateReadErrorsAndInvalidGeometry(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		fail       bool
	}{
		{"truncated", `{"activePanel":2,"panels":[`, true},
		{"wrong type", `{"panels":"not a list"}`, true},
		{"invalid dimensions", `{"window":{"width":-1,"height":480},"activePanel":2}`, false},
		{"oversized dimensions", `{"window":{"width":2147483647,"height":480},"activePanel":2}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dataDir := t.TempDir()
			path := filepath.Join(dataDir, "state.json")
			if err := os.WriteFile(path, []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			state, err := loadState(dataDir)
			if (err != nil) != tc.fail {
				t.Fatalf("state=%+v err=%v", state, err)
			}
			if tc.fail && !reflect.DeepEqual(state, sessionState{}) {
				t.Fatal("corrupt JSON returned partial session state")
			}
			if !tc.fail && (state.Window != (windowState{}) || state.ActivePanel != 2) {
				t.Fatal("invalid geometry affected valid workspace state")
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != tc.data {
				t.Fatal("reading invalid state changed its file")
			}
		})
	}
	// An unreadable destination is different from a missing file.
	dataDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dataDir, "state.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := loadState(dataDir); err == nil {
		t.Fatal("state read error was suppressed")
	}
	if _, err := loadState(""); err == nil {
		t.Fatal("empty data folder read from the working directory")
	}
}

func TestStateSaveFailureCleansUpTemporaryFile(t *testing.T) {
	dataDir := t.TempDir()
	destination := filepath.Join(dataDir, "state.json")
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "keep"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := saveState(dataDir, sessionState{}); err == nil {
		t.Fatal("failed state replacement was not reported")
	}
	data, err := os.ReadFile(filepath.Join(destination, "keep"))
	if err != nil || string(data) != "original" {
		t.Fatal("failed save damaged the existing destination")
	}
	entries, err := os.ReadDir(dataDir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "state.json" {
		t.Fatal("failed save left a temporary state file")
	}
	if err := saveState("", sessionState{}); err == nil {
		t.Fatal("empty data folder wrote to the working directory")
	}
}

func TestStartingLocations(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	first, second, third := t.TempDir(), t.TempDir(), t.TempDir()
	saved := []panelState{{Path: first}, {Path: second, Drives: true}, {Path: third}}
	for _, tc := range []struct {
		name  string
		args  []string
		saved []panelState
		want  []panelState
	}{
		{"first run", nil, nil, []panelState{{Path: home}, {Path: home}, {Path: home}}},
		{"restore", nil, saved, saved},
		{"partial state", nil, saved[:1], []panelState{{Path: first}, {Path: home}, {Path: home}}},
		{"one explicit path", []string{third}, saved, []panelState{{Path: third}, {Path: third}, {Path: third}}},
		{"two explicit paths", []string{second, first}, saved, []panelState{{Path: second}, {Path: first}, {Path: second}}},
		{"invalid saved path", nil, []panelState{{Path: "relative"}}, []panelState{{Path: home}, {Path: home}, {Path: home}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := startingLocations(tc.args, tc.saved)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got=%+v want=%+v err=%v", got, tc.want, err)
			}
		})
	}
	if _, err := startingLocations([]string{first, second, third, home}, saved); err == nil {
		t.Fatal("more than three paths were accepted")
	}
	got, err := startingLocations([]string{"."}, saved)
	if err != nil || !filepath.IsAbs(got[0].Path) || got[0].Drives {
		t.Fatalf("relative startup folder was not resolved: %+v err=%v", got, err)
	}
}

func TestWorkspaceStateRestoresRealLocationsAndActivePanel(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "note.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	locations := []panelState{{Path: root}, {Path: other}, {Path: root}}
	v := newRestoredView(locations, 0, nil)
	defer v.loader.close()
	for i := range v.panels {
		waitLoaded(t, v, i)
	}
	send, _ := panelInputFrames(v)
	selectName(t, &v.panels[0], "child")
	send(pressed(key.NameReturn))
	waitLoaded(t, v, 0)
	send(pressed(key.NameRightArrow))
	// A failed or pending navigation must save the last displayed folder.
	v.load(1, filepath.Join(other, "missing"), "")
	if got := v.sessionState(windowState{}); got.Panels[1].Path != other {
		t.Fatal("pending navigation changed the saved folder")
	}
	waitLoaded(t, v, 1)
	if v.panels[1].err == nil {
		t.Fatal("missing folder was accepted")
	}
	window := windowState{X: -100, Y: 70, Width: 900, Height: 500, Positioned: true}
	dataDir := t.TempDir()
	if err := saveState(dataDir, v.sessionState(window)); err != nil {
		t.Fatal(err)
	}
	saved, err := loadState(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	restored := newRestoredView(saved.Panels, saved.ActivePanel, nil)
	defer restored.loader.close()
	for i := range restored.panels {
		waitLoaded(t, restored, i)
	}
	if restored.active != 1 || restored.panels[0].path != child ||
		restored.panels[1].path != other || restored.panels[2].path != root || saved.Window != window {
		t.Fatalf("session did not restore: %+v", restored.sessionState(saved.Window))
	}
	send, _ = panelInputFrames(restored)
	send(pressed(key.NameDownArrow))
	if restored.panels[1].selected != 1 || restored.panels[0].selected != 0 {
		t.Fatal("restored active panel did not receive keyboard input")
	}
}

func TestRestoreDrivesAndUnavailableFolder(t *testing.T) {
	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	missing := filepath.Join(t.TempDir(), "missing")
	for _, active := range []int{-1, 50} {
		v := newRestoredView([]panelState{{Path: root, Drives: true}, {Path: missing}}, active, nil)
		for i := range v.panels {
			waitLoaded(t, v, i)
		}
		state := v.sessionState(windowState{})
		if v.active != 0 || !state.Panels[0].Drives || state.Panels[0].Path != root ||
			state.Panels[1].Path != missing || v.panels[1].err == nil || v.panels[1].loading {
			t.Fatal("restore lost the drive location or failed folder")
		}
		if status, failed := v.panels[1].status(); !failed || status != "Folder not found" {
			t.Fatal("unavailable saved folder did not show the existing inline error")
		}
		v.loader.close()
	}
}
