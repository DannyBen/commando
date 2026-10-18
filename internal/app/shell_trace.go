package app

import "github.com/dannyben/commando/internal/diagnostics"

func traceShellCompletion(result shellCompletion) {
	diagnostics.Record(diagnostics.Shell, result.session.traceID, "NativeCompleted", map[string]any{
		"action": result.session.request.action, "queue_ms": result.queue.Seconds() * 1000,
		"timings": result.result.Timings, "cancelled": result.result.Cancelled,
		"effect": result.result.Effect, "error": diagnostics.Error(result.err),
	})
}
