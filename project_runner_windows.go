package main

import (
	"os/exec"
	"syscall"
)

func init() {
	configureProjectProcess = func(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
}
