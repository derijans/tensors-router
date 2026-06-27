//go:build !windows && !linux

package processcontrol

import (
	"os"
	"os/exec"
	"syscall"
)

func Prepare(cmd *exec.Cmd, options Options) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func Guard(cmd *exec.Cmd, options Options) {
}

func Interrupt(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGINT); err == nil {
		return nil
	}
	return cmd.Process.Signal(os.Interrupt)
}

func Kill(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err == nil {
		return nil
	}
	return cmd.Process.Kill()
}
