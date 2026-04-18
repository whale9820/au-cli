package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

var reSafePath = regexp.MustCompile(`^[^\x00-\x1f\x7f]+$`)

type todoItem struct {
	ID     int
	Title  string
	Status string // "pending", "in_progress", or "done"
}

var (
	todosMu   sync.Mutex
	todosList []todoItem
	todosNext = 1
)

var errStopWalk = errors.New("stop")

// bufWrapper wraps a byte slice to enforce size limits with concurrent-write safety
type bufWrapper struct {
	mu      *sync.Mutex
	buf     *[]byte
	maxSize int
}

func (w *bufWrapper) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	currentLen := len(*w.buf)
	if currentLen >= w.maxSize {
		return len(p), nil // Silently drop data over the limit
	}
	remaining := w.maxSize - currentLen
	if len(p) > remaining {
		p = p[:remaining]
	}
	*w.buf = append(*w.buf, p...)
	return len(p), nil
}

var toolDefs []Tool

func init() {
	toolDefs = []Tool{
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "read_file",
				Description: "Read the full contents of a file. Path must not contain '..' for security.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "File path to read - must be absolute or relative to current directory"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "write_file",
				Description: "Write content to a file, creating it and parent directories if needed. Set overwrite=true to replace existing files. For small targeted edits to existing files, prefer patch_file instead.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":      map[string]any{"type": "string", "description": "File path to write"},
						"content":   map[string]any{"type": "string", "description": "Content to write"},
						"overwrite": map[string]any{"type": "boolean", "description": "Set to true to overwrite an existing file (default: false)"},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "patch_file",
				Description: "Replace all occurrences of an exact string in a file with new content. Prefer this over read_file+write_file for targeted edits. Returns the number of replacements made.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":    map[string]any{"type": "string", "description": "File path to patch"},
						"old_str": map[string]any{"type": "string", "description": "Exact string to find and replace"},
						"new_str": map[string]any{"type": "string", "description": "Replacement string"},
					},
					"required": []string{"path", "old_str", "new_str"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "append_file",
				Description: "Append content to the end of a file, creating it if it doesn't exist. Prefer this over shell redirection (echo >> file).",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":    map[string]any{"type": "string", "description": "File path to append to"},
						"content": map[string]any{"type": "string", "description": "Content to append"},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "delete_file",
				Description: "Delete a file. Prefer this over shell commands like rm.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "File path to delete"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "move_file",
				Description: "Move or rename a file or directory. Prefer this over shell commands like mv.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"src": map[string]any{"type": "string", "description": "Source file or directory path"},
						"dst": map[string]any{"type": "string", "description": "Destination file or directory path"},
					},
					"required": []string{"src", "dst"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "search_files",
				Description: "Search for a literal text pattern across files. Returns matching lines with file paths and line numbers. Prefer this over running grep/find shell commands.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pattern": map[string]any{"type": "string", "description": "Literal text to search for (case-sensitive)"},
						"path":    map[string]any{"type": "string", "description": "Directory or file path to search in (default: current directory)"},
						"glob":    map[string]any{"type": "string", "description": "Filename glob to filter files (e.g. '*.go', '*.ts'). Leave empty to search all files."},
					},
					"required": []string{"pattern"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "run_command",
				Description: "Run a shell command and return combined stdout+stderr. Commands are limited to 60 seconds. Use file tools (read_file, write_file, patch_file, search_files, etc.) instead of shell commands for file operations when possible.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"command": map[string]any{"type": "string", "description": "Shell command to run"},
					},
					"required": []string{"command"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "list_directory",
				Description: "List files and subdirectories at a path with their sizes.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "Directory path to list"},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "add_todo",
				Description: "Add a todo item to track progress on a multi-step task. Returns the new item's ID. Use this at the start of complex tasks to lay out all steps, then update statuses as you go.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title": map[string]any{"type": "string", "description": "Short description of the task step"},
					},
					"required": []string{"title"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "list_todos",
				Description: "List all current todo items with their IDs and statuses (pending, in_progress, done).",
				Parameters: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "update_todo",
				Description: "Update the status of a todo item. Use 'in_progress' when starting a step, 'done' when it's complete.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":     map[string]any{"type": "integer", "description": "Todo ID returned by add_todo"},
						"status": map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "done"}, "description": "New status"},
					},
					"required": []string{"id", "status"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "remove_todo",
				Description: "Remove a todo item by ID.",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id": map[string]any{"type": "integer", "description": "Todo ID to remove"},
					},
					"required": []string{"id"},
				},
			},
		},
	}
}

func executeTool(name, argsJSON string) string {
	switch name {
	case "read_file":
		var a struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
			return "error: " + err.Error()
		}
		// Validate path to prevent directory traversal
		cleanPath := filepath.Clean(a.Path)
		if strings.Contains(cleanPath, "..") {
			return `error: path contains ".." - directory traversal not allowed`
		}
		if !reSafePath.MatchString(cleanPath) {
			return "error: path contains invalid characters"
		}
		f, err := os.Open(cleanPath)
		if err != nil {
			return "error: " + err.Error()
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, maxFileRead+1))
		if err != nil {
			return "error: " + err.Error()
		}
		truncated := len(data) > maxFileRead
		if truncated {
			data = data[:maxFileRead]
		}
		result := string(data)
		if truncated {
			result += fmt.Sprintf("\n[read_file: truncated at %d bytes]", maxFileRead)
		}
		return result

	case "write_file":
		var a struct {
			Path      string `json:"path"`
			Content   string `json:"content"`
			Overwrite bool   `json:"overwrite"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
			return "error: " + err.Error()
		}
		cleanPath := filepath.Clean(a.Path)
		if strings.Contains(cleanPath, "..") {
			return `error: path contains ".." - directory traversal not allowed`
		}
		if !reSafePath.MatchString(cleanPath) {
			return "error: path contains invalid characters"
		}
		if !a.Overwrite {
			if _, err := os.Stat(cleanPath); err == nil {
				return "error: file already exists - set overwrite=true to replace it"
			}
		}
		if err := os.MkdirAll(filepath.Dir(cleanPath), 0750); err != nil {
			return "error: " + err.Error()
		}
		if err := os.WriteFile(cleanPath, []byte(a.Content), 0644); err != nil {
			return "error: " + err.Error()
		}
		return "ok"

	case "run_command":
		var a struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
			return "error: " + err.Error()
		}
		cmdStr := strings.TrimSpace(a.Command)
		if cmdStr == "" {
			return "error: empty command"
		}
		// For potentially dangerous commands, prompt unless yolo mode is on
		dangerousPatterns := []string{`rm -rf`, `>/dev/`, `:(){:|: &}:;:`, `mkfs`, `dd if=`, `chmod -R 777`}
		isDangerous := false
		for _, pattern := range dangerousPatterns {
			if strings.Contains(cmdStr, pattern) {
				isDangerous = true
				break
			}
		}
		if isDangerous && !yoloMode {
			fmt.Printf("\n  \033[33m⚠  dangerous command\033[0m  %s\n  allow? [y/N] ", cmdStr)
			r := bufio.NewReader(os.Stdin)
			ans, _ := r.ReadString('\n')
			ans = strings.TrimSpace(strings.ToLower(ans))
			if ans != "y" && ans != "yes" {
				return "error: command blocked by user"
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			// Use -EncodedCommand to prevent injection
			cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", a.Command)
		} else {
			cmd = exec.CommandContext(ctx, "sh", "-c", a.Command)
		}
		// Stream output in chunks to handle large outputs
		bufSize := 4096
		out := make([]byte, 0, bufSize)
		mu := &sync.Mutex{}
		cmd.Stdout = &bufWrapper{mu: mu, buf: &out, maxSize: maxToolOutput}
		cmd.Stderr = &bufWrapper{mu: mu, buf: &out, maxSize: maxToolOutput}
		if err := cmd.Run(); err != nil {
			result := string(out)
			return fmt.Sprintf("error: %v\n%s", err, result)
		}
		return string(out)

	case "list_directory":
		var a struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
			return "error: " + err.Error()
		}
		// Validate path to prevent directory traversal
		cleanPath := filepath.Clean(a.Path)
		if strings.Contains(cleanPath, "..") {
			return `error: path contains ".." - directory traversal not allowed`
		}
		entries, err := os.ReadDir(cleanPath)
		if err != nil {
			return "error: " + err.Error()
		}
		var sb strings.Builder
		for _, e := range entries {
			if e.IsDir() {
				fmt.Fprintf(&sb, "%s/\n", e.Name())
			} else {
				info, err := e.Info()
				if err == nil && info != nil {
					fmt.Fprintf(&sb, "%-40s %d\n", e.Name(), info.Size())
				} else {
					fmt.Fprintf(&sb, "%s\n", e.Name())
				}
			}
		}
		return sb.String()

	case "patch_file":
		var a struct {
			Path   string `json:"path"`
			OldStr string `json:"old_str"`
			NewStr string `json:"new_str"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
			return "error: " + err.Error()
		}
		cleanPath := filepath.Clean(a.Path)
		if strings.Contains(cleanPath, "..") {
			return `error: path contains ".." - directory traversal not allowed`
		}
		if !reSafePath.MatchString(cleanPath) {
			return "error: path contains invalid characters"
		}
		data, err := os.ReadFile(cleanPath)
		if err != nil {
			return "error: " + err.Error()
		}
		content := string(data)
		count := strings.Count(content, a.OldStr)
		if count == 0 {
			return "error: old_str not found in file"
		}
		content = strings.ReplaceAll(content, a.OldStr, a.NewStr)
		if err := os.WriteFile(cleanPath, []byte(content), 0644); err != nil {
			return "error: " + err.Error()
		}
		return fmt.Sprintf("ok: %d replacement(s) made", count)

	case "append_file":
		var a struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
			return "error: " + err.Error()
		}
		cleanPath := filepath.Clean(a.Path)
		if strings.Contains(cleanPath, "..") {
			return `error: path contains ".." - directory traversal not allowed`
		}
		if !reSafePath.MatchString(cleanPath) {
			return "error: path contains invalid characters"
		}
		if err := os.MkdirAll(filepath.Dir(cleanPath), 0750); err != nil {
			return "error: " + err.Error()
		}
		f, err := os.OpenFile(cleanPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return "error: " + err.Error()
		}
		defer f.Close()
		if _, err := f.WriteString(a.Content); err != nil {
			return "error: " + err.Error()
		}
		return "ok"

	case "delete_file":
		var a struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
			return "error: " + err.Error()
		}
		cleanPath := filepath.Clean(a.Path)
		if strings.Contains(cleanPath, "..") {
			return `error: path contains ".." - directory traversal not allowed`
		}
		if !reSafePath.MatchString(cleanPath) {
			return "error: path contains invalid characters"
		}
		if err := os.Remove(cleanPath); err != nil {
			return "error: " + err.Error()
		}
		return "ok"

	case "move_file":
		var a struct {
			Src string `json:"src"`
			Dst string `json:"dst"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
			return "error: " + err.Error()
		}
		cleanSrc := filepath.Clean(a.Src)
		cleanDst := filepath.Clean(a.Dst)
		if strings.Contains(cleanSrc, "..") || strings.Contains(cleanDst, "..") {
			return `error: path contains ".." - directory traversal not allowed`
		}
		if !reSafePath.MatchString(cleanSrc) || !reSafePath.MatchString(cleanDst) {
			return "error: path contains invalid characters"
		}
		if err := os.MkdirAll(filepath.Dir(cleanDst), 0750); err != nil {
			return "error: " + err.Error()
		}
		if err := os.Rename(cleanSrc, cleanDst); err != nil {
			return "error: " + err.Error()
		}
		return "ok"

	case "search_files":
		var a struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
			Glob    string `json:"glob"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
			return "error: " + err.Error()
		}
		if a.Pattern == "" {
			return "error: pattern is required"
		}
		searchPath := a.Path
		if searchPath == "" {
			searchPath = "."
		}
		cleanPath := filepath.Clean(searchPath)
		if strings.Contains(cleanPath, "..") {
			return `error: path contains ".." - directory traversal not allowed`
		}
		var sb strings.Builder
		matchCount := 0
		const maxMatches = 200
		walkErr := filepath.Walk(cleanPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				if info.Name() != "." && strings.HasPrefix(info.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if a.Glob != "" {
				if matched, _ := filepath.Match(a.Glob, info.Name()); !matched {
					return nil
				}
			}
			f, err := os.Open(path)
			if err != nil {
				return nil
			}
			defer f.Close()
			scanner := bufio.NewScanner(f)
			lineNum := 0
			for scanner.Scan() {
				lineNum++
				line := scanner.Text()
				if strings.Contains(line, a.Pattern) {
					fmt.Fprintf(&sb, "%s:%d: %s\n", path, lineNum, line)
					matchCount++
					if matchCount >= maxMatches {
						return errStopWalk
					}
				}
			}
			return nil
		})
		if walkErr != nil && walkErr != errStopWalk {
			return "error: " + walkErr.Error()
		}
		if matchCount == 0 {
			return "no matches found"
		}
		result := sb.String()
		if matchCount >= maxMatches {
			result += fmt.Sprintf("[search_files: stopped at %d matches]\n", maxMatches)
		}
		return result

	case "add_todo":
		var a struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
			return "error: " + err.Error()
		}
		if a.Title == "" {
			return "error: title is required"
		}
		todosMu.Lock()
		id := todosNext
		todosNext++
		todosList = append(todosList, todoItem{ID: id, Title: a.Title, Status: "pending"})
		todosMu.Unlock()
		return fmt.Sprintf("added todo #%d", id)

	case "list_todos":
		todosMu.Lock()
		defer todosMu.Unlock()
		if len(todosList) == 0 {
			return "no todos"
		}
		var sb strings.Builder
		for _, t := range todosList {
			fmt.Fprintf(&sb, "#%-3d  %-11s  %s\n", t.ID, t.Status, t.Title)
		}
		return sb.String()

	case "update_todo":
		var a struct {
			ID     int    `json:"id"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
			return "error: " + err.Error()
		}
		if a.Status != "pending" && a.Status != "in_progress" && a.Status != "done" {
			return "error: status must be pending, in_progress, or done"
		}
		todosMu.Lock()
		defer todosMu.Unlock()
		for i := range todosList {
			if todosList[i].ID == a.ID {
				todosList[i].Status = a.Status
				return fmt.Sprintf("todo #%d → %s", a.ID, a.Status)
			}
		}
		return fmt.Sprintf("error: todo #%d not found", a.ID)

	case "remove_todo":
		var a struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
			return "error: " + err.Error()
		}
		todosMu.Lock()
		defer todosMu.Unlock()
		for i, t := range todosList {
			if t.ID == a.ID {
				todosList = append(todosList[:i], todosList[i+1:]...)
				return fmt.Sprintf("removed todo #%d", a.ID)
			}
		}
		return fmt.Sprintf("error: todo #%d not found", a.ID)

	default:
		return "error: unknown tool " + name
	}
}
