# au

> **Alpha software.** Expect bugs, broken provider endpoints, and rough edges. Use at your own risk.

A minimal AI coding agent for the terminal, built to run on hardware that nothing else will.
CLEARLY NOT FINISHED!!! I'll accept any PR that looks right and keeps it small.

## Why

The terminal AI coding CLI space has a Node.js problem. OpenCode freezes. Claude Code will OOM a small server instantly. Crush is slow to load. Pi is the smallest of the bunch but still slow to start and still requires npm — same as all the others. They all ship as Node.js apps, which means a runtime, a `node_modules` folder, and a startup time measured in seconds before you can type anything.

`au` is a single statically-linked Go binary. No Node.js. No npm. No runtime. It starts in under 50ms, uses ~8 MB of RAM at idle, and gives you a full agentic coding loop — read files, write files, run commands, iterate — on the smallest VPS you can rent. It connects to any OpenAI-compatible API, so you pick the model and the cost.

## Install

### Linux

One-liner — downloads the binary, makes it executable, moves it to your PATH:

```sh
curl -fsSL https://github.com/cfpy67/au-cli/releases/download/v0.3.6-alpha/au-linux-amd64 -o au && chmod +x au && sudo mkdir -p /usr/local/bin && sudo mv au /usr/local/bin/au
```

For ARM64 (Raspberry Pi, Ampere VPS):

```sh
curl -fsSL https://github.com/cfpy67/au-cli/releases/download/v0.3.6-alpha/au-linux-arm64 -o au && chmod +x au && sudo mkdir -p /usr/local/bin && sudo mv au /usr/local/bin/au
```

Then just run `au`.

### macOS

Intel:

```sh
curl -fsSL https://github.com/cfpy67/au-cli/releases/download/v0.3.6-alpha/au-darwin-amd64 -o au && chmod +x au && sudo mkdir -p /usr/local/bin && sudo mv au /usr/local/bin/au
```

Apple Silicon (M1/M2/M3):

```sh
curl -fsSL https://github.com/cfpy67/au-cli/releases/download/v0.3.6-alpha/au-darwin-arm64 -o au && chmod +x au && sudo mkdir -p /usr/local/bin && sudo mv au /usr/local/bin/au
```

Then just run `au`.

### Windows

Download [`au-windows-amd64.exe`](https://github.com/cfpy67/au-cli/releases/download/v0.3.6-alpha/au-windows-amd64.exe), rename it to `au.exe`, and place it somewhere on your `PATH` (e.g. `C:\Windows\System32` or any folder in your user PATH).

Then open PowerShell or Windows Terminal and run:

```powershell
au
```

ANSI colors are enabled automatically. Shell commands run via `powershell.exe -NoProfile -NonInteractive`.

## Build from source

Requires Go 1.22+:

```sh
git clone https://github.com/cfpy67/au-cli
cd au-cli
go build -o au .
```

## Usage

On first run, use `/connect` to pick a provider and model.

## Commands

| Command | Description |
|---|---|
| `/connect` | Interactive provider + model setup wizard |
| `/use <name>` | Switch provider by name — `/use custom` for a manual endpoint |
| `/key [value]` | Set API key for current provider |
| `/model <id>` | Switch to model by ID |
| `/models` | List available models from the current provider |
| `/providers` | List all built-in providers |
| `/thinking <n>` | Set reasoning effort 0–10 (0 = off) |
| `/update` | Check for a new release and self-update |
| `/reset` | Clear conversation history |
| `/help` | Show available commands |
| `/exit` `/quit` `/q` | Exit |

Ctrl+C also exits cleanly.

## Features

- Streams responses with markdown→ANSI rendering (bold, inline code, code blocks with line numbers, tables, headings, bullets)
- Full filesystem access via tool calls: read files, write files, run shell commands, list directories
- Agentic loop — the model keeps calling tools until the task is done
- 40+ preconfigured providers (OpenAI, Z.AI, Groq, Together, Fireworks, Mistral, Cloudflare, Azure, and more)
- Persistent config at `~/.config/au/config.json` (plaintext JSON, permissions 0600)
- Pinned status bar showing current model and thinking intensity
- Command autocomplete with Tab, full cursor movement (arrows, Home/End, Ctrl+A/E/K/U/W, Alt+B/F), Up/Down history navigation, persistent history across sessions
- Thinking intensity control (0–10) for models that support `reasoning_effort`
- Self-update: `/update` checks GitHub releases and replaces the binary in-place, then relaunches
- Single static binary, ~9 MB, ~8 MB RAM at idle
- Windows support: PowerShell for `run_command`, ANSI VT processing via `golang.org/x/sys/windows`
- HTTP client with 60s timeout, connection pooling, and retry with exponential backoff
- API key redacted from error messages

## Config

Stored at `~/.config/au/config.json` (or `~/Library/Application Support/au/config.json` on macOS, `%AppData%\au\config.json` on Windows):

```json
{
  "base_url": "https://api.openai.com/v1",
  "api_key": "sk-...",
  "model": "gpt-4o",
  "thinking": 0
}
```

The config file is written with `0600` permissions (owner read/write only).

## Tools

The agent has access to four tools:

- `read_file` — read any file (path traversal blocked)
- `write_file` — write a file; set `overwrite: true` to replace an existing file (creates parent directories)
- `run_command` — run a shell command via `sh -c` on Unix or `powershell.exe` on Windows (60s timeout, 50 KB output cap)
- `list_directory` — list directory contents with sizes

## Providers

Run `/providers` inside `au` for the full list. Highlights:

- OpenAI, Z.AI, Groq, Together AI, Fireworks, Mistral
- Cloudflare Workers AI (requires account ID)
- Azure OpenAI (requires endpoint + deployment)
- OpenRouter, DeepInfra, Perplexity, Cohere, Replicate, and more

## License

MIT
