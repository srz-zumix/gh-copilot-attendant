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
requests without a recorded outcome). It also reports each session's recorded usage totals
(premium requests, AIU, and token counts), including unfinished sessions, summed
overall and broken down by working directory; usage is counted independently of the
permission-request filters below, since it is not recorded per permission request.
Request-level `session.usage_record` events and cumulative `session.usage_checkpoint` and
`session.shutdown` snapshots are combined without double counting: each snapshot replaces
the counters it covers, and subsequent request records add new usage. Replayed request
records are deduplicated using their source-session accounting identity.
When request records or `modelMetrics` are available, `MODEL_USAGE` also reports model names, API
request counts (not permission requests), premium requests, AIU, and token counts per
model. JSON output includes this breakdown in `ByModelUsage`. Checkpoints can record AIU
and premium totals without model, request, or token details; those totals are included
without inventing missing details. `UsageIncompleteSessions` marks incomplete breakdowns,
and request/token counts for those sessions are recorded subtotals.
`AGENT_USAGE` (`ByRoleUsage` in JSON) separates `main`, `subagent`, and `unknown` usage
using recorded request metadata, not model names. Older or partially recorded usage stays
`unknown`. This role breakdown is never truncated by `--top`; it reports API requests,
premium requests, AIU, and tokens, not API duration. API request totals are also exported
as `UsageRequests`. Model usage follows the same session scope, time bounds, and `--top`
limit as working-directory usage. `--all`,
`--cwd`, and `--worktree`/`-W` are mutually exclusive and all optional; when none are given,
only sessions whose recorded working directory is inside the current git worktree are
counted. `--session` restricts to a single session ID. `--since` and `--until` accept
RFC3339 timestamps, `YYYY-MM-DD` dates, or a relative duration such as `7d`/`24h`/`30m`, and
default to no bound (lifetime usage totals are bound by each session's latest recorded usage
update time, not individual API request times). `--period` is shorthand for `--since` using a coarser
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

`ASK_USER` (`AskUser` in JSON) reports sessions with questions, question counts, and
answered/cancelled/failed/unresolved/unknown outcomes from paired `ask_user` tool events.
Answered questions with valid timestamps contribute to total, average, and maximum
wait in milliseconds; `TimedAnswers` and `UntimedAnswers` expose timing coverage.
Average and maximum are `null` (`n/a` in tables) without timed answers. Unknown completions
are not inferred as answers. Wait runs from tool start to completion and includes
UI/transport overhead; overlapping per-question intervals are summed, not deduplicated
as wall-clock idle time. Session/directory scope applies; time bounds select question
starts (inclusive `--since`, exclusive `--until`), while later completions still resolve
selected questions. Permission filters and `--top` do not affect this summary.
No question or answer text is exported. Older logs without recorded events cannot
reconstruct missing questions.

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
gh copilot-attendant vscode stats [--all | --cwd <path> | --worktree <path> | -W <path>] [--session <id>] [--since <time> | --period <period>] [--until <time>] [--tool <pattern>]... [--model <pattern>]... [--agent <pattern>]... [--include-log-stats] [--top <n>] [--format <format>] [--jq <expression>] [--template <template>]
```

Scans the local VS Code Copilot Chat debug logs and reports tool usage, LLM token and usage
totals, turn counts, and subagent invocations. Subagent log files referenced by each session
are included recursively, with each file counted once. Their model requests, tokens,
AIU, tool calls, and turns are attributed to the parent workspace; session counts still
refer to parent chat sessions. Internal
title-generation child logs and unreferenced files are excluded. Time bounds apply to
each event, including child activity whose invocation predates `--since`.
When `--agent` is specified, all activity is scoped to matching subagents' own child logs:
model requests, AIU, tokens, tool calls, turns, and workspace totals. Main-agent usage is
excluded, and differently named descendants are excluded unless they also match an agent
pattern. Without `--agent`, parent and referenced child usage is included as before.
Tool and model filters apply independently within that scope. Missing child logs produce
a warning, while their recorded matching invocations remain counted.

`--all`, `--cwd`, and `--worktree`/`-W` are mutually exclusive and all optional; when none
are given, only sessions belonging to the
current git worktree are counted. `--session` restricts to a single session ID. `--since`
and `--until` accept RFC3339 timestamps, `YYYY-MM-DD` dates, or a relative duration such as
`7d`/`24h`/`30m`, and default to no bound. `--period` is shorthand for `--since` using a
coarser window such as `7d`/`3w`/`6m`/`1y`, and is mutually exclusive with `--since`.
`--tool` and `--model` restrict tool calls and LLM requests; `--agent` scopes activity by
subagent name. All three are optional and default to no filter. They match the given
regular expression (a plain substring is also a valid, unanchored regular expression)
and may each be repeated to match any of multiple values.
`--include-log-stats` is optional (default `false`) and adds versioned `LogDetails` to JSON
output. Details contain each parent chat's identity and workspace, unique log identities,
main/child classification, all referencing agent names, normalized own-activity metrics,
and invocation identities in traversal order. These details are not truncated by `--top`
and contain no raw events or conversation content. They support local agent filtering
without double-counting shared logs or parent chats; positive TTFT sample counts and sums
are retained for accurate averaging. This flag cannot be combined with `--agent`.
`--top` keeps only the top N entries per statistic (default `10`; `0` keeps every entry).
`--format`, `--jq`, and `--template` follow the standard `gh` JSON export flags; without
`--format json`, results are printed as tables.

For example, report only Explore agents' own usage across all workspaces over the last
seven days:

```sh
gh copilot-attendant vscode stats --all --period 7d --agent '^Explore$'
```

`USER_QUESTIONS` (`UserQuestions` in JSON) reports completed `vscode_askQuestions`
calls, sessions with calls, and answered/skipped/cancelled/failed/unknown outcomes.
Each multi-question batch counts as one call; a partially skipped batch with at least
one recorded answer counts as answered, while a fully skipped batch counts as skipped.
Only answered calls with valid recorded `dur` contribute to total/average/maximum
wait in milliseconds. `TimedAnswers` and `UntimedAnswers` expose timing coverage;
average/maximum are `null` (`n/a` in tables) without timed answers.
Time bounds select call starts, even when completion falls after `--until`.
Session/directory/agent/tool filters apply; model filters and `--top` do not.
Duration includes tool/UI/transport overhead; overlapping intervals are summed,
not deduplicated as wall-clock idle time. In-flight questions cannot be reconstructed
from completed-call logs. No question or answer text is exported. Log details include
these untruncated per-log metrics for exact local filtering and parent-session counting.

## Copilot app dashboard

This repository ships a canvas extension for the GitHub Copilot app in
[`.github/extensions/attendant-dashboard`](.github/extensions/attendant-dashboard/README.md).
When a Copilot session runs inside this repository, ask the agent to open the
"Copilot attendant dashboard" to browse `session stats` and `vscode stats` results as
interactive charts, filter by clicking entries, and hand the data back to the agent.
The VS Code canvas fetches log details once and processes Agent and Top changes locally,
without re-running gh. Refresh, scope/time/session changes, and tool/model filter changes
fetch a new dataset. Filtered display, summaries, and Ask agent use the same projected data.
Filterable panels have a Clear button that removes only that panel's conditions.
Workspaces and Working directories clear the directory scope back to all workspaces.
Other filters, the time window, session selection, and Top remain unchanged.
Both tabs show a User questions panel with answer outcomes and total/average/maximum
wait; VS Code counts completed tool calls, while CLI pairs recorded question events.
User questions follows Workspaces on both tabs and precedes Agent usage on CLI.
The CLI Permissions summary uses the same bordered panel style as the other panels.

## Development

```bash
go test ./...
go run . --help
```

The canvas ships a standalone RE2JS matcher with its MIT license; end users do not need
npm or a runtime package installation. To update the pinned matcher bundle:

```sh
npm ci
npm run vendor:canvas
```
