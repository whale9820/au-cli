//go:build windows

package main

// enterTaskMode on Windows is a no-op for now — the terminal stays in its
// current mode during tasks. The input bar won't be interactive on Windows
// until termios-level support is added.
func enterTaskMode(fd int) (func(), error) {
	return func() {}, nil
}

func pollStdin(fd int, timeoutMs int) bool {
	return false
}

func runInputBar(row int) (func(), *string) {
	b := ""
	return func() {}, &b
}