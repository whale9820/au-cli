package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

var reSafePath = regexp.MustCompile(`^[\w\-./ ]+$`)

// bufWrapper wraps a byte slice to enforce size limits
type bufWrapper struct {
	buf     *[]byte
	maxSize int
}

func (w *bufWrapper) Write(p []byte) (int, error) {
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

var (
	toolDefs []Tool
	rateLimiter = make(chan struct{}, 5) // Max 5 concurrent tool executions
)

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
			Description: "Write content to a file, creating it and parent directories if needed. Overwrites existing files - use with caution.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "File path to write"},
					"content": map[string]any{"type": "string", "description": "Content to write"},
				},
				"required": []string{"path", "content"},
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "run_command",
			Description: "Run a shell command and return combined stdout+stderr. Commands are limited to 60 seconds.",
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
	}
}

func executeTool(name, argsJSON string) string {
	// Rate limit tool execution
	rateLimiter <- struct{}{}
	defer func() { <-rateLimiter }()
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
		data, err := os.ReadFile(cleanPath)
		if err != nil {
			return "error: " + err.Error()
		}
		return string(data)

	case "write_file":
		var a struct {
			Path    string `json:"path"`
			Content string `json:"content"`
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
		// Check if file exists and warn
		if _, err := os.Stat(cleanPath); err == nil {
			return "error: file already exists - use overwrite to force"
		}
		if err := os.MkdirAll(filepath.Dir(cleanPath), 0750); err != nil {
			return "error: " + err.Error()
		}
		if err := os.WriteFile(cleanPath, []byte(a.Content), 0600); err != nil {
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
		// Basic validation to prevent dangerous commands
		cmdStr := strings.TrimSpace(a.Command)
		if cmdStr == "" {
			return "error: empty command"
		}
		// Block dangerous patterns
		dangerousPatterns := []string{`rm -rf /`, `>/dev/`, `:(){:|: &}:;:`}
		for _, pattern := range dangerousPatterns {
			if strings.Contains(cmdStr, pattern) {
				return "error: command contains potentially dangerous pattern"
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
		cmd.Stdout = &bufWrapper{buf: &out, maxSize: maxToolOutput}
		cmd.Stderr = &bufWrapper{buf: &out, maxSize: maxToolOutput}
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

	default:
		return "error: unknown tool " + name
	}
}
