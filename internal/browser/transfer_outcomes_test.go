package browser

import (
	"fmt"
	"path/filepath"
	"testing"
)

func assertTransferSummary(t *testing.T, got, want TransferSummary) {
	t.Helper()
	if got != want {
		t.Fatalf("summary %+v, want %+v", got, want)
	}
	if got.Completed+got.Skipped+got.Partial+got.Failed+got.Unfinished != got.Requested {
		t.Fatal("source outcomes overlap or omit a requested entry")
	}
}

func TestTransferOutcomesReplayWindowsConflictTrace(t *testing.T) {
	// Sequences captured in the native trace: pending precedes success,
	// cancellation, or user-ignored. Straight copies report success directly.
	for _, tc := range []struct {
		name     string
		statuses []uint32
		want     TransferSummary
	}{
		{"replace", []uint32{0x0027000b, 0}, TransferSummary{Requested: 1, Completed: 1}},
		{"cancel", []uint32{0x0027000b, 0x80270000}, TransferSummary{Requested: 1, Unfinished: 1, cancelled: true}},
		{"straight copy", []uint32{0}, TransferSummary{Requested: 1, Completed: 1}},
		{"skip", []uint32{0x0027000b, 0x00270005}, TransferSummary{Requested: 1, Skipped: 1}},
		{"deferred batch replace", []uint32{0x00270011, 0}, TransferSummary{Requested: 1, Completed: 1}},
		{"retry succeeds", []uint32{0x80070005, 0x00270004, 0}, TransferSummary{Requested: 1, Completed: 1}},
		{"failure becomes skip", []uint32{0x80070005, 0x00270005}, TransferSummary{Requested: 1, Skipped: 1}},
		{"failed", []uint32{0x80070005}, TransferSummary{Requested: 1, Failed: 1, failure: 0x80070005}},
		{"still pending", []uint32{0x0027000b}, TransferSummary{Requested: 1, Unfinished: 1}},
		{"unknown information", []uint32{0x0027ffff}, TransferSummary{Requested: 1, Unfinished: 1}},
		{"duplicate skips", []uint32{0x00270005, 0x00270005}, TransferSummary{Requested: 1, Skipped: 1}},
		{"duplicate success", []uint32{0, 0x0027000a, 0x00270005}, TransferSummary{Requested: 1, Completed: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := filepath.Join("source", "file.txt")
			outcomes := newTransferOutcomes(TransferRequest{Paths: []string{source}})
			for _, hr := range tc.statuses {
				outcomes.observe(source, hr)
			}
			assertTransferSummary(t, outcomes.summary(false), tc.want)
		})
	}
	for _, status := range []uint32{0, 0x00270005} {
		request := TransferRequest{}
		for i := range 5 {
			request.Paths = append(request.Paths, filepath.Join("source", fmt.Sprintf("file-%d.txt", i)))
		}
		outcomes := newTransferOutcomes(request)
		for _, source := range request.Paths {
			outcomes.observe(source, 0x0027000b)
		}
		for _, source := range request.Paths {
			outcomes.observe(source, status)
		}
		want := TransferSummary{Requested: 5, Skipped: 5}
		if status == 0 {
			want.Completed, want.Skipped = 5, 0
		}
		assertTransferSummary(t, outcomes.summary(false), want)
	}
}

func TestTransferOutcomesIgnoreUnrequestedSourcesAndDeduplicateRoots(t *testing.T) {
	file := filepath.Join("source", "file.txt")
	outcomes := newTransferOutcomes(TransferRequest{Paths: []string{file, file}})
	outcomes.observe(filepath.Join("target", "unrelated.txt"), 0x00270005)
	outcomes.observe(file, 0)
	outcomes.observe(file, 0)
	assertTransferSummary(t, outcomes.summary(false), TransferSummary{Requested: 1, Completed: 1})
}

func TestTransferOutcomesSeparateFolderMappingsFromCompletion(t *testing.T) {
	root := filepath.Join("source", "folder")
	for _, tc := range []struct {
		name       string
		rootStatus uint32
		children   []uint32
		aborted    bool
		want       TransferSummary
	}{
		{"merge all skipped", 0x00270006, []uint32{0x00270005, 0x00270005}, false, TransferSummary{Requested: 1, Skipped: 1}},
		{"merge partly copied", 0x00270006, []uint32{0, 0x00270005}, false, TransferSummary{Requested: 1, Partial: 1}},
		{"new folder with skipped child", 0, []uint32{0x00270005}, false, TransferSummary{Requested: 1, Partial: 1}},
		{"merge complete", 0x00270006, []uint32{0, 0}, false, TransferSummary{Requested: 1, Completed: 1}},
		{"empty merge", 0x00270006, nil, false, TransferSummary{Requested: 1, Completed: 1}},
		{"interrupted merge", 0x00270006, []uint32{0}, true, TransferSummary{Requested: 1, Partial: 1}},
		{"cancelled child", 0x00270006, []uint32{0, 0x80270000}, false, TransferSummary{Requested: 1, Partial: 1, cancelled: true}},
		{"child fails", 0x00270006, []uint32{0, 0x80070005}, false, TransferSummary{Requested: 1, Partial: 1, failure: 0x80070005}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			outcomes := newTransferOutcomes(TransferRequest{Paths: []string{root}})
			outcomes.observe(root, tc.rootStatus)
			for i, hr := range tc.children {
				outcomes.observe(filepath.Join(root, fmt.Sprintf("child-%d", i)), hr)
			}
			assertTransferSummary(t, outcomes.summary(tc.aborted), tc.want)
		})
	}
	outcomes := newTransferOutcomes(TransferRequest{Paths: []string{root}})
	outcomes.observe(root, 0x00270006)
	child := filepath.Join(root, "retry.txt")
	outcomes.observe(child, 0x80070005)
	outcomes.observe(child, 0x00270004)
	outcomes.observe(child, 0)
	assertTransferSummary(t, outcomes.summary(false), TransferSummary{Requested: 1, Completed: 1})
}

func TestTransferOutcomesBoundSuccessfulAndSkippedFolderBookkeeping(t *testing.T) {
	root := filepath.Join("source", "folder")
	outcomes := newTransferOutcomes(TransferRequest{Paths: []string{root}})
	outcomes.observe(root, 0x00270006)
	for i := range 10000 {
		hr := uint32(0)
		if i%2 == 0 {
			hr = 0x00270005
		}
		outcomes.observe(filepath.Join(root, fmt.Sprintf("child-%d", i)), hr)
	}
	assertTransferSummary(t, outcomes.summary(false), TransferSummary{Requested: 1, Partial: 1})
	if len(outcomes.roots) != 1 || len(outcomes.roots[transferPathKey(root)].childFailures) != 0 {
		t.Fatal("ordinary child callbacks accumulated per-file bookkeeping")
	}
}
