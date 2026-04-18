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

var ui *TUI

func displayUserMessage(msg string) {
	fmt.Println()
	lines := strings.Split(msg, "\n")
	for i, line := range lines {
		if i == 0 {
			fmt.Printf("  \033[36myou\033[0m  %s\n", line)
		} else {
			fmt.Printf("       %s\n", line)
		}
	}
	fmt.Println()
}

func prompt(label string) string {
	fmt.Printf("  %s: ", label)
	r := bufio.NewReader(os.Stdin)
	s, _ := r.ReadString('\n')
	return strings.TrimSpace(s)
}

func findProvider(name string) *Provider {
	lower := strings.ToLower(name)
	// Try exact match first (case-insensitive)
	for i := range providers {
		if strings.ToLower(providers[i].Name) == lower {
			return &providers[i]
		}
	}
	// Try partial match only if no exact match
	for i := range providers {
		if strings.Contains(strings.ToLower(providers[i].Name), lower) {
			return &providers[i]
		}
	}
	return nil
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
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	stop := make(chan struct{}, 1)
	done := make(chan struct{}, 1)
	go func() {
		defer close(done)
		i := 0
		for {
			select {
			case <-stop:
				fmt.Printf("\r\033[K")
				return
			default:
				fmt.Printf("\r\033[2m%s\033[0m", frames[i%len(frames)])
				i++
				time.Sleep(80 * time.Millisecond)
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			select {
			case stop <- struct{}{}:
			case <-time.After(100 * time.Millisecond):
			}
			<-done
		})
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

func connectFlow(cfg *Config, st *Store) {
	fmt.Println()
	fmt.Println("  \033[1mproviders\033[0m")
	for i, p := range providers {
		fmt.Printf("  \033[2m%2d\033[0m  \033[1m%-28s\033[0m  \033[2m%s\033[0m\n", i+1, p.Name, p.Tag)
	}
	fmt.Println()

	choice := prompt("provider number or name")
	if choice == "" {
		fmt.Println("  cancelled")
		return
	}

	var p *Provider
	if n, err := strconv.Atoi(choice); err == nil && n >= 1 && n <= len(providers) {
		p = &providers[n-1]
	} else {
		p = findProvider(choice)
	}
	if p == nil {
		fmt.Println("  unknown provider")
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

	fmt.Printf("\n  fetching models from %s...\n", p.Name)
	models, err := listModels(*cfg)
	if err != nil {
		fmt.Printf("  \033[31merror\033[0m  %s\n", err)
		fmt.Println("  tip: set model manually with /model <id>")
		st.save()
		return
	}

	fmt.Println()
	for i, m := range models {
		fmt.Printf("  \033[2m%2d\033[0m  %s\n", i+1, m)
	}
	fmt.Println()

	mchoice := prompt("model number or id")
	if mchoice == "" {
		fmt.Println("  cancelled — keeping current model")
	} else if n, err := strconv.Atoi(mchoice); err == nil && n >= 1 && n <= len(models) {
		cfg.Model = models[n-1]
		st.Model = cfg.Model
	} else {
		cfg.Model = mchoice
		st.Model = cfg.Model
	}

	st.save()
	fmt.Printf("\n  connected to %s  model %s\n  saved → %s\n\n", p.Name, cfg.Model, configPath())
}

func firstRunSetup(cfg *Config) {
	if cfg.APIKey == "" {
		fmt.Println("  no api key configured — run /connect to set up a provider")
		fmt.Println()
	}
}

func useCmd(args string, cfg *Config, st *Store) {
	if args == "" {
		fmt.Println("  usage: /use <provider name> | /use custom")
		fmt.Println("  tip:   /providers to list all")
		return
	}

	if strings.ToLower(args) == "custom" {
		url := prompt("base url")
		if url == "" {
			fmt.Println("  cancelled")
			return
		}
		if key := prompt("api key (blank to keep)"); key != "" {
			cfg.APIKey = key
			st.APIKey = key
		}
		if model := prompt("model (blank to keep)"); model != "" {
			cfg.Model = model
			st.Model = model
		}
		cfg.BaseURL = url
		st.BaseURL = url
		st.save()
		fmt.Printf("  saved → %s\n", configPath())
		return
	}

	p := findProvider(args)
	if p == nil {
		fmt.Println("  unknown provider — try /providers or /use custom")
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
	st.save()
	fmt.Printf("  switched to %s\n  saved → %s\n", p.Name, configPath())
}

func main() {
	enableVT()
	st := loadStore()
	cfg := loadConfig(st)
	skills := discoverSkills()
	msgs := []Message{{Role: "system", Content: buildSystemPrompt(skills)}}
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

	fmt.Printf("\033[1mau\033[0m  \033[2m%s\033[0m  \033[33malpha — many endpoints misconfigured, use at own risk\033[0m\n", version)
	fmt.Printf("   model   %s\n", cfg.Model)
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

		switch {
		case input == "/q", input == "/quit", input == "/exit":
			st.History = ui.history
			st.save()
			ui.Teardown()
			os.Exit(0)

		case input == "/reset":
			msgs = []Message{{Role: "system", Content: buildSystemPrompt(skills)}}
			activatedSkills = make(map[string]bool)
			fmt.Println("  context cleared")

		case input == "/connect":
			connectFlow(&cfg, st)
			ui.Refresh(cfg.Model, cfg.Thinking)

		case input == "/providers":
			fmt.Println()
			for _, p := range providers {
				label := p.Name + " (" + p.Tag + ")"
				fmt.Printf("  \033[1m%-38s\033[0m  \033[2m%s\033[0m\n", label, p.BaseURL)
			}
			fmt.Println("  \033[2m/use custom  to set a custom endpoint\033[0m")
			fmt.Println()

		case input == "/update":
			updateCmd()

		case input == "/skills":
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

		case strings.HasPrefix(input, "/skill "):
			name := strings.TrimSpace(input[7:])
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

		case input == "/help":
			fmt.Println()
			for _, c := range cmdList {
				fmt.Printf("  \033[1m%-14s\033[0m  \033[2m%s\033[0m\n", c.name, c.desc)
			}
			fmt.Println()

		case input == "/models":
			models, err := listModels(cfg)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  error: %s\n", err)
			} else {
				fmt.Println()
				for _, m := range models {
					fmt.Printf("  %s\n", m)
				}
				fmt.Println()
			}

		case strings.HasPrefix(input, "/model "):
			cfg.Model = strings.TrimSpace(input[7:])
			st.Model = cfg.Model
			st.save()
			fmt.Printf("  model → %s\n", cfg.Model)
			ui.Refresh(cfg.Model, cfg.Thinking)

		case strings.HasPrefix(input, "/key"):
			val := strings.TrimSpace(input[4:])
			if val == "" {
				val = prompt("api key")
			}
			if val != "" {
				cfg.APIKey = val
				st.APIKey = val
				st.save()
				fmt.Printf("  saved → %s\n", configPath())
			}

		case strings.HasPrefix(input, "/thinking"):
			arg := strings.TrimSpace(input[9:])
			if arg == "" {
				if cfg.Thinking == 0 {
					fmt.Println("  thinking off")
				} else {
					fmt.Printf("  thinking %d  %s\n", cfg.Thinking, thinkingStr(cfg.Thinking))
				}
			} else {
				n, err := strconv.Atoi(arg)
				if err != nil || n < 0 || n > 10 {
					fmt.Println("  usage: /thinking <0-10>  (0 = off)")
				} else {
					cfg.Thinking = n
					st.Thinking = n
					st.save()
					if n == 0 {
						fmt.Println("  thinking off")
					} else {
						fmt.Printf("  thinking %d  %s\n", n, thinkingStr(n))
					}
					ui.Refresh(cfg.Model, cfg.Thinking)
				}
			}

		case input == "/use" || strings.HasPrefix(input, "/use "):
			args := ""
			if len(input) > 4 {
				args = strings.TrimSpace(input[5:])
			}
			useCmd(args, &cfg, st)
			ui.Refresh(cfg.Model, cfg.Thinking)

		case input == "/yolo":
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

			// Enforce history limit to prevent context overflow
			if len(msgs) > maxHistoryLength {
				// Keep system prompt and last N messages
				newMsgs := []Message{msgs[0]}
				newMsgs = append(newMsgs, msgs[len(msgs)-maxHistoryLength+1:]...)
				msgs = newMsgs
			}

			fmt.Println()

			displayUserMessage(input)
		start := time.Now()

		// Remember where msgs was before this turn for clean rollback on error.
		preUserLen := len(msgs) - 1
		for {
			renderer := newLineRenderer()
			stopSpinner := startSpinner()
			content, toolCalls, err := complete(cfg, msgs, toolDefs,
				func() { stopSpinner() },
				func(tok string) { renderer.Feed(tok) },
			)
			stopSpinner()
			renderer.Flush()

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

			if len(toolCalls) == 0 {
				break
			}

			if content != "" {
				fmt.Println()
			}
			for _, tc := range toolCalls {
				displayToolCall(tc)
				result := executeTool(tc.Function.Name, tc.Function.Arguments)
				displayToolResult(tc, result)
				msgs = append(msgs, Message{
					Role:       "tool",
					Content:    result,
					ToolCallID: tc.ID,
				})
			}
			// Trim history after tool results to prevent unbounded growth
			if len(msgs) > maxHistoryLength {
				newMsgs := []Message{msgs[0]}
				newMsgs = append(newMsgs, msgs[len(msgs)-maxHistoryLength+1:]...)
				msgs = newMsgs
			}
		}

		elapsed := time.Since(start)
			dur := fmt.Sprintf(" %.1fs ", elapsed.Seconds())
			w := ui.Width()
			leftLen := max(1, w-len(dur)-1)
			sep := strings.Repeat("─", leftLen) + dur + "─"
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
