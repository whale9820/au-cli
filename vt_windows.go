//go:build windows

package main

import (
	"os"

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
