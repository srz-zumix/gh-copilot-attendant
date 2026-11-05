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

| Field | Type | Default | CLI flag |
| --- | --- | --- | --- |
| `filters` | `object` | none | `session`: `operation`, `kind`, `command`, `path`, `url`, `decisionSource` (`--decision-source`); `vscode`: `tool`, `model`, `agent` |
| `fusionOnly` | `boolean` | `false` | Locally show only explicitly Fusion-tagged usage records on the Copilot CLI tab; no CLI flag |
| `path` | `string` | empty | `--cwd` when `scope` is `cwd`, `--worktree` when `scope` is `worktree` |
| `period` | `string` | `30d` | `--period` (mutually exclusive with `since`) |
| `scope` | `string` | `worktree` | `worktree` (current git worktree), `all` (`--all`), or `cwd` |
| `session` | `string` | empty | `--session` |
| `since` | `string` | empty | `--since` |
| `source` | `string` | `session` | `session stats` or `vscode stats` |
| `top` | `integer` | `20` | `--top` (`0` keeps every entry) |
| `until` | `string` | empty | `--until` |

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
  identify the actual data-fetch command for VS Code local filtering.
- **Calendar periods** — Period includes **This month** (month start to now) and
  **Last month** (previous month start up to, but excluding, this month start).
  Boundaries use the browser's local time zone and populate Since/Until as exact
  timestamps. Apply submits these concrete dates; saved queries keep those dates,
  rather than advancing automatically next month. Editing either bound returns
  the calendar selection to None. Selecting a rolling period clears both bounds;
  selecting None preserves them.
- **Fusion usage only** — the Copilot CLI filter projects only explicitly
  Fusion-tagged usage records, not all usage in sessions that used Fusion.
  Permission statistics, user questions, resolution events, planned phases, and
  checkpoint-only usage are excluded. Existing permission filters are retained
  but inactive, and are restored when the switch is turned off. The switch has
  no effect on the VS Code tab. The filtered view groups records by model,
  workspace, agent role, and Fusion phase, with usage-only session counts.
  Records without timestamps are excluded when a time window is selected.
  Missing models or workspaces remain unknown; roles come from request
  provenance, not Fusion phase roles; missing agent provenance remains unknown.
  `≥` marks a recorded subtotal and `–`
  means unavailable, not zero. Metric coverage reports complete, observed, and
  excluded records. Premium requests sum cost multipliers for user or
  unspecified initiators, following the shared collector; other initiators are
  excluded. Unit costs are shown only when their inputs are
  complete, and use usage records rather than distinct API calls.
  Model and workspace share bars compare recorded AIU subtotals for listed
  groups; missing AIU is excluded, so incomplete coverage can affect proportions.
  The cards and Models, Workspaces, and Agent usage tables follow the normal
  view's order and notation where comparable, while keeping usage records
  distinct from API requests. Cache read, cache write, and uncached input
  totals appear in both views. Fusion model and workspace token bars require
  complete prompt, cache read, cache write, uncached input, and output totals,
  with prompt equal to the sum of its components. Cache hit requires complete
  prompt and cache-read totals and a valid nonzero denominator; otherwise it is
  unavailable.
  Token counts use the same compact notation as the normal view. Hover over
  a token total for its exact recorded count and coverage. Partial token,
  AIU, and API-time subtotals are formatted without overstating their `≥` values.
  Top limits each breakdown independently, not totals. Models use the same
  unit selector and cost comparison bars as the normal view: per session,
  per usage record (default), or per 1M tokens. Click a heading to sort the
  selected rows; Top selects models by recorded AIU before local sorting.
  Model unit and sort preferences persist independently of the normal view.
  Workspaces use the same unit selector and cost comparison bars as Models:
  per session, per usage record, or per 1M tokens. Their unit and sort
  preferences are stored independently from Models and the normal view;
  sorting applies to the groups selected by Top. A matching
  cached result is projected immediately without running gh;
  Refresh and scope/time/session/Top changes fetch again. Display, `get_data`,
  `get_summary`, and Ask agent use the same projection. An older or unsupported
  CLI reports an explicit error; update it or use the `source` runner. The
  footer shows the unfiltered data-fetch command, not an equivalent filtered CLI
  command.
- **Panel Clear** — filterable panel headers clear only their corresponding filter group:
  VS Code Models (including model shares), Tools, and Subagents; CLI Decision sources,
  Read-only vs read-write, Tool kinds, Commands, Paths, and URLs. Workspaces, workspace
  shares, and Working directories clear directory scope to all workspaces. Other
  filters, period/time bounds, session, Top, and display preferences remain unchanged.
  Clear is disabled without an active matching condition, including panels with no rows.
  CLI model, role, and Fusion panels do not have Clear buttons because those
  axes are not filters.
- **Usage panel order** — both views show the model AIU share, Models table,
  workspace AIU share, then the Workspaces table, when data is available. In
  the normal CLI view, User questions and Agent usage follow, then the Fusion
  panel and Permissions summary. Fusion-only excludes user questions and
  permissions. CLI, Fusion-only, and VS Code summary cards use the same compact,
  responsive, equal-width layout and start with sessions, AIU, and a request or
  record count. Usage card titles stay on one line; other content may wrap.
  Within each card row, title height grows only when a title wraps; titles
  bottom-align and values stay aligned without extra space for single-line rows.
  Count semantics remain source-specific.
  Table column headings can wrap to two lines to reduce horizontal scrolling.
  Panels use the same bordered style as the other panels.
- **Copilot CLI usage** — AIU, premium requests, prompt/output tokens, cache
  read/write, uncached input, cache hit rate, API requests, and API time,
  including unfinished sessions. Across CLI and VS Code, AIU totals and unit
  costs use at most two decimal places; positive values below 0.01 display as
  `<0.01`. Normal and Fusion-only cards use the same order but retain their
  different request/record counting semantics. The normal collector does not
  expose per-metric record coverage, so its aggregate totals do not imply
  Fusion-level completeness. Request usage records and cumulative
  checkpoints/shutdown snapshots are combined without double counting.
  Refresh reads the latest recorded totals; logs are not continuously polled.
  Incomplete model/request/token breakdowns are noted explicitly. Permission filters
  do not apply to usage totals. With Fusion-capable CLI builds, time bounds
  select recorded usage updates; cumulative snapshots are differenced against
  earlier recorded totals and may include usage before the selected period.
- **Copilot CLI agent usage** — an Agent usage table separates main, subagent,
  and unknown sessions, requests, premium requests, AIU, and tokens using
  recorded request metadata, not model names. Unknown includes
  older/unattributed snapshot usage. In incomplete breakdowns, request/token
  subtotals are marked with `≥`; `–` means unavailable, not zero usage.
  Normal-view per-session, per-request, and per-1M-token AIU costs require
  available row inputs and no reported incomplete session breakdowns; this is
  not per-record coverage. Fusion-only costs require complete record coverage.
  The Fusion-only model and agent tables also show recorded API time; the normal
  model and agent tables omit it because comparable per-record time coverage is
  unavailable. Click any column heading to sort and click again to reverse. The
  normal view defaults to role ascending and is not limited by Top. The
  Fusion-only view defaults to recorded AIU descending; Top selects role
  groups by recorded AIU before local sorting, so set Top to 0 for all groups.
  Unavailable values sort last in either direction, and partial values sort by
  their recorded subtotal. Preferences persist across reloads and are
  independent for the normal and Fusion-only views.
- **Copilot Fusion** — resolved turns are grouped by recorded pattern, and
  usage records by pattern, phase, role, and usage model. Distinct turn and
  phase counts require their recorded IDs; phase counts require both a Fusion
  ID and a phase ID. A turn or phase may appear in multiple rows. AIU is a
  subtotal from request records, with coverage shown: `≥` marks a partial
  subtotal and `–` means no record reported AIU. Checkpoint-only usage and
  planned phases are not attributed. Missing Fusion metadata does not prove
  that Fusion was not used. Click any Phase usage column heading to sort, and
  click again to reverse the order. The default is record count descending;
  AIU coverage sorts by the fraction of records reporting AIU. Unavailable AIU
  and undefined coverage sort last in either direction. Sorting applies only
  to the rows selected by Top, which ranks phase groups by record count; set
  Top to 0 for all groups. The sort preference is shared by the normal and
  Fusion-only views and persists across reloads. Time bounds select resolution
  and usage timestamps separately; permission filters do not apply, and Top
  limits rows but not totals. The same information is available to `get_data`, `get_summary`, and
  Ask agent. Older CLI builds without `Fusion` omit the panel; use an updated
  CLI or the `source` runner to view it. Active sessions may change while
  scanning, so shared and Fusion totals are not an atomic snapshot.
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
- **Model comparison** — in the normal view on either tab, click any Models column heading to sort and click
  again to reverse the order. Numeric columns start descending, model names ascending;
  undefined ratios stay last. Sorting applies to listed models only (set Top to 0 to
  include all models). Unit and sort preferences are independent for each tab.
  Both tabs offer Session, Request (default), and 1M tokens as unit-cost denominators.
  AIU per session uses each model's recorded `Sessions` count, not the overall session
  count. Missing session counts or zero denominators show a dash; missing counts also
  display an explanation. Current VS Code statistics do not report model session counts.
- **Workspace comparison** — in the normal view, both tabs use the Workspaces title and compare AIU per session,
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
  The permission-suggestion preset is unavailable in Fusion-only mode.
- The footer shows the `gh copilot-attendant` command for the current query, with each argument
  quoted for a POSIX shell (not PowerShell or `cmd.exe`) so it can be copied and pasted safely.

## Actions

| Action | Description |
| --- | --- |
| `get_data` | Return the current filtered JSON result. |
| `get_summary` | Summarize the current result without re-running it. |
| `refresh` | Optionally patch the query and return a summary. |
| `reset_query` | Reset the query to its defaults and refresh. |

`get_data` excludes raw log details. `refresh` fetches by default
(`force: true`); `force: false` reuses matching results and locally projects
VS Code Agent/Top or Copilot CLI `fusionOnly` changes.

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
