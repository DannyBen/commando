package app

import (
	"errors"
	"io/fs"

	"github.com/dannyben/commando/internal/browser"
)

func fileTransferError(err error) string {
	message := "Cannot transfer"
	switch {
	case errors.Is(err, browser.ErrNoClipboardFiles):
		return "No clipboard files"
	case errors.Is(err, browser.ErrInvalidName):
		return "Invalid filename"
	case errors.Is(err, fs.ErrExist):
		message = "Name already exists"
	case errors.Is(err, fs.ErrPermission):
		message = "Access denied"
	case errors.Is(err, fs.ErrNotExist):
		message = "Item not found"
	case errors.Is(err, browser.ErrTransferIntoSelf):
		message = "Destination inside source"
	case errors.Is(err, browser.ErrOverlappingSources):
		message = "Overlapping clipboard items"
	case errors.Is(err, browser.ErrSourceChanged):
		message = "Copied; source changed"
	case errors.Is(err, browser.ErrSameTransferFile):
		message = "Same file"
	case errors.Is(err, browser.ErrConflictChanged):
		return "Files changed; retry"
	case errors.Is(err, browser.ErrTransferType):
		return "File/folder type conflict"
	case errors.Is(err, browser.ErrDriveMapping), errors.Is(err, browser.ErrDriveMappingCycle), errors.Is(err, browser.ErrPathIdentity):
		return "Backing path unavailable"
	case errors.Is(err, errors.ErrUnsupported):
		message = "Unsupported file type"
	}
	return message
}
