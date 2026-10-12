package browser

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

// Transfer tracing is opt-in. It observes the shell and application outcomes;
// logging failures never change callback return values or transfer decisions.
var activeTransferTrace atomic.Pointer[transferTrace]

type transferTrace struct {
	mu       sync.Mutex
	writer   io.Writer
	sequence uint64
	batches  atomic.Uint64
}

type transferBatchTrace struct {
	trace *transferTrace
	id    uint64
}

type transferTraceRecord struct {
	Time     time.Time `json:"time"`
	Sequence uint64    `json:"sequence"`
	Batch    uint64    `json:"batch,omitempty"`
	Event    string    `json:"event"`
	Data     any       `json:"data,omitempty"`
}

// EnableTransferTrace opens a separate log for this application session in the
// shared data folder. Stop tracing after the directory worker has drained.
func EnableTransferTrace(dataDir string) (path string, stop func() error, err error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return "", nil, err
	}
	f, err := os.CreateTemp(dataDir, "transfer-trace-*.jsonl")
	if err != nil {
		return "", nil, err
	}
	t := &transferTrace{writer: f}
	activeTransferTrace.Store(t)
	executable, _ := os.Executable()
	build, _ := debug.ReadBuildInfo()
	t.record(0, "SessionStarted", map[string]any{
		"schema": 2, "pid": os.Getpid(), "executable": executable,
		"go": runtime.Version(), "build": build,
	})
	var once sync.Once
	var closeErr error
	return filepath.Clean(f.Name()), func() error {
		once.Do(func() {
			activeTransferTrace.CompareAndSwap(t, nil)
			t.record(0, "SessionEnded", nil)
			t.mu.Lock()
			defer t.mu.Unlock()
			closeErr = f.Close()
			t.writer = nil
		})
		return closeErr
	}, nil
}

func beginTransferTrace(request TransferRequest) *transferBatchTrace {
	t := activeTransferTrace.Load()
	if t == nil {
		return nil
	}
	b := &transferBatchTrace{trace: t, id: t.batches.Add(1)}
	b.record("Requested", map[string]any{"source_count": len(request.Paths), "request": request})
	return b
}

func (t *transferTrace) record(batch uint64, event string, data any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.writer == nil {
		return
	}
	t.sequence++
	_ = json.NewEncoder(t.writer).Encode(transferTraceRecord{
		Time: time.Now().UTC(), Sequence: t.sequence, Batch: batch, Event: event, Data: data,
	})
}

func (b *transferBatchTrace) record(event string, data any) {
	if b != nil {
		b.trace.record(b.id, event, data)
	}
}

func (b *transferBatchTrace) result(result TransferResult, err error) {
	b.record("Result", map[string]any{
		"transferred_count": result.Summary.Completed, "skipped_count": result.Summary.Skipped,
		"reconciliation_item_count": len(result.Items),
		"result":                    result, "error": traceError(err),
	})
}

// TraceTransferFeedback links the footer's actual message to its native batch.
func TraceTransferFeedback(result TransferResult, message string) {
	result.trace.record("Feedback", map[string]any{"message": message})
}

func traceError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func traceHRESULT(hr uint32) map[string]string {
	name := ""
	switch hr {
	case 0:
		name = "S_OK"
	case 1:
		name = "S_FALSE"
	case 0x00270001:
		name = "COPYENGINE_S_YES"
	case 0x00270003:
		name = "COPYENGINE_S_NOT_HANDLED"
	case 0x00270004:
		name = "COPYENGINE_S_USER_RETRY"
	case 0x00270005:
		name = "COPYENGINE_S_USER_IGNORED"
	case 0x00270006:
		name = "COPYENGINE_S_MERGE"
	case 0x00270008:
		name = "COPYENGINE_S_DONT_PROCESS_CHILDREN"
	case 0x0027000a:
		name = "COPYENGINE_S_ALREADY_DONE"
	case 0x0027000b:
		name = "COPYENGINE_S_PENDING"
	case 0x0027000c:
		name = "COPYENGINE_S_KEEP_BOTH"
	case 0x0027000d:
		name = "COPYENGINE_S_CLOSE_PROGRAM"
	case 0x0027000e:
		name = "COPYENGINE_S_COLLISIONRESOLVED"
	case 0x0027000f:
		name = "COPYENGINE_S_PROGRESS_PAUSE"
	case 0x00270010:
		name = "COPYENGINE_S_PENDING_DELETE"
	case 0x00270011:
		name = "COPYENGINE_S_PENDING_BATCH_COPY"
	case 0x80270000:
		name = "COPYENGINE_E_USER_CANCELLED"
	case 0x80270001:
		name = "COPYENGINE_E_CANCELLED"
	case 0x800704c7:
		name = "ERROR_CANCELLED"
	case 0x80070005:
		name = "E_ACCESSDENIED"
	case 0x80004004:
		name = "E_ABORT"
	}
	return map[string]string{"hex": fmt.Sprintf("0x%08X", hr), "name": name}
}
