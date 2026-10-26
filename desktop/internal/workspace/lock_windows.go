//go:build windows

package workspace

import (
	"os"
	"syscall"
	"unsafe"
)

var lockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")

func acquireLock(path string) (*os.File, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	var overlapped syscall.Overlapped
	r, _, err := lockFileEx.Call(f.Fd(), 3, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	if r == 0 {
		f.Close()
		return nil, err
	}
	return f, nil
}
func releaseLock(f *os.File) { _ = f.Close() }

// Windows file Sync above flushes file buffers; directory fsync is not exposed
// by os.File and must not prevent atomic replacement on that platform.
func syncDirectory(_ *os.File) error { return nil }
