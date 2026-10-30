package artifact

import (
	"io"
	"os"
)

// Files is injected for disk-failure tests. Only this package uses these paths;
// none of its filesystem methods are bound to renderer IPC.
type Files interface {
	Lstat(string) (os.FileInfo, error)
	CreateTemp(string, string) (TempFile, error)
	Rename(string, string) error
	Link(string, string) error
	Remove(string) error
}
type TempFile interface {
	io.Writer
	Sync() error
	Close() error
	Name() string
}
type OSFiles struct{}

func (OSFiles) Lstat(p string) (os.FileInfo, error)              { return os.Lstat(p) }
func (OSFiles) CreateTemp(dir, pattern string) (TempFile, error) { return os.CreateTemp(dir, pattern) }
func (OSFiles) Rename(old, new string) error                     { return os.Rename(old, new) }
func (OSFiles) Link(old, new string) error                       { return os.Link(old, new) }
func (OSFiles) Remove(p string) error                            { return os.Remove(p) }
