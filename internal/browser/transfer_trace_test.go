package browser

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func decodeTransferTrace(t *testing.T, reader io.Reader) []transferTraceRecord {
	t.Helper()
	var records []transferTraceRecord
	decoder := json.NewDecoder(reader)
	for {
		var record transferTraceRecord
		err := decoder.Decode(&record)
		if err == io.EOF {
			return records
		}
		if err != nil {
			t.Fatal(err)
		}
		if record.Sequence != uint64(len(records)+1) || record.Time.IsZero() {
			t.Fatal("trace lost its ordering or timestamp")
		}
		records = append(records, record)
	}
}

func TestTransferTraceCapturesRequestFailureAndFeedback(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	path, stop, err := EnableTransferTrace(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	// Refuse before native execution on every platform, including Windows CI.
	request := TransferRequest{Directory: t.TempDir(), Paths: []string{"relative-file.txt"}}
	result, err := Transfer(request)
	if !errors.Is(err, fs.ErrInvalid) || result.Attempted {
		t.Fatalf("tracing changed refusal: %+v %v", result, err)
	}
	TraceTransferFeedback(result, "Cannot transfer")
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	records := decodeTransferTrace(t, bytes.NewReader(data))
	want := []string{"SessionStarted", "Requested", "Result", "Feedback", "SessionEnded"}
	if len(records) != len(want) {
		t.Fatalf("events: %+v", records)
	}
	for i, record := range records {
		if record.Event != want[i] {
			t.Fatalf("event %d: %s", i, record.Event)
		}
		if i > 0 && i < len(want)-1 && record.Batch != 1 {
			t.Fatal("request, outcome and feedback lost their batch identity")
		}
	}
	requested := records[1].Data.(map[string]any)
	if requested["source_count"] != float64(1) || requested["request"].(map[string]any)["Paths"].([]any)[0] != "relative-file.txt" {
		t.Fatal("trace did not capture the source request")
	}
	if records[3].Data.(map[string]any)["message"] != "Cannot transfer" {
		t.Fatal("trace lost the actual footer")
	}
	TraceTransferFeedback(result, "after close")
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(data, after) || beginTransferTrace(request) != nil {
		t.Fatal("closed trace remained active")
	}
	second, stopSecond, err := EnableTransferTrace(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer stopSecond()
	if second == path {
		t.Fatal("another diagnostic session replaced the previous log")
	}
}

func TestTransferTracePreservesRepeatedRawStatusesAndResultCounts(t *testing.T) {
	var buf bytes.Buffer
	trace := &transferBatchTrace{trace: &transferTrace{writer: &buf}, id: 7}
	for _, status := range []uint32{0x00270011, 0x00270005, 0x00270005} {
		trace.record("PostCopyItem", map[string]any{"hresult": traceHRESULT(status)})
	}
	result := TransferResult{Summary: TransferSummary{Requested: 3, Skipped: 3}, Attempted: true, trace: trace}
	trace.result(result, nil)
	TraceTransferFeedback(result, "0 transferred; 3 skipped")
	records := decodeTransferTrace(t, &buf)
	if len(records) != 5 || records[0].Data.(map[string]any)["hresult"].(map[string]any)["name"] != "COPYENGINE_S_PENDING_BATCH_COPY" {
		t.Fatal("trace did not retain the pending status and repeated callbacks")
	}
	if records[3].Data.(map[string]any)["skipped_count"] != float64(3) {
		t.Fatal("trace altered or lost current counts")
	}
	if traceHRESULT(0xf1234567)["hex"] != "0xF1234567" {
		t.Fatal("unknown HRESULT was discarded")
	}
}

func TestTransferTraceSerializesConcurrentCallbacks(t *testing.T) {
	var buf bytes.Buffer
	trace := &transferBatchTrace{trace: &transferTrace{writer: &buf}, id: 1}
	var workers sync.WaitGroup
	for range 10 {
		workers.Go(func() {
			for range 10 {
				trace.record("UpdateProgress", nil)
			}
		})
	}
	workers.Wait()
	if records := decodeTransferTrace(t, &buf); len(records) != 100 {
		t.Fatal("trace lost concurrent callbacks")
	}
}

type failingTraceWriter struct{}

func (failingTraceWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func TestUnavailableTraceWriterDoesNotChangeResults(t *testing.T) {
	trace := &transferBatchTrace{trace: &transferTrace{writer: failingTraceWriter{}}, id: 1}
	trace.result(TransferResult{Summary: TransferSummary{Requested: 2, Failed: 2}}, errors.New("original failure"))
	TraceTransferFeedback(TransferResult{trace: trace}, "Transfer stopped")
	var disabled *transferBatchTrace
	disabled.record("disabled", nil)
	disabled.result(TransferResult{}, nil)
	TraceTransferFeedback(TransferResult{}, "disabled")
	if name := traceHRESULT(0x00270011)["name"]; !strings.Contains(name, "PENDING") {
		t.Fatal("pending status lost its meaning")
	}
}
