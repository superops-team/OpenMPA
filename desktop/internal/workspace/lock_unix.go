//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package workspace

import (
	"os"
	"syscall"
)

// The lock file is never unlinked: removing an inode while another process
// holds it would let a third process acquire a different, concurrent lock.
func acquireLock(path string) (*os.File, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, e
	}
	return f, nil
}
func releaseLock(f *os.File)         { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }
func syncDirectory(f *os.File) error { return f.Sync() }
