package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var toolDefs = []Tool{
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "read_file",
			Description: "Read the full contents of a file",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "File path to read"},
				},
				"required": []string{"path"},
			},
		},
	},
	{
		Type: "function",
		Function: ToolFunction{
			Name:        "write_file",
			Description: "Write content to a file, creating it and parent directories if needed",
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
			Description: "Run a shell command and return combined stdout+stderr",
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
			Description: "List files and subdirectories at a path",
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

func executeTool(name, argsJSON string) string {
	switch name {
	case "read_file":
		var a struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
			return "error: " + err.Error()
		}
		data, err := os.ReadFile(a.Path)
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
		if err := os.MkdirAll(filepath.Dir(a.Path), 0755); err != nil {
			return "error: " + err.Error()
		}
		if err := os.WriteFile(a.Path, []byte(a.Content), 0644); err != nil {
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
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "sh", "-c", a.Command)
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		cmd.Run()
		out := buf.String()
		if len(out) > 50000 {
			out = out[:50000] + "\n... (truncated)"
		}
		return out

	case "list_directory":
		var a struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal([]byte(argsJSON), &a); err != nil {
			return "error: " + err.Error()
		}
		entries, err := os.ReadDir(a.Path)
		if err != nil {
			return "error: " + err.Error()
		}
		var sb strings.Builder
		for _, e := range entries {
			if e.IsDir() {
				fmt.Fprintf(&sb, "%s/\n", e.Name())
			} else {
				info, _ := e.Info()
				if info != nil {
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
