# Contributing to Vial

Thanks for your interest in making Vial better. Whether it's a bug report, a new alias rule, a matching tier improvement, or a full feature — contributions are welcome.

## Getting Started

### Prerequisites

- Go 1.22+
- Node.js 18+ (only for dashboard development)
- macOS or Linux

### Setup

```bash
git clone https://github.com/cheesejaguar/vial.git
cd vial
go mod tidy
make build-quick    # build without rebuilding the dashboard
make test           # run the test suite
```

If you're working on the dashboard:

```bash
cd web
npm install
npm run dev         # dev server with hot reload at localhost:5173
```

### Project Layout

```
cmd/vial/           → entry point
internal/
  vault/            → encryption, storage, CRUD (the security core)
  parser/           → .env file parsing and writing
  matcher/          → 5-tier matching engine
  alias/            → alias store, pattern rules, variant detection
  project/          → project registry for batch operations
  llm/              → LLM provider abstraction
  scanner/          → source code env var extraction
  dashboard/        → Go backend + embedded SPA
  keyring/          → OS keychain session caching
  config/           → YAML configuration
  sync/             → vault sync backends
  cli/              → all Cobra command definitions
web/                → SvelteKit dashboard frontend
vscode/             → VS Code extension scaffold
```

## How to Contribute

### Reporting Bugs

Open an issue with:
- What you expected to happen
- What actually happened
- Steps to reproduce
- Your OS and Go version (`go version`)
- Vial version (`vial version`)

### Suggesting Features

Open an issue describing the use case. Explain the *problem* you're solving, not just the solution you're imagining. The best feature requests start with "I was trying to..." or "Every time I..."

### Submitting Code

1. Fork the repo and create a branch from `main`
2. Write your code
3. Add or update tests for your changes
4. Run the full check: `make test && make vet`
5. Open a pull request

#### PR Guidelines

- **Keep it focused.** One logical change per PR. A bug fix and a new feature are two PRs.
- **Write tests.** We aim for 80%+ coverage on `vault`, `parser`, `matcher`, and `alias` packages. Table-driven tests are preferred.
- **Don't break the security model.** Changes to `internal/vault/` get extra scrutiny. Never store secrets in plaintext outside the vault file and `.env` outputs. Never accept secret values as CLI positional arguments.
- **Use fast KDF params in tests.** Call `vm.SetKDFParams(vault.TestKDFParams())` to avoid 64 MiB Argon2id derivations in tests.
- **Match the existing style.** No linter config to fight with — just be consistent with what's already there.

### Contributing Alias Rules

One of the easiest and most valuable contributions is improving the built-in variant detection in `internal/alias/autodetect.go`. If you've found a common env var naming pattern that Vial doesn't recognize, add it to `builtinVariantRules` with a test case.

Example: if you notice that `MAILGUN_API_KEY` and `MAILGUN_KEY` should be recognized as the same thing, that's already handled by the `_KEY` ↔ `_API_KEY` rule. But if there's a new pattern — add it and open a PR.

### Contributing Matchers

The matching engine is designed to be extensible. Each tier implements the `Matcher` interface:

```go
type Matcher interface {
    Match(requestedKey string, vaultKeys []string) ([]MatchResult, error)
    Tier() int
    Name() string
}
```

If you have an idea for a matching strategy that doesn't fit tiers 1–5, open an issue to discuss it before building.

### Contributing Scanner Patterns

The source code scanner in `internal/scanner/` supports JS, Python, Go, Ruby, Rust, and PHP. Adding a new language is straightforward — add a `langPattern` entry to the `languages` slice with the file extensions and regex patterns for env var access.

## Development

### Running Tests

```bash
make test               # run with -race
make test-verbose       # verbose output
make test-cover         # generate coverage report
```

### Building

```bash
make build-quick        # Go binary only (fast)
make build              # rebuild dashboard + Go binary (full)
make install            # install to $GOPATH/bin
```

### Generating Man Pages

```bash
make man                # outputs to man/
```

### Code Quality

```bash
make vet                # go vet
make lint               # golangci-lint (if installed)
```

## Architecture Decisions

A few things that are intentional and should stay that way:

- **Secrets never appear in CLI arguments.** Always stdin or prompts. This keeps them out of shell history and `ps` output.
- **Atomic vault writes.** Write to `.tmp`, then `os.Rename()`. No half-written vault files.
- **File locking.** `syscall.Flock` prevents concurrent vault corruption.
- **memguard for key material.** KEK and DEK live in `mlock`'d memory. The caller who receives a `*LockedBuffer` owns it and must call `Destroy()`.
- **Tier confidence ceiling.** LLM matches (Tier 5) are capped at 0.75 confidence so they never silently override deterministic matches.
- **Fail open on LLM errors.** If the LLM provider is down or returns garbage, Vial continues without it. It's an enhancement, not a dependency.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
