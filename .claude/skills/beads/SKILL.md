---
name: beads
description: File, query, or manage issues with the beads (bd) issue tracker
disable-model-invocation: false
argument-hint: "[command or description]"
---

# Beads Issue Tracker

Run the user's request using `bd`. If `$ARGUMENTS` is a bd subcommand (e.g. `ready`, `show`, `list`, `create`, `close`, `stats`), execute it directly:

```bash
bd $ARGUMENTS
```

If `$ARGUMENTS` is a free-form description (e.g. "show draft status on PR page"), create an issue:

```bash
bd create --title="<concise title>" --description="<why and what>" --type=<task|bug|feature> --priority=2
```

## Quick reference

| Command | Purpose |
|---------|---------|
| `bd ready` | Show issues ready to work |
| `bd list --status=open` | All open issues |
| `bd list --status=in_progress` | Active work |
| `bd show <id>` | Issue details |
| `bd create --title="..." --description="..." --type=task --priority=2` | New issue |
| `bd update <id> --claim` | Claim work |
| `bd close <id>` | Mark complete |
| `bd close <id1> <id2> ...` | Close multiple |
| `bd stats` | Project statistics |
| `bd search <query>` | Search issues |

## Rules

- Priority: 0-4 (0=critical, 4=backlog). Default to 2 if unspecified.
- Types: `task`, `bug`, `feature`
- Do NOT use `bd edit` — it opens an interactive editor that blocks agents.
- When creating issues, ONLY create the issue. Do NOT claim it or start working on it unless the user explicitly asks you to.
