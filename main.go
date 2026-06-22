package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	reBold   = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reCode   = regexp.MustCompile("`(.+?)`")
	yoloMode bool
)

func ri(s string) string {
	s = reBold.ReplaceAllString(s, "\033[1m$1\033[0m")
	s = reCode.ReplaceAllString(s, "\033[2m$1\033[0m")
	return s
}

func renderTable(rows []string) {
	type row []string
	var parsed []row
	var widths []int
	for _, r := range rows {
		r = strings.Trim(strings.TrimSpace(r), "|")
		cells := strings.Split(r, "|")
		sep := true
		for _, c := range cells {
			for _, ch := range strings.TrimSpace(c) {
				if ch != '-' && ch != ':' && ch != ' ' {
					sep = false
					break
				}
			}
		}
		if sep {
			continue
		}
		tr := make(row, len(cells))
		for i, c := range cells {
			tr[i] = strings.TrimSpace(c)
			for len(widths) <= i {
				widths = append(widths, 0)
			}
			if len(tr[i]) > widths[i] {
				widths[i] = len(tr[i])
			}
		}
		parsed = append(parsed, tr)
	}
	if len(parsed) == 0 {
		return
	}
	fmt.Println()
	for i, r := range parsed {
		fmt.Print("  ")
		for j, cell := range r {
			w := 0
			if j < len(widths) {
				w = widths[j]
			}
			if i == 0 {
				fmt.Printf("\033[1m%-*s\033[0m  ", w, cell)
			} else {
				fmt.Printf("%-*s  ", w, cell)
			}
		}
		fmt.Println()
		if i == 0 {
			fmt.Print("  ")
			for j := range r {
				w := 0
				if j < len(widths) {
					w = widths[j]
				}
				fmt.Printf("%s  ", strings.Repeat("─", w))
			}
			fmt.Println()
		}
	}
	fmt.Println()
}

type lineRenderer struct {
	pending  strings.Builder
	inCode   bool
	codeLang string
	codeN    int
	tbl      []string
}

func newLineRenderer() *lineRenderer { return &lineRenderer{} }

func (r *lineRenderer) Feed(tok string) {
	for _, ch := range tok {
		if ch == '\n' {
			r.line(r.pending.String())
			r.pending.Reset()
		} else {
			r.pending.WriteRune(ch)
		}
	}
}

func (r *lineRenderer) Flush() {
	if r.pending.Len() > 0 {
		r.line(r.pending.String())
		r.pending.Reset()
	}
	r.flushTbl()
}

func (r *lineRenderer) flushTbl() {
	if len(r.tbl) == 0 {
		return
	}
	renderTable(r.tbl)
	r.tbl = nil
}

func (r *lineRenderer) line(s string) {
	if r.inCode {
		// Check for code block closing - only at start of line
		trimmed := strings.TrimSpace(s)
		if trimmed == "```" || strings.HasPrefix(trimmed, "```") {
			r.inCode = false
			fmt.Printf("  \033[2m╰─\033[0m\n")
			return
		}
		r.codeN++
		fmt.Printf("  \033[2m%4d\033[0m  %s\n", r.codeN, s)
		return
	}

	trimmed := strings.TrimSpace(s)

	if strings.HasPrefix(trimmed, "```") {
		r.flushTbl()
		r.inCode = true
		r.codeLang = trimmed[3:]
		r.codeN = 0
		if r.codeLang != "" {
			fmt.Printf("\n  \033[2m╭╴%s\033[0m\n", r.codeLang)
		} else {
			fmt.Printf("\n  \033[2m╭─\033[0m\n")
		}
		return
	}

	if strings.HasPrefix(trimmed, "|") {
		r.tbl = append(r.tbl, trimmed)
		return
	}
	r.flushTbl()

	if trimmed == "" {
		fmt.Println()
		return
	}
	if strings.HasPrefix(s, "# ") {
		fmt.Printf("\n  \033[1;4m%s\033[0m\n\n", ri(s[2:]))
		return
	}
	if strings.HasPrefix(s, "## ") {
		fmt.Printf("\n  \033[1m%s\033[0m\n", ri(s[3:]))
		return
	}
	if strings.HasPrefix(s, "### ") {
		fmt.Printf("  \033[1m%s\033[0m\n", ri(s[4:]))
		return
	}
	if strings.HasPrefix(s, "- ") || strings.HasPrefix(s, "* ") {
		fmt.Printf("  \033[2m•\033[0m  %s\n", ri(s[2:]))
		return
	}
	fmt.Printf("  %s\n", ri(s))
}

func fmtSize(n int64) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

var (
	ui        *TUI
	outMu    sync.Mutex
	queueMu  sync.Mutex
	msgQueue []string
)

func enqueue(s string) {
	queueMu.Lock()
	msgQueue = append(msgQueue, s)
	queueMu.Unlock()
}

func drainQueue() []string {
	queueMu.Lock()
	defer queueMu.Unlock()
	out := append([]string(nil), msgQueue...)
	msgQueue = msgQueue[:0]
	return out
}

func approxTokens(s string) int {
	count := 0
	inWord := false
	for _, r := range s {
		switch {
		case r == ' ' || r == '\n' || r == '\t' || r == '\r':
			inWord = false
		case strings.ContainsRune(".,;:!?()[]{}<>/\\|+-=*\"'`", r):
			count++
			inWord = false
		default:
			if !inWord {
				count++
				inWord = true
			}
		}
	}
	if count == 0 && s != "" {
		return 1
	}
	return count
}

func prompt(label string) string {
	fmt.Printf("  %s: ", label)
	r := bufio.NewReader(os.Stdin)
	s, _ := r.ReadString('\n')
	return strings.TrimSpace(s)
}

func collectPlaceholders(st *Store, url string) error {
	matches := rePlaceholder.FindAllStringSubmatch(url, -1)
	for _, m := range matches {
		key := m[1]
		if cur := st.Vars[key]; cur != "" {
			fmt.Printf("  %s (current: %s, blank to keep): ", key, cur)
		} else {
			fmt.Printf("  %s: ", key)
		}
		r := bufio.NewReader(os.Stdin)
		val, err := r.ReadString('\n')
		if err != nil {
			return err
		}
		val = strings.TrimSpace(val)
		if val != "" {
			st.Vars[key] = val
		}
	}
	return nil
}

func promptAPIKey(st *Store) error {
	if st.APIKey == "" {
		if key := prompt("api key"); key != "" {
			st.APIKey = key
		}
	} else {
		if key := prompt("api key (blank to keep)"); key != "" {
			st.APIKey = key
		}
	}
	return nil
}

func thinkingStr(level int) string {
	if level == 0 {
		return ""
	}
	return "[" + strings.Repeat("●", level) + strings.Repeat("○", 10-level) + "]"
}

func startSpinner() func() {
	stop, _ := startLabeledSpinner("")
	return stop
}

// startLabeledSpinner animates a braille spinner with an optional dim label.
// When the label is non-empty, a trailing byte counter can be displayed by
// feeding bytes via the returned update func (pass -1 to hide it).
func startLabeledSpinner(label string) (stop func(), update func(int)) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	type state struct {
		bytes int
		label string
	}
	stCh := make(chan state, 8)
	stopCh := make(chan struct{}, 1)
	doneCh := make(chan struct{}, 1)
	stCh <- state{label: label}
	go func() {
		defer close(doneCh)
		i := 0
		cur := state{label: label}
		for {
			select {
			case <-stopCh:
				outMu.Lock()
				fmt.Printf("\r\033[K")
				outMu.Unlock()
				return
			case s := <-stCh:
				cur = s
			default:
				line := "\r\033[K\033[2m" + frames[i%len(frames)] + "\033[0m"
				if cur.label != "" {
					line += "  \033[2m" + cur.label + "\033[0m"
					if cur.bytes >= 0 {
						line += "  \033[2m" + fmtSize(int64(cur.bytes)) + "\033[0m"
					}
				}
					outMu.Lock()
					fmt.Print(line)
					outMu.Unlock()
					i++
				time.Sleep(80 * time.Millisecond)
			}
		}
	}()
	var once sync.Once
	stop = func() {
		once.Do(func() {
			select {
			case stopCh <- struct{}{}:
			case <-time.After(100 * time.Millisecond):
			}
			<-doneCh
		})
	}
	update = func(n int) {
		select {
		case stCh <- state{label: label, bytes: n}:
		default:
		}
	}
	return stop, update
}

// toolLabel returns a short human-readable verb for a tool name, used in the
// progress spinner shown while that tool runs.
func dangerousCommand(cmdStr string) bool {
	patterns := []string{`rm -rf`, `>/dev/`, `:(){:|: &}:;:`, `mkfs`, `dd if=`, `chmod -R 777`}
	for _, pattern := range patterns {
		if strings.Contains(cmdStr, pattern) {
			return true
		}
	}
	return false
}

func confirmTool(tc ToolCallMsg) bool {
	if tc.Function.Name != "run_command" || yoloMode {
		return true
	}
	var a struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &a); err != nil {
		return true
	}
	cmdStr := strings.TrimSpace(a.Command)
	if !dangerousCommand(cmdStr) {
		return true
	}
	fmt.Printf("\n  \033[33m⚠  dangerous command\033[0m  %s\n  allow? [y/N] ", cmdStr)
	r := bufio.NewReader(os.Stdin)
	ans, _ := r.ReadString('\n')
	ans = strings.TrimSpace(strings.ToLower(ans))
	return ans == "y" || ans == "yes"
}

func toolLabel(name string) string {
	switch name {
	case "read_file":
		return "reading"
	case "write_file":
		return "writing"
	case "patch_file":
		return "patching"
	case "append_file":
		return "appending"
	case "delete_file":
		return "deleting"
	case "move_file":
		return "moving"
	case "search_files":
		return "searching"
	case "list_directory":
		return "listing"
	case "run_command":
		return "running"
	case "add_todo", "update_todo", "list_todos", "remove_todo":
		return "todo"
	default:
		return "working"
	}
}

func displayToolCall(tc ToolCallMsg) {
	switch tc.Function.Name {
	case "read_file":
		var a struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &a); err != nil {
			fmt.Printf("  \033[31merror: failed to parse tool call\033[0m\n")
			return
		}
		fmt.Printf("  \033[36m←\033[0m  \033[2mread\033[0m    %s\n", a.Path)

	case "write_file":
		var a struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &a); err != nil {
			fmt.Printf("  \033[31merror: failed to parse tool call\033[0m\n")
			return
		}
		fmt.Printf("  \033[32m▸\033[0m  \033[2mwrite\033[0m   \033[1m%s\033[0m\n", a.Path)
		lines := strings.Split(a.Content, "\n")
		limit := 40
		show := lines
		truncated := 0
		if len(lines) > limit {
			show = lines[:limit]
			truncated = len(lines) - limit
		}
		for i, line := range show {
			fmt.Printf("  \033[2m┆ %4d\033[0m  %s\n", i+1, line)
		}
		if truncated > 0 {
			fmt.Printf("  \033[2m┆ ... %d more lines\033[0m\n", truncated)
		}
		fmt.Println()

	case "run_command":
		var a struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &a); err != nil {
			fmt.Printf("  \033[31merror: failed to parse tool call\033[0m\n")
			return
		}
		fmt.Printf("  \033[33m$\033[0m  %s\n", a.Command)

	case "list_directory":
		var a struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &a); err != nil {
			fmt.Printf("  \033[31merror: failed to parse tool call\033[0m\n")
			return
		}
		fmt.Printf("  \033[34m≡\033[0m  \033[2mls\033[0m      %s\n", a.Path)

	case "patch_file":
		var a struct {
			Path   string `json:"path"`
			OldStr string `json:"old_str"`
			NewStr string `json:"new_str"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &a); err != nil {
			fmt.Printf("  \033[31merror: failed to parse tool call\033[0m\n")
			return
		}
		fmt.Printf("  \033[33m≈\033[0m  \033[2mpatch\033[0m   \033[1m%s\033[0m\n", a.Path)
		oldPreview := strings.ReplaceAll(a.OldStr, "\n", "↵")
		newPreview := strings.ReplaceAll(a.NewStr, "\n", "↵")
		if len(oldPreview) > 72 {
			oldPreview = oldPreview[:72] + "…"
		}
		if len(newPreview) > 72 {
			newPreview = newPreview[:72] + "…"
		}
		fmt.Printf("  \033[2m┆\033[0m  \033[31m- %s\033[0m\n", oldPreview)
		fmt.Printf("  \033[2m┆\033[0m  \033[32m+ %s\033[0m\n", newPreview)
		fmt.Println()

	case "append_file":
		var a struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &a); err != nil {
			fmt.Printf("  \033[31merror: failed to parse tool call\033[0m\n")
			return
		}
		fmt.Printf("  \033[32m+\033[0m  \033[2mappend\033[0m  \033[1m%s\033[0m\n", a.Path)

	case "delete_file":
		var a struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &a); err != nil {
			fmt.Printf("  \033[31merror: failed to parse tool call\033[0m\n")
			return
		}
		fmt.Printf("  \033[31m✕\033[0m  \033[2mdelete\033[0m  %s\n", a.Path)

	case "move_file":
		var a struct {
			Src string `json:"src"`
			Dst string `json:"dst"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &a); err != nil {
			fmt.Printf("  \033[31merror: failed to parse tool call\033[0m\n")
			return
		}
		fmt.Printf("  \033[33m→\033[0m  \033[2mmove\033[0m    %s → %s\n", a.Src, a.Dst)

	case "search_files":
		var a struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
			Glob    string `json:"glob"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &a); err != nil {
			fmt.Printf("  \033[31merror: failed to parse tool call\033[0m\n")
			return
		}
		searchIn := a.Path
		if searchIn == "" {
			searchIn = "."
		}
		if a.Glob != "" {
			fmt.Printf("  \033[35m/\033[0m  \033[2msearch\033[0m  %q  \033[2min %s (%s)\033[0m\n", a.Pattern, searchIn, a.Glob)
		} else {
			fmt.Printf("  \033[35m/\033[0m  \033[2msearch\033[0m  %q  \033[2min %s\033[0m\n", a.Pattern, searchIn)
		}

	case "add_todo":
		var a struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &a); err != nil {
			fmt.Printf("  \033[31merror: failed to parse tool call\033[0m\n")
			return
		}
		fmt.Printf("  \033[36m☐\033[0m  \033[2mtodo\033[0m    %s\n", a.Title)

	case "list_todos":
		fmt.Printf("  \033[36m☐\033[0m  \033[2mtodos\033[0m\n")

	case "update_todo":
		var a struct {
			ID     int    `json:"id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &a); err != nil {
			fmt.Printf("  \033[31merror: failed to parse tool call\033[0m\n")
			return
		}
		switch a.Status {
		case "done":
			fmt.Printf("  \033[32m✓\033[0m  \033[2mtodo\033[0m    #%d → done\n", a.ID)
		case "in_progress":
			fmt.Printf("  \033[33m◐\033[0m  \033[2mtodo\033[0m    #%d → in progress\n", a.ID)
		default:
			fmt.Printf("  \033[36m☐\033[0m  \033[2mtodo\033[0m    #%d → %s\n", a.ID, a.Status)
		}

	case "remove_todo":
		var a struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &a); err != nil {
			fmt.Printf("  \033[31merror: failed to parse tool call\033[0m\n")
			return
		}
		fmt.Printf("  \033[31m✕\033[0m  \033[2mtodo\033[0m    #%d removed\n", a.ID)
	}
}

func displayToolResult(tc ToolCallMsg, result string) {
	switch tc.Function.Name {
	case "run_command":
		if result == "" {
			fmt.Println()
			return
		}
		lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
		limit := 30
		show := lines
		hidden := 0
		if len(lines) > limit {
			show = lines[:limit]
			hidden = len(lines) - limit
		}
		for _, line := range show {
			fmt.Printf("  \033[2m│\033[0m  %s\n", line)
		}
		if hidden > 0 {
			fmt.Printf("  \033[2m│  ... %d more lines\033[0m\n", hidden)
		}
		fmt.Println()

	case "list_directory":
		if result == "" {
			fmt.Println()
			return
		}
		lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
		for _, line := range lines {
			if strings.HasSuffix(line, "/") {
				fmt.Printf("  \033[1m%s\033[0m\n", line)
			} else {
				parts := strings.Fields(line)
				if len(parts) == 2 {
					name := parts[0]
					size, _ := strconv.ParseInt(parts[1], 10, 64)
					fmt.Printf("  %-38s  \033[2m%s\033[0m\n", name, fmtSize(size))
				} else {
					fmt.Printf("  %s\n", line)
				}
			}
		}
		fmt.Println()

	case "search_files":
		if result == "" || result == "no matches found" {
			fmt.Printf("  \033[2mno matches\033[0m\n\n")
			return
		}
		lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
		limit := 30
		show := lines
		hidden := 0
		if len(lines) > limit {
			show = lines[:limit]
			hidden = len(lines) - limit
		}
		for _, line := range show {
			fmt.Printf("  \033[2m│\033[0m  %s\n", line)
		}
		if hidden > 0 {
			fmt.Printf("  \033[2m│  ... %d more lines\033[0m\n", hidden)
		}
		fmt.Println()

	case "list_todos":
		if result == "" || result == "no todos" {
			fmt.Printf("  \033[2mno todos\033[0m\n\n")
			return
		}
		lines := strings.Split(strings.TrimRight(result, "\n"), "\n")
		for _, line := range lines {
			if strings.Contains(line, "done") {
				fmt.Printf("  \033[32m│\033[0m  \033[2m%s\033[0m\n", line)
			} else if strings.Contains(line, "in_progress") {
				fmt.Printf("  \033[33m│\033[0m  %s\n", line)
			} else {
				fmt.Printf("  \033[2m│  %s\033[0m\n", line)
			}
		}
		fmt.Println()
	}
}

func providerItems(providers []Provider) []selectItem {
	items := make([]selectItem, 0, len(providers))
	for _, p := range providers {
		detail := p.BaseURL
		if len(p.Env) > 0 {
			detail += "  key: " + strings.Join(p.Env, ", ")
		}
		items = append(items, selectItem{label: p.Name, detail: detail})
	}
	return items
}

func currentProvider(cat ProviderCatalog, cfg Config) *Provider {
	for i := range cat.Providers {
		if cfg.BaseURL == cat.Providers[i].BaseURL || cfg.BaseURL == strings.TrimRight(cat.Providers[i].BaseURL, "/") {
			return &cat.Providers[i]
		}
	}
	return nil
}

func modelItems(models []string, p *Provider) []selectItem {
	items := make([]selectItem, 0, len(models))
	for _, id := range models {
		detail := ""
		if p != nil {
			if m := p.findModel(id); m != nil {
				parts := []string{}
				if m.Context > 0 {
					parts = append(parts, fmt.Sprintf("ctx %dk", m.Context/1000))
				}
				if m.Output > 0 {
					parts = append(parts, fmt.Sprintf("out %dk", m.Output/1000))
				}
				if m.Tools != nil && *m.Tools {
					parts = append(parts, "tools")
				}
				if m.Reasoning != nil && *m.Reasoning {
					parts = append(parts, "reasoning")
				}
				if m.Vision != nil && *m.Vision {
					parts = append(parts, "vision")
				}
				detail = strings.Join(parts, " · ")
			}
		}
		items = append(items, selectItem{label: id, detail: detail})
	}
	return items
}

func cacheModels(st *Store, cfg Config, models []string) {
	if len(models) == 0 {
		return
	}
	st.Models = append([]string(nil), models...)
	st.ModelsForURL = cfg.BaseURL
	st.save()
}

func cachedModels(st *Store, cfg Config) []string {
	if st.ModelsForURL != cfg.BaseURL || len(st.Models) == 0 {
		return nil
	}
	return append([]string(nil), st.Models...)
}

func refreshModels(st *Store, cfg Config) {
	models, err := listModels(cfg)
	if err != nil {
		return
	}
	cacheModels(st, cfg, models)
}

func autosaveOn(st *Store) bool {
	return st.AutoSave == nil || *st.AutoSave
}

func setAutosave(st *Store, on bool) {
	st.AutoSave = &on
	st.save()
}

func sessionName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = time.Now().Format("20060102-150405")
	}
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' || r == 0 {
			return '-'
		}
		return r
	}, name)
}

func ensureSession(st *Store, current *string) string {
	if *current == "" {
		*current = filepath.Join(sessionsDir(), sessionName("")+".json")
		addSession(st, *current)
	}
	return *current
}

func addSession(st *Store, path string) {
	for _, p := range st.Conversations {
		if p == path {
			return
		}
	}
	st.Conversations = append(st.Conversations, path)
	st.save()
}

func saveSessionPath(path string, msgs []Message) error {
	data, err := json.MarshalIndent(msgs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func autosaveSession(st *Store, current *string, msgs []Message) {
	if !autosaveOn(st) || len(msgs) <= 1 {
		return
	}
	path := ensureSession(st, current)
	if err := saveSessionPath(path, msgs); err != nil {
		fmt.Fprintf(os.Stderr, "  error autosaving conversation: %s\n", err)
	}
}

func loadSession(path string) ([]Message, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var msgs []Message
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

func sessionItems(st *Store) []selectItem {
	items := make([]selectItem, 0, len(st.Conversations))
	for _, p := range st.Conversations {
		base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		items = append(items, selectItem{label: base, detail: p})
	}
	return items
}

func connectFlow(cfg *Config, st *Store, cat ProviderCatalog) {
	_, idx, ok := ui.SelectRich("provider>", providerItems(cat.Providers))
	if !ok {
		fmt.Println("  cancelled")
		return
	}
	p := &cat.Providers[idx]

	url := p.BaseURL
	if err := collectPlaceholders(st, url); err != nil {
		fmt.Printf("  error: %v\n", err)
		return
	}
	url = st.resolve(url)

	if p.Tag != "Local" {
		if err := promptAPIKey(st); err != nil {
			fmt.Printf("  error: %v\n", err)
			return
		}
		cfg.APIKey = st.APIKey
	}

	cfg.BaseURL = url
	st.BaseURL = p.BaseURL

	fmt.Printf("\n  fetching models from %s...\n", p.Name)
	models, err := listModels(*cfg)
	if err != nil {
		fmt.Printf("  \033[31merror\033[0m  %s\n", err)
		fmt.Println("  tip: run /model to type a model id")
		st.save()
		return
	}
	cacheModels(st, *cfg, models)

	if model, _, ok := ui.SelectRich("model>", modelItems(models, p)); ok {
		cfg.Model = model
		st.Model = cfg.Model
	} else {
		fmt.Println("  cancelled — keeping current model")
	}

	st.save()
	fmt.Printf("\n  connected to %s  model %s\n  saved → %s\n\n", p.Name, cfg.Model, configPath())
}

func boolPtrString(v *bool) string {
	if v == nil {
		return ""
	}
	if *v {
		return "yes"
	}
	return "no"
}

func parseBoolPtr(s string) *bool {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}
	v := s == "y" || s == "yes" || s == "true" || s == "1"
	return &v
}

func promptInt(label string, cur int) int {
	if cur > 0 {
		label = fmt.Sprintf("%s (current: %d, blank to keep)", label, cur)
	}
	s := prompt(label)
	if s == "" {
		return cur
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		fmt.Println("  invalid number — keeping current")
		return cur
	}
	return n
}

func promptBool(label string, cur *bool) *bool {
	if cur != nil {
		label = fmt.Sprintf("%s (current: %s, blank to keep)", label, boolPtrString(cur))
	}
	s := prompt(label + " yes/no")
	if s == "" {
		return cur
	}
	return parseBoolPtr(s)
}

func customCmd(args string, cfg *Config, st *Store, cat *ProviderCatalog) {
	args = strings.TrimSpace(args)
	if args == "" {
		items := []selectItem{
			{label: "list", detail: "show saved custom providers"},
			{label: "add", detail: "create and switch to a custom provider"},
			{label: "edit", detail: "change a saved custom provider"},
			{label: "remove", detail: "delete a saved custom provider"},
		}
		choice, _, ok := ui.SelectRich("custom>", items)
		if !ok {
			fmt.Println("  cancelled")
			return
		}
		args = choice
	}
	switch strings.ToLower(args) {
	case "list", "ls":
		if len(st.CustomProviders) == 0 {
			fmt.Println("  no custom providers")
			return
		}
		for i, p := range st.CustomProviders {
			model := p.Model
			if model == "" && p.Spec != nil {
				model = p.Spec.ID
			}
			fmt.Printf("  \033[2m%2d\033[0m  \033[1m%-28s\033[0m  %s  \033[2m%s\033[0m\n", i+1, p.Name, model, p.BaseURL)
		}
	case "add":
		p, ok := promptCustomProvider(Provider{}, *cat)
		if !ok {
			return
		}
		if key := prompt("api key (blank to keep)"); key != "" {
			cfg.APIKey = key
			st.APIKey = key
		}
		st.CustomProviders = append(st.CustomProviders, p)
		*cat = loadProviderCatalog(st.CustomProviders)
		applyProvider(&p, cfg, st)
		st.save()
		fmt.Printf("  added %s\n  saved → %s\n", p.Name, configPath())
	case "edit":
		idx := selectCustomProvider(st)
		if idx < 0 {
			return
		}
		p, ok := promptCustomProvider(st.CustomProviders[idx], *cat)
		if !ok {
			return
		}
		st.CustomProviders[idx] = p
		*cat = loadProviderCatalog(st.CustomProviders)
		st.save()
		fmt.Printf("  updated %s\n", p.Name)
	case "remove", "rm":
		idx := selectCustomProvider(st)
		if idx < 0 {
			return
		}
		name := st.CustomProviders[idx].Name
		st.CustomProviders = append(st.CustomProviders[:idx], st.CustomProviders[idx+1:]...)
		*cat = loadProviderCatalog(st.CustomProviders)
		st.save()
		fmt.Printf("  removed %s\n", name)
	default:
		fmt.Println("  run /custom and choose an action")
	}
}

func selectCustomProvider(st *Store) int {
	if len(st.CustomProviders) == 0 {
		fmt.Println("  no custom providers")
		return -1
	}
	items := providerItems(st.CustomProviders)
	_, idx, ok := ui.SelectRich("custom provider>", items)
	if !ok {
		fmt.Println("  cancelled")
		return -1
	}
	return idx
}

func promptCustomProvider(cur Provider, cat ProviderCatalog) (Provider, bool) {
	p := cur
	if p.Name == "" {
		p.Name = prompt("name")
	} else if s := prompt("name (blank to keep)"); s != "" {
		p.Name = s
	}
	if p.Name == "" {
		fmt.Println("  cancelled")
		return Provider{}, false
	}
	if p.BaseURL == "" {
		p.BaseURL = prompt("base url")
	} else if s := prompt("base url (blank to keep)"); s != "" {
		p.BaseURL = s
	}
	if p.BaseURL == "" {
		fmt.Println("  cancelled")
		return Provider{}, false
	}
	if p.Model == "" {
		p.Model = prompt("model")
	} else if s := prompt("model (blank to keep)"); s != "" {
		p.Model = s
	}
	if p.Model == "" {
		fmt.Println("  cancelled")
		return Provider{}, false
	}
	var base *ModelSpec
	if cp := cat.findProvider(p.Name); cp != nil {
		base = cp.findModel(p.Model)
	}
	if base == nil {
		for _, cp := range cat.Providers {
			if m := cp.findModel(p.Model); m != nil {
				base = m
				break
			}
		}
	}
	spec := mergeModelSpec(base, p.Spec)
	if spec == nil {
		spec = &ModelSpec{ID: p.Model}
	}
	spec.ID = p.Model
	if base != nil {
		fmt.Printf("  using model metadata: context %d output %d tools %s reasoning %s vision %s\n", spec.Context, spec.Output, boolPtrString(spec.Tools), boolPtrString(spec.Reasoning), boolPtrString(spec.Vision))
	}
	spec.Context = promptInt("context length", spec.Context)
	spec.Output = promptInt("output length", spec.Output)
	spec.Tools = promptBool("supports tools", spec.Tools)
	spec.Reasoning = promptBool("supports reasoning", spec.Reasoning)
	spec.Vision = promptBool("supports vision", spec.Vision)
	p.Spec = spec
	p.Models = []ModelSpec{*spec}
	p.Custom = true
	p.Tag = "Custom"
	return p, true
}

func applyProvider(p *Provider, cfg *Config, st *Store) {
	cfg.BaseURL = st.resolve(p.BaseURL)
	st.BaseURL = p.BaseURL
	if p.Model != "" {
		cfg.Model = p.Model
		st.Model = p.Model
	}
}

func firstRunSetup(cfg *Config) {
	if cfg.APIKey == "" {
		fmt.Println("  no api key configured — run /connect to set up a provider")
		fmt.Println()
	}
}

func useCmd(args string, cfg *Config, st *Store, cat ProviderCatalog) {
	if args == "" {
		_, idx, ok := ui.SelectRich("provider>", providerItems(cat.Providers))
		if !ok {
			fmt.Println("  cancelled")
			return
		}
		args = cat.Providers[idx].Name
	}

	if strings.ToLower(args) == "custom" {
		customCmd("add", cfg, st, &cat)
		return
	}

	p := cat.findProvider(args)
	if p == nil {
		fmt.Println("  unknown provider — run /use to choose or /custom to add one")
		return
	}

	url := p.BaseURL
	if err := collectPlaceholders(st, url); err != nil {
		fmt.Printf("  error: %v\n", err)
		return
	}
	url = st.resolve(url)

	if p.Tag != "Local" {
		if err := promptAPIKey(st); err != nil {
			fmt.Printf("  error: %v\n", err)
			return
		}
		cfg.APIKey = st.APIKey
	}

	cfg.BaseURL = url
	st.BaseURL = p.BaseURL
	if p.Model != "" {
		cfg.Model = p.Model
		st.Model = p.Model
	}
	st.save()
	fmt.Printf("  switched to %s\n  saved → %s\n", p.Name, configPath())
}

func isUpdateArg(args []string) bool {
	return len(args) > 1 && (args[1] == "update" || args[1] == "/update")
}

func main() {
	enableVT()
	if isUpdateArg(os.Args) {
		updateCmd()
		return
	}
	st := loadStore()
	cat := loadProviderCatalog(st.CustomProviders)
	cfg := loadConfig(st)
	go refreshModels(st, cfg)
	skills := discoverSkills()
	msgs := []Message{{Role: "system", Content: buildSystemPrompt(skills)}}
	currentSession := ""
	activatedSkills := make(map[string]bool)

	ui = newTUI()
	ui.setHistory(st.History)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		st.History = ui.history
		st.save()
		ui.Teardown()
		fmt.Println()
		os.Exit(0)
	}()

	// Start background update check before printing banner
	updateAvail := make(chan string, 1)
	go func() {
		if tag, _, err := checkUpdate(); err == nil && isNewer(tag, version) {
			updateAvail <- tag
		}
	}()

	fmt.Printf("\033[1mau\033[0m  \033[2m%s\033[0m  \033[33malpha\033[0m\n", version)
	modelLabel := cfg.Model
	if modelLabel == "" {
		modelLabel = "not set — type /models or /connect"
	}
	fmt.Printf("   model   %s\n", modelLabel)
	fmt.Printf("   url     %s\n", cfg.BaseURL)
	fmt.Printf("   config  %s\n", configPath())
	if len(skills) > 0 {
		fmt.Printf("   skills  %d available\n", len(skills))
	}
	fmt.Println()

	firstRunSetup(&cfg)

	// Show update notice if check already completed
	select {
	case tag := <-updateAvail:
		fmt.Printf("  \033[33m↑ new version %s available — /update to install\033[0m\n\n", tag)
	default:
	}

	ui.Refresh(cfg.Model, cfg.Thinking)

	for {
		input := ui.ReadLine()
		if input == "" {
			continue
		}

		cmd := input
		if strings.HasPrefix(input, "/") {
			parts := strings.Fields(input)
			if len(parts) > 0 {
				cmd = parts[0]
			}
		}

		switch {
		case cmd == "/q", cmd == "/quit", cmd == "/exit":
			st.History = ui.history
			st.save()
			ui.Teardown()
			os.Exit(0)

			case cmd == "/reset":
				msgs = []Message{{Role: "system", Content: buildSystemPrompt(skills)}}
				activatedSkills = make(map[string]bool)
				autosaveSession(st, &currentSession, msgs)
				fmt.Println("  context cleared")

			case cmd == "/new":
				msgs = []Message{{Role: "system", Content: buildSystemPrompt(skills)}}
				activatedSkills = make(map[string]bool)
				currentSession = ""
				fmt.Println("  new conversation")

			case cmd == "/autosave":
				arg := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(input, "/autosave")))
				switch arg {
				case "on", "true", "1":
					setAutosave(st, true)
					fmt.Println("  autosave on")
				case "off", "false", "0":
					setAutosave(st, false)
					fmt.Println("  autosave off")
				default:
					if autosaveOn(st) {
						fmt.Println("  autosave on  \033[2m(/autosave off to disable)\033[0m")
					} else {
						fmt.Println("  autosave off  \033[2m(/autosave on to enable)\033[0m")
					}
				}

			case cmd == "/resume":
				arg := strings.TrimSpace(strings.TrimPrefix(input, "/resume"))
				path := ""
				if arg != "" {
					for _, p := range st.Conversations {
						base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
						if base == arg {
							path = p
							break
						}
					}
					if path == "" {
						path = filepath.Join(sessionsDir(), arg+".json")
					}
				} else {
					items := sessionItems(st)
					if len(items) == 0 {
						fmt.Println("  no saved conversations")
						break
					}
					_, idx, ok := ui.SelectRich("resume>", items)
					if !ok {
						fmt.Println("  cancelled")
						break
					}
					path = items[idx].detail
				}
				loaded, err := loadSession(path)
				if err != nil {
					fmt.Printf("  \033[31merror\033[0m  %s\n", err)
				} else {
					msgs = loaded
					currentSession = path
					addSession(st, path)
					activatedSkills = make(map[string]bool)
					fmt.Printf("  resumed %s\n", strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
				}

			case cmd == "/connect":
			connectFlow(&cfg, st, cat)
			ui.Refresh(cfg.Model, cfg.Thinking)

		case cmd == "/providers":
			fmt.Println()
			if cat.Fallback {
				fmt.Println("  \033[33musing fallback providers — models.dev unavailable\033[0m")
			}
			for _, p := range cat.Providers {
				label := p.Name + " (" + p.Tag + ")"
				fmt.Printf("  \033[1m%-38s\033[0m  \033[2m%s\033[0m\n", label, p.BaseURL)
			}
			fmt.Println("  \033[2mrun /use to switch · run /custom to manage custom endpoints\033[0m")
			fmt.Println()

		case cmd == "/update":
			updateCmd()

		case cmd == "/skills":
			if len(skills) == 0 {
				fmt.Println("  no skills found")
				fmt.Printf("  install skills in ~/.agents/skills/ or ./.agents/skills/\n")
			} else {
				fmt.Println()
				for _, s := range skills {
					active := ""
					if activatedSkills[s.Name] {
						active = "  \033[32m●\033[0m"
					}
					fmt.Printf("  \033[1m%-28s\033[0m  \033[2m%s\033[0m%s\n", s.Name, s.Description, active)
				}
				fmt.Println()
			}

		case cmd == "/skill":
			items := make([]selectItem, 0, len(skills))
			for _, s := range skills {
				detail := s.Description
				if activatedSkills[s.Name] {
					detail = "active · " + detail
				}
				items = append(items, selectItem{label: s.Name, detail: detail})
			}
			name, _, ok := ui.SelectRich("skill>", items)
			if !ok {
				fmt.Println("  cancelled")
				break
			}
			sk := findSkill(skills, name)
			if sk == nil {
				fmt.Printf("  skill %q not found — /skills to list available\n", name)
			} else if activatedSkills[sk.Name] {
				fmt.Printf("  skill %s already active\n", sk.Name)
			} else {
				body, err := loadSkillBody(sk.Location)
				if err != nil {
					fmt.Printf("  \033[31merror loading skill:\033[0m %v\n", err)
				} else {
					skillDir := filepath.Dir(sk.Location)
					content := fmt.Sprintf("<skill_content name=%q>\n%s\n\nSkill directory: %s\nRelative paths in this skill are relative to the skill directory.\n</skill_content>", sk.Name, body, skillDir)
					msgs = append(msgs, Message{Role: "system", Content: content})
					activatedSkills[sk.Name] = true
					fmt.Printf("  \033[32m●\033[0m  skill \033[1m%s\033[0m activated\n", sk.Name)
				}
			}

		case cmd == "/help":
			fmt.Println()
			for _, c := range cmdList {
				fmt.Printf("  \033[1m%-14s\033[0m  \033[2m%s\033[0m\n", c.name, c.desc)
			}
			fmt.Println()

			case cmd == "/models":
				models := cachedModels(st, cfg)
				if len(models) == 0 {
					fmt.Println("  no cached models yet — fetching...")
					var err error
					models, err = listModels(cfg)
					if err != nil {
						fmt.Fprintf(os.Stderr, "  error: %s\n", err)
						break
					}
					cacheModels(st, cfg, models)
				} else {
					go refreshModels(st, cfg)
				}
				if model, _, ok := ui.SelectRich("model>", modelItems(models, currentProvider(cat, cfg))); ok {
					cfg.Model = model
					st.Model = model
					st.save()
					fmt.Printf("  model → %s\n", cfg.Model)
					ui.Refresh(cfg.Model, cfg.Thinking)
				}

		case cmd == "/model":
			model := prompt("model id")
			if model != "" {
				cfg.Model = model
				st.Model = cfg.Model
				st.save()
				fmt.Printf("  model → %s\n", cfg.Model)
				ui.Refresh(cfg.Model, cfg.Thinking)
			}

		case cmd == "/key":
			val := prompt("api key")
			if val != "" {
				cfg.APIKey = val
				st.APIKey = val
				st.save()
				fmt.Printf("  saved → %s\n", configPath())
			}

		case cmd == "/thinking":
			items := []selectItem{{label: "0", detail: "off"}}
			for i := 1; i <= 10; i++ {
				items = append(items, selectItem{label: strconv.Itoa(i), detail: thinkingStr(i)})
			}
			level, _, ok := ui.SelectRich("thinking>", items)
			if !ok {
				fmt.Println("  cancelled")
				break
			}
			n, _ := strconv.Atoi(level)
			cfg.Thinking = n
			st.Thinking = n
			st.save()
			if n == 0 {
				fmt.Println("  thinking off")
			} else {
				fmt.Printf("  thinking %d  %s\n", n, thinkingStr(n))
			}
			ui.Refresh(cfg.Model, cfg.Thinking)

		case cmd == "/custom":
			customCmd("", &cfg, st, &cat)
			ui.Refresh(cfg.Model, cfg.Thinking)

		case cmd == "/use":
			useCmd("", &cfg, st, cat)
			ui.Refresh(cfg.Model, cfg.Thinking)

		case cmd == "/yolo":
			yoloMode = !yoloMode
			if yoloMode {
				fmt.Println("  \033[33m⚡ yolo mode on\033[0m  dangerous commands will run without confirmation")
			} else {
				fmt.Println("  yolo mode off  dangerous commands will prompt for confirmation")
			}

		case strings.HasPrefix(input, "/"):
			fmt.Println("  unknown command")

			default:
				msgs = append(msgs, Message{Role: "user", Content: input})
				autosaveSession(st, &currentSession, msgs)

				fmt.Println()

			start := time.Now()
			streamTokens := 0
			var streamStart, streamEnd time.Time

			// Remember where msgs was before this turn for clean rollback on error.
			preUserLen := len(msgs) - 1
				for {
					restoreTask, _ := enterTaskMode(ui.fd)
					stopInput, _ := runInputBar(ui.height())
					renderer := newLineRenderer()
					stopSpinner := startSpinner()
					// Track byte count for the in-flight tool argument so we can
					// show live progress while the model is generating it.
				var (
					curToolName    string
					curToolBytes   int
					stopArgSpinner func()
					updateArgBytes func(int)
					argSpinActive  bool
				)
				beginArgSpinner := func() {
					if argSpinActive {
						return
					}
					argSpinActive = true
					label := curToolName
					if label == "" {
						label = "tool"
					}
					stopArgSpinner, updateArgBytes = startLabeledSpinner("writing " + label)
				}
				content, toolCalls, err := complete(cfg, msgs, toolDefs,
					func() { stopSpinner() },
					func(tok string) {
						if streamStart.IsZero() {
							streamStart = time.Now()
						}
						streamEnd = time.Now()
							streamTokens += approxTokens(tok)
							outMu.Lock()
							renderer.Feed(tok)
							outMu.Unlock()
					},
					func(name, argsChunk string) {
						if streamStart.IsZero() {
							streamStart = time.Now()
						}
						streamEnd = time.Now()
						if name != "" {
							curToolName = name
						}
						if argsChunk != "" {
							streamTokens += approxTokens(argsChunk)
							curToolBytes += len(argsChunk)
							beginArgSpinner()
							if updateArgBytes != nil {
								updateArgBytes(curToolBytes)
							}
						}
					},
				)
					if argSpinActive && stopArgSpinner != nil {
						stopArgSpinner()
					}
					stopSpinner()
					if stopInput != nil {
						stopInput()
					}
					if restoreTask != nil {
						restoreTask()
					}
					outMu.Lock()
					renderer.Flush()
					outMu.Unlock()

				if err != nil {
					errMsg := err.Error()
					fmt.Fprintf(os.Stderr, "  \033[31merror\033[0m  %s\n", errMsg)
					if strings.Contains(errMsg, "deadline exceeded") || strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "connection reset") {
						fmt.Fprintf(os.Stderr, "  \033[2mtip: press ↑ to retry\033[0m\n")
					}
					// Roll back all messages added during this turn (user msg + any tool exchanges).
					msgs = msgs[:preUserLen]
					break
				}

				asst := Message{Role: "assistant", Content: content}
				if len(toolCalls) > 0 {
					asst.ToolCalls = toolCalls
				}
					msgs = append(msgs, asst)
					autosaveSession(st, &currentSession, msgs)

					if len(toolCalls) == 0 {
						queued := drainQueue()
						if len(queued) == 0 {
							break
						}
						for _, q := range queued {
							msgs = append(msgs, Message{Role: "user", Content: q})
						}
						autosaveSession(st, &currentSession, msgs)
						fmt.Printf("  \033[2m↳ injected %d queued message(s)\033[0m\n", len(queued))
						continue
					}

				if content != "" {
					fmt.Println()
				}
					for _, tc := range toolCalls {
						displayToolCall(tc)
						if !confirmTool(tc) {
							result := "error: command blocked by user"
							displayToolResult(tc, result)
							msgs = append(msgs, Message{Role: "tool", Content: result, ToolCallID: tc.ID})
							autosaveSession(st, &currentSession, msgs)
							continue
						}
						// Run the tool with an animated progress line so the UI
						// never looks stalled during long-running operations.
						resultCh := make(chan string, 1)
						go func() {
							resultCh <- executeTool(tc.Function.Name, tc.Function.Arguments)
						}()
						stopToolSpin, _ := startLabeledSpinner(toolLabel(tc.Function.Name))
						result := <-resultCh
						stopToolSpin()
						displayToolResult(tc, result)
						msgs = append(msgs, Message{
							Role:       "tool",
							Content:    result,
							ToolCallID: tc.ID,
						})
						autosaveSession(st, &currentSession, msgs)
					}
					queued := drainQueue()
					if len(queued) > 0 {
						for _, q := range queued {
							msgs = append(msgs, Message{Role: "user", Content: q})
						}
						autosaveSession(st, &currentSession, msgs)
						fmt.Printf("  \033[2m↳ injected %d queued message(s)\033[0m\n", len(queued))
					}
				}

			elapsed := time.Since(start)
			tps := 0.0
			// TPS reflects generation speed, so measure over the actual
			// streaming window (first token → last token), excluding idle
			// time-to-first-token latency and tool execution time.
			streamDur := 0.0
			if !streamStart.IsZero() && !streamEnd.IsZero() {
				streamDur = streamEnd.Sub(streamStart).Seconds()
			}
			if streamDur > 0 {
				tps = float64(streamTokens) / streamDur
			}
			stats := fmt.Sprintf(" %.1f tps  %.1fs ", tps, elapsed.Seconds())
			w := ui.Width()
			leftLen := max(1, w-len(stats)-1)
			sep := strings.Repeat("─", leftLen) + stats + "─"
			fmt.Printf("\033[2m%s\033[0m\n\n", sep)
		}
	}
}

func buildSystemPrompt(skills []Skill) string {
	cwd, _ := os.Getwd()
	shell := "sh (bash/zsh)"
	if runtime.GOOS == "windows" {
		shell = "powershell"
	}
	base := "You are a coding assistant with full filesystem access. " +
		"Use tools to read files, write code, run commands, and complete tasks. " +
		"Continue the tool-use loop autonomously until the task is complete. " +
		"Working directory: " + cwd + ". " +
		"Shell: " + shell + ". " +
		"Prefer dedicated file tools over shell commands for file operations: " +
		"use search_files instead of grep/find, patch_file for targeted edits instead of read+write, " +
		"delete_file instead of rm, move_file instead of mv, append_file instead of echo >>. " +
		"For tasks with multiple steps, use add_todo at the start to list all steps, " +
		"then update_todo (in_progress/done) as you work through them. " +
		"Respond in plain text. No markdown tables, no markdown headers, no bullet formatting. " +
		"Only use code blocks (triple backtick) when showing actual code snippets inline. " +
		"When writing code to disk, use write_file instead."
	if agentsMD := loadAgentsMD(); agentsMD != "" {
		base += "\n\n" + agentsMD
	}
	return base + buildSkillCatalog(skills)
}
