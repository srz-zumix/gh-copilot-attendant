# attendant-dashboard

A Copilot CLI canvas extension that visualizes local GitHub Copilot usage in the
GitHub Copilot app's side panel. It runs `gh copilot-attendant <session|vscode> stats --format json`
and renders the result as an interactive dashboard.

## Requirements

- The `gh` CLI with the `copilot-attendant` extension installed, **or** Go and a
  `gh-copilot-attendant` checkout to build the binary from (used automatically when the gh
  extension is not installed; see [Runner resolution](#runner-resolution)).

## Usage

Ask the agent to open the dashboard, for example:

```text
Open the Copilot attendant dashboard for all sessions in the last 7 days
Show VS Code Copilot usage for this worktree in the attendant dashboard
```

The agent opens it via `open_canvas` with `canvasId: "copilot-attendant-dashboard"` and an optional
input (all fields are optional):

| Field     | Type     | Description                                                                                     |
| --------- | -------- | ----------------------------------------------------------------------------------------------- |
| `profile` | `string` | Name the last query is remembered under for the current working directory (default `default`).  |
| `query`   | `object` | Partial query applied on open; see [Query](#query).                                             |
| `runner`  | `string` | `auto` (default), `gh` (installed gh extension only), or `source` (build from a checkout).      |
| `source`  | `string` | Tab to show first: `session` (Copilot CLI) or `vscode` (VS Code Copilot Chat).                  |

### Query

| Field     | Type      | Default    | CLI flag                                                                                       |
| --------- | --------- | ---------- | ---------------------------------------------------------------------------------------------- |
| `filters` | `object`  | none       | `session`: `operation`, `kind`, `command`, `path`, `url`; `vscode`: `tool`, `model`, `agent`   |
| `path`    | `string`  | empty      | `--cwd` when `scope` is `cwd`, `--worktree` when `scope` is `worktree`                         |
| `period`  | `string`  | `30d`      | `--period` (mutually exclusive with `since`)                                                   |
| `scope`   | `string`  | `worktree` | `worktree` (current git worktree), `all` (`--all`), or `cwd`                                   |
| `session` | `string`  | empty      | `--session`                                                                                    |
| `since`   | `string`  | empty      | `--since`                                                                                      |
| `source`  | `string`  | `session`  | `session stats` or `vscode stats`                                                              |
| `top`     | `integer` | `10`       | `--top` (`0` keeps every entry)                                                                |
| `until`   | `string`  | empty      | `--until`                                                                                      |

The `path` and `url` session filters and all `vscode` filters are regular expressions in
Go's RE2 syntax. Constructs RE2 does not support, such as lookaround and backreferences, are
rejected before the command runs.

## Dashboard

- **Tabs** — switch between Copilot CLI permission statistics and VS Code Copilot Chat usage.
  Each tab keeps its own result; switching only re-runs a tab whose result no longer matches
  the query.
- **Query bar and filters** — scope, time window, session ID, top N, and the source-specific
  repeatable filters. Filter values appear as removable chips.
- **Copilot CLI usage** — when the CLI reports it, AIU, premium requests, prompt/output tokens,
  cache hit rate, and API time from each session's `session.shutdown` event, with an AIU share
  bar and a per-working-directory table. Permission filters do not apply to usage totals.
- **Copilot CLI permissions** — sessions, permission requests, and approved/denied/unresolved totals, with
  stacked bars per result, decision source (when the CLI reports it), read-only/read-write,
  tool kind, command, path, URL, and working directory. Click a tool kind, command, path, or URL
  entry to add it as a filter, or a working directory to scope to it.
- **VS Code** — sessions, turns, LLM requests, tool calls, token and AIU totals, a usage share
  bar per model, model token breakdown (cached / uncached input / output), tool calls with
  error rates and durations, subagents, and workspaces. Click an entry to filter or scope.
- **Ask agent** — preset prompts (summarize, suggest allow rules, tool health, token cost) and a
  free-text box; the current tab's data is sent to the agent as delimited, untrusted JSON.
  Asking is only available once the current tab has a successful result for the current query.
- The footer shows the exact `gh copilot-attendant` command for the current query.

## Actions

| Action        | Description                                                                         |
| ------------- | ----------------------------------------------------------------------------------- |
| `get_data`    | Return the full JSON result last loaded for a source.                               |
| `get_summary` | Return a compact summary of the current result without re-running it.               |
| `refresh`     | Optionally patch the query, re-run the active source, and return a compact summary. |
| `reset_query` | Reset the query to its defaults and refresh.                                        |

## Storage

The last query is saved per working directory and profile in
`$COPILOT_HOME/extension-data/attendant-dashboard/queries.json` (`$COPILOT_HOME` defaults
to `~/.copilot`). It is kept outside the installed extension directory so that
`gh copilot-attendant copilot extension update` and `uninstall` do not discard it. Results are
not persisted; they are re-computed from the local logs.

## Runner resolution

With `runner: "auto"`, `$COPILOT_ATTENDANT_BIN` is executed when set; otherwise the installed
`gh copilot-attendant` is used. Only when gh reports that the extension is not installed (or `gh`
is missing) is the binary built with `go build` into a temporary directory. `runner: "source"`
always builds.

The binary is built from the first `gh-copilot-attendant` checkout (a directory whose `go.mod`
declares `github.com/srz-zumix/gh-copilot-attendant`) found in this order:

1. The repository containing the extension, when it is loaded from that repository's
   `.github/extensions/attendant-dashboard`.
2. The session's working directory, then each of its parent directories.

A user-scoped install (`$COPILOT_HOME/extensions/attendant-dashboard`) does not include the
sources, so building from source requires the session to run inside a checkout. The checkout is
built and executed as is, so only run the dashboard from checkouts you trust.

## Security

The dashboard server binds to `127.0.0.1` on an ephemeral port. API requests must carry a
per-instance token embedded in the page, a matching `Host` header, and (when sent) a matching
`Origin`. Commands are spawned without a shell, and every value is passed as a single
`--flag=value` argument.

## Development

```sh
node --test .github/extensions/attendant-dashboard/test/
```
