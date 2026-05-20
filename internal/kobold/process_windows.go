package kobold

import (
	"os"
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func prepareCommand(cmd *exec.Cmd, config ProcessConfig) {
	if !config.HideWindow {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

func terminateCommand(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Signal(os.Interrupt)
}

func killCommand(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
