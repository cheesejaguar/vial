# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
make build-quick          # build Go binary only (fast, uses cached dashboard)
make build                # rebuild dashboard SPA + Go binary (full)
make test                 # go test -race on all testable packages
make test-verbose         # verbose test output
make test-cover           # generate coverage report (coverage.html)
make vet                  # go vet ./...
make lint                 # golangci-lint (config in .golangci.yml)
make dashboard            # rebuild Svelte SPA + copy to embed dir
make man                  # generate man pages from Cobra commands
```

Run a single test:
```bash
go test -race -run TestVaultFullLifecycle ./internal/vault/...
```

The test target explicitly lists packages with test files to avoid the Go 1.25+ `covdata` tool error on packages without tests (cli, dashboard, keyring).

## Architecture

### Encryption Model

```
Master Password → Argon2id (64MiB, 3 iter) → KEK → encrypts DEK → DEK encrypts each secret value via AES-256-GCM
```

All key material uses `memguard.LockedBuffer` (mlock'd memory). The caller who receives a `*LockedBuffer` owns it and must call `Destroy()`. The DEK is owned by `VaultManager` and destroyed in `Lock()`.

Use `vault.TestKDFParams()` in tests — it uses 1 MiB / 1 iteration instead of the production 64 MiB / 3 iterations.

### Vault File (SOPS-style)

Key names are plaintext JSON map keys; values are individually AES-256-GCM encrypted with per-value random nonces. This allows readable diffs while keeping values secret. The file lives at `~/.local/share/vial/vault.json` with 0600 permissions. Writes are atomic (temp file + `os.Rename`) with `syscall.Flock` for concurrency.

### 5-Tier Matching Engine

The `matcher.Chain` runs matchers in tier order, stopping at the first result with confidence >= 0.9:

1. **Exact** — case-sensitive string match (confidence 1.0)
2. **Normalize** — case-insensitive + framework prefix stripping (`NEXT_PUBLIC_`, `VITE_`, `REACT_APP_`, etc.)
3. **Alias** — user-defined aliases, regex patterns, auto-detected variants (`_KEY` ↔ `_API_KEY` ↔ `_SECRET_KEY`)
4. **Comment** — keyword extraction from `.env.example` comments matched against service name vocabulary
5. **LLM** — calls an inference API; confidence capped at 0.75 to never outrank deterministic tiers; fails open on errors

Each tier implements `matcher.Matcher` interface: `Match(requestedKey, vaultKeys) → []MatchResult`.

### CLI Command Pattern

Commands use Cobra. Shared state flows through helpers in `cli/helpers.go`:

- `requireUnlockedVault()` → tries keyring session cache → tries `VIAL_MASTER_KEY` env var → prompts interactively
- `loadConfig()` → Viper-based YAML from `~/.config/vial/config.yaml`
- `isInteractive()` → checks `term.IsTerminal(stdin)`

Alchemical names are primary (`pour`, `cork`, `uncork`, `brew`, `distill`, `shelf`, `label`), with standard aliases (`lock`, `unlock`, `run`, `import`, `project`, `alias`).

Styled output uses helpers from `cli/styles.go` (purple/gold theme via lipgloss). Use `successIcon()`, `errorIcon()`, `keyName()`, `mutedText()` etc. rather than raw fmt.

### Dashboard

The Svelte SPA (`web/`) builds to static files, copied into `internal/dashboard/static/`, and embedded via `//go:embed static`. The Go server (`dashboard/server.go`) serves the SPA with correct MIME types and falls back to `index.html` for client-side routing. API routes under `/api/` are protected by a Bearer token generated at startup. The token is passed to the browser via URL fragment (never logged).

To iterate on the dashboard: `cd web && npm run dev` (proxies API to localhost:9876).

### MCP Server

`internal/mcp/` implements JSON-RPC 2.0 over stdin/stdout for the Model Context Protocol. Tools are defined in `mcp/tools.go`. Read-only by default; `--allow-writes` enables mutations.

### Security Invariants

- Secret values are **never** accepted as CLI positional arguments — always stdin/prompt
- Vault writes are atomic (temp file + rename) with file locking
- AES-GCM nonces always from `crypto/rand`, fresh per encryption
- Dashboard binds to `127.0.0.1` only, with CORS and Host header validation
- Auth token comparison uses `crypto/subtle.ConstantTimeCompare`
- LLM tier rejects hallucinated key names (verifies match exists in vault)
