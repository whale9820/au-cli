//go:build windows

package main

import (
	"os"
	"os/exec"

	"golang.org/x/sys/windows"
)

func enableVT() {
	for _, fd := range []uintptr{os.Stdout.Fd(), os.Stderr.Fd()} {
		var st uint32
		if err := windows.GetConsoleMode(windows.Handle(fd), &st); err != nil {
			continue
		}
		windows.SetConsoleMode(windows.Handle(fd), st|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	}
}

// execReplace on Windows can't use syscall.Exec, so start + exit.
func execReplace(exe string) error {
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	os.Exit(0)
	return nil
}
