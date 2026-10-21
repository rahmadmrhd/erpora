package httpserver

import "io/fs"

// emptyFS dipakai bila Deps.Assets nil.
type emptyFS struct{}

func (emptyFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }
