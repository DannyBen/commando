package browser

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/dannyben/commando/internal/diagnostics"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Native integration tests use the same fixed log and recorder as production.
func EnableTransferTrace(dir string) (string, func() error, error) {
	err := diagnostics.Configure(dir, diagnostics.FileOperations)
	return filepath.Join(dir, "trace-log.jsonl"), diagnostics.Close, err
}

type transferTraceRecord struct {
	Time     time.Time `json:"time"`
	Sequence uint64    `json:"sequence"`
	Batch    uint64    `json:"operation"`
	Group    string    `json:"group"`
	Event    string    `json:"event"`
	Data     any       `json:"data"`
}

func decodeTransferTrace(t *testing.T, reader io.Reader) []transferTraceRecord {
	t.Helper()
	var records []transferTraceRecord
	decoder := json.NewDecoder(reader)
	var previous uint64
	for {
		var r transferTraceRecord
		err := decoder.Decode(&r)
		if err == io.EOF {
			return records
		}
		if err != nil {
			t.Fatal(err)
		}
		if r.Sequence <= previous || r.Time.IsZero() {
			t.Fatal("trace lost ordering or timestamp")
		}
		previous = r.Sequence
		if r.Group == "File operations" {
			records = append(records, r)
		}
	}
}

func TestTransferTraceCapturesRequestFailureAndFeedback(t *testing.T) {
	dir := t.TempDir()
	path, stop, err := EnableTransferTrace(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
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
	want := []string{"Requested", "Result", "Feedback"}
	if len(records) != len(want) {
		t.Fatalf("events: %+v", records)
	}
	for i, r := range records {
		if r.Event != want[i] || r.Batch == 0 || r.Batch != records[0].Batch {
			t.Fatalf("lost operation event or identity: %+v", r)
		}
	}
	requested := records[0].Data.(map[string]any)
	if requested["source_count"] != float64(1) || requested["request"].(map[string]any)["Paths"].([]any)[0] != "relative-file.txt" {
		t.Fatal("lost source request")
	}
	if records[2].Data.(map[string]any)["message"] != "Cannot transfer" {
		t.Fatal("lost footer")
	}
	TraceTransferFeedback(result, "after close")
	after, _ := os.ReadFile(path)
	if !bytes.Equal(data, after) || beginTransferTrace(request) != nil {
		t.Fatal("closed trace remained active")
	}
}

func TestTransferTracePreservesRepeatedRawStatusesAndResultCounts(t *testing.T) {
	path, stop, err := EnableTransferTrace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	trace := &transferBatchTrace{id: diagnostics.NextID()}
	for _, status := range []uint32{0x00270011, 0x00270005, 0x00270005} {
		trace.record("PostCopyItem", map[string]any{"hresult": traceHRESULT(status)})
	}
	result := TransferResult{Summary: TransferSummary{Requested: 3, Skipped: 3}, Attempted: true, trace: trace}
	trace.result(result, nil)
	TraceTransferFeedback(result, "0 transferred; 3 skipped")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	records := decodeTransferTrace(t, bytes.NewReader(data))
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

func TestDisabledTraceAndUnknownNativeStatus(t *testing.T) {
	var disabled *transferBatchTrace
	disabled.record("disabled", nil)
	disabled.result(TransferResult{}, nil)
	TraceTransferFeedback(TransferResult{}, "disabled")
	if name := traceHRESULT(0x00270011)["name"]; !strings.Contains(name, "PENDING") {
		t.Fatal("pending status lost its meaning")
	}
	if traceHRESULT(0xf1234567)["hex"] != "0xF1234567" {
		t.Fatal("unknown HRESULT discarded")
	}
}
