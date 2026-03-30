# au

> **Alpha software.** Expect bugs, broken provider endpoints, and rough edges. Use at your own risk.

A minimal AI coding agent for the terminal, built to run on hardware that nothing else will.

## Why

The terminal AI coding CLI space has a Node.js problem. OpenCode freezes. Claude Code will OOM a small server instantly. Crush is slow to load. Pi is the smallest of the bunch but still slow to start and still requires npm — same as all the others. They all ship as Node.js apps, which means a runtime, a `node_modules` folder, and a startup time measured in seconds before you can type anything.

`au` is a single statically-linked Go binary. No Node.js. No npm. No runtime. It starts in under 50ms, uses ~8 MB of RAM at idle, and gives you a full agentic coding loop — read files, write files, run commands, iterate — on the smallest VPS you can rent. It connects to any OpenAI-compatible API, so you pick the model and the cost.

## Install (Linux)

One-liner — downloads the binary, makes it executable, moves it to your PATH:

```sh
curl -fsSL https://github.com/cfpy67/au-cli/releases/latest/download/au-linux-amd64 -o au && chmod +x au && sudo mv au /usr/local/bin/au
```

For ARM64 (Raspberry Pi, Ampere VPS):

```sh
curl -fsSL https://github.com/cfpy67/au-cli/releases/latest/download/au-linux-arm64 -o au && chmod +x au && sudo mv au /usr/local/bin/au
```

Then just run:

```sh
au
```

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
| `/use <n>` | Switch to provider number `n` from the list |
| `/key <k>` | Set API key for current provider |
| `/model <m>` | Switch to model `m` |
| `/models` | List available models from the current provider |
| `/providers` | List all built-in providers |
| `/thinking <n>` | Set reasoning effort 0–10 (0 = off) |
| `/reset` | Clear conversation history |
| `/exit` `/quit` `/q` | Exit |

Ctrl+C also exits cleanly.

## Features

- Streams responses with markdown→ANSI rendering (bold, inline code, code blocks with line numbers, tables, headings, bullets)
- Full filesystem access via tool calls: read files, write files, run shell commands, list directories
- Agentic loop — the model keeps calling tools until the task is done
- 40+ preconfigured providers (OpenAI, Z.AI, Groq, Together, Fireworks, Mistral, Cloudflare, Azure, and more)
- Persistent config at `~/.config/au/config.json` (plaintext JSON)
- Pinned status bar showing current model and thinking intensity
- Command autocomplete with Tab, Up/Down history navigation
- Thinking intensity control (0–10) for models that support `reasoning_effort`
- Single static binary, ~9 MB, ~8 MB RAM at idle
- Zero external dependencies except `golang.org/x/term`

## Config

Stored at `~/.config/au/config.json` (or `~/Library/Application Support/au/config.json` on macOS):

```json
{
  "base_url": "https://api.openai.com/v1",
  "api_key": "sk-...",
  "model": "gpt-4o",
  "thinking": 0
}
```

## Tools

The agent has access to four tools:

- `read_file` — read any file
- `write_file` — write or overwrite any file (creates parent directories)
- `run_command` — run any shell command via `sh -c` (60s timeout)
- `list_directory` — list directory contents with sizes

## Providers

Run `/providers` inside `au` for the full list. Highlights:

- OpenAI, Z.AI, Groq, Together AI, Fireworks, Mistral
- Cloudflare Workers AI (requires account ID)
- Azure OpenAI (requires endpoint + deployment)
- OpenRouter, DeepInfra, Perplexity, Cohere, Replicate, and more

## License

MIT
