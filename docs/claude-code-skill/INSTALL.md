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

Claude Code discovers skills automatically from `SKILL.md` files placed in a `.claude/skills/<skill-name>/` directory. No configuration or restart is needed — just create the file and it's available immediately.

### Option A: Global Installation (All Projects)

Install for your user so the skill is available in every project:

```bash
mkdir -p ~/.claude/skills/vial
curl -o ~/.claude/skills/vial/SKILL.md \
  https://raw.githubusercontent.com/cheesejaguar/vial/main/docs/claude-code-skill/vial.md
```

Or if you have the repo cloned:
```bash
mkdir -p ~/.claude/skills/vial
cp docs/claude-code-skill/vial.md ~/.claude/skills/vial/SKILL.md
```

### Option B: Project-Level Installation (Recommended for Teams)

Install the skill into a specific project so all contributors benefit:

```bash
mkdir -p .claude/skills/vial
curl -o .claude/skills/vial/SKILL.md \
  https://raw.githubusercontent.com/cheesejaguar/vial/main/docs/claude-code-skill/vial.md
```

Commit it so your team gets the skill automatically:
```bash
git add .claude/skills/vial/SKILL.md
git commit -m "Add Vial skill for Claude Code"
```

Anyone who clones the project and uses Claude Code will automatically have the Vial skill available.

## Verify Installation

Open Claude Code and try one of:

- Type `/vial` to invoke the skill directly
- Ask: *"Set up this project's environment variables"*
- Ask: *"I need to configure my .env file"*

Claude should recognize the request and use the Vial workflow — checking installation, assessing project state, and running `vial pour` to populate your `.env`.

## Optional: MCP Server

For deeper vault integration (letting Claude directly query and search your vault), add the Vial MCP server to your Claude Code settings.

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

Remove the skill directory:

```bash
# Global installation
rm -r ~/.claude/skills/vial

# Project-level installation
rm -r .claude/skills/vial
```
