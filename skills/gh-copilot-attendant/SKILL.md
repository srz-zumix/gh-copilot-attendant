---
name: gh-copilot-attendant
description: Basic skill package for the gh-copilot-attendant GitHub CLI extension.
allowed-tools: Bash(git status:*), Bash(git branch --show-current:*), Bash(gh copilot-attendant skills:*), Bash(gh copilot-attendant copilot extension:*), Bash(gh copilot-attendant metrics report:*), Bash(gh copilot-attendant session stats:*), Bash(gh copilot-attendant vscode stats:*)
---

# gh-copilot-attendant

Use `gh-copilot-attendant` to inspect local Copilot usage, explore the interactive
dashboard, and generate shareable organization or enterprise metrics reports.
The [README](../../README.md) provides the overview and quick start; the
[usage guide](../../docs/usage.md) contains detailed commands, defaults, and
accounting semantics.

## Commands

### copilot extension install / update / uninstall / list / status

Manages the bundled Copilot CLI canvas extension (`attendant-dashboard`). See the
[usage guide](../../docs/usage.md#copilot-extension-install) for the full flag reference.

### metrics report

Reads the Copilot usage metrics API of an organization (`--org`) or enterprise
(`--enterprise`) and renders a shareable summary for people who cannot view the
usage metrics dashboard. Requires usage metrics API access (e.g. an org owner)
and the "Copilot usage metrics" policy. Uses the latest 28-day report by
default, or 1-day reports with `--day` or `--since`/`--until`. `--section`
selects kpi, trend, feature, language, model, ide, repository, team, and user
(user is opt-in). Active-user metrics are never summed across days; missing
values are `n/a`, and missing days are reported as partial data (`--strict`
fails instead). `--format md|html` renders Markdown with mermaid charts or
self-contained HTML with collapsible sections and tables. JavaScript enables
table sorting and numeric heat colors. Sections are expanded by default; tables
are expanded in normal HTML and collapsed with `--rich`. In either HTML mode,
hovering or focusing a day in a time-series chart reveals its available points
and tooltip; missing days remain empty. Stacked-area points mark cumulative
boundaries, while tooltips show each series' share. Without JavaScript, browsers
supporting CSS `:has()` show a simplified tooltip on hover or focus, placed
next to the target where CSS anchor positioning is supported; it can be scrolled
with the pointer. Each chart's **Chart values** disclosure lists the full
values. With
JavaScript enabled, printing temporarily expands all sections and tables, then
restores their screen state. `--rich` adds KPI cards and breakdown charts to
HTML and requires `--format html`.
The per-user section, including login names, is included only with
`--section user` or `--section all`, in either mode. Model usage per chat
mode and Model usage per language detail tables are present in Markdown and
HTML without requiring `--rich`. They list every reported model/chat-mode or
language/model pair regardless of `--top`; counts sum available values, and
shares use valid nonnegative counts in the corresponding detail breakdown.
Missing counts and zero-denominator shares are `n/a`. Empty names appear as
`(not reported)`; Other and Unknown names remain separate in the tables.

Requests per chat mode appears below Average chat
requests per active user as a daily stacked bar chart using
`user_initiated_interaction_count` for `chat_panel_*_mode` entries in each
day's `totals_by_feature`, including Plan. Missing counts remain `n/a`; incomplete days are marked with `*` and
show a subtotal of known counts (or `n/a` if none), without total shares. When
no chat-mode feature is reported, the chart is omitted. Chart areas, points, and
bars show labels and values in tooltips on hover or keyboard focus. Daily
chart tooltips show all series for that day, with missing values shown as
`n/a`. Stacked-bar tooltips list every displayed series and, when complete, the
bar total with counts and shares; missing values are `n/a`. Tooltips omit chart
titles. With JavaScript, they align numeric columns, and long tooltips can be
scrolled over the chart or with arrow keys while focused. Model usage per day
and Language usage per day are 100% stacked area charts, using each day's
reported model interaction or language activity total as 100%. Zero-total days
have no area. Boundaries are smoothly interpolated between days with data;
missing series taper to no area at their day, but remain `n/a` in tooltips. These
area charts do not use the section theme; named series are colored by a hue
gradient from blue toward red and purple, ordered by total over the report
period, with gray for Other.
Language usage uses a percentage pie
chart, grouping languages below 1%
into Other; exactly 1%
stays separate, and tooltips retain counts. Model usage per language uses a
vertical stacked bar chart titled "Model usage per language (code-generation
activities)"
(horizontal axis: languages; stacks: models; vertical axis: percentage of
code-generation activities across all languages). Its axis maximum follows the
largest language total percentage, rounded up to a whole percentage point
(minimum 1%). Other, Others, and Unknown (including Unkown) are combined as
Other in these charts; the Other language bar is placed rightmost. Chat model
usage (interactions) is a percentage pie chart based on model interactions;
shares below 1% and unknown models are grouped into Other, with counts in
tooltips. Additive activity charts are capped at seven categories (the palette
size), including Other; retain the largest totals and combine the rest. Stacked
charts cap their breakdown segments separately. IDE user series remain separate
because users can overlap.

Model usage per language keeps the top five languages by activity total plus
Other, and appears beside the language usage pie chart (vertically on narrow
screens). By team includes paired
Team usage pie charts for user-initiated interactions and code-generation
activities, stacking vertically on narrow screens. Each independently uses all
reported teams regardless of `--top`, groups shares below 1% and excess
categories into Other, and shows at most seven categories including Other.
Tooltips show reported counts and shares of activity attributed using same-day
team membership. Multi-team users count in each team, so shares are not an
organization-wide breakdown. Ambiguous team labels include team IDs. Missing
and zero counts have no slices; charts without positive counts are omitted,
and a single available chart uses the full width. By user includes rose-themed
paired charts using all reported users regardless of `--top`: Interaction per
active days (top seven, reported interactions divided by days with a user
record, including zero-interaction days) beside users by
surface (vertical bars; users may count in several), then reported days
distribution (`1`, `2–3`, `4–7`, `8–14`, `15+` days) beside acceptance rate
distribution (`<20%` to `≥80%`, including rates above 100%). Missing interaction values are excluded
from totals; users without reported interactions are omitted. By repository includes
horizontal stacked bars for reviewed, created, and merged pull requests. The
reviewed chart compares PRs reviewed with Copilot to those without a Copilot
review; created and merged charts compare Copilot-created PRs with
other-created PRs. Their tooltips show counts and shares. Copilot review
suggestions by repository compares generated and applied counts in separate
bars, not as an acceptance rate; the events may concern different suggestions.
Partial period totals are marked
and show only known counts. Copilot reviews
per day by repository stacks daily counts by repository. Available days with
no repository activity count as zero; missing reports or values remain `n/a`.
Incomplete days are marked, and their tooltips show a displayed subtotal
without shares. Each chart independently shows the top seven repositories plus
Other, using all reported repositories regardless of `--top`. Review counts
sum daily reports, so a PR reviewed on multiple days can be counted more than
once. Model usage per chat mode is a vertical stacked percentage chart using
only `chat_panel_*_mode`
entries in the model/feature breakdown (Agent, Ask, Edit, Plan, Custom, and
unknown as Other). The horizontal axis is model; percentages use the total
interactions across all chat modes. Inline chat and other non-mode features are
excluded. The chart retains the top five models by chat-mode interaction total
plus Other, beside the Chat model usage (interactions) pie chart (vertically on
narrow screens).

Blue numeric table backgrounds encode relative values within each
column. Bars are not normalized separately per language. Pie slices and legend
entries also support tooltips. User-level reports are also fetched when
selected team data or aggregate IDE active-user counts require them. The IDE
active users card estimates distinct users with a reported IDE from the first
day of the calendar month through the report end day, deduplicating across
days and IDEs. It can differ from GitHub's dashboard when IDE details are
unavailable. Daily user reports outside the selected period are fetched when
needed for this card only and do not change the user or team tables. Missing
days make the estimate partial, or `n/a` when no day is available; `--strict`
fails if these reports are missing. Inline completion charts use only the
`code_completion` feature: suggested and
accepted counts, plus accepted / suggested as a percentage. The charts appear
side by side (vertically on narrow screens) and exclude chat and agent
activity. Use `--output-dir` for multiple formats.
`--create-issue` or `--issue` attaches the reports as issue artifacts in
`--repo` (public repositories need `--allow-public`); artifacts with the same
name are updated. Artifact names replace unsupported characters with spaces
to meet GitHub's naming restrictions; issue titles are unchanged.

`--pages --format html` publishes HTML and a history index to an existing
Organization-owned private/internal project repository on GitHub Enterprise
Cloud. `--repo` is optional and defaults to the current repository; the
destination must have an initial commit and cannot be an Organization site
(`<org>.github.io`). Pages publication cannot be combined with `--create-issue`,
`--issue`, or `--allow-public`. It does not create the repository.

If the destination has no Pages site, `--pages` initializes it: the CLI creates
an isolated `copilot-metrics-pages` branch with no inherited files, or reuses
it only when it already holds a valid managed history (so re-running resumes an
interrupted setup). It verifies private visibility before enabling the branch
source and checks the managed site before fetching metrics; an empty or
placeholder site may briefly be public during setup. Unrelated existing sites
and unmanaged branches are rejected, not overwritten. Failed initialization may
leave resources to inspect before retrying.
Initialization needs repository administration access and Pages write
permission; publication needs Contents write and Pages read permission.
Organization policies, SSO authorization, and branch rules still apply.

Only repository readers can view the private site; internal repositories can
have a wider Enterprise audience. Reports may include repository and team names
by default and login names with the user section. Keep visibility and publishing
settings restricted: pre-commit checks cannot prevent later administrator
changes. History keeps one entry per source host, Organization/Enterprise, and
date range; reruns update that entry without a tool-imposed history limit.
Previous contents remain in Git history, and GitHub's limits still apply.
Pages with multiple formats requires `--output-dir` to retain Markdown locally.
Success reports the source URL/branch/commit, not verified deployment; check
Pages separately. A workflow's `GITHUB_TOKEN` does not trigger branch-based
Pages builds.

```sh
gh copilot-attendant metrics report --org example-org --format html --rich \
  --pages --repo example-org/private-reports
```

Per-user data may be sensitive, so confirm before requesting
`--section user`, `--section all`, or publishing. See the
[usage guide](../../docs/usage.md#metrics-report) for the full flag reference.

### session stats

Reports tool, path, and URL permission statistics, plus per-session usage totals (premium
requests, AIU, token counts) from local GitHub Copilot CLI session history. Usage is broken
down by working directory and model, including unfinished sessions. Request usage records
and cumulative checkpoints/shutdown snapshots are combined without double counting.
Model request counts are separate from permission requests. The model breakdown uses
request records or `modelMetrics` and appears as `MODEL_USAGE` / `ByModelUsage`.
`AGENT_USAGE` / `ByRoleUsage` separates main, subagent, and unknown usage from request
metadata; do not infer roles from model names. Checkpoint-only usage may have no model,
request, or token breakdown. `UsageIncompleteSessions` flags incomplete breakdowns,
whose request/token counts are recorded subtotals. `UsageRequests` is the API request
total. Usage time bounds select recorded updates; cumulative snapshots are
differenced against earlier recorded totals and may include usage before the
selected period. Role usage is never truncated by `--top`.
`FUSION` / `Fusion` counts recorded resolved turns by pattern and usage records
by pattern, phase, role, and usage model. A phase count requires both a Fusion
ID and phase ID; unidentified records and incomplete AIU coverage are shown
separately. AIU is a recorded subtotal, not inferred from checkpoints or planned
phases. A turn or phase can appear in multiple rows. Directory/session scope
applies, and time bounds select each Fusion event independently; events without
timestamps are excluded when a time bound is set. Permission filters do not
apply. `--top` limits Fusion breakdowns, not totals. Older or
incomplete logs cannot establish non-use of Fusion. Active sessions may change
while scanning, so shared and Fusion totals are not an atomic snapshot.
JSON `Fusion.Usage` version 1 contains independently aggregated totals and
model, workspace, and request-role groups for explicitly tagged usage records.
Each metric reports observed, complete, and excluded record counts; missing
values are not zero. Premium requests use multipliers for user or unspecified
initiators, following the shared collector; other initiators are excluded.
See the
[usage guide](../../docs/usage.md#session-stats) for the full flag reference.

### vscode stats

Reports tool usage, LLM token/usage, turn, and subagent statistics from VS Code GitHub
Copilot Chat debug logs for the Local harness, including recursively referenced
subagent logs counted once each. Child activity contributes to the parent workspace
without increasing parent session counts. Internal title-generation child logs and
unreferenced files are excluded.
Time bounds apply to individual events, even when the child invocation is older.
`--agent` scopes models, AIU, tokens, tools, turns, and workspace totals to matching
subagents' own logs. Main-agent and differently named descendant usage is excluded;
multiple patterns include any matching name. Without `--agent`, all parent and referenced
child activity is included. Tool and model filters apply independently within that scope.
Missing child logs produce warnings without dropping recorded matching invocations.
`--include-log-stats` (optional, default false; incompatible with `--agent`) exports
versioned, untruncated normalized per-log metrics and parent/invocation identities in JSON.
No raw events or conversation content are exported.
See the [usage guide](../../docs/usage.md#vscode-stats)
for the full flag reference.

VS Code Copilot harness (Agent Host) sessions recorded locally under
`~/.copilot/session-state` use CLI-format logs and are included in `session stats`,
not `vscode stats`. The source depends on the harness, not the VS Code version.

## Dashboard canvas

The dashboard defaults Top to 20; Reset and an empty Top field also use 20.
Previously saved Top values are preserved.

In the GitHub Copilot app, the `copilot-attendant-dashboard` canvas (provided by
`.github/extensions/attendant-dashboard` in this repository) visualizes the same
`session stats` and `vscode stats` results. See its
[README](../../.github/extensions/attendant-dashboard/README.md) for open inputs and actions.

Period offers This month and Last month using the browser's local calendar
boundaries. This month runs from month start to now; Last month ends exclusively
at this month's start. Apply saves concrete Since/Until timestamps, not a rolling
calendar preset. Editing either bound clears the calendar selection; choosing a
rolling period clears both bounds, while None preserves them.

The VS Code tab always displays a harness warning, even while loading or without
data. It reads Copilot Chat debug logs from the Local harness. Locally recorded
VS Code Copilot harness (Agent Host) sessions are included in the Copilot CLI tab.
The tab depends on the harness, not the VS Code version.

Click a VS Code Subagents entry to scope all usage to that subagent's own logs, excluding
the main agent and differently named descendants. Remove the Agent filter chip to return
to the unfiltered agent scope; other filters remain in effect.
VS Code Agent and Top changes project the cached dataset locally without invoking gh.
Initial loading, explicit Refresh, and scope/time/session/tool/model changes fetch new
log details. Parent chats and shared logs are deduplicated before Top is applied.
`get_data`, `get_summary`, and Ask agent all use the same filtered projection. The displayed
command is an equivalent CLI command; `fetchCommand` in summaries is the actual data fetch.
Use `refresh` with `force: false` and a query patch to change Agent filters without fetching;
the action's default `force: true` deliberately refreshes the dataset.

On the Copilot CLI tab, enable **Fusion usage only** or set query
`fusionOnly: true` to include only usage records explicitly tagged as Fusion,
not entire sessions. The view and agent-facing data exclude permissions, user
questions, resolution events, planned phases, and checkpoint-only usage.
Permission filters are retained but inactive until the switch is off; the
switch does not affect VS Code. Unknown model/workspace values stay unknown.
Role groups use request provenance, not Fusion phase roles; missing agent
provenance remains unknown. Coverage distinguishes
missing from recorded zero, and usage records are not necessarily distinct API
calls. Per-session, per-record, and per-1M-token costs require complete inputs.
Model and workspace share bars compare recorded AIU subtotals for listed groups;
missing AIU is excluded, so incomplete coverage can affect proportions.
The cards and Models, Workspaces, and Agent usage tables follow the normal
view's order and notation where comparable, but usage records are not API
requests. Both views show cache read, cache write, and uncached input totals.
AIU and AIU unit costs use at most two decimal places in both CLI and VS Code
views; positive values below 0.01 display as `<0.01`.
Counts retain source-specific semantics. Fusion model and workspace token bars require complete prompt, cache read,
cache write, uncached input, and output totals, with prompt equal to the sum
of its components. Cache hit requires complete prompt and cache-read totals
and a valid nonzero denominator; otherwise it is unavailable.
Token counts use the normal view's compact notation. Hover over a token total
for its exact recorded count and coverage. Partial token, AIU, and API-time
subtotals are formatted without overstating their `≥` values.
Top independently limits rows, not totals. Models use the normal view's unit
selector and cost comparison bars, with per-session, per-usage-record (default),
or per-1M-token costs. Click a heading to sort; Top selects models by recorded
AIU before local sorting. Model unit and sort preferences persist separately
from the normal view. Workspaces also use the per-session, per-usage-record, or
per-1M-token unit selector and comparison bars, with separate unit and sort
preferences from Models and the normal view. Sorting applies to workspace
groups selected by Top.
Matching cached data is
projected immediately; scope/time/session/Top changes or Refresh fetch again.
`get_data`, `get_summary`, and Ask agent share the projection; the permission
suggestion preset is unavailable. The displayed command fetches unfiltered
stats, so it is not equivalent to the local Fusion filter. Unsupported CLI
builds fail explicitly; update the CLI or use the `source` runner.

In the normal view, both tabs have a Models table comparing AIU per session, per API request (default),
or per 1M tokens. Session units use each model's recorded `Sessions` count, never
the overall session count. Missing counts or zero denominators show a dash;
missing counts also display an explanation. Current VS Code statistics do not
report model session counts.
In the
normal CLI view, User questions precedes Agent usage and the Copilot Fusion
panel; Fusion-only excludes user questions and permissions.
The CLI tab also shows Agent usage (main / subagent / unknown) and incomplete-usage
notices. Normal-view agent unit costs require available row inputs and no
reported incomplete session breakdowns; this is not per-record coverage.
Fusion-only costs require complete record coverage. Fusion-only model and agent
tables include recorded API time; the normal model and agent tables omit it
because comparable per-record time coverage is unavailable. The normal
collector does not expose Fusion's per-metric record coverage. Click any Agent
usage heading to sort and click again to reverse. The
normal view defaults to role ascending and ignores Top. The Fusion-only view
defaults to recorded AIU descending; Top selects role groups by recorded AIU
before sorting, so set Top to 0 for all groups. Unavailable values sort last
and partial values use their recorded subtotal. Sort preferences persist across
reloads independently for the normal and Fusion-only views. Refresh reads the
latest recorded totals; it does not continuously poll logs.
Click any column heading to sort the listed models, and click again to reverse
the order. Set Top to 0 to include all models in the comparison.

In the normal view, both Workspaces tables compare AIU per session, per API request, or per 1M tokens
(default: session), with comparison bars and clickable metric headings. The default
sort is AIU descending; missing metrics and zero denominators show a dash and sort
last. Set Top to 0 to include all workspaces. Preferences are independent per tab
and per table.
Filterable panels have a Clear button that clears only their corresponding filter
group. Workspaces, workspace shares, and Working directories clear scope to all
workspaces. Other filter groups, period/time bounds, session, and Top are preserved.
Buttons are disabled when no matching condition is active. CLI model, role
usage, and Fusion panels have no Clear button because those axes do not support
filtering.

CLI workspace request counts use `ByCWDUsage.Requests`, never permission or premium
requests. VS Code workspace sessions use `ByWorkspace.Sessions`, and API request
counts use `ByWorkspace.LLMRequests`. Older builds may lack these fields; do not
infer them from tool calls or turns. Workspace token accounting matches Models.

CLI models use `ByModelUsage` from recorded `modelMetrics` or request usage; checkpoints
may have usage totals without a model breakdown. CLI prompt tokens include uncached
input plus cache reads and writes; VS Code input already includes cached tokens.
Model session counts can overlap, and AIU shares cover listed models only.
CLI model names cannot be used as filters (`session stats` has no `--model` flag).

The CLI Copilot Fusion panel and agent summaries use the `Fusion` object from
`session stats`. They distinguish resolved turns from turns and phases with
recorded usage, show AIU record coverage and incomplete identifiers, and do not
infer missing activity. Rows may share turns or phases. Click a Phase usage
column heading to sort and click again to reverse; record count descending is
the default. AIU coverage sorts by the fraction of records reporting AIU;
unavailable AIU and undefined coverage stay last. Sorting applies to the rows
selected by Top, which ranks phase groups by record count. Set Top to 0 for all
groups. The preference persists across reloads and is shared between normal and
Fusion-only views. Older CLI builds omit the panel; use an updated CLI or the
canvas `source` runner to view it.

CLI `session stats` also reports `ASK_USER` (`AskUser` in JSON), and the CLI canvas shows
a User questions panel. Start/completion events are paired by session, agent, and tool
call identity; replayed events count once. Outcomes distinguish answered, cancelled,
failed, unresolved, and unknown. Only valid answered pairs contribute to total/average/
maximum wait in milliseconds. `TimedAnswers` and `UntimedAnswers` expose missing timing;
average/maximum are null without timed answers. Elapsed tool time includes UI/transport
overhead and sums overlapping waits, not unique wall-clock idle time. Question starts
are selected by time bounds, and later completions still resolve them. Session/directory
scope applies; permission filters and Top do not. No question/answer text is exported.
Older payloads without `AskUser` do not display invented question metrics.

CLI `PERMISSION_WAIT` (`PermissionWait` in JSON) also reports request-to-completion
elapsed time in the Permissions panel, separately from User questions. `HumanResponses`
contains only recorded `human_response` decisions. `NonHumanDecisions` and
`UnknownSourceDecisions` keep other and missing-source decision times separate; never
infer human wait from result or duration. Each group exposes completed/timed/untimed
counts and total/average/maximum milliseconds; average/maximum are null without timed
decisions. Invalid timing and unresolved requests have no inferred duration. Replayed
events count once by session/agent/request identity. All permission filters, session/
directory scope, and request start-time bounds apply; Top does not. Later completions
still resolve selected requests. Intervals include UI/transport overhead and sum overlaps,
not unique idle time. Older payloads without this field omit the metrics.
VS Code Local harness debug logs do not independently identify ordinary permission
request/response waits; confirmation-tool or operation duration is not a substitute
for human approval wait.

VS Code `vscode stats` also reports `USER_QUESTIONS` (`UserQuestions` in JSON), shown in
the VS Code canvas's User questions panel. `Calls` counts completed `vscode_askQuestions`
tool invocations, not individual questions in a batch. Outcomes distinguish answered,
fully skipped, cancelled, failed, and unknown calls. Partially skipped batches with an
answer count as answered. Only answered calls with valid recorded duration contribute
to total/average/maximum wait; missing timing is recorded in `UntimedAnswers`.
Average/maximum remain null without timed answers. In-flight questions are unavailable.
Directory/session/agent/tool filters and call start-time bounds apply; model filters and
Top do not. Agent filtering projects untruncated per-log metrics locally, deduplicates
shared logs, and counts each parent chat once. UI, summaries, and Ask agent agree.
Older datasets without `UserQuestions` omit the panel rather than inventing zero counts.
Elapsed tool duration includes UI/transport overhead and sums overlapping intervals,
not unique wall-clock idle time. Question and answer text is not exported.
