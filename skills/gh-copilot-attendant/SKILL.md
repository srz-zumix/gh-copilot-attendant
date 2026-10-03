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

Both tabs have a Models table comparing AIU per API request or per 1M tokens.
Usage panels follow the same order: model AIU share, Models table,
directory/workspace AIU share, then the directory/workspace table.
Click any column heading to sort the listed models, and click again to reverse
the order. Set Top to 0 to include all models in the comparison.

CLI models require `ByModelUsage` from recorded `modelMetrics`; older sessions may
have usage totals without a model breakdown. CLI prompt tokens include uncached
input plus cache reads and writes; VS Code input already includes cached tokens.
Model session counts can overlap, and AIU shares cover listed models only.
CLI model names cannot be used as filters (`session stats` has no `--model` flag).
