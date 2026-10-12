package app

import (
	"testing"

	"github.com/dannyben/commando/internal/browser"
	"github.com/dannyben/commando/internal/feedback"
)

func TestTransferFeedbackUsesSourceOutcomesRatherThanReconciliationItems(t *testing.T) {
	for _, tc := range []struct {
		name    string
		summary browser.TransferSummary
		aborted bool
		items   int
		want    string
		kind    feedback.Kind
	}{
		{"replace", browser.TransferSummary{Requested: 1, Completed: 1}, false, 1, "1 copied", feedback.Success},
		{"skip one", browser.TransferSummary{Requested: 1, Skipped: 1}, false, 0, "0 transferred; 1 skipped", feedback.Warning},
		{"skip five", browser.TransferSummary{Requested: 5, Skipped: 5}, false, 0, "0 transferred; 5 skipped", feedback.Warning},
		{"cancel", browser.TransferSummary{Requested: 1, Unfinished: 1}, true, 0, "Transfer stopped", feedback.Info},
		{"partial folder", browser.TransferSummary{Requested: 1, Partial: 1}, false, 1, "Transfer partly completed", feedback.Warning},
		{"cancel partial folder", browser.TransferSummary{Requested: 1, Partial: 1}, true, 1, "1 partial; operation stopped", feedback.Info},
		{"missing outcome", browser.TransferSummary{Requested: 1, Unfinished: 1}, false, 0, "Transfer incomplete", feedback.Warning},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := newPreviewView()
			result := loadResult{request: loadRequest{panel: 1, transfer: &transferRequest{}},
				transferred: browser.TransferResult{Summary: tc.summary, Aborted: tc.aborted, Items: make([]browser.TransferredItem, tc.items)}}
			v.acceptFileOperation(result)
			message := v.panels[1].feedback.Current()
			if message == nil || message.Text != tc.want || message.Kind != tc.kind {
				t.Fatalf("feedback: %+v", message)
			}
			assertBriefFooter(t, message.Text, "filename.txt")
		})
	}
}
