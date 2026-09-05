//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func setUtf8Console() {}

// qrStyleForConsole 非 Windows 终端默认西文字体,用半块渲染。
func qrStyleForConsole() qrStyle {
	return qrHalfBlock
}

// prepCmd 让子进程独立进程组,便于整组停止。
func prepCmd(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killGroup(pid int, sig string) {
	syscall.Kill(-pid, sigOf(sig))
}

func killPid(pid int, sig string) {
	syscall.Kill(pid, sigOf(sig))
}

func sigOf(sig string) syscall.Signal {
	if sig == "KILL" {
		return syscall.SIGKILL
	}
	return syscall.SIGTERM
}
