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

### copilot

`gh copilot-attendant copilot` groups commands that manage the GitHub Copilot CLI canvas
extensions bundled with this repository (currently `attendant-dashboard`). Extensions are
downloaded from this repository's GitHub sources. All `[name...]` arguments are optional;
when omitted, every bundled extension is targeted.

#### copilot extension install

```sh
gh copilot-attendant copilot extension install [name...] [--scope <user|repo>] [--prefix <dir>] [--ref <ref>] [--dry-run] [--force]
```

Downloads and installs the given extensions. `--scope` selects the installation scope:
`user` (default) installs under `$COPILOT_HOME/extensions` (`~/.copilot/extensions` when
unset) and `repo` installs into the current repository's `.github/extensions`. `--prefix`
overrides the install directory and takes precedence over `--scope`. `--ref` overrides the
default git ref (`main`). `--dry-run` prints what would be installed without writing files.
`--force` overwrites an existing destination directory that is not managed by this command.

#### copilot extension list

```sh
gh copilot-attendant copilot extension list
```

Lists the bundled extensions with their source URL and default ref.

#### copilot extension status

```sh
gh copilot-attendant copilot extension status [name...] [--scope <user|repo>] [--prefix <dir>]
```

Shows whether the given extensions are installed and which ref and commit they were installed
from. Only the local filesystem is inspected. `--scope` defaults to `user`.

#### copilot extension uninstall

```sh
gh copilot-attendant copilot extension uninstall [name...] [--scope <user|repo>] [--prefix <dir>] [--dry-run] [--force]
```

Removes the given extensions. `--scope` defaults to `user`. `--dry-run` prints what would be
removed. `--force` removes a destination directory even if it is not managed by this command.

#### copilot extension update

```sh
gh copilot-attendant copilot extension update [name...] [--scope <user|repo>] [--prefix <dir>] [--ref <ref>] [--dry-run] [--force]
```

Re-installs the given installed extensions when their ref resolves to a different commit than
the installed one. Fails if an extension is not installed yet. `--scope` defaults to `user`.
`--ref` overrides the default git ref (`main`). `--dry-run` prints what would be updated.
`--force` re-installs even when already up to date, or overwrites an unmanaged directory.

### session

`gh copilot-attendant session` groups commands that inspect the local GitHub Copilot CLI
session history recorded under `~/.copilot/session-state` (or `$COPILOT_HOME/session-state`).

#### session stats

```sh
gh copilot-attendant session stats [--all | --cwd <path> | --worktree <path> | -W <path>] [--session <id>] [--since <time> | --period <period>] [--until <time>] [--operation <read|write>]... [--kind <kind>]... [--decision-source <source>]... [--command <identifier>]... [--path <pattern>]... [--url <pattern>]... [--top <n>] [--format <format>] [--jq <expression>] [--template <template>]
```

Scans the local Copilot CLI session history and reports how often each tool, command, file
path, and URL was requested, broken down by whether it was approved or denied, as well as
by the CLI's own `decisionSource` for that outcome (e.g. `human_response`; `unknown` when
no decision source was recorded, including sessions from older CLI versions and unresolved
requests without a recorded outcome). It also reports each session's usage totals
(premium requests, AIU, and token counts) recorded by its `session.shutdown` event, summed
overall and broken down by working directory; usage is counted independently of the
permission-request filters below, since it is not recorded per permission request.
When `modelMetrics` is recorded, the `MODEL_USAGE` table also reports model names, API
request counts (not permission requests), premium requests, AIU, and token counts per
model. JSON output includes this breakdown in `ByModelUsage`. Sessions without model
metrics contribute to the overall totals but have no model breakdown. Model usage follows
the same session scope, time bounds, and `--top` limit as working-directory usage. `--all`,
`--cwd`, and `--worktree`/`-W` are mutually exclusive and all optional; when none are given,
only sessions whose recorded working directory is inside the current git worktree are
counted. `--session` restricts to a single session ID. `--since` and `--until` accept
RFC3339 timestamps, `YYYY-MM-DD` dates, or a relative duration such as `7d`/`24h`/`30m`, and
default to no bound (usage totals are bound by each session's `session.shutdown` event time
instead of individual request times). `--period` is shorthand for `--since` using a coarser
window such as `7d`/`3w`/`6m`/`1y`, and is mutually exclusive with `--since`. `--operation`
restricts to non-mutating (`read`) or mutating (`write`) requests. `--kind` restricts to an
exact tool kind (e.g. `shell`, `read`, `write`); `--decision-source` restricts to permission
outcomes with an exact decision source (e.g. `human_response`, `unattended_fallback`, or `unknown`
when no decision source was recorded, including older CLI versions and unresolved requests); `--command` restricts to requests that include an
exact command identifier; `--path` and `--url` restrict to requests with a recorded path or
URL matching the given regular expression (a plain substring is also a valid, unanchored
regular expression). `--operation`, `--kind`, `--decision-source`, `--command`, `--path`, and
`--url` may each be repeated to match any of multiple values. `--top` keeps only the top N
entries per statistic (default `10`; `0` keeps every entry).
`--format`, `--jq`, and `--template` follow the standard `gh` JSON export flags; without
`--format json`, results are printed as tables.

### skills

`gh copilot-attendant skills` manages the agent skills bundled with this extension. Run
`gh copilot-attendant skills --help` for its subcommands.

### vscode

`gh copilot-attendant vscode` groups commands that inspect the local VS Code GitHub Copilot
Chat extension's debug logs, recorded under
`~/Library/Application Support/Code/User/workspaceStorage`. Only the stable, non-Insiders
VS Code installation on macOS is supported.

#### vscode stats

```sh
gh copilot-attendant vscode stats [--all | --cwd <path> | --worktree <path> | -W <path>] [--session <id>] [--since <time> | --period <period>] [--until <time>] [--tool <pattern>]... [--model <pattern>]... [--agent <pattern>]... [--top <n>] [--format <format>] [--jq <expression>] [--template <template>]
```

Scans the local VS Code Copilot Chat debug logs and reports tool usage, LLM token and usage
totals, turn counts, and subagent invocations. `--all`, `--cwd`, and `--worktree`/`-W` are
mutually exclusive and all optional; when none are given, only sessions belonging to the
current git worktree are counted. `--session` restricts to a single session ID. `--since`
and `--until` accept RFC3339 timestamps, `YYYY-MM-DD` dates, or a relative duration such as
`7d`/`24h`/`30m`, and default to no bound. `--period` is shorthand for `--since` using a
coarser window such as `7d`/`3w`/`6m`/`1y`, and is mutually exclusive with `--since`.
`--tool`, `--model`, and `--agent` restrict to tool calls, LLM requests, or subagent
invocations matching the given regular expression (a plain substring is also a valid,
unanchored regular expression), and may each be repeated to match any of multiple values.
`--top` keeps only the top N entries per statistic (default `10`; `0` keeps every entry).
`--format`, `--jq`, and `--template` follow the standard `gh` JSON export flags; without
`--format json`, results are printed as tables.

## Copilot app dashboard

This repository ships a canvas extension for the GitHub Copilot app in
[`.github/extensions/attendant-dashboard`](.github/extensions/attendant-dashboard/README.md).
When a Copilot session runs inside this repository, ask the agent to open the
"Copilot attendant dashboard" to browse `session stats` and `vscode stats` results as
interactive charts, filter by clicking entries, and hand the data back to the agent.

## Development

```bash
go test ./...
go run . --help
```
