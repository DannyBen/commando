package app

import (
	"errors"
	"io/fs"

	"github.com/dannyben/commando/internal/browser"
)

func fileTransferError(err error) string {
	message := "Cannot transfer"
	switch {
	case errors.Is(err, browser.ErrArchiveChanged), errors.Is(err, browser.ErrArchiveRecovery), errors.Is(err, browser.ErrInvalidArchive):
		return deleteError(err)
	case errors.Is(err, browser.ErrArchiveSourceChanged):
		return "Source changed; retry"
	case errors.Is(err, browser.ErrArchiveSelfCopy):
		return "Archive cannot contain itself"
	case errors.Is(err, browser.ErrNoClipboardFiles):
		return "Nothing to paste"
	case errors.Is(err, browser.ErrClipboardImage):
		return "Unsupported clipboard image"
	case errors.Is(err, browser.ErrInvalidName):
		return "Invalid filename"
	case errors.Is(err, fs.ErrExist):
		message = "Name already exists"
	case errors.Is(err, fs.ErrPermission):
		message = "Access denied"
	case errors.Is(err, fs.ErrNotExist):
		message = "Item not found"
	case errors.Is(err, browser.ErrTransferUnavailable):
		return "Windows required"
	case errors.Is(err, browser.ErrDriveMapping), errors.Is(err, browser.ErrDriveMappingCycle), errors.Is(err, browser.ErrPathIdentity):
		return "Backing path unavailable"
	case errors.Is(err, errors.ErrUnsupported):
		message = "Unsupported file type"
	}
	return message
}
