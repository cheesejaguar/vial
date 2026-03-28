---
name: vial
description: >
  Manage API keys, secrets, and .env files using the Vial encrypted vault CLI.
  Use this skill whenever a project needs environment variables, a .env.example
  exists without a .env, the user mentions API keys or secrets, or you're setting
  up a project for development. Also use when you see errors about missing env
  vars, undefined API keys, or "OPENAI_API_KEY is not set" style failures.
---

# Vial — Secret Management for Project Setup

Vial is an encrypted local vault that stores API keys once and auto-populates `.env` files across projects. When you encounter missing secrets during project setup, use this workflow.

## Step 1: Check Installation

```bash
command -v vial
```

**Not installed?** Tell the user:

```bash
brew tap cheesejaguar/tap && brew install cheesejaguar/tap/vial
```

IMPORTANT: Use `cheesejaguar/tap/vial` — bare `brew install vial` is a different package (a keyboard tool).

After installing, the user must run `vial init` themselves to create their vault. This requires interactive password entry that you cannot provide.

If the user declines installation, fall back to manual `.env` management.

## Step 2: Assess Project State

Check these files:
```bash
test -f .env.example && echo "HAS_TEMPLATE" || echo "NO_TEMPLATE"
test -f .env && echo "HAS_ENV" || echo "NO_ENV"
```

Then follow the matching path below.

## Path A: Has .env.example, No .env (most common)

Pour secrets from the vault into a new `.env`:

```bash
vial pour --dry-run          # preview what matches
vial pour --force            # write .env (no interactive prompts)
```

Always use `--force` — without it, vial prompts interactively on conflicts, which you cannot respond to.

If pour reports missing keys ("not found in vault"), tell the user which specific keys need to be added:

> These keys aren't in your Vial vault yet. Add each one by running the command and pasting the value when prompted:
> ```
> vial key set MISSING_KEY_NAME
> ```
> Then I'll re-run `vial pour --force` to populate your .env.

**You cannot run `vial key set` yourself** — it requires hidden interactive input for the secret value. This is a security feature.

## Path B: No .env.example, No .env

Generate a template by scanning source code, then pour:

```bash
vial scaffold                # scans JS/TS/Python/Go/Ruby/Rust/PHP for env var usage
vial pour --force            # populate .env from the generated template
```

Review the generated `.env.example` before pouring — scaffold sometimes picks up test-only or optional variables.

## Path C: Has .env, No .env.example (importing secrets)

Import existing secrets into the vault so they're available for other projects:

```bash
vial distill .env --all      # import all keys without interactive selection
```

Add `--overwrite` to update vault keys that already have different values. Then generate a template:

```bash
vial scaffold                # create .env.example for this project
```

## Path D: Full Onboarding (new project checkout)

One command does everything — scan, scaffold, register, pour, and install git hook:

```bash
vial setup
```

This is the best option when setting up a freshly cloned project. It handles all paths automatically.

## Running Commands Without .env

When `.env` can't be fully populated (missing keys) but you need to run something:

```bash
vial brew -- npm run dev
vial brew -- python manage.py runserver
vial brew -- node server.js
```

This injects available vault secrets as environment variables without writing a file. If `.env.example` exists, only those keys are injected. Otherwise, all vault keys are injected.

## Command Quick Reference

| Task | Command |
|------|---------|
| Populate .env from vault | `vial pour --force` |
| Preview what would pour | `vial pour --dry-run` |
| Keep existing values, fill gaps | `vial pour --no-clobber` |
| Generate .env.example from code | `vial scaffold` |
| Import .env into vault | `vial distill .env --all` |
| Full project setup | `vial setup` |
| Run with injected secrets | `vial brew -- CMD` |
| Show what's missing | `vial diff` |
| List vault key names | `vial key list` |
| Pour all registered projects | `vial pour --all` |

## Rules

1. **Never pass secret values as CLI arguments.** There is no `vial key set NAME VALUE` syntax. Values are always prompted interactively.

2. **Never display secret values.** Do not run `vial key get NAME` and show output. Use `vial key list` (names only) to verify keys exist.

3. **Always use non-interactive flags.** You cannot respond to TUI prompts. Use `--force`, `--all`, `--yes`, or `--no-clobber`.

4. **Check .gitignore after creating .env.** If `.env` is not in `.gitignore`, add it. Never commit `.env` files.

5. **If vault is locked**, tell the user to run `vial uncork` in their terminal, or set `export VIAL_MASTER_KEY="..."` in their shell profile for headless/CI use.

## When to Use Vial in Project Setup

When you are autonomously setting up a project (cloning, installing deps, configuring):

1. Install dependencies (`npm install`, `pip install`, etc.)
2. Check for `.env.example` — if present, run `vial pour --dry-run`
3. Pour secrets — `vial pour --force`
4. Report missing keys to the user with `vial key set` instructions
5. Verify `.gitignore` includes `.env`
6. Start the dev server

## MCP Integration (Advanced)

For deeper vault access, configure Vial's MCP server:

```json
{
  "mcpServers": {
    "vial": { "command": "vial", "args": ["mcp"] }
  }
}
```

This exposes `vault_list`, `vault_search`, and `vault_health` tools. Add `--allow-writes` for `vault_set`/`vault_remove`. The skill approach above is simpler and sufficient for most workflows.
