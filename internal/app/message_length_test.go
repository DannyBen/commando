package app

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"github.com/dannyben/commando/internal/browser"
)

// Count prefixes and long filenames used to expand otherwise brief errors.
// Exercise the produced footers, including confirmed partial outcomes.
func TestOperationErrorsFitShortPanelFooter(t *testing.T) {
	errorsToReport := []error{
		browser.ErrNoClipboardFiles, browser.ErrInvalidName, fs.ErrExist, fs.ErrPermission, fs.ErrNotExist,
		browser.ErrTransferUnavailable,
		browser.ErrDriveMapping, browser.ErrDriveMappingCycle, browser.ErrPathIdentity,
		browser.ErrRecycleUnavailable, browser.ErrRecycleUnconfirmed, browser.ErrDeleteUnavailable, browser.ErrDeleteUnconfirmed, errors.ErrUnsupported, errors.New("other"),
	}
	filename := "a very long filename with many words.txt"
	for _, err := range errorsToReport {
		for _, partial := range []bool{false, true} {
			wrapped := &os.PathError{Op: "test", Path: filename, Err: err}
			v := newPreviewView()
			result := loadResult{request: loadRequest{panel: 0, transfer: &transferRequest{}}, err: wrapped}
			if partial {
				result.transferred.Items = []browser.TransferredItem{{Entry: browser.Entry{Name: "completed.txt"}}}
				result.transferred.Summary = browser.TransferSummary{Requested: 1, Completed: 1}
			}
			v.acceptFileOperation(result)
			assertBriefFooter(t, v.panels[0].feedback.Current().Text, filename)

			v = newPreviewView()
			result = loadResult{request: loadRequest{panel: 0, removal: &removalRequest{}}, err: wrapped}
			if partial {
				result.removed.Names = []string{"completed.txt"}
			}
			v.acceptRemoval(result)
			assertBriefFooter(t, v.panels[0].feedback.Current().Text, filename)

			v = newPreviewView()
			result.request.removal.permanent = true
			v.acceptRemoval(result)
			assertBriefFooter(t, v.panels[0].feedback.Current().Text, filename)
		}
		for _, text := range []string{readError(err), renameError(err), createError(err, "folder"), createError(err, "file")} {
			assertBriefFooter(t, text, filename)
		}
	}
}

func assertBriefFooter(t *testing.T, message, filename string) {
	t.Helper()
	if len(strings.Fields(message)) > 5 || strings.Contains(message, filename) {
		t.Fatalf("footer is too long or repeats a filename: %q", message)
	}
}
