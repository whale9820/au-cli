//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func enableVT() {}

func execReplace(exe string) error {
	// Attempt in-place replacement for clean terminal handoff.
	if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
		// Fall back to spawn+exit (handles codesign / permission edge cases).
		fmt.Printf("  \033[2mrelaunch via syscallexec failed: %v — trying spawn\033[0m\n", err)
		spawn := exec.Command(exe, os.Args[1:]...)
		spawn.Stdin = os.Stdin
		spawn.Stdout = os.Stdout
		spawn.Stderr = os.Stderr
		if err2 := spawn.Start(); err2 != nil {
			return err2
		}
		os.Exit(0)
	}
	return nil
}