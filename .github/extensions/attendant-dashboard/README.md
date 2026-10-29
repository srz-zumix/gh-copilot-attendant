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

| Field     | Type      | Default    | CLI flag                                                                                                                             |
| --------- | --------- | ---------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| `filters` | `object`  | none       | `session`: `operation`, `kind`, `command`, `path`, `url`, `decisionSource` (`--decision-source`); `vscode`: `tool`, `model`, `agent` |
| `path`    | `string`  | empty      | `--cwd` when `scope` is `cwd`, `--worktree` when `scope` is `worktree`                                                               |
| `period`  | `string`  | `30d`      | `--period` (mutually exclusive with `since`)                                                                                         |
| `scope`   | `string`  | `worktree` | `worktree` (current git worktree), `all` (`--all`), or `cwd`                                                                         |
| `session` | `string`  | empty      | `--session`                                                                                                                          |
| `since`   | `string`  | empty      | `--since`                                                                                                                            |
| `source`  | `string`  | `session`  | `session stats` or `vscode stats`                                                                                                    |
| `top`     | `integer` | `10`       | `--top` (`0` keeps every entry)                                                                                                      |
| `until`   | `string`  | empty      | `--until`                                                                                                                            |

The `path` and `url` session filters and all `vscode` filters are regular expressions in
Go's RE2 syntax. Constructs RE2 does not support, such as lookaround and backreferences, are
rejected before the command runs.
VS Code Agent filters are evaluated locally with the bundled RE2JS matcher. No npm install
is required to run the canvas.

## Dashboard

- **Tabs** — switch between Copilot CLI permission statistics and VS Code Copilot Chat usage.
  Each tab keeps its own result; switching only re-runs a tab whose result no longer matches
  the query.
  The VS Code tab always shows a warning that it reads Copilot Chat debug logs from the
  Local harness, including while loading or when no data is available. Locally recorded
  VS Code Copilot harness (Agent Host) sessions use CLI-format logs and are included in
  the Copilot CLI tab. The distinction follows the selected
  [agent harness](https://code.visualstudio.com/docs/agents/run/agent-harnesses),
  not the VS Code version.
- **Query bar and filters** — scope, time window, session ID, top N, and the source-specific
  repeatable filters. Filter values appear as removable chips.
  VS Code Agent and Top changes are applied to the cached dataset without executing gh.
  Initial loading, explicit Refresh, and scope/time/session/tool/model changes fetch new
  untruncated log details using `--include-log-stats`. Top is applied after local filtering.
  The last data-fetch time is retained while local filters change; Refresh reads new logs.
  An unsupported CLI reports an error rather than falling back to per-click commands.
  Display, `get_data`, `get_summary`, and Ask agent share the same projected results.
  The footer shows an equivalent CLI command; its tooltip and summary `fetchCommand`
  identify the actual data-fetch command.
- **Panel Clear** — filterable panel headers clear only their corresponding filter group:
  VS Code Models (including model shares), Tools, and Subagents; CLI Decision sources,
  Read-only vs read-write, Tool kinds, Commands, Paths, and URLs. Workspaces, workspace
  shares, and Working directories clear directory scope to all workspaces. Other
  filters, period/time bounds, session, Top, and display preferences remain unchanged.
  Clear is disabled without an active matching condition, including panels with no rows.
  CLI model and role usage panels do not have Clear buttons because those axes are not filters.
- **Usage panel order** — both tabs show the model AIU share, Models table, workspace
  AIU share, then the Workspaces table, followed by User questions, when the corresponding
  data is available. On CLI, Agent usage follows User questions, then the Permissions
  summary uses the same bordered panel style as the other panels.
- **Copilot CLI usage** — AIU, premium requests, prompt/output tokens, cache hit rate,
  API requests, and API time, including unfinished sessions. Request usage records and
  cumulative checkpoints/shutdown snapshots are combined without double counting.
  Refresh reads the latest recorded totals; logs are not continuously polled.
  Incomplete model/request/token breakdowns are noted explicitly. Permission filters
  do not apply to usage totals. Time bounds select lifetime totals by latest usage update.
- **Copilot CLI agent usage** — an Agent usage table separates main, subagent, and unknown
  requests, premium requests, AIU, and tokens using recorded request metadata, not model
  names. Unknown includes older/unattributed snapshot usage. In incomplete breakdowns,
  request/token subtotals are marked with `≥`; `–` means unavailable, not zero usage.
- **Copilot CLI user questions** — the User questions panel reports `ask_user` counts
  and answered/cancelled/failed/unresolved/unknown outcomes. Total, average, and maximum
  answer wait use only answered pairs with valid timestamps; missing timing is noted.
  Without timed answers, average/maximum show a dash. Time bounds select question
  starts; completions after the upper bound still resolve them. Directory/session scope
  applies, but permission filters and Top do not. Elapsed tool time includes UI/transport
  overhead and sums overlapping intervals, not unique wall-clock idle time.
  Question and answer text is not exported. Older CLI payloads without `AskUser` omit
  this panel. Display, summaries, and Ask agent receive the same question metrics.
- **Copilot CLI models** — when `ByModelUsage` is available, an AIU share bar and a sortable
  Models table with sessions, API requests, premium requests, AIU, token breakdown, cache hit
  rate, and AIU per session, per API request, or per 1M tokens. Prompt tokens include uncached input,
  cache reads, and cache writes; output tokens are also included in the per-1M-token unit.
  Recorded `modelMetrics` and request usage are counted; checkpoints may contribute to
  overall usage without a model breakdown. Shares cover listed models, not all session usage;
  a session using multiple models counts under each model. Model names are not filters:
  the CLI does not support `session stats --model`.
- **Copilot CLI permissions** — sessions, permission requests, and approved/denied/unresolved totals, with
  stacked bars per result, decision source (when the CLI reports it), read-only/read-write,
  tool kind, command, path, URL, and working directory. Click a decision source, tool kind,
  command, path, or URL entry to add it as a filter, or a working directory to scope to it.
  The same panel shows human permission response wait (total/average/maximum) from
  paired request/completion timestamps. Only `human_response` decisions contribute;
  other and unknown-source decision timing stays separate. Completed/timed/untimed
  counts expose coverage; missing/negative timing and unresolved requests have no inferred
  duration. All permission filters and request start-time bounds apply; Top does not.
  UI/transport overhead is included, and overlapping waits are summed, not unique idle
  time. Older payloads without `PermissionWait` omit these metrics. Display, summaries,
  and Ask agent agree. VS Code Local harness debug logs do not independently identify
  ordinary permission request/response waits, so tool duration is not used as a substitute.
- **VS Code user questions** — the User questions panel reports completed
  `vscode_askQuestions` calls (`UserQuestions` in JSON) and answered/skipped/cancelled/
  failed/unknown outcomes. Each multi-question batch shares one interval and counts once;
  partially skipped batches with an answer count as answered. Valid answered tool
  durations contribute to total/average/maximum wait, including UI/transport overhead.
  Missing duration is noted, and average/maximum are unavailable without timed answers.
  In-flight questions cannot be reconstructed from these completed-call logs.
  Directory/session/agent/tool filters and call start-time bounds apply; model filters
  and Top do not. Agent changes project per-log question metrics locally without gh,
  deduplicating shared logs and counting each parent chat once. Display, summaries,
  and Ask agent agree. Older datasets without question metrics omit the panel.
- **VS Code** — sessions, turns, LLM requests, tool calls, token and AIU totals, a usage share
  bar per model, model token breakdown (cached / uncached input / output) with a unit-cost
  column (AIU per session, per request, or per 1M tokens), tool calls with error rates and durations,
  subagents, and workspaces. Referenced subagent logs contribute their model usage and
  activity to the parent workspace; sessions count parent chats, not child logs.
  Clicking a Subagents entry scopes models, AIU, tokens, tool calls, turns, and workspace
  totals to that subagent's own logs. Main-agent and differently named descendant usage
  is excluded; multiple Agent filters match any selected name. Removing the Agent filter
  restores the unfiltered agent scope, while other filters remain in effect. Tool and
  model filters apply independently inside the selected scope. Missing child logs produce
  warnings without dropping recorded matching invocations.
  When the CLI reports per-workspace
  usage, workspaces also get an AIU share bar and a table of sessions, AIU (total and per
  turn), and token breakdown. Click an entry to filter or scope.
- **Model comparison** — in either tab, click any Models column heading to sort and click
  again to reverse the order. Numeric columns start descending, model names ascending;
  undefined ratios stay last. Sorting applies to listed models only (set Top to 0 to
  include all models). Unit and sort preferences are independent for each tab.
  Both tabs offer Session, Request (default), and 1M tokens as unit-cost denominators.
  AIU per session uses each model's recorded `Sessions` count, not the overall session
  count. Missing session counts or zero denominators show a dash; missing counts also
  display an explanation. Current VS Code statistics do not report model session counts.
- **Workspace comparison** — both tabs use the Workspaces title and compare AIU per session,
  per API request, or per 1M tokens (default: session), with the same unit controls and
  comparison bars as Models. CLI request counts come from `ByCWDUsage.Requests`, not
  permission requests or premium requests; VS Code uses `ByWorkspace.LLMRequests` and
  `ByWorkspace.Sessions`. Token units use prompt plus output, with the same cache accounting
  as Models. Missing counts and zero denominators show a dash, not zero cost.
  Click any metric column heading to sort and click again to reverse; the default is AIU
  descending. Unknown values stay last. Sorting covers listed workspaces only (set Top to 0
  to include all). Each tab's Workspaces preferences are independent of Models.
  Older CLI builds without workspace usage retain the VS Code activity-only table.
- **Ask agent** — preset prompts (summarize, suggest allow rules, tool health, token cost) and a
  free-text box; the current tab's data is sent to the agent as delimited, untrusted JSON.
  Asking is only available once the current tab has a successful result for the current query.
- The footer shows the `gh copilot-attendant` command for the current query, with each argument
  quoted for a POSIX shell (not PowerShell or `cmd.exe`) so it can be copied and pasted safely.

## Actions

| Action        | Description                                                                         |
| ------------- | ----------------------------------------------------------------------------------- |
| `get_data`    | Return the full JSON result projected for the current filters, without raw log details. |
| `get_summary` | Return a compact summary of the current result without re-running it.               |
| `refresh`     | Optionally patch the query and return a summary. Default `force: true` fetches; `force: false` reuses matching data and locally projects VS Code Agent/Top changes. |
| `reset_query` | Reset the query to its defaults and refresh.                                        |

## Storage

The last query is saved per working directory and profile in
`$COPILOT_HOME/extension-data/attendant-dashboard/queries.json` (`$COPILOT_HOME` defaults
to `~/.copilot`). It is kept outside the installed extension directory so that
`gh copilot-attendant copilot extension update` and `uninstall` do not discard it. Results are
not persisted; VS Code details stay in memory until a data refresh or extension restart.
The normalized dataset contains no raw events, tool inputs, or conversation content.

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
`Origin`. The page that carries the token is served only at an unguessable per-instance URL
handed to the canvas; `/` and `/index.html` return 404. Commands are spawned without a shell, and every value is passed as a single
`--flag=value` argument.

## Development

```sh
node --test .github/extensions/attendant-dashboard/test/*.test.mjs
```

To regenerate the standalone matcher and its license from the pinned dependency, run
these commands from the repository root:

```sh
npm ci
npm run vendor:canvas
```
