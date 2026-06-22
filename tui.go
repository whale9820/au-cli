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

type selectItem struct {
	label  string
	detail string
}

var cmdList = []cmdDef{
	{"/connect", "choose provider, key, and model"},
	{"/models", "search and switch models"},
	{"/use", "search and switch providers"},
	{"/providers", "show available providers"},
	{"/model", "set model by id"},
	{"/key", "set api key"},
	{"/thinking", "set reasoning depth"},
	{"/custom", "manage custom providers"},
	{"/skills", "show available skills"},
	{"/skill", "activate a skill"},
	{"/update", "install latest au"},
	{"/reset", "clear chat context"},
	{"/yolo", "toggle command confirmations"},
	{"/help", "show commands"},
	{"/exit", "quit"},
}

type TUI struct {
	buf      []rune
	cursor   int
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
	fmt.Print("\033[?2004h") // enable bracketed paste
	return true
}

func (t *TUI) restore() error {
	fmt.Print("\033[?2004l") // disable bracketed paste
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

func bestCommandMatch(input string, matches []cmdDef, selIdx int) string {
	input = strings.TrimSpace(input)
	if len(matches) == 0 || strings.Contains(input, " ") {
		return input
	}
	idx := selIdx
	if idx < 0 || idx >= len(matches) {
		idx = 0
	}
	return matches[idx].name
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

	selected := t.selIdx
	if selected < 0 && len(ms) > 0 {
		selected = 0
	}
	for i, m := range ms {
		if i == selected {
			fmt.Printf("\n\r\033[7m  %-14s\033[0m  %s\033[K", m.name, m.desc)
		} else {
			fmt.Printf("\n\r  \033[1m%-14s\033[0m  \033[2m%s\033[0m\033[K", m.name, m.desc)
		}
	}
	if len(ms) > 0 {
		fmt.Print("\n\r  \033[2mEnter to run · Tab to complete · ↑↓ to choose\033[0m\033[K")
		fmt.Printf("\033[%dA", len(ms)+1)
	}

	fmt.Printf("\r\033[1m>\033[0m %s", string(t.buf))
	t.shown = len(ms)
	if len(ms) > 0 {
		t.shown++
	}

	// Position terminal cursor at t.cursor (move left from end of buf)
	if back := len(t.buf) - t.cursor; back > 0 {
		fmt.Printf("\033[%dD", back)
	}
}

func wordLeft(buf []rune, pos int) int {
	for pos > 0 && buf[pos-1] == ' ' {
		pos--
	}
	for pos > 0 && buf[pos-1] != ' ' {
		pos--
	}
	return pos
}

func wordRight(buf []rune, pos int) int {
	n := len(buf)
	for pos < n && buf[pos] == ' ' {
		pos++
	}
	for pos < n && buf[pos] != ' ' {
		pos++
	}
	return pos
}

func (t *TUI) insertAt(runes []rune) {
	newBuf := make([]rune, len(t.buf)+len(runes))
	copy(newBuf, t.buf[:t.cursor])
	copy(newBuf[t.cursor:], runes)
	copy(newBuf[t.cursor+len(runes):], t.buf[t.cursor:])
	t.buf = newBuf
	t.cursor += len(runes)
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

func (t *TUI) Select(title string, items []string) (string, int, bool) {
	rich := make([]selectItem, 0, len(items))
	for _, item := range items {
		rich = append(rich, selectItem{label: item})
	}
	return t.SelectRich(title, rich)
}

func (t *TUI) SelectRich(title string, items []selectItem) (string, int, bool) {
	if len(items) == 0 {
		fmt.Printf("  \033[2mnothing to select\033[0m\n")
		return "", -1, false
	}
	if !t.raw() {
		for i, item := range items {
			if item.detail == "" {
				fmt.Printf("  %2d  %s\n", i+1, item.label)
			} else {
				fmt.Printf("  %2d  %s  %s\n", i+1, item.label, item.detail)
			}
		}
		fmt.Printf("  %s: ", title)
		r := bufio.NewReader(os.Stdin)
		line, _ := r.ReadString('\n')
		line = strings.TrimSpace(line)
		for i, item := range items {
			text := item.label + " " + item.detail
			if strings.EqualFold(item.label, line) || strings.Contains(strings.ToLower(text), strings.ToLower(line)) {
				return item.label, i, true
			}
		}
		return "", -1, false
	}

	query := []rune{}
	sel := 0
	shown := 0
	filter := func() []int {
		q := strings.ToLower(string(query))
		out := []int{}
		for i, item := range items {
			text := item.label + " " + item.detail
			if q == "" || strings.Contains(strings.ToLower(text), q) {
				out = append(out, i)
			}
		}
		return out
	}
	clear := func() {
		for i := 0; i < shown; i++ {
			fmt.Print("\n\r\033[K")
		}
		if shown > 0 {
			fmt.Printf("\033[%dA", shown)
		}
		shown = 0
	}
	draw := func() []int {
		idxs := filter()
		if sel >= len(idxs) {
			sel = len(idxs) - 1
		}
		if sel < 0 {
			sel = 0
		}
		clear()
		fmt.Printf("\r\033[K\033[1m%s\033[0m %s", title, string(query))
		limit := len(idxs)
		if limit > 10 {
			limit = 10
		}
		start := 0
		if len(idxs) > limit && sel >= limit {
			start = sel - limit + 1
		}
		for i := 0; i < limit; i++ {
			row := start + i
			item := items[idxs[row]]
			if row == sel {
				fmt.Printf("\n\r\033[7m  %s\033[0m\033[K", item.label)
			} else {
				fmt.Printf("\n\r  \033[1m%s\033[0m\033[K", item.label)
			}
			if item.detail != "" {
				fmt.Printf("\n\r    \033[2m%s\033[0m\033[K", item.detail)
			}
		}
		if len(idxs) == 0 {
			fmt.Print("\n\r  \033[2mNo matches. Keep typing or press Esc.\033[0m\033[K")
			shown = 1
		} else {
			shown = limit
			for i := 0; i < limit; i++ {
				if items[idxs[start+i]].detail != "" {
					shown++
				}
			}
			if len(idxs) > limit {
				fmt.Printf("\n\r  \033[2m%d-%d of %d\033[0m\033[K", start+1, start+limit, len(idxs))
				shown++
			}
		}
		fmt.Print("\n\r  \033[2mType to filter · Enter to choose · ↑↓ to move · Esc to cancel\033[0m\033[K")
		shown++
		if shown > 0 {
			fmt.Printf("\033[%dA", shown)
		}
		fmt.Printf("\r\033[1m%s\033[0m %s", title, string(query))
		return idxs
	}

	b := make([]byte, 1024)
	idxs := draw()
	for {
		n, err := os.Stdin.Read(b)
		if err != nil || n == 0 {
			clear()
			fmt.Print("\r\n")
			t.restore()
			return "", -1, false
		}
		switch {
		case n == 1 && b[0] == 3:
			clear()
			fmt.Print("\r\n")
			t.restore()
			os.Exit(0)
		case n == 1 && b[0] == 27:
			clear()
			fmt.Print("\r\n")
			t.restore()
			return "", -1, false
		case n == 1 && (b[0] == 13 || b[0] == 10):
			idxs = filter()
			clear()
			fmt.Print("\r\n")
			t.restore()
			if len(idxs) == 0 {
				return "", -1, false
			}
			return items[idxs[sel]].label, idxs[sel], true
		case n == 1 && (b[0] == 127 || b[0] == 8):
			if len(query) > 0 {
				query = query[:len(query)-1]
				sel = 0
			}
			idxs = draw()
		case n >= 3 && b[0] == 27 && b[1] == '[' && b[2] == 'A':
			if len(idxs) > 0 {
				if sel <= 0 {
					sel = len(idxs) - 1
				} else {
					sel--
				}
			}
			idxs = draw()
		case n >= 3 && b[0] == 27 && b[1] == '[' && b[2] == 'B':
			if len(idxs) > 0 {
				if sel >= len(idxs)-1 {
					sel = 0
				} else {
					sel++
				}
			}
			idxs = draw()
		case n == 1 && b[0] >= 32 && b[0] < 127:
			query = append(query, rune(b[0]))
			sel = 0
			idxs = draw()
		case n > 1 && b[0] != 27:
			for _, r := range bytes.Runes(b[:n]) {
				if r >= 32 {
					query = append(query, r)
				}
			}
			sel = 0
			idxs = draw()
		}
	}
}

func (t *TUI) ReadLine() string {
	t.buf = t.buf[:0]
	t.cursor = 0
	t.selIdx = -1

	if !t.raw() {
		fmt.Print("> ")
		r := bufio.NewReader(os.Stdin)
		line, _ := r.ReadString('\n')
		return strings.TrimSpace(line)
	}

	t.refreshLayout()

	fmt.Printf("\033[1m>\033[0m ")

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

		case n == 1 && b[0] == 1: // Ctrl+A — beginning of line
			t.cursor = 0
			t.redraw()

		case n == 1 && b[0] == 5: // Ctrl+E — end of line
			t.cursor = len(t.buf)
			t.redraw()

		case n == 1 && b[0] == 11: // Ctrl+K — kill to end of line
			t.buf = t.buf[:t.cursor]
			t.redraw()

		case n == 1 && b[0] == 21: // Ctrl+U — kill whole line
			t.buf = t.buf[:0]
			t.cursor = 0
			t.selIdx = -1
			t.redraw()

		case n == 1 && b[0] == 23: // Ctrl+W — kill word backward
			newPos := wordLeft(t.buf, t.cursor)
			t.buf = append(t.buf[:newPos], t.buf[t.cursor:]...)
			t.cursor = newPos
			t.selIdx = -1
			t.redraw()

		case n == 1 && (b[0] == 13 || b[0] == 10): // Enter
			ms := t.matches()
			result := bestCommandMatch(string(t.buf), ms, t.selIdx)
			t.clearComps()
			fmt.Print("\r\n")
			t.restore()
			t.buf = t.buf[:0]
			t.cursor = 0
			t.selIdx = -1
			if result != "" {
				t.history = append(t.history, result)
				if len(t.history) > 500 {
					t.history = t.history[len(t.history)-500:]
				}
				t.histPos = len(t.history)
			}
			return result

		case n == 1 && (b[0] == 127 || b[0] == 8): // Backspace
			if t.cursor > 0 {
				copy(t.buf[t.cursor-1:], t.buf[t.cursor:])
				t.buf = t.buf[:len(t.buf)-1]
				t.cursor--
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
				t.cursor = len(t.buf)
				t.selIdx = -1
				t.redraw()
			}

		case n == 2 && b[0] == 27 && b[1] == 'b': // Alt+B — word left
			t.cursor = wordLeft(t.buf, t.cursor)
			t.redraw()

		case n == 2 && b[0] == 27 && b[1] == 'f': // Alt+F — word right
			t.cursor = wordRight(t.buf, t.cursor)
			t.redraw()

		case n >= 3 && b[0] == 27 && b[1] == '[': // ESC [ sequences
			ms := t.matches()

			// Bracketed paste: \033[200~ ... content ... \033[201~
			seq := string(b[:n])
			if strings.HasPrefix(seq, "\033[200~") {
				content := strings.TrimPrefix(seq, "\033[200~")
				content = strings.TrimSuffix(content, "\033[201~")
				// Replace newlines with spaces so the paste lands on one line
				content = strings.ReplaceAll(content, "\r\n", " ")
				content = strings.ReplaceAll(content, "\n", " ")
				content = strings.ReplaceAll(content, "\r", " ")
				runes := []rune(content)
				var printable []rune
				for _, r := range runes {
					if r >= 32 {
						printable = append(printable, r)
					}
				}
				if len(printable) > 0 {
					t.insertAt(printable)
					t.selIdx = -1
					t.redraw()
				}
				continue
			}
			switch {
			case b[2] == 'A': // Up
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
					t.cursor = len(t.buf)
					t.redraw()
				}
			case b[2] == 'B': // Down
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
					t.cursor = len(t.buf)
					t.redraw()
				}
			case b[2] == 'C': // Right arrow
				if t.cursor < len(t.buf) {
					t.cursor++
					t.redraw()
				}
			case b[2] == 'D': // Left arrow
				if t.cursor > 0 {
					t.cursor--
					t.redraw()
				}
			case b[2] == 'H': // Home
				t.cursor = 0
				t.redraw()
			case b[2] == 'F': // End
				t.cursor = len(t.buf)
				t.redraw()
			case n >= 4 && b[2] == '3' && b[3] == '~': // Delete
				if t.cursor < len(t.buf) {
					copy(t.buf[t.cursor:], t.buf[t.cursor+1:])
					t.buf = t.buf[:len(t.buf)-1]
				}
				t.redraw()
			case n >= 4 && b[2] == '1' && b[3] == '~': // Home (VT)
				t.cursor = 0
				t.redraw()
			case n >= 4 && b[2] == '4' && b[3] == '~': // End (VT)
				t.cursor = len(t.buf)
				t.redraw()
			case n >= 6 && b[2] == '1' && b[3] == ';' && b[4] == '5' && b[5] == 'D': // Ctrl+Left
				t.cursor = wordLeft(t.buf, t.cursor)
				t.redraw()
			case n >= 6 && b[2] == '1' && b[3] == ';' && b[4] == '5' && b[5] == 'C': // Ctrl+Right
				t.cursor = wordRight(t.buf, t.cursor)
				t.redraw()
			}

		case n == 1 && b[0] >= 32 && b[0] < 127: // printable ASCII
			t.insertAt([]rune{rune(b[0])})
			t.selIdx = -1
			t.redraw()

		case n > 1 && b[0] != 27: // paste or multi-byte UTF-8
			// Decode as UTF-8; skip control chars (keep printable + non-ASCII)
			runes := bytes.Runes(b[:n])
			var printable []rune
			for _, r := range runes {
				if r >= 32 || r > 126 {
					printable = append(printable, r)
				}
			}
			if len(printable) > 0 {
				t.insertAt(printable)
				t.selIdx = -1
				t.redraw()
			}
		}
	}
}
