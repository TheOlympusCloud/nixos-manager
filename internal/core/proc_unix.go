//go:build unix

package core

import (
	"os/exec"
	"syscall"
)

// ownGroup gives cmd a process group of its own, killed whole when its context
// ends: an ssh to a builder or a git fetch that nix started dies with it.
func ownGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
