---
name: gh-copilot-attendant
description: Basic skill package for the gh-copilot-attendant GitHub CLI extension.
allowed-tools: Bash(git status:*), Bash(git branch --show-current:*), Bash(gh copilot-attendant skills:*), Bash(gh copilot-attendant copilot extension:*), Bash(gh copilot-attendant metrics report:*), Bash(gh copilot-attendant session stats:*), Bash(gh copilot-attendant vscode stats:*)
---

# gh-copilot-attendant

This is a minimal skill bundle for `gh-copilot-attendant`.

## Commands

### copilot extension install / update / uninstall / list / status

Manages the bundled Copilot CLI canvas extension (`attendant-dashboard`). See the
[README](../../README.md#copilot-extension-install) for the full flag reference.

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
self-contained HTML with collapsible sections and sortable, color-coded
tables. Sections are expanded by default; tables are expanded in normal HTML
and collapsed with `--rich`. In either HTML mode, hovering or focusing a day in
a time-series chart reveals its available points and tooltip; missing days
remain empty. Stacked-area points mark cumulative boundaries, while tooltips
show each series' share. With JavaScript enabled, printing temporarily expands
all sections and tables, then restores their screen state. `--rich`
adds KPI cards and breakdown charts to HTML and requires `--format html`.
The per-user section, including login names, is included only with
`--section user` or `--section all`, in either mode. Model usage per chat
mode and Model usage per language detail tables are present in Markdown and
HTML without requiring `--rich`. They list every reported model/chat-mode or
language/model pair regardless of `--top`; counts sum available values, and
shares use valid nonnegative counts in the corresponding detail breakdown.
Missing counts and zero-denominator shares are `n/a`. Empty names appear as
`(not reported)`; Other and Unknown names remain separate in the tables.

KPI cards and paired pie/stacked charts sit side by side when space allows and
stack on narrow screens. Requests per chat mode appears below Average chat
requests per active user as a daily stacked bar chart using
`user_initiated_interaction_count` for `chat_panel_*_mode` entries in each
day's `totals_by_feature`, including Plan. It uses a green theme with gray for
Other. Missing counts remain `n/a`; incomplete days are marked with `*` and
show a subtotal of known counts (or `n/a` if none), without total shares. When
no chat-mode feature is reported, the chart is omitted. When both have data,
the code activity and completion charts each appear beside their
acceptance-rate chart; they stack on narrow screens. Chart areas, points, and
bars show labels and values in tooltips on hover or keyboard focus. Daily
chart tooltips show all series for that day, with missing values shown as
`n/a`. Stacked-bar tooltips list every displayed series and, when complete, the
bar total with counts and shares; missing values are `n/a`. Tooltips omit chart
titles and align numeric columns. Long tooltips can be scrolled over the chart
or with arrow keys while focused. Model usage per day and Language usage per
day are 100% stacked area charts, using each day's reported model interaction
or language activity total as 100%. Zero-total days have no area. Boundaries
are smoothly interpolated between days with data; missing series taper to no
area at their day, but remain `n/a` in tooltips. These area charts do not use
the section theme; named series are colored by a hue gradient from blue toward
red and purple, ordered by total over the report period, with gray for Other.
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
screens). Line charts use compact fonts and height. Reports share typography
sizes across headings, chart titles, KPI labels, tables, legends, and tooltips,
with smaller-width compensation for paired SVG charts. By team includes paired
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
distribution (`<20%` to `≥80%`, including rates above 100%). Pairs use half
widths only when both charts exist. Missing interaction values are excluded
from totals; users without reported interactions are omitted. By repository includes
horizontal stacked bars for reviewed, created, and merged pull requests. The
reviewed chart compares PRs reviewed with Copilot to those without a Copilot
review; created and merged charts compare Copilot-created PRs with
other-created PRs. Their tooltips show counts and shares. Copilot review
suggestions by repository compares generated and applied counts in separate
bars, not as an acceptance rate; the events may concern different suggestions.
When both are available, the reviewed and suggestions charts appear side by
side, stacking vertically on narrow screens. Partial period totals are marked
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

The model section uses a blue theme and the language section an orange theme,
with gray for Other. A model or language keeps one color in HTML table keys;
pie charts use darker shades for larger categories.
Each stacked chat mode or model keeps one color in every bar and in detail-table
keys. Blue numeric table backgrounds encode relative values within each
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
Per-user data may be sensitive, so confirm before requesting
`--section user`, `--section all`, or publishing. See the
[README](../../README.md#metrics-report) for the full flag reference.

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
total. Usage time bounds use the latest recorded update for lifetime totals; role usage
is never truncated by `--top`. See the
[README](../../README.md#session-stats) for the full flag reference.

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
See the [README](../../README.md#vscode-stats)
for the full flag reference.

VS Code Copilot harness (Agent Host) sessions recorded locally under
`~/.copilot/session-state` use CLI-format logs and are included in `session stats`,
not `vscode stats`. The source depends on the harness, not the VS Code version.

## Dashboard canvas

In the GitHub Copilot app, the `copilot-attendant-dashboard` canvas (provided by
`.github/extensions/attendant-dashboard` in this repository) visualizes the same
`session stats` and `vscode stats` results. See its
[README](../../.github/extensions/attendant-dashboard/README.md) for open inputs and actions.

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

Both tabs have a Models table comparing AIU per session, per API request (default),
or per 1M tokens. Session units use each model's recorded `Sessions` count, never
the overall session count. Missing counts or zero denominators show a dash;
missing counts also display an explanation. Current VS Code statistics do not
report model session counts.
Usage panels follow the same order: model AIU share, Models table,
workspace AIU share, then the Workspaces table, followed by User questions.
On CLI, User questions precedes Agent usage. The Permissions summary below uses
the same bordered panel style as the other panels.
The CLI tab also shows Agent usage (main / subagent / unknown) and incomplete-usage
notices. Refresh reads the latest recorded totals; it does not continuously poll logs.
Click any column heading to sort the listed models, and click again to reverse
the order. Set Top to 0 to include all models in the comparison.

Both Workspaces tables compare AIU per session, per API request, or per 1M tokens
(default: session), with comparison bars and clickable metric headings. The default
sort is AIU descending; missing metrics and zero denominators show a dash and sort
last. Set Top to 0 to include all workspaces. Preferences are independent per tab
and per table.
Filterable panels have a Clear button that clears only their corresponding filter
group. Workspaces, workspace shares, and Working directories clear scope to all
workspaces. Other filter groups, period/time bounds, session, and Top are preserved.
Buttons are disabled when no matching condition is active. CLI model and role usage
panels have no Clear button because those axes do not support filtering.

CLI workspace request counts use `ByCWDUsage.Requests`, never permission or premium
requests. VS Code workspace sessions use `ByWorkspace.Sessions`, and API request
counts use `ByWorkspace.LLMRequests`. Older builds may lack these fields; do not
infer them from tool calls or turns. Workspace token accounting matches Models.

CLI models use `ByModelUsage` from recorded `modelMetrics` or request usage; checkpoints
may have usage totals without a model breakdown. CLI prompt tokens include uncached
input plus cache reads and writes; VS Code input already includes cached tokens.
Model session counts can overlap, and AIU shares cover listed models only.
CLI model names cannot be used as filters (`session stats` has no `--model` flag).

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
