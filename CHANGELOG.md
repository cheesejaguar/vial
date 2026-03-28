# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-03-27

### Added

#### Phase 5 — Vibecoding Ecosystem (Research-Driven Improvements)
- **MCP Server** — Model Context Protocol server for AI coding tools (Claude Code, Cursor); `vial mcp` with read-only and read-write modes
- **Secret Leak Prevention** — Git pre-commit hook scanning staged files for vault secrets; `vial hook install/uninstall/check` with `.vialignore` support
- **Secret Health Scoring** — Configurable rotation policies per key with CLI health report and enhanced dashboard; `vial health --set-rotation KEY=DAYS`
- **Auto-Scaffold** — Source code scanner generates `.env.example` from env var references; `vial scaffold` supports JS, TS, Python, Go, Ruby, Rust, PHP
- **Zero-Config Setup** — One-command project onboarding: scan, scaffold, register, pour, hook; `vial setup`
- **Export Formats** — Docker env-file, Kubernetes Secret YAML, GitHub Actions, shell export formats; `vial export --format=<format> --keys=<glob>`
- **External Importers** — Import secrets from 1Password, Doppler, Vercel, and JSON; `vial distill --from=<source>`
- **Secret Sharing** — Encrypted, time-limited bundles with per-bundle passphrase; `vial share` / `vial share receive`
- **Audit Log** — JSONL audit trail for all vault operations with CLI and dashboard views; `vial audit`
- **GitHub Actions** — Reusable composite action for CI/CD secret injection; `action.yaml`

#### Phase 1 — Core MVP
- Encrypted vault with Argon2id KDF and AES-256-GCM per-value encryption
- `vial init` — create a new vault with master password
- `vial key set/get/list/rm` — secret CRUD (values always via stdin, never CLI args)
- `vial pour` — populate `.env` from vault using `.env.example` as template
- `vial cork` / `vial uncork` — lock and unlock the vault
- OS keychain session caching with configurable TTL
- Shell completion generation (bash, zsh, fish)
- GoReleaser config + GitHub Actions CI/CD

#### Phase 2 — Smart Matching
- Full `.env` parser: escape sequences, multi-line values, variable interpolation (`${VAR:-default}`)
- Alias system with user-defined aliases, regex pattern rules, and auto-detected variants
- Tier 2 matching: case-insensitive with framework prefix stripping (`NEXT_PUBLIC_`, `VITE_`, etc.)
- Tier 3 matching: alias-based and variant-based matching
- `vial distill` — import keys from existing `.env` files
- `vial shelf add/list/rm` — project registry for batch operations
- `vial label set/list/rm` — alias and tag management
- `vial diff` — compare `.env.example` vs vault
- `vial pour --all` — batch pour all registered projects

#### Phase 3 — Intelligence Layer
- LLM provider abstraction (OpenAI, Anthropic, OpenRouter, configurable)
- Tier 4 matching: comment-informed keyword matching
- Tier 5 matching: LLM-assisted with confidence ceiling (0.75) and hallucination rejection
- Source code scanner for env var references (JS, Python, Go, Ruby, Rust, PHP)
- `vial brew` — run commands with secrets injected as environment variables
- `vial dashboard` — local web dashboard (Svelte SPA embedded in binary)

#### Phase 4 — Ecosystem
- CI/CD headless mode via `VIAL_MASTER_KEY` environment variable
- Vault sync with filesystem and git backends
- `vial sync push/pull/status` — sync vault to/from remote locations
- Man page generation from Cobra command tree
- VS Code extension scaffold
