package credentials

import "github.com/egoist/mygo"

// New has no filesystem, native-store, or framework I/O. The default MyGo
// singleton chooses the actual app identifier, including its .dev suffix.
// Do not use Service to hard-code a shared production/development namespace.
func New() *Store { return NewWithBackend(mygo.Secrets) }
