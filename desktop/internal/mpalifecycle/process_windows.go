//go:build windows

package mpalifecycle

import "os/exec"

func configureProcess(command *exec.Cmd) {}
func killProcess(command *exec.Cmd) {
	if command.Process != nil {
		_ = command.Process.Kill()
	}
}
