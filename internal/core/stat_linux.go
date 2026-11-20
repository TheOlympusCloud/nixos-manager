package core

import (
	"io/fs"
	"syscall"
	"time"
)

// fileID returns what the working-tree fingerprint needs beyond fs.FileInfo:
// the change time, which every write moves even when the modification time is
// put back, and the inode, which a replaced file changes.
func fileID(fi fs.FileInfo) (ctime time.Time, ino uint64, ok bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, 0, false
	}
	return time.Unix(st.Ctim.Unix()), st.Ino, true
}
