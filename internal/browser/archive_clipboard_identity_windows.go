package browser

import (
	"fmt"
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

func archiveClipboardIdentity(source *os.File, _ fs.FileInfo) (string, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(windows.Handle(source.Fd()), &info); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x:%x:%x", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow), nil
}
