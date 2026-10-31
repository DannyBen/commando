package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestVisitedFoldersDisabledLeavesDiskUntouched(t *testing.T) {
	for _, filename := range []string{visitedFoldersFile, previousVisitedFoldersFile} {
		t.Run(filename, func(t *testing.T) {
			dataDir := t.TempDir()
			path := filepath.Join(dataDir, filename)
			before := []byte("invalid history must not be read or replaced")
			if err := os.WriteFile(path, before, 0600); err != nil {
				t.Fatal(err)
			}
			history, err := loadVisitedFolders(dataDir, false)
			if err != nil || len(history) != 0 {
				t.Fatal("disabled history loaded persisted paths", history, err)
			}
			history.remember(filepath.Join(dataDir, "session-folder"))
			if history.best("session-folder") == "" {
				t.Fatal("disabled persistence prevented session history")
			}
			if err := saveVisitedFolders(dataDir, history, false); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("disabled history changed the existing file", err)
			}
			entries, err := os.ReadDir(dataDir)
			if err != nil || len(entries) != 1 {
				t.Fatal("disabled history created files", entries, err)
			}
		})
	}
	missing := filepath.Join(t.TempDir(), "missing")
	if err := saveVisitedFolders(missing, nil, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("disabled history created its data folder", err)
	}
}

func TestVisitedFoldersPersistAndRank(t *testing.T) {
	root := t.TempDir()
	commando := filepath.Join(root, "go", "commando")
	commands := filepath.Join(root, "work", "commands")
	var history visitedFolders
	history.remember(commando)
	history.remember(commands)
	history.remember(commando)
	if len(history) != 2 || history[0] != commando || history.best("gcom") != commando || history.best("comm") != commando {
		t.Fatalf("history order or fuzzy match: %v", history)
	}
	if err := saveVisitedFolders(root, history, true); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadVisitedFolders(root, true)
	if err != nil || len(loaded) != 2 || loaded[0] != commando || loaded[1] != commands {
		t.Fatalf("saved history = %v, %v", loaded, err)
	}
	loaded.forget(commando)
	if loaded.best("gcom") != "" {
		t.Fatalf("forgotten folder remains: %v", loaded)
	}
	if _, err := os.Stat(filepath.Join(root, visitedFoldersFile)); err != nil {
		t.Fatal(err)
	}
}

func TestVisitedFoldersBounded(t *testing.T) {
	root := t.TempDir()
	var history visitedFolders
	for i := 0; i < visitedFoldersLimit+10; i++ {
		history.remember(filepath.Join(root, strconv.Itoa(i)))
	}
	if len(history) != visitedFoldersLimit || history[0] != filepath.Join(root, strconv.Itoa(visitedFoldersLimit+9)) || history[len(history)-1] != filepath.Join(root, "10") {
		t.Fatalf("history was not bounded: %d", len(history))
	}
}

func TestVisitedFoldersSkipRoots(t *testing.T) {
	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	child := filepath.Join(t.TempDir(), "visited")
	var history visitedFolders
	history.remember(root)
	history.remember(child)
	if len(history) != 1 || history[0] != child {
		t.Fatalf("root was remembered: %v", history)
	}
	dataDir := t.TempDir()
	if err := saveVisitedFolders(dataDir, visitedFolders{root, child}, true); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadVisitedFolders(dataDir, true)
	if err != nil || len(loaded) != 1 || loaded[0] != child {
		t.Fatalf("persisted root was restored: %v, %v", loaded, err)
	}
}

func TestVisitedFoldersReadsPreviousFilename(t *testing.T) {
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, "visited")
	data, err := json.Marshal([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, previousVisitedFoldersFile), data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadVisitedFolders(dataDir, true)
	if err != nil || len(loaded) != 1 || loaded[0] != path {
		t.Fatalf("previous history was lost: %v, %v", loaded, err)
	}
	if err := saveVisitedFolders(dataDir, loaded, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, visitedFoldersFile)); err != nil {
		t.Fatalf("new history filename was not written: %v", err)
	}
}
