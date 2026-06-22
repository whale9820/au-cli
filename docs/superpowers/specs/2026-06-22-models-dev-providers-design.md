# models.dev Provider Catalog Design

## Goal

Replace the hardcoded provider catalog with a live catalog from `https://models.dev/api.json`, while preserving startup reliability through built-in fallback providers. Add manageable custom providers that can reuse model metadata from `models.dev` unless the user overrides it.

## Architecture

Add a focused `catalog.go` file that owns provider catalog loading, parsing, and lookup. The app will keep a small built-in fallback provider list for offline/error cases. Runtime commands will receive a catalog object instead of reading a global hardcoded provider list.

`Store` will persist user custom providers and optional model metadata overrides. Custom providers are appended to the runtime catalog and marked as `Custom`.

## Components

- `catalog.go`
  - Defines `ModelSpec`, `Provider`, `ProviderCatalog`, and `models.dev` JSON structs.
  - Fetches `https://models.dev/api.json` with a short timeout and user agent.
  - Converts providers with string `api` fields into OpenAI-compatible providers.
  - Exposes lookup helpers for provider names, model metadata, and merged custom providers.
- `config.go`
  - Keeps config loading and fallback default selection.
  - Removes the large hardcoded provider table from config loading.
- `store.go`
  - Persists `CustomProviders []Provider` in `~/.config/au/config.json`.
  - Initializes nil maps/slices safely.
- `main.go`
  - Loads the catalog once at startup.
  - Uses the catalog in `/connect`, `/providers`, `/use`, and `/models` flows.
  - Adds `/custom` for list/add/edit/remove custom providers.

## Custom Provider Flow

`/custom` opens a prompt-based manager:

- `list`: display stored custom providers and their base URLs/models.
- `add`: ask for name, base URL, API key, model, optional context/output/tool/reasoning/vision overrides.
- `edit`: select a custom provider and update fields with blank-to-keep prompts.
- `remove`: select and delete a custom provider.

When adding or editing, if the user enters a model that exists in `models.dev`, the app preloads context length, output length, and capabilities. If the user provides override values, custom values win.

## Data Flow

Startup:

1. Load `Store`.
2. Load `ProviderCatalog` from `models.dev`.
3. If fetch or parse fails, use `fallbackProviders`.
4. Append `Store.CustomProviders` to the catalog.
5. Load `Config` using stored/environment config and fallback default provider URL.

Provider commands:

- `/providers` lists live/fallback providers plus custom providers.
- `/connect` lists providers and fetches `/models` from the selected base URL.
- `/use <provider>` switches provider by exact or partial name.
- `/use custom` forwards to custom provider add/use flow.

## Error Handling

- Network failures loading `models.dev` never block startup.
- Catalog status is shown only when useful: `/providers` can note fallback mode.
- Providers with missing or non-string `api` fields are skipped because this client only speaks OpenAI-compatible HTTP endpoints.
- Custom provider validation requires a name, base URL, and model for add.
- API keys remain stored as currently implemented.

## Testing

Add unit tests for:

- Parsing a small `models.dev` fixture into providers.
- Skipping providers with null/non-string API values.
- Looking up model metadata by provider/model.
- Merging custom providers after catalog providers.
- Applying custom model overrides over catalog metadata.

Run:

```bash
go test ./...
go vet ./...
go build -o au .
```

## Release

Bump to `v0.3.8-alpha`, build all five release binaries in `dist/`, update README release URLs, commit, tag, push `main --tags`, and create the GitHub release with all binaries attached.
