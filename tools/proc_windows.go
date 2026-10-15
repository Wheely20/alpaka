//go:build windows

package tools

import (
	"os/exec"
	"strconv"
)

func setupCmdProcessGroup(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			// /T kills the process including all child processes
			return exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		}
		return nil
	}
}
