# AGENTS.md

## Project Overview

**au** is a minimal Go CLI chat client for OpenAI-compatible APIs. It provides a TUI (terminal UI) with streaming responses, tool-use support (file read/write, shell commands, directory listing), and provider management for 30+ LLM providers. The project is in alpha.

- **Language**: Go 1.22
- **Module**: `au` (not `github.com/...`)
- **Dependencies**: `golang.org/x/term`, `golang.org/x/sys`
- **No external test framework**: No test files exist currently

## Commands

```bash
go build -o au .          # Build the binary
go run .                  # Run directly
go vet ./...              # Static analysis
```

There are no Makefiles, CI configs, linters, or test suites configured.

## File Structure

| File | Purpose |
|------|---------|
| `main.go` | Entry point, REPL loop, slash commands, markdown rendering, tool call display, system prompt |
| `api.go` | OpenAI-compatible chat completions (streaming SSE), model listing, API types (`Message`, `ToolCallMsg`, `Tool`, `chatRequest`, `streamChunk`) |
| `config.go` | `Provider` registry (30+ providers), `Config` struct, config loading from env vars + store |
| `store.go` | Persistent JSON config at `~/.config/au/config.json` (or platform equivalent). `Store` struct with URL template variable resolution |
| `tools.go` | Tool definitions (`read_file`, `write_file`, `run_command`, `list_directory`) and `executeTool()` dispatch |
| `tui.go` | Raw terminal TUI: line editing, history, command autocomplete popup, status bar |
| `vt_windows.go` | Windows virtual terminal (ANSI escape code) enablement — build tag `windows` |
| `vt_other.go` | No-op `enableVT()` on non-Windows — build tag `!windows` |

## Code Conventions

- **Package**: Everything is `package main` — single-binary project with no internal packages
- **Naming**: Short names preferred (`ri`, `st`, `cfg`, `tc`, `buf`, `out`). Functions use camelCase. No godoc comments.
- **Error handling**: Errors returned to callers; bare `json.Unmarshal` ignores errors when data is from trusted sources (e.g., API responses). User-facing errors printed to stderr with ANSI red.
- **No struct methods on core types**: `executeTool` is a standalone function, not a method. `Store.save()` and `Store.resolve()` are the exceptions.
- **ANSI escape codes**: Used directly throughout (not via a library). Colors: `\033[1m` bold, `\033[2m` dim, `\033[31m` red, `\033[32m` green, `\033[33m` yellow, `\033[36m` cyan, `\033[7m` reverse, `\033[0m` reset.
- **Build tags**: Platform-specific files use `//go:build windows` and `//go:build !windows`

## Architecture Notes

- **Tool-use loop**: `main()` runs a `for` loop calling `complete()` (streaming SSE) → collects tool calls → executes via `executeTool()` → appends tool results → loops until no more tool calls
- **Streaming**: The `complete()` function reads SSE line-by-line with a 4MB scanner buffer, assembles tool call deltas by index, and calls `onToken` / `onFirstToken` callbacks
- **Config precedence**: Environment variables (`AU_BASE_URL`, `AU_API_KEY`, `AU_MODEL`) → stored config → defaults. Falls back to `OPENAI_API_KEY` if `AU_API_KEY` is unset
- **URL template variables**: Provider URLs can contain `{placeholder}` tokens (e.g., `{ACCOUNT_ID}`) resolved from `Store.Vars`
- **Thinking/reasoning**: Mapped to `reasoning_effort` field: 0=off, 1-3=low, 4-7=medium, 8-10=high
- **TUI**: Uses `golang.org/x/term` for raw mode. Status bar pinned to bottom row via ANSI scroll region. Command autocomplete on `/` prefix with arrow-key selection.

## Slash Commands

`/connect`, `/use <provider>`, `/use custom`, `/key [value]`, `/model <id>`, `/models`, `/providers`, `/thinking <0-10>`, `/reset`, `/q` `/quit` `/exit`

## Gotchas

- Module name is just `au`, not a full path — imports like `"golang.org/x/term"` work because they're external, but any future internal packages would need the `au` prefix
- No tests exist — adding `_test.go` files is safe but no test runner is set up
- The binary name `au` is in `.gitignore` — it's a build artifact
- Windows requires `enableVT()` for ANSI codes to render; this is handled at startup in `main()`
- Tool command execution uses `powershell.exe` on Windows and `sh -c` on other platforms (in `tools.go:117-120`)
- `run_command` has a 60-second timeout and 50KB output truncation
- The `complete()` function uses `http.DefaultClient` with no custom timeout
