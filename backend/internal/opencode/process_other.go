//go:build !linux

package opencode

import (
	"os"
	"os/exec"
	"time"
)

func configureProcess(cmd *exec.Cmd) { cmd.WaitDelay = 2 * time.Second }
func processIdentity(int) string     { return "" }
func TerminateProcessGroup(pid int, _ string) error {
	if pid <= 0 {
		return os.ErrProcessDone
	}
	return os.ErrProcessDone
}
