package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

type cmdDef struct {
	name string
	desc string
}

var cmdList = []cmdDef{
	{"/connect",   "guided provider + model setup"},
	{"/use",       "switch provider — /use <name> | /use custom"},
	{"/key",       "set api key — /key [value]"},
	{"/model",     "set active model — /model <id>"},
	{"/models",    "list models from current provider"},
	{"/providers", "list all providers"},
	{"/thinking",  "set reasoning depth 0-10 — /thinking <n>"},
	{"/reset",     "clear conversation context"},
	{"/help",      "show available commands"},
	{"/exit",      "exit au"},
}

type TUI struct {
	buf      []rune
	history  []string
	histPos  int
	selIdx   int
	shown    int
	fd       int
	old      *term.State
	model    string
	thinking int
}

func newTUI() *TUI {
	return &TUI{fd: int(os.Stdin.Fd()), selIdx: -1}
}

func (t *TUI) setHistory(history []string) {
	t.history = history
	t.histPos = len(t.history)
}

func (t *TUI) raw() bool {
	st, err := term.MakeRaw(t.fd)
	if err != nil {
		return false
	}
	t.old = st
	return true
}

func (t *TUI) restore() error {
	if t.old != nil {
		if err := term.Restore(t.fd, t.old); err != nil {
			return err
		}
		t.old = nil
	}
	return nil
}

	func (t *TUI) Width() int {
		w, _, err := term.GetSize(t.fd)
		if err != nil || w <= 0 {
			return 80
		}
		return w
	}

func (t *TUI) height() int {
	_, h, err := term.GetSize(t.fd)
	if err != nil || h <= 0 {
		return 24
	}
	return h
}

func (t *TUI) drawStatus() {
	h := t.height()
	var right string
	if t.thinking > 0 {
		filled := t.thinking
		empty := 10 - filled
		bar := strings.Repeat("●", filled) + strings.Repeat("○", empty)
		right = fmt.Sprintf("  %s  [%s]  ", t.model, bar)
	} else {
		right = fmt.Sprintf("  %s  ", t.model)
	}
	w := t.Width()
	pad := max(0, w-len(right))
	fmt.Printf("\0337\033[%d;1H\033[2K\033[2m%s%s\033[0m\0338", h, strings.Repeat(" ", pad), right)
}

func (t *TUI) setScrollRegion() {
	h := t.height()
	fmt.Printf("\033[1;%dr\033[%d;1H", h-1, h-1)
}

func (t *TUI) Refresh(model string, thinking int) {
	t.model = model
	t.thinking = thinking
	t.setScrollRegion()
	t.drawStatus()
}

func (t *TUI) Teardown() {
	h := t.height()
	fmt.Printf("\033[r\033[%d;1H\033[2K", h)
	if err := t.restore(); err != nil {
		// Attempt to restore terminal state even if restore fails
		if t.old != nil {
			term.Restore(t.fd, t.old)
		}
	}
}

func (t *TUI) refreshLayout() {
	t.setScrollRegion()
	t.drawStatus()
}

	func (t *TUI) matches() []cmdDef {
		s := string(t.buf)
		if s == "/" {
			return cmdList
		}
		if !strings.HasPrefix(s, "/") {
			return nil
		}
		var out []cmdDef
		for _, c := range cmdList {
			if strings.HasPrefix(c.name, s) {
				out = append(out, c)
			}
		}
		return out
	}

func (t *TUI) redraw() {
	ms := t.matches()

	fmt.Printf("\r\033[K\033[1m>\033[0m %s", string(t.buf))

	for i := 0; i < t.shown; i++ {
		fmt.Print("\n\r\033[K")
	}
	if t.shown > 0 {
		fmt.Printf("\033[%dA", t.shown)
	}

	for i, m := range ms {
		if i == t.selIdx {
			fmt.Printf("\n\r\033[7m  %-14s  %s\033[0m\033[K", m.name, m.desc)
		} else {
			fmt.Printf("\n\r  \033[1m%-14s\033[0m  \033[2m%s\033[0m\033[K", m.name, m.desc)
		}
	}
	if len(ms) > 0 {
		fmt.Printf("\033[%dA", len(ms))
	}

	fmt.Printf("\r\033[1m>\033[0m %s", string(t.buf))
	t.shown = len(ms)
}

func (t *TUI) clearComps() {
	for i := 0; i < t.shown; i++ {
		fmt.Print("\n\r\033[K")
	}
	if t.shown > 0 {
		fmt.Printf("\033[%dA", t.shown)
	}
	t.shown = 0
}

	func (t *TUI) ReadLine() string {
		t.buf = t.buf[:0]
		t.selIdx = -1

		if !t.raw() {
			fmt.Print("> ")
			r := bufio.NewReader(os.Stdin)
			line, _ := r.ReadString('\n')
			return strings.TrimSpace(line)
		}

		t.refreshLayout()

		fmt.Printf("\033[1m>\033[0m ")

		// Use a larger buffer for multi-byte UTF-8
		b := make([]byte, 1024)
		for {
		n, err := os.Stdin.Read(b)
		if err != nil || n == 0 {
			t.clearComps()
			fmt.Print("\r\n")
			t.restore()
			return ""
		}

		switch {
		case n == 1 && b[0] == 3: // Ctrl+C
			t.clearComps()
			fmt.Print("\r\n")
			t.restore()
			os.Exit(0)

		case n == 1 && (b[0] == 13 || b[0] == 10): // Enter
			ms := t.matches()
			if t.selIdx >= 0 && t.selIdx < len(ms) {
				t.buf = append([]rune(ms[t.selIdx].name), ' ')
				t.selIdx = -1
				t.redraw()
				continue
			}
			t.clearComps()
			fmt.Print("\r\n")
			result := strings.TrimSpace(string(t.buf))
			t.restore()
			t.buf = t.buf[:0]
			t.selIdx = -1
			if result != "" {
				t.history = append(t.history, result)
				t.histPos = len(t.history)
			}
			return result

		case n == 1 && (b[0] == 127 || b[0] == 8): // Backspace
			if len(t.buf) > 0 {
				t.buf = t.buf[:len(t.buf)-1]
				t.selIdx = -1
			}
			t.redraw()

		case n == 1 && b[0] == 9: // Tab
			ms := t.matches()
			if len(ms) > 0 {
				idx := t.selIdx
				if idx < 0 {
					idx = 0
				}
				t.buf = append([]rune(ms[idx].name), ' ')
				t.selIdx = -1
				t.redraw()
			}

		case n >= 3 && b[0] == 27 && b[1] == '[': // ESC sequences
			ms := t.matches()
			switch b[2] {
			case 'A': // Up
				if len(ms) > 0 {
					if t.selIdx <= 0 {
						t.selIdx = len(ms) - 1
					} else {
						t.selIdx--
					}
					t.redraw()
				} else if t.histPos > 0 {
					t.histPos--
					t.buf = []rune(t.history[t.histPos])
					t.redraw()
				}
			case 'B': // Down
				if len(ms) > 0 {
					if t.selIdx >= len(ms)-1 {
						t.selIdx = 0
					} else {
						t.selIdx++
					}
					t.redraw()
				} else if t.histPos < len(t.history) {
					t.histPos++
					if t.histPos < len(t.history) {
						t.buf = []rune(t.history[t.histPos])
					} else {
						t.buf = t.buf[:0]
					}
					t.redraw()
				}
			case 'C': // Right — reserved
			case 'D': // Left — reserved
			}

		case n == 1 && b[0] >= 32 && b[0] < 127: // printable ASCII
			t.buf = append(t.buf, rune(b[0]))
			t.selIdx = -1
			t.redraw()

		case n > 1 && b[0] >= 0xC0: // multi-byte UTF-8
			// Decode UTF-8 properly
			runes := bytes.Runes(b[:n])
			t.buf = append(t.buf, runes...)
			t.selIdx = -1
			t.redraw()
		}
	}
}

