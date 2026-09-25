---
name: gh-copilot-attendant
description: Basic skill package for the gh-copilot-attendant GitHub CLI extension.
allowed-tools: Bash(git status:*), Bash(git branch --show-current:*), Bash(gh copilot-attendant skills:*), Bash(gh copilot-attendant session stats:*), Bash(gh copilot-attendant vscode stats:*)
---

# gh-copilot-attendant

This is a minimal skill bundle for `gh-copilot-attendant`.

## Commands

### session stats

Reports tool, path, and URL permission statistics from local GitHub Copilot CLI session
history. See the [README](../../README.md#session-stats) for the full flag reference.

### vscode stats

Reports tool usage, LLM token/usage, turn, and subagent statistics from local VS Code
GitHub Copilot Chat debug logs. See the [README](../../README.md#vscode-stats) for the full
flag reference.
