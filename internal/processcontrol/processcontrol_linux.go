//go:build linux

package processcontrol

import (
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

const defaultParentDeathGracePeriod = 10 * time.Second

func Prepare(cmd *exec.Cmd, options Options) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGTERM,
	}
}

func Guard(cmd *exec.Cmd, options Options) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	gracePeriod := options.ParentDeathGracePeriod
	if gracePeriod <= 0 {
		gracePeriod = defaultParentDeathGracePeriod
	}
	guardian := exec.Command(
		"/bin/sh",
		"-c",
		`parent="$1"; group="$2"; grace="$3"; while kill -0 "$parent" 2>/dev/null && kill -0 -"$group" 2>/dev/null; do sleep 1; done; if kill -0 -"$group" 2>/dev/null; then kill -TERM -"$group" 2>/dev/null; sleep "$grace"; kill -KILL -"$group" 2>/dev/null; fi`,
		"process-guardian",
		strconv.Itoa(os.Getpid()),
		strconv.Itoa(cmd.Process.Pid),
		strconv.FormatFloat(gracePeriod.Seconds(), 'f', -1, 64),
	)
	guardian.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	guardian.Stdin = nil
	guardian.Stdout = io.Discard
	guardian.Stderr = io.Discard
	if err := guardian.Start(); err != nil {
		return
	}
	go func() {
		_ = guardian.Wait()
	}()
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
