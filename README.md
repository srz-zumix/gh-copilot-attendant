# gh-copilot-attendant

**Understand where your GitHub Copilot usage goes, and turn it into actionable reports.**

A GitHub CLI extension that brings local Copilot activity and organization or
enterprise usage metrics into your terminal, shareable reports, and an interactive
Copilot app dashboard.

## Why gh-copilot-attendant?

- **See what consumes your usage.** Compare AIU, premium requests, and tokens by
  model and workspace, inspect main-agent and subagent activity, and explore
  explicitly recorded Copilot Fusion usage.
- **Understand agent interactions.** Review tool and permission activity,
  approval/denial outcomes by recorded Agent mode, and user-question
  and permission wait times.
- **Share insights beyond the metrics dashboard.** Turn organization or enterprise
  metrics into Markdown or self-contained HTML reports with trends and breakdowns.
  Publish them as GitHub issue artifacts when needed.
- **Explore visually with your agent.** Browse local usage in the Copilot app
  dashboard, click entries to filter, compare usage costs, and ask the agent to
  analyze the selected data.

Local statistics use recorded logs; missing data is not treated as zero.
Organization and enterprise reports require access to the Copilot usage metrics
API and the enabled "Copilot usage metrics" policy.

## Installation

Install with the [GitHub CLI](https://cli.github.com/):

```sh
gh extension install srz-zumix/gh-copilot-attendant
```

Optionally, create a shorter alias:

```sh
gh alias set ca copilot-attendant
```

Then use `gh ca` in place of `gh copilot-attendant`.

## Shell completion

Completion is available for bash, zsh, fish, and PowerShell. Run
`gh copilot-attendant completion --help` for setup instructions.

## Get started

### copilot

```sh
gh copilot-attendant copilot extension install attendant-dashboard
```

Install the interactive dashboard for the current user (the default scope).
In the GitHub Copilot app, ask: "Open the Copilot attendant dashboard for all
sessions in the last 7 days."

### metrics

```sh
gh copilot-attendant metrics report --org example-org \
  --format html --rich --output-dir ./copilot-metrics
```

Replace `example-org` with your organization to write a rich HTML report to
`./copilot-metrics/report.html`. By default, it uses the latest 28-day report
and excludes the per-user ranking. Enterprise reports use `--enterprise`
instead of `--org`.

To publish the report and maintain a history on a private or internal Pages
site, add `--pages` and specify an existing destination repository:

```sh
gh copilot-attendant metrics report --org example-org --format html --rich \
  --pages --repo example-org/private-reports
```

Pages publication requires an Organization-owned private or internal project
repository on GitHub Enterprise Cloud. It cannot be combined with issue
publication. See the [agent skill](skills/gh-copilot-attendant/SKILL.md) for
initialization, permissions, and privacy details.

### session

```sh
gh copilot-attendant session stats --period 7d
gh copilot-attendant session stats --period 7d --agent-mode plan
gh copilot-attendant session stats --period 7d --result denied
```

Inspect the last seven days of local Copilot CLI activity in the current git
worktree (the default scope). Add `--all` to include every working directory,
or `--format json` for structured output. The second command filters permission
requests by their recorded Agent mode and limits usage to sessions with a
matching request. Filter permission outcomes by exact result with `--result`;
the dashboard also filters permission requests by clicking Results or
Read-only/read-write rows.
The dashboard also shows sessions with recorded events by local date and hour;
this is not a continuous-activity measurement. When `--agent-mode` is selected,
the heatmap counts matching permission requests by their recorded request time.

### vscode

```sh
gh copilot-attendant vscode stats --period 7d
```

Inspect VS Code Copilot Chat Local harness activity for the current git worktree
over the last seven days. This log source supports stable VS Code on macOS only.
Locally recorded Copilot harness (Agent Host) sessions use `session stats` instead.

## Documentation

| Guide | What you will find |
| --- | --- |
| [Usage guide](docs/usage.md) | Full command usage, optional flags, defaults, filtering, and accounting details |
| [Dashboard guide](.github/extensions/attendant-dashboard/README.md) | Canvas requirements, queries, filters, and agent actions |
| [Agent skill](skills/gh-copilot-attendant/SKILL.md) | Guidance for agents using this extension |
| [Development](docs/development.md) | Local checks and updating the bundled dashboard matcher |
