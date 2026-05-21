# AIM Loadout CLI

`aiman` is the command-line interface for AIM Loadout: a local-first, Git-backed
manager for AI-agent inventory.

An AIM inventory is the loadout of an AI-powered developer: reusable skills,
MCP server definitions, and the local metadata needed to apply them into the
AI tools you already use. `aiman sync` is the quick reload that puts that
loadout back on a new machine.

```text
skills/review-code.md  ->  ~/.claude/skills/review-code/SKILL.md
                       ->  ~/.cursor/skills/review-code/SKILL.md
                       ->  ~/.codex/skills/review-code/SKILL.md

mcp/context7.yaml      ->  ~/.claude/settings.json
                       ->  ~/.cursor/mcp.json
                       ->  ~/.codex/config.toml
```

AIM is currently pre-production. The CLI is usable, the core workflow is
implemented, and the public installation/distribution story is still being
polished.

## Why AIM Exists

AI development tools do not share one inventory model. Claude Code, Cursor, and
Codex CLI all have their own directories, file formats, and MCP configuration
surfaces. That becomes painful when you use several tools, move between
machines, or maintain a growing set of custom skills.

AIM gives that inventory one source of truth:

- skills live as Markdown files in `skills/`
- MCP servers live as YAML files in `mcp/`
- Git moves the inventory between machines
- adapters translate the same inventory into each AI environment
- local secrets and machine-specific paths stay out of Git

The product focus is not "another dotfiles repo". AIM understands AI-tool
inventory and applies it through environment-specific adapters.

## Two Workflows

### Inner Loop: Develop Locally

Use `aiman apply` while writing or refining skills. It applies the current
working tree into local AI tools without Git fetch, commit, push, or network
access.

```bash
$EDITOR skills/review-code.md
aiman apply

# Test the skill in Claude Code, Cursor, or Codex.
# If it needs more work, edit and apply again.

aiman status
aiman push
```

This keeps unfinished skills local until they are ready to publish.

### Outer Loop: Sync Everywhere

Use `aiman push` to publish a validated inventory, then `aiman sync` to apply
that inventory on another machine.

```bash
# Machine A
aiman push

# Machine B
aiman init git@github.com:you/aim-loadout.git
aiman sync
```

This is the "new machine, same AI setup" path.

## Quick Start

Package channels are being prepared. For now, build from source:

```bash
git clone git@github.com:axsmak/aim.git
cd aim
make build

./bin/aiman --version
./bin/aiman doctor
```

Use `./bin/aiman` from the checkout, or put `bin/` on your `PATH` and use
`aiman` directly.

Connect an inventory repository:

```bash
aiman init git@github.com:you/aim-loadout.git
aiman sync
```

If you are starting with an empty inventory, add a skill and publish it:

```bash
mkdir -p skills
$EDITOR skills/review-code.md

aiman apply --dry-run
aiman apply
aiman push
```

## Inventory Structure

```text
<inventory>/
├── skills/
│   └── review-code.md
├── mcp/
│   └── context7.yaml
├── aim.yaml
└── .gitignore
```

`aim.local.yaml` is created locally on each machine and must not be committed.
It stores machine-specific paths, sync markers, and MCP environment values.

## Skill Format

```markdown
---
name: review-code
description: Review code for correctness, regressions, and missing tests
---

# Role

You are a senior engineer reviewing a code change.

# Task

Find correctness issues first, then summarize residual risk.
```

A skill is installed as `SKILL.md` in each supported AI environment.

## MCP Format

```yaml
name: context7
description: Library documentation in the assistant context
command: npx
args:
  - -y
  - "@upstash/context7-mcp"
targets:
  - claude-code
  - cursor
env:
  - name: UPSTASH_REDIS_REST_URL
    description: Redis REST URL
    required: true
  - name: UPSTASH_REDIS_REST_TOKEN
    description: Redis REST token
    required: true
```

Required env values are requested during sync and stored only in
`aim.local.yaml`.

## Commands

| Command | Purpose |
|---------|---------|
| `aiman init <repo-url> [--path <dir>]` | Clone/connect an inventory repository and create local config |
| `aiman switch <path>` | Switch the active inventory repository |
| `aiman apply [--dry-run]` | Apply local inventory without Git operations |
| `aiman push [--dry-run]` | Validate, commit, and push inventory changes |
| `aiman sync [--dry-run] [--force]` | Fetch and apply the published inventory |
| `aiman status` | Show local, remote, and environment sync state |
| `aiman doctor` | Diagnose AI environments, skills, and MCP env values |

`aiman list` exists today for skills, but the full inventory/loadout view is not
part of the current public contract yet.

## Supported AI Tools

| Tool | Skills | MCP |
|------|--------|-----|
| Claude Code | `~/.claude/skills/<name>/SKILL.md` | `~/.claude/settings.json` |
| Cursor | `~/.cursor/skills/<name>/SKILL.md` | `~/.cursor/mcp.json` |
| Codex CLI | `~/.codex/skills/<name>/SKILL.md` | `~/.codex/config.toml` |

Support is adapter-based. Adding a new AI environment should be an adapter, not
a rewrite of the core inventory model.

## Safety Model

- AIM is local-first: no account, no cloud service, no hosted backend.
- Inventory is transported through a Git repository you control.
- `aim.local.yaml` is gitignored and stores machine-local paths and MCP secrets.
- `aiman push` blocks publishing if `aim.local.yaml` is staged.
- `aiman apply` is offline-capable and does not mutate Git state.
- `aiman sync` updates sync markers only after the inventory is applied.

## Limitations

- Pre-production packaging is incomplete; build from source is the reliable path.
- Claude Code, Cursor, and Codex CLI are the supported adapters today.
- MCP support is focused on command-based stdio servers.
- Secrets are local to each machine and must be entered again where required.
- Named loadouts and richer inventory listing are planned, not current contract.

## Documentation

Public user documentation lives in the `aim-site` repository:

- Guide: `aim-site/ru/guide/`
- Reference: `aim-site/ru/reference/`

CLI technical notes that should stay close to the implementation:

- [`docs/adapter-architecture.md`](docs/adapter-architecture.md)
- [`docs/sync-push-algorithm.md`](docs/sync-push-algorithm.md)
- [`docs/versioning.md`](docs/versioning.md)
- [`CLAUDE.md`](CLAUDE.md)

Historical PRDs, epics, old wiki pages, and launch plans were moved to
`aim-workspace/context/history/aim-cli/`.

## Development

```bash
make build
make test
go test ./...
```
