# Installing the Vial Skill for Claude Code

The Vial skill teaches Claude Code how to manage your secrets automatically. When Claude encounters missing `.env` files, API key errors, or project setup tasks, it will use Vial to populate secrets from your encrypted vault.

## Prerequisites

1. **Vial CLI installed:**
   ```bash
   brew tap cheesejaguar/tap && brew install cheesejaguar/tap/vial
   ```
   > **Important:** Use `cheesejaguar/tap/vial` — bare `brew install vial` installs a different package (a keyboard tool).

   Or via Go:
   ```bash
   go install github.com/cheesejaguar/vial/cmd/vial@latest
   ```

2. **Vault initialized:**
   ```bash
   vial init
   ```

3. **Claude Code installed:** Available as a [CLI](https://docs.anthropic.com/en/docs/claude-code), [VS Code extension](https://marketplace.visualstudio.com/items?itemName=anthropic.claude-code), or [JetBrains plugin](https://plugins.jetbrains.com/plugin/claude-code).

## Install the Skill

### Option A: Install from URL (Recommended)

Run this slash command inside Claude Code:

```
/install-skill https://raw.githubusercontent.com/cheesejaguar/vial/main/docs/claude-code-skill/vial.md
```

Claude Code will download the skill and add it to your configuration.

### Option B: Manual Installation

1. Create the skills directory if it doesn't exist:
   ```bash
   mkdir -p ~/.claude/skills
   ```

2. Copy the skill file:
   ```bash
   curl -o ~/.claude/skills/vial.md \
     https://raw.githubusercontent.com/cheesejaguar/vial/main/docs/claude-code-skill/vial.md
   ```

   Or if you have the repo cloned:
   ```bash
   cp docs/claude-code-skill/vial.md ~/.claude/skills/vial.md
   ```

### Option C: Project-Level Installation

To install the skill for a specific project only (so all contributors benefit):

1. Create the directory in your project:
   ```bash
   mkdir -p .claude/skills
   ```

2. Copy the skill file:
   ```bash
   curl -o .claude/skills/vial.md \
     https://raw.githubusercontent.com/cheesejaguar/vial/main/docs/claude-code-skill/vial.md
   ```

3. Commit it:
   ```bash
   git add .claude/skills/vial.md
   git commit -m "Add Vial skill for Claude Code"
   ```

This way, anyone who clones the project and uses Claude Code will automatically have the Vial skill available.

## Verify Installation

Open Claude Code and ask:

> "Set up this project's environment variables"

or

> "I need to configure my .env file"

Claude should recognize the request and use the Vial workflow — checking installation, assessing project state, and running `vial pour` to populate your `.env`.

## Optional: MCP Server

For deeper vault integration (letting Claude directly query and search your vault), add the Vial MCP server to your Claude Code settings:

**Global** (`~/.claude/settings.json`):
```json
{
  "mcpServers": {
    "vial": {
      "command": "vial",
      "args": ["mcp"]
    }
  }
}
```

**Project-level** (`.claude/settings.json` in your repo):
```json
{
  "mcpServers": {
    "vial": {
      "command": "vial",
      "args": ["mcp"]
    }
  }
}
```

The MCP server is read-only by default. Add `"--allow-writes"` to the `args` array to let Claude store secrets (it will still prompt you for values interactively).

## What the Skill Does

Once installed, Claude Code will automatically:

- Detect when a project needs environment variables (missing `.env`, `.env.example` present, API key errors)
- Run `vial pour --force` to populate `.env` from your vault
- Run `vial scaffold` to generate `.env.example` if none exists
- Tell you which keys are missing and how to add them (`vial key set KEY_NAME`)
- Check `.gitignore` to prevent accidental secret commits
- Use `vial brew` to run commands with injected secrets when `.env` can't be fully populated

## Uninstalling

Remove the skill file:

```bash
# Global installation
rm ~/.claude/skills/vial.md

# Project-level installation
rm .claude/skills/vial.md
```
