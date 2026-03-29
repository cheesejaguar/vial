<p align="center">
  <br />
  <img src="https://img.shields.io/badge/built_with-Go-00ADD8?style=flat-square&logo=go" alt="Go" />
  <img src="https://img.shields.io/github/license/cheesejaguar/vial?style=flat-square&color=6B46C1" alt="MIT License" />
  <img src="https://img.shields.io/badge/platform-macOS%20%7C%20Linux-D69E2E?style=flat-square" alt="Platform" />
</p>

<h1 align="center">🧪 Vial</h1>

<p align="center">
  <strong>The centralized secret vault for vibe coders.</strong><br />
  <em>Store your API keys once. Pour them everywhere.</em>
</p>

<p align="center">
  <a href="https://one-vial.org">one-vial.org</a>
</p>

---

## The Problem

You're vibe coding. You've spun up 30 projects this month. Every single one needs some combination of `OPENAI_API_KEY`, `STRIPE_SECRET_KEY`, `SUPABASE_URL`, and a dozen more. You're copy-pasting from a password manager, a Notion doc, three old `.env` files, and that one Slack DM to yourself.

Then you rotate a key. Good luck updating all 30 projects.

## The Fix

```bash
brew install cheesejaguar/tap/vial    # use the tap; "brew install vial" is a different package
go install github.com/cheesejaguar/vial/cmd/vial@latest

vial init            # create your encrypted vault
vial key set OPENAI_API_KEY
# → paste value (hidden)
# → ✓ Stored.

cd ~/projects/my-new-app
vial pour
# → ✓ OPENAI_API_KEY → matched from vault
# → ✓ STRIPE_SECRET_KEY → matched from vault
# → ✓ NEXT_PUBLIC_SUPABASE_URL → matched (prefix-stripped: SUPABASE_URL)
# → ✗ DATABASE_URL → not found in vault
# → .env created with 3/4 keys populated
```

That's it. Your `.env.example` is the template. Your vault is the source of truth. Vial pours the secrets in.

---

## 🔮 Features

### Encrypted Vault
Your secrets live in a single encrypted file. Argon2id key derivation → AES-256-GCM per-value encryption. Key names stay readable for diffing; values are individually encrypted. Secured in memory with `memguard`.

### 🫗 Pour
The core ritual. Reads your `.env.example`, matches each variable against your vault, and writes a `.env`. Handles conflicts intelligently — prompts you when an existing value differs from the vault.

```bash
vial pour                    # populate .env from vault
vial pour --dry-run          # preview without writing
vial pour --force            # overwrite without asking
vial pour --all              # pour every registered project at once
```

### 🧠 Smart Matching (5-Tier Engine)

Vial doesn't just do exact matches. It *understands* your keys.

| Tier | Method | Example |
|------|--------|---------|
| 1 | Exact match | `OPENAI_API_KEY` = `OPENAI_API_KEY` |
| 2 | Normalize | `NEXT_PUBLIC_SUPABASE_URL` → `SUPABASE_URL` |
| 3 | Alias & variants | `OPENAI_KEY` → `OPENAI_API_KEY` |
| 4 | Comment-informed | `# Your Stripe secret key` → `STRIPE_SECRET_KEY` |
| 5 | LLM-assisted | Calls an inference API for truly ambiguous cases |

Framework prefixes like `NEXT_PUBLIC_`, `VITE_`, `REACT_APP_` are automatically stripped. Common suffix variants (`_KEY` ↔ `_API_KEY` ↔ `_SECRET_KEY`) are auto-detected.

### 🏷️ Aliases

```bash
vial label set OPENAI_KEY=OPENAI_API_KEY
# Now any project asking for OPENAI_KEY gets your OPENAI_API_KEY
```

### 🧪 Distill

Already have secrets scattered in `.env` files? Import them.

```bash
vial distill .env            # import keys from an existing .env
vial distill --overwrite     # update existing vault keys
```

### 🍺 Brew

Run a command with secrets injected — no `.env` file needed.

```bash
vial brew -- node server.js
vial brew -- python manage.py runserver
```

### 📂 Shelf (Project Registry)

Register projects for batch operations.

```bash
vial shelf add ~/projects/my-app
vial shelf add ~/projects/api-server
vial pour --all              # pour every shelved project
```

### 🔄 Key Rotation

Rotate once, propagate everywhere.

```bash
vial key set OPENAI_API_KEY  # update the value
vial pour --all              # re-pour all projects
# → 12 projects updated
```

### 🔍 Scaffold

Auto-generate `.env.example` from your source code.

```bash
vial scaffold                # scan current project for env var references
vial scaffold ./my-project   # scan specific directory
# → Generates .env.example with all discovered variables
```

Detects env vars in JavaScript, TypeScript, Python, Go, Ruby, Rust, and PHP.

### 🚀 Setup (Zero-Config Onboarding)

One command to set up a new project — scan, scaffold, register, pour, and hook.

```bash
cd ~/projects/my-new-app
vial setup                   # does everything below in one step:
# ① Scans source code for env var references
# ② Generates .env.example if missing
# ③ Registers project in shelf
# ④ Pours secrets from vault
# ⑤ Installs git pre-commit hook
```

### 🛡️ Secret Leak Prevention

Git pre-commit hook that scans staged files for leaked vault secrets.

```bash
vial hook install            # install pre-commit hook
vial hook check --staged     # manually check staged files
vial hook uninstall          # remove the hook
```

Create a `.vialignore` file to suppress false positives.

### 🩺 Secret Health

Track the age, rotation status, and health of your secrets.

```bash
vial health                                  # show health report
vial health --set-rotation STRIPE_SECRET_KEY=90  # rotate every 90 days
vial health --json                           # machine-readable output
```

### 🤖 MCP Server (AI Tool Integration)

Model Context Protocol server for Claude Code, Cursor, and other AI coding tools.

```bash
vial mcp                     # start read-only MCP server
vial mcp --allow-writes      # enable write operations
```

Configure in your MCP client (e.g. Claude Code `settings.json`):
```json
{
  "mcpServers": {
    "vial": { "command": "vial", "args": ["mcp"] }
  }
}
```

### 📦 Export Formats

Export secrets in various formats for containers, CI/CD, and scripts.

```bash
vial export --confirm-plaintext --format=docker-env-file  # Docker
vial export --confirm-plaintext --format=k8s-secret       # Kubernetes
vial export --confirm-plaintext --format=github-actions    # GitHub Actions
vial export --confirm-plaintext --format=shell             # Shell exports
vial export --confirm-plaintext --format=json --keys="STRIPE_*"  # Filtered
```

### 📥 Import from External Sources

Import secrets from popular secret managers.

```bash
vial distill --from=json secrets.json        # JSON file
vial distill --from=1password                # 1Password CLI
vial distill --from=doppler                  # Doppler
vial distill --from=vercel                   # Vercel
```

### 🔗 Secret Sharing

Create encrypted, time-limited secret bundles for teammates.

```bash
vial share OPENAI_API_KEY STRIPE_*           # create a bundle
vial share --all --expires=1h                # share everything, 1-hour expiry
vial share receive team-secrets.bundle       # import from a bundle
```

### 📋 Audit Log

Track all vault activity.

```bash
vial audit                   # show last 20 entries
vial audit --limit 50        # show more
vial audit --csv             # export for compliance
```

### ⚡ GitHub Actions

Use vial in CI/CD workflows with the included GitHub Action.

```yaml
- uses: cheesejaguar/vial@v1
  env:
    VIAL_MASTER_KEY: ${{ secrets.VIAL_MASTER_KEY }}
```

### 🖥️ Dashboard

A local web UI for browsing your vault, managing aliases, and checking secret health.

```bash
vial dashboard
# → Opens http://localhost:9876 in your browser
```

Dark-themed Svelte SPA with vault browser, alias management, project registry, and secret health indicators (age, rotation status, staleness).

### 🪄 Claude Code Skill

Teach Claude Code to manage your secrets automatically. When Claude encounters missing `.env` files or API key errors during project setup, it will use Vial to populate secrets from your vault.

Install globally (all projects):
```bash
mkdir -p ~/.claude/skills/vial
curl -o ~/.claude/skills/vial/SKILL.md \
  https://raw.githubusercontent.com/cheesejaguar/vial/main/docs/claude-code-skill/vial.md
```

Or install project-wide so all contributors benefit:
```bash
mkdir -p .claude/skills/vial
curl -o .claude/skills/vial/SKILL.md \
  https://raw.githubusercontent.com/cheesejaguar/vial/main/docs/claude-code-skill/vial.md
git add .claude/skills/vial/SKILL.md && git commit -m "Add Vial skill for Claude Code"
```

See the full [skill installation guide](docs/claude-code-skill/INSTALL.md) for more options, including MCP server setup.

### 🔁 Sync

Keep your vault in sync across machines.

```bash
vial sync push --backend filesystem --remote ~/iCloud/vial/vault.json
vial sync pull --backend filesystem --remote ~/iCloud/vial/vault.json
vial sync status --backend filesystem --remote ~/iCloud/vial/vault.json
```

Supports filesystem sync (iCloud, Dropbox, any mounted path). Git-based sync is experimental.

---

## 🪄 Command Reference

Vial uses alchemical command names with standard aliases for the conventionally-minded.

| Command | Alias | Description |
|---------|-------|-------------|
| `vial init` | | Create a new encrypted vault |
| `vial cork` | `lock` | Lock the vault, clear session |
| `vial uncork` | `unlock` | Unlock with master password |
| `vial key set NAME` | `set` | Store a secret (value via stdin) |
| `vial key get NAME` | `get` | Retrieve a secret |
| `vial key list` | `list`, `ls` | List all stored key names |
| `vial key rm NAME` | `rm` | Remove a secret |
| `vial pour` | | Populate `.env` from vault |
| `vial distill [FILE]` | `import` | Import keys from `.env` file or external source |
| `vial brew -- CMD` | `run` | Run command with injected secrets |
| `vial diff` | | Compare `.env.example` vs vault |
| `vial scaffold` | | Auto-generate `.env.example` from source code |
| `vial setup` | | Zero-config project onboarding |
| `vial health` | | Secret health report & rotation policies |
| `vial hook install/uninstall/check` | | Git pre-commit hook for leak prevention |
| `vial mcp` | | Start MCP server for AI coding tools |
| `vial export` | | Export secrets in various formats |
| `vial share` | | Create encrypted secret bundles |
| `vial share receive` | | Import from a shared bundle |
| `vial audit` | | View vault audit log |
| `vial shelf add/list/rm` | `project` | Manage project registry |
| `vial label set/list/rm` | `alias` | Manage key aliases |
| `vial dashboard` | | Launch web dashboard |
| `vial sync push/pull/status` | | Sync vault to/from remote |
| `vial completion bash/zsh/fish` | | Generate shell completions |

---

## 🔐 Security Model

**What we protect against:**
- Disk theft without master password — vault encrypted at rest with AES-256-GCM
- Shell history leaks — secret values never accepted as CLI arguments
- Swap exposure — `memguard` uses `mlock()` to prevent secrets in swap
- Backup exposure — vault file contains only ciphertext

**What we don't protect against:**
- Active malware with root access on a running system
- Plaintext `.env` files (these are on *you* — use `.gitignore` and full-disk encryption)

**Encryption stack:**
```
Master Password
  → Argon2id (64 MiB, 3 iterations, 1 parallelism)
  → 256-bit KEK
  → Encrypts randomly-generated DEK
  → DEK encrypts individual values via AES-256-GCM
```

**Accepted trade-off:** Reusing API keys across projects increases blast radius if the vault is compromised, in exchange for zero-friction deployment. This is the reality of solo developer workflows. Vial embraces it while providing the strongest practical encryption for the vault itself.

---

## ⚙️ Configuration

Config lives at `~/.config/vial/config.yaml`:

```yaml
vault_path: ~/.local/share/vial/vault.json
session_timeout: 4h
env_example: .env.example
log_level: warn

# LLM-assisted matching (optional)
# llm:
#   provider: openrouter
#   endpoint: https://openrouter.ai/api/v1
#   model: anthropic/claude-sonnet-4-6
#   vault_key_ref: OPENROUTER_API_KEY
```

Override with environment variables (`VIAL_VAULT_PATH`, `VIAL_SESSION_TIMEOUT`, etc.) or the `--config` flag.

### CI/CD

Set `VIAL_MASTER_KEY` to unlock the vault non-interactively:

```bash
export VIAL_MASTER_KEY="your-master-password"
vial pour  # no prompt needed
```

---

## 🏗️ Architecture

```
cmd/vial/main.go              → entry point
internal/
  vault/                       → Argon2id KDF, AES-256-GCM, CRUD, atomic storage
  parser/                      → .env parser (quotes, escapes, interpolation, multi-line)
  matcher/                     → 5-tier matching engine
  alias/                       → alias store, pattern rules, auto-detection
  project/                     → project registry
  llm/                         → LLM provider abstraction (OpenAI, Anthropic, OpenRouter)
  scanner/                     → source code env var extraction (JS, Python, Go, Ruby, Rust, PHP)
  dashboard/                   → embedded Svelte web dashboard
  keyring/                     → OS keychain session caching
  config/                      → YAML config via Viper
  sync/                        → vault sync (filesystem, git)
web/                           → SvelteKit dashboard SPA
vscode/                        → VS Code extension (coming soon)
```

Single static binary. No runtime dependencies. ~9 MB with the embedded dashboard.

---

## 📄 License

MIT — do whatever you want with it.

---

<p align="center">
  <em>Potent secrets, safely contained.</em>
</p>
