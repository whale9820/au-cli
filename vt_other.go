//go:build !windows

package main

import (
	"os"
	"syscall"
)

func enableVT() {}

func execReplace(exe string) error {
	return syscall.Exec(exe, os.Args, os.Environ())
}
