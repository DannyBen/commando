package browser

import (
	"fmt"
	"github.com/dannyben/commando/internal/diagnostics"
)

// Native callbacks and the resulting feedback retain one operation identity.
type transferBatchTrace struct{ id uint64 }

func beginTransferTrace(request TransferRequest) *transferBatchTrace {
	if !diagnostics.Enabled(diagnostics.FileOperations) {
		return nil
	}
	b := &transferBatchTrace{id: diagnostics.NextID()}
	b.record("Requested", map[string]any{"source_count": len(request.Paths), "request": request})
	return b
}

func beginRemovalTrace(paths []string, permanent bool) *transferBatchTrace {
	if !diagnostics.Enabled(diagnostics.FileOperations) {
		return nil
	}
	b := &transferBatchTrace{id: diagnostics.NextID()}
	b.record("RemovalRequested", map[string]any{"paths": paths, "permanent": permanent})
	return b
}

func (b *transferBatchTrace) record(event string, data any) {
	if b != nil {
		diagnostics.Record(diagnostics.FileOperations, b.id, event, data)
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
