---
name: gh-copilot-attendant
description: Basic skill package for the gh-copilot-attendant GitHub CLI extension.
allowed-tools: Bash(git status:*), Bash(git branch --show-current:*), Bash(gh copilot-attendant skills:*), Bash(gh copilot-attendant copilot extension:*), Bash(gh copilot-attendant session stats:*), Bash(gh copilot-attendant vscode stats:*)
---

# gh-copilot-attendant

This is a minimal skill bundle for `gh-copilot-attendant`.

## Commands

### copilot extension install / update / uninstall / list / status

Manages the bundled Copilot CLI canvas extension (`attendant-dashboard`). See the
[README](../../README.md#copilot-extension-install) for the full flag reference.

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
