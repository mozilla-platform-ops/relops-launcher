//go:build darwin || linux

package launcher

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
	"unsafe"
)

func terminalGroup(file *os.File, request uintptr, group *int32) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), request, uintptr(unsafe.Pointer(group)))
	if errno != 0 {
		return errno
	}
	return nil
}

func runTool(ctx context.Context, cmd *exec.Cmd) (int, error) {
	if err := ctx.Err(); err != nil {
		return 1, err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second
	// Only take foreground ownership if this launcher already owns the terminal.
	var terminal *os.File
	var parentGroup int32
	if input, ok := cmd.Stdin.(*os.File); ok {
		var foreground int32
		if terminalGroup(input, syscall.TIOCGPGRP, &foreground) == nil && foreground == int32(syscall.Getpgrp()) {
			terminal, parentGroup = input, foreground
		}
	}
	if terminal != nil {
		// Restoring the foreground group happens while the launcher is backgrounded.
		signal.Ignore(syscall.SIGTTOU)
		defer signal.Reset(syscall.SIGTTOU)
	}
	if err := cmd.Start(); err != nil {
		return 1, fmt.Errorf("start tool (check its shebang/interpreter and prerequisites): %w", err)
	}
	pid := cmd.Process.Pid
	if terminal != nil {
		defer func() { _ = terminalGroup(terminal, syscall.TIOCSPGRP, &parentGroup) }()
		childGroup := int32(pid)
		if err := terminalGroup(terminal, syscall.TIOCSPGRP, &childGroup); err != nil {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			_ = cmd.Wait()
			return 1, fmt.Errorf("give tool terminal control: %w", err)
		}
		// A fast child may have tried reading before the foreground transfer.
		_ = syscall.Kill(-pid, syscall.SIGCONT)
	}
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	var err error
	select {
	case err = <-wait:
	case <-ctx.Done():
		sig := syscall.SIGTERM
		if cause, ok := context.Cause(ctx).(interrupted); ok {
			sig = cause.signal
		}
		_ = syscall.Kill(-pid, sig)
		timer := time.NewTimer(5 * time.Second)
		select {
		case err = <-wait:
			if !timer.Stop() {
				<-timer.C
			}
		case <-timer.C:
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			err = <-wait
		}
	}
	// Reap the leader first, then stop descendants left in its process group before
	// deleting their checkout. Descendants must not outlive this invocation.
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	if err == nil {
		return 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		status := exit.Sys().(syscall.WaitStatus)
		if status.Signaled() {
			return 128 + int(status.Signal()), nil
		}
		return exit.ExitCode(), nil
	}
	return 1, fmt.Errorf("wait for tool: %w", err)
}
