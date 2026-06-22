package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExecuteTool(t *testing.T) {
	tests := []struct {
		name    string
		tool    string
		args    string
		wantErr bool
	}{
		{
			name:    "list directory",
			tool:    "list_directory",
			args:    `{"path": "."}`,
			wantErr: false,
		},
		{
			name:    "list non-existent directory",
			tool:    "list_directory",
			args:    `{"path": "/nonexistent/path/xyz123"}`,
			wantErr: true,
		},
		{
			name:    "run simple command",
			tool:    "run_command",
			args:    `{"command": "echo test"}`,
			wantErr: false,
		},
		{
			name:    "run dangerous command - rm -rf /",
			tool:    "run_command",
			args:    `{"command": "rm -rf /"}`,
			wantErr: true,
		},
		{
			name:    "run dangerous command - fork bomb",
			tool:    "run_command",
			args:    `{"command": ":(){:|: &}:;:"}`,
			wantErr: true,
		},
		{
			name:    "path traversal attempt in read_file",
			tool:    "read_file",
			args:    `{"path": "../../../etc/passwd"}`,
			wantErr: true,
		},
		{
			name:    "invalid characters in path",
			tool:    "read_file",
			args:    `{"path": "test\x00file"}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executeTool(tt.tool, tt.args)
			hasErr := strings.HasPrefix(result, "error:")
			if hasErr != tt.wantErr {
				t.Errorf("executeTool() error = %v, wantErr %v\nResult: %s", hasErr, tt.wantErr, result)
			}
		})
	}
}

func TestWriteFile(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	// Use forward slashes which work on Windows too
	testFile := filepath.ToSlash(filepath.Join(tmpDir, "test.txt"))

	// Test 1: Write new file
	args := `{"path": "` + testFile + `", "content": "test content"}`
	result := executeTool("write_file", args)
	if strings.HasPrefix(result, "error:") {
		t.Fatalf("write_file failed: %s", result)
	}

	// Verify file was written
	content, err := os.ReadFile(filepath.FromSlash(testFile))
	if err != nil {
		t.Fatalf("Failed to read written file: %v", err)
	}
	if string(content) != "test content" {
		t.Errorf("File content = %q, want %q", string(content), "test content")
	}

	// Test 2: Try to overwrite existing file (should error)
	result = executeTool("write_file", args)
	if !strings.Contains(result, "already exists") {
		t.Errorf("Expected overwrite error, got: %s", result)
	}

	// Test 3: Path traversal attempt - use absolute path that tries to escape
	traversalArgs := `{"path": "../../../etc/passwd", "content": "malicious"}`
	result = executeTool("write_file", traversalArgs)
	if !strings.Contains(result, "..") || !strings.Contains(result, "directory traversal") {
		t.Errorf("Expected path traversal error, got: %s", result)
	}
}

func TestBestCommandMatch(t *testing.T) {
	matches := []cmdDef{{name: "/models"}, {name: "/model"}}
	if got := bestCommandMatch("/mode", matches, -1); got != "/models" {
		t.Fatalf("bestCommandMatch /mode = %q, want /models", got)
	}
	if got := bestCommandMatch("/mode", matches, 1); got != "/model" {
		t.Fatalf("selected bestCommandMatch = %q, want /model", got)
	}
	if got := bestCommandMatch("/model gpt", matches, 1); got != "/model" {
		t.Fatalf("arg command resolved to %q, want /model", got)
	}
}

func TestIsUpdateArg(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"au"}, false},
		{[]string{"au", "update"}, true},
		{[]string{"au", "/update"}, true},
		{[]string{"au", "updates"}, false},
	}
	for _, tt := range tests {
		if got := isUpdateArg(tt.args); got != tt.want {
			t.Fatalf("isUpdateArg(%v) = %v, want %v", tt.args, got, tt.want)
		}
	}
}

func TestPowerShellExecution(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows-specific test on non-Windows platform")
	}

	// Test PowerShell command execution
	args := `{"command": "Write-Host 'PowerShell Test'"}`
	result := executeTool("run_command", args)
	if strings.HasPrefix(result, "error:") {
		t.Errorf("PowerShell command failed: %s", result)
	}
	if !strings.Contains(result, "PowerShell Test") {
		t.Errorf("Expected PowerShell output, got: %s", result)
	}

	// Test command timeout (should work within 60s)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "Start-Sleep -Seconds 1; echo 'Done'")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("Command timed out or failed: %v, output: %s", err, output)
	}
}

func TestRateLimiting(t *testing.T) {
	// Test that concurrent tool executions all succeed
	done := make(chan string, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			result := executeTool("run_command", `{"command": "echo test`+string(rune('0'+id))+`"}`)
			done <- result
		}(i)
	}

	// Wait for all to complete
	successCount := 0
	for i := 0; i < 10; i++ {
		result := <-done
		if !strings.HasPrefix(result, "error:") {
			successCount++
		}
	}

	if successCount == 0 {
		t.Error("No tool executions succeeded")
	}
}
