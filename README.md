# au

A minimal AI coding agent for the terminal.

## Features

- Streams responses with markdown→ANSI rendering (bold, inline code, code blocks with line numbers, tables, headings, bullets)
- Full filesystem access via tool calls: read files, write files, run shell commands, list directories
- Agentic loop — the model keeps calling tools until the task is done
- 40+ preconfigured providers (OpenAI, Anthropic via proxy, Z.AI, Groq, Together, Fireworks, Mistral, Cloudflare, Azure, and more)
- Persistent config at `~/.config/au/config.json` (plaintext JSON, no encryption)
- Pinned status bar showing current model and thinking intensity
- Command autocomplete overlay with Tab, Up/Down history navigation
- Thinking intensity control (0–10) for models that support `reasoning_effort`
- Zero external dependencies except `golang.org/x/term`

## Install

```
git clone https://github.com/jellylarper/au-cli
cd au-cli
go build -o au .
```

Move the binary somewhere on your `$PATH`:

```
mv au /usr/local/bin/au
```

Requires Go 1.22+.

## Usage

```
au
```

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

## Config

Stored at `~/.config/au/config.json` on Linux/macOS (or `~/Library/Application Support/au/config.json` on macOS):

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
