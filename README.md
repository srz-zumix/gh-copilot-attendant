# gh-copilot-attendant

A GitHub CLI extension for inspecting and reporting on local GitHub Copilot CLI usage.

## Installation

```sh
gh extension install srz-zumix/gh-copilot-attendant
```

Optionally, set up a shorter alias:

```sh
gh alias set ca copilot-attendant
```

This lets you run commands as `gh ca <command>` instead of `gh copilot-attendant <command>`.

## Shell completion

`gh copilot-attendant completion` generates a shell completion script for bash, zsh, fish, or
PowerShell. Run `gh copilot-attendant completion --help` for the full list of shells and setup
instructions.

## Commands

### session

`gh copilot-attendant session` groups commands that inspect the local GitHub Copilot CLI
session history recorded under `~/.copilot/session-state` (or `$COPILOT_HOME/session-state`).

#### session stats

```sh
gh copilot-attendant session stats [--all | --cwd <path> | --worktree <path> | -W <path>] [--session <id>] [--since <time> | --period <period>] [--until <time>] [--operation <read|write>]... [--kind <kind>]... [--command <identifier>]... [--path <pattern>]... [--url <pattern>]... [--top <n>] [--format <format>] [--jq <expression>] [--template <template>]
```

Scans the local Copilot CLI session history and reports how often each tool, command, file
path, and URL was requested, broken down by whether it was approved or denied. `--all`,
`--cwd`, and `--worktree`/`-W` are mutually exclusive and all optional; when none are given,
only sessions whose recorded working directory is inside the current git worktree are
counted. `--session` restricts to a single session ID. `--since` and `--until` accept
RFC3339 timestamps, `YYYY-MM-DD` dates, or a relative duration such as `7d`/`24h`/`30m`, and
default to no bound. `--period` is shorthand for `--since` using a coarser window such as
`7d`/`3w`/`6m`/`1y`, and is mutually exclusive with `--since`. `--operation` restricts to
non-mutating (`read`) or mutating (`write`) requests. `--kind` restricts to an exact tool
kind (e.g. `shell`, `read`, `write`); `--command` restricts to requests that include an
exact command identifier; `--path` and `--url` restrict to requests with a recorded path or
URL matching the given regular expression (a plain substring is also a valid, unanchored
regular expression). `--operation`, `--kind`, `--command`, `--path`, and `--url` may each be
repeated to match any of multiple values. `--top` keeps only the top N entries per
statistic (default `10`; `0` keeps every entry).
`--format`, `--jq`, and `--template` follow the standard `gh` JSON export flags; without
`--format json`, results are printed as tables.

### skills

`gh copilot-attendant skills` manages the agent skills bundled with this extension. Run
`gh copilot-attendant skills --help` for its subcommands.

## Development

```bash
go test ./...
go run . --help
```
