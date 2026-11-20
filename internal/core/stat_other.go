//go:build !linux

package core

import (
	"io/fs"
	"time"
)

// fileID is only known on Linux; elsewhere the working tree is never cached.
func fileID(fs.FileInfo) (time.Time, uint64, bool) { return time.Time{}, 0, false }
