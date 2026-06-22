# models.dev Providers Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fetch providers from `https://models.dev/api.json`, add custom provider management, and release `v0.3.8-alpha`.

**Architecture:** Add `catalog.go` for live catalog loading/parsing and model metadata. Persist custom providers in `Store`, merge them into the runtime catalog, and thread the catalog through provider commands.

**Tech Stack:** Go 1.22, standard library HTTP/JSON, existing raw terminal prompt flows.

---

### Task 1: Catalog types and parser

**Files:**
- Create: `catalog.go`
- Create: `catalog_test.go`

- [ ] Add `ModelSpec`, `Provider`, `ProviderCatalog`, fallback provider list, and parser for `models.dev`.
- [ ] Test parsing providers with string `api`, skipping null API values, model metadata extraction, and custom override merging.
- [ ] Run `go test ./...`.

### Task 2: Store custom providers

**Files:**
- Modify: `store.go`
- Modify: `config.go`

- [ ] Add `CustomProviders []Provider` to `Store`.
- [ ] Initialize nil custom provider slices in `loadStore`.
- [ ] Update `loadConfig` to use fallback default provider URL.
- [ ] Run `go test ./...`.

### Task 3: Wire catalog into commands

**Files:**
- Modify: `main.go`
- Modify: `tui.go`

- [ ] Change `findProvider` to search a `ProviderCatalog`.
- [ ] Change `connectFlow` and `useCmd` to receive the catalog.
- [ ] Change `/providers` to list catalog providers and fallback status.
- [ ] Add `/custom` to autocomplete/help.
- [ ] Run `go test ./...`.

### Task 4: Custom provider manager

**Files:**
- Modify: `main.go`

- [ ] Add `/custom` list/add/edit/remove flow.
- [ ] Add model metadata prompts that prefill from `models.dev` when available.
- [ ] Ensure custom values override catalog metadata.
- [ ] Run `go test ./...` and `go vet ./...`.

### Task 5: Release

**Files:**
- Modify: `update.go`
- Modify: `README.md`
- Create/overwrite: `dist/au-*`

- [ ] Bump version to `v0.3.8-alpha`.
- [ ] Update all README release URLs to `v0.3.8-alpha`.
- [ ] Build all five platform binaries.
- [ ] Run `go test ./...`, `go vet ./...`, and `go build -o au .`.
- [ ] Commit, tag, push `main --tags`.
- [ ] Create GitHub release with `dist/au-*` assets.
