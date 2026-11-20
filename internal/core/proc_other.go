//go:build !unix

package core

import "os/exec"

func ownGroup(*exec.Cmd) {} // WaitDelay alone bounds the wait
