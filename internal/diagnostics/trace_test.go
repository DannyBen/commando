package diagnostics

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func readLog(t *testing.T, dir string) []map[string]any {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, "trace-log.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var records []map[string]any
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return records
}

func TestFilteringLiveChangesAndAppend(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(func() { Close() })
	if err := Configure(dir, 0); err != nil {
		t.Fatal(err)
	}
	Record(Shell, 0, "Disabled", nil)
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatal("OFF created a file")
	}
	if err := Configure(dir, FileOperations|UI); err != nil {
		t.Fatal(err)
	}
	id := NextID()
	Record(FileOperations, id, "Requested", nil)
	Record(Shell, id, "Excluded", nil)
	Record(UI, id, "Confirmed", nil)
	if err := Configure(dir, Shell); err != nil {
		t.Fatal(err)
	}
	Record(FileOperations, id, "Excluded", nil)
	Record(Shell, id, "Native", nil)
	if err := Close(); err != nil {
		t.Fatal(err)
	}
	first := readLog(t, dir)
	if err := Configure(dir, All); err != nil {
		t.Fatal(err)
	}
	Record(Browsing, 0, "Read", nil)
	Close()
	records := readLog(t, dir)
	if len(records) <= len(first) || records[len(first)]["event"] != "SessionStarted" || records[0]["session"] == records[len(first)]["session"] {
		t.Fatal("sessions did not append independently")
	}
	want := []string{"Requested", "Confirmed", "Native", "Read"}
	var got []string
	for _, r := range records {
		switch r["event"] {
		case "SessionStarted", "SessionEnded", "MaskChanged":
			continue
		}
		got = append(got, r["event"].(string))
	}
	if len(got) != len(want) {
		t.Fatalf("events: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events: %v", got)
		}
	}
	if records[2]["operation"] != float64(id) || records[3]["operation"] != float64(id) {
		t.Fatal("operation identity lost")
	}
}

func TestConcurrentRecordsAndInvalidConfiguration(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(func() { Close() })
	if err := Configure(dir, All); err != nil {
		t.Fatal(err)
	}
	if err := Configure(dir, 128); err == nil {
		t.Fatal("unknown bits accepted")
	}
	if !Enabled(Shell) {
		t.Fatal("invalid configuration changed mask")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 20 {
				Record(UI, NextID(), "Event", nil)
			}
		})
	}
	wg.Wait()
	Close()
	records := readLog(t, dir)
	if len(records) != 163 {
		t.Fatalf("lost records: %d", len(records))
	}
	for i, r := range records {
		if r["sequence"] != float64(i+1) {
			t.Fatal("unordered records")
		}
	}
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := Configure(blocked, UI); err == nil {
		t.Fatal("expected open failure")
	}
	Record(UI, 0, "NoPanic", nil)
}

func TestMaskLabels(t *testing.T) {
	for mask, want := range map[Mask]string{0: "0 — OFF", 1: "1 — File operations", 2: "2 — UI", 3: "3", 4: "4 — Shell", 8: "8 — Browsing", 16: "16 — Storage", 32: "32 — Lifecycle", 64: "64 — Feedback", 127: "127 — ALL"} {
		if got := mask.String(); got != want {
			t.Errorf("%d: %s", mask, got)
		}
	}
}
