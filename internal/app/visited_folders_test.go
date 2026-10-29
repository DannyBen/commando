package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

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
	if err := saveVisitedFolders(root, history); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadVisitedFolders(root)
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
	if err := saveVisitedFolders(dataDir, visitedFolders{root, child}); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadVisitedFolders(dataDir)
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
	loaded, err := loadVisitedFolders(dataDir)
	if err != nil || len(loaded) != 1 || loaded[0] != path {
		t.Fatalf("previous history was lost: %v, %v", loaded, err)
	}
	if err := saveVisitedFolders(dataDir, loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, visitedFoldersFile)); err != nil {
		t.Fatalf("new history filename was not written: %v", err)
	}
}
