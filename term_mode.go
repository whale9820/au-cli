//go:build !windows

package main

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// ioctl constants — TCGETS on Linux, TIOCGETA on Darwin/BSDs.
var ioctlR, ioctlW uint

func init() {
	if runtime.GOOS == "linux" {
		// TCGETS / TCSETS — Linux only
		ioctlR = 0x5401
		ioctlW = 0x5402
	} else {
		// TIOCGETA / TIOCSETA — Darwin / BSDs
		ioctlR = 0x40487413
		ioctlW = 0x80487414
	}
}

// enterTaskMode switches the terminal to a mode suitable for concurrent task
// processing: canonical mode off (character-by-character input), echo off,
// signal generation off, but output processing kept ON (\n → \r\n).
// Returns a restore function that must be called when the task ends.
func enterTaskMode(fd int) (func(), error) {
	orig, err := unix.IoctlGetTermios(fd, ioctlR)
	if err != nil {
		return nil, err
	}
	raw := *orig
	raw.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP | unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	raw.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	raw.Cflag &^= unix.CSIZE | unix.PARENB
	raw.Cflag |= unix.CS8
	raw.Oflag |= unix.OPOST | unix.ONLCR
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, ioctlW, &raw); err != nil {
		return nil, err
	}
	return func() { unix.IoctlSetTermios(fd, ioctlW, orig) }, nil
}

// pollStdin checks stdin for readability with a timeout. Returns true if data
// is available, false on timeout or error.
func pollStdin(fd int, timeoutMs int) bool {
	pfd := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
	n, err := unix.Poll(pfd, timeoutMs)
	return err == nil && n > 0 && (pfd[0].Revents&unix.POLLIN) != 0
}

// runInputBar starts a goroutine that reads stdin character-by-character,
// manages a simple input buffer, draws the input bar on the given row, and
// queues completed messages. Returns a stop function and the current buffer
// pointer (for inspecting content during pauses).
func runInputBar(row int) (stop func(), buf *string) {
	b := ""
	buf = &b
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	inEscape := false

	draw := func() {
		outMu.Lock()
		fmt.Printf("\0337\033[%d;1H\033[2K\033[1m>\033[0m %s\0338", row, b)
		outMu.Unlock()
	}

	draw()

	go func() {
		defer close(doneCh)
		fd := int(os.Stdin.Fd())
		rb := make([]byte, 1024)

		for {
			if !pollStdin(fd, 100) {
				select {
				case <-stopCh:
					return
				default:
				}
				continue
			}

			select {
			case <-stopCh:
				return
			default:
			}

			n, err := syscall.Read(fd, rb)
			if err != nil || n <= 0 {
				time.Sleep(50 * time.Millisecond)
				continue
			}

			for _, ch := range rb[:n] {
				if inEscape {
					if ch >= 64 && ch <= 126 {
						inEscape = false
					}
					continue
				}
				switch {
				case ch == 27:
					inEscape = true
				case ch == 3:
					if len(b) > 0 {
						b = ""
					}
				case ch == 13 || ch == 10:
					if len(b) > 0 {
						enqueue(b)
						msg := b
						b = ""
						outMu.Lock()
						disp := msg
						if len(disp) > 60 {
							disp = disp[:60] + "…"
						}
						fmt.Printf("  \033[2m↳ %s (queued)\033[0m\n", disp)
						outMu.Unlock()
					}
				case ch == 127 || ch == 8:
					if len(b) > 0 {
						b = b[:len(b)-1]
					}
				default:
					if ch >= 32 && ch < 127 {
						b += string(ch)
					}
				}
			}
			draw()
		}
	}()

	stop = func() {
		close(stopCh)
		<-doneCh
	}
	return
}