//go:build windows

package core

import (
	"golang.org/x/sys/windows"
)

func fileIdentity(path string) (FileID, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return FileID{}, err
	}
	h, err := windows.CreateFile(
		p,
		windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return FileID{}, err
	}
	defer windows.CloseHandle(h)
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return FileID{}, err
	}
	ino := uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)
	return FileID{Dev: uint64(info.VolumeSerialNumber), Ino: ino, Valid: true}, nil
}
