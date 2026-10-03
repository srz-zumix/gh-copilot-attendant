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
down by working directory and, when `modelMetrics` is recorded, by model, including API
request counts. Model request counts are separate from permission requests. The model
breakdown appears as `MODEL_USAGE` in tables and `ByModelUsage` in JSON; sessions without
model metrics have no model breakdown. See the
[README](../../README.md#session-stats) for the full flag reference.

### vscode stats

Reports tool usage, LLM token/usage, turn, and subagent statistics from local VS Code
GitHub Copilot Chat debug logs. See the [README](../../README.md#vscode-stats) for the full
flag reference.

## Dashboard canvas

In the GitHub Copilot app, the `copilot-attendant-dashboard` canvas (provided by
`.github/extensions/attendant-dashboard` in this repository) visualizes the same
`session stats` and `vscode stats` results. See its
[README](../../.github/extensions/attendant-dashboard/README.md) for open inputs and actions.

Both tabs have a Models table comparing AIU per session, per API request (default),
or per 1M tokens. Session units use each model's recorded `Sessions` count, never
the overall session count. Missing counts or zero denominators show a dash;
missing counts also display an explanation. Current VS Code statistics do not
report model session counts.
Usage panels follow the same order: model AIU share, Models table,
workspace AIU share, then the Workspaces table.
Click any column heading to sort the listed models, and click again to reverse
the order. Set Top to 0 to include all models in the comparison.

Both Workspaces tables compare AIU per session, per API request, or per 1M tokens
(default: session), with comparison bars and clickable metric headings. The default
sort is AIU descending; missing metrics and zero denominators show a dash and sort
last. Set Top to 0 to include all workspaces. Preferences are independent per tab
and per table.

CLI workspace request counts use `ByCWDUsage.Requests`, never permission or premium
requests. VS Code workspace sessions use `ByWorkspace.Sessions`, and API request
counts use `ByWorkspace.LLMRequests`. Older builds may lack these fields; do not
infer them from tool calls or turns. Workspace token accounting matches Models.

CLI models require `ByModelUsage` from recorded `modelMetrics`; older sessions may
have usage totals without a model breakdown. CLI prompt tokens include uncached
input plus cache reads and writes; VS Code input already includes cached tokens.
Model session counts can overlap, and AIU shares cover listed models only.
CLI model names cannot be used as filters (`session stats` has no `--model` flag).
