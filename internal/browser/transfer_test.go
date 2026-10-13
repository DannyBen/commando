package browser

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestShellTransferCompletionStatuses(t *testing.T) {
	for _, tc := range []struct {
		name      string
		hr        uint32
		completed bool
	}{
		{"OK", 0, true},
		{"merge", 0x00270006, true},
		{"moved without processing children", 0x00270008, true},
		{"already done", 0x0027000a, true},
		{"keep both", 0x0027000c, true},
		{"collision resolved", 0x0027000e, true},
		{"false", 1, false},
		{"yes decision", 0x00270001, false},
		{"not handled", 0x00270003, false},
		{"retry", 0x00270004, false},
		{"ignored", 0x00270005, false},
		{"pending", 0x0027000b, false},
		{"close program", 0x0027000d, false},
		{"pause", 0x0027000f, false},
		{"cancelled", 0x80270000, false},
		{"access denied", 0x80070005, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shellTransferCompleted(tc.hr); got != tc.completed {
				t.Fatalf("completion status 0x%08x: completed=%v, want %v", tc.hr, got, tc.completed)
			}
		})
	}
}

func TestShellJournalBoundsLargeFolderBookkeepingAndTracksOpenDescendants(t *testing.T) {
	root, target := filepath.Join(t.TempDir(), "folder"), filepath.Join(t.TempDir(), "folder (2)")
	journal := newTransferJournal(TransferRequest{Paths: []string{root}, OpenFolders: []string{filepath.Join(root, "nested", "open")}})
	for i := range 10000 {
		name := fmt.Sprintf("file-%d", i)
		journal.observe(transferObservation{source: filepath.Join(root, "other", name), destination: filepath.Join(target, "other", name)})
	}
	if len(journal.snapshot()) != 1 {
		t.Fatal("journal grew with unrelated child files")
	}
	if journal.needs(filepath.Join(root, "other", "another-file")) {
		t.Fatal("unrelated children still require filesystem metadata")
	}
	nested := filepath.Join(root, "nested")
	if !journal.needs(nested) {
		t.Fatal("open descendant ancestor must still be observed")
	}
	journal.observe(transferObservation{source: nested, destination: filepath.Join(target, "renamed-nested"), directory: true})
	if len(journal.snapshot()) != 2 {
		t.Fatal("open descendant mapping not retained")
	}
	// A root completion supplies its definitive destination after partial child
	// observations; native collision names must win over inferred names.
	actual := filepath.Join(filepath.Dir(target), "actual-root")
	journal.observe(transferObservation{source: root, destination: actual, directory: true})
	if journal.observations[transferPathKey(root)].destination != actual {
		t.Fatal("final root name did not replace inference")
	}
}

func writeTransferFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTransferPreparationDispatchesConflictsWithoutMutating(t *testing.T) {
	source, target := resolvedTempDir(t), resolvedTempDir(t)
	writeTransferFiles(t, source, map[string]string{"same.txt": "source"})
	writeTransferFiles(t, target, map[string]string{"same.txt": "target"})
	request, err := prepareTransfer(TransferRequest{Paths: []string{filepath.Join(source, "same.txt")}, Directory: target, Move: true, Owner: 42})
	if err != nil || request.Owner != 42 || !request.Move || request.Paths[0] != filepath.Join(source, "same.txt") || request.Directory != target {
		t.Fatalf("dispatch preparation: %+v %v", request, err)
	}
	for path, want := range map[string]string{request.Paths[0]: "source", filepath.Join(target, "same.txt"): "target"} {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != want {
			t.Fatal("preparation changed files")
		}
	}
}

func TestDuplicatePreparationKeepsEditableNamesAndClipboardIndependent(t *testing.T) {
	root := t.TempDir()
	writeTransferFiles(t, root, map[string]string{"résumé.txt": "source", "résumé - Copy.txt": "existing", ".env": "dotfile"})
	plan, err := PlanDuplicate([]string{filepath.Join(root, "résumé.txt")}, root)
	if err != nil {
		t.Fatal(err)
	}
	entry, name := plan.DuplicateSubject()
	if name != "résumé - Copy (2).txt" || entry.Name != "résumé.txt" || entry.Directory {
		t.Fatal("wrong suggestion or subject")
	}
	request, err := plan.Request("edited.txt", 72)
	if err != nil || request.Move || !request.Duplicate || request.Owner != 72 || request.Names[0] != "edited.txt" {
		t.Fatalf("named dispatch: %+v %v", request, err)
	}
	if _, err := plan.Request("../outside", 0); !errors.Is(err, ErrInvalidName) {
		t.Fatal("path accepted as a name")
	}
	if _, err := plan.Request("résumé - Copy.txt", 0); !errors.Is(err, fs.ErrExist) {
		t.Fatal("existing name accepted in editor")
	}
	batch, err := prepareTransfer(TransferRequest{Paths: []string{filepath.Join(root, ".env"), filepath.Join(root, "résumé.txt")}, Directory: root, Duplicate: true})
	if err != nil || batch.Names[0] != ".env - Copy" || batch.Names[1] != name {
		t.Fatalf("batch names: %+v %v", batch, err)
	}
}

func TestTransferPreparationResolvesParentAliasesAndKeepsFinalLinks(t *testing.T) {
	source, target := resolvedTempDir(t), resolvedTempDir(t)
	writeTransferFiles(t, source, map[string]string{"file": "data"})
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Skip(err)
	}
	link := filepath.Join(source, "link")
	if err := os.Symlink("file", link); err != nil {
		t.Skip(err)
	}
	request, err := prepareTransfer(TransferRequest{Paths: []string{filepath.Join(alias, "link")}, Directory: target})
	if err != nil || request.Paths[0] != link {
		t.Fatalf("alias/final-link dispatch: %+v %v", request, err)
	}
}

func TestPartialShellMoveAcceptsStateAndReportsMovedDescendants(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	writeTransferFiles(t, source, map[string]string{"folder/remaining": "untouched"})
	writeTransferFiles(t, target, map[string]string{"folder (2)/nested/child": "moved"})
	root := filepath.Join(source, "folder")
	child := filepath.Join(root, "nested")
	result := transferResults(TransferRequest{Paths: []string{root}, Directory: target, Move: true}, []string{"clipboard-alias"}, []transferObservation{
		{source: filepath.Join(child, "child"), destination: filepath.Join(target, "folder (2)", "nested", "child")},
		{source: child, destination: filepath.Join(target, "folder (2)", "nested"), directory: true},
	}, true, TransferSummary{Requested: 1, Partial: 1})
	if !result.Attempted || !result.Aborted || result.Summary.Partial != 1 || result.Summary.Completed != 0 || result.Summary.Skipped != 0 || len(result.Items) != 1 || result.Items[0].Moved || result.Items[0].Source != "clipboard-alias" || result.Items[0].Entry.Name != "folder (2)" {
		t.Fatalf("partial outcome: %+v", result)
	}
	if len(result.Changes) != 2 || !result.Changes[1].Moved || !result.Changes[1].Directory {
		t.Fatal("confirmed child folder not followed")
	}
	if data, err := os.ReadFile(filepath.Join(root, "remaining")); err != nil || string(data) != "untouched" {
		t.Fatal("reconciliation mutated the source")
	}
}

func TestShellCompletionRetainsClipboardOriginalAndActualDestinationName(t *testing.T) {
	source, target := filepath.Join(t.TempDir(), "old.txt"), t.TempDir()
	writeTransferFiles(t, target, map[string]string{"actual.txt": "data"})
	request := TransferRequest{Paths: []string{source}, Directory: target, Move: true}
	result := transferResults(request, []string{"alias-path"}, []transferObservation{{source: source, destination: filepath.Join(target, "actual.txt")}}, false, TransferSummary{Requested: 1, Completed: 1})
	if len(result.Items) != 1 || result.Items[0].Source != "alias-path" || !result.Items[0].Moved || result.Items[0].Entry.Name != "actual.txt" || !result.Changes[0].Moved {
		t.Fatalf("completion mapping: %+v", result)
	}
}

func TestShellBatchCompletionMatchesSourcesAndPrefersExactNames(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	root, skipped, file := filepath.Join(source, "folder"), filepath.Join(source, "skipped"), filepath.Join(source, "file")
	request := TransferRequest{Paths: []string{root, skipped, file}, Directory: target}
	result := transferResults(request, []string{"folder-alias", "skipped-alias", "file-alias"}, []transferObservation{
		{source: filepath.Join(root, "child"), destination: filepath.Join(target, "inferred", "child")},
		{source: file, destination: filepath.Join(target, "renamed-file")},
		{source: root, destination: filepath.Join(target, "actual-folder"), directory: true},
		{source: root, destination: filepath.Join(target, "later-folder"), directory: true},
	}, false, TransferSummary{Requested: 3, Completed: 2, Skipped: 1})
	if len(result.Items) != 2 || result.Items[0].Source != "folder-alias" || result.Items[0].Entry.Name != "actual-folder" || !result.Items[0].Entry.Directory || result.Items[1].Source != "file-alias" || result.Items[1].Entry.Name != "renamed-file" {
		t.Fatalf("unordered batch completion: %+v", result)
	}
}

func BenchmarkTransferResults(b *testing.B) {
	for _, count := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			root := b.TempDir()
			request := TransferRequest{Directory: root}
			var observations []transferObservation
			for i := range count {
				source := filepath.Join(root, "source", fmt.Sprint(i))
				request.Paths = append(request.Paths, source)
				if i%2 == 0 {
					observations = append(observations, transferObservation{source: source, destination: filepath.Join(root, "target", fmt.Sprint(i))})
				}
			}
			b.ResetTimer()
			for b.Loop() {
				transferResults(request, request.Paths, observations, false, TransferSummary{})
			}
		})
	}
}
