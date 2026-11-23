# gh-copilot-attendant

**Understand where your GitHub Copilot usage goes, and turn it into actionable reports.**

A GitHub CLI extension that brings local Copilot activity and organization or
enterprise usage metrics into your terminal, shareable reports, and an interactive
Copilot app dashboard. It also manages Copilot code review policies and
organization runner settings.

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
- **Manage code review settings.** View and update organization or enterprise
  policies and organization runner configurations.
- **Approve trusted commands everywhere.** Use a Copilot CLI permission hook to
  allow or deny commands and tools in every session from one config file.

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

View or update Copilot code review policies and organization runner settings:

```sh
gh copilot-attendant copilot settings policy view --org example-org
gh copilot-attendant copilot settings policy set --org example-org --state enabled
gh copilot-attendant copilot settings runner view --org example-org
gh copilot-attendant copilot settings runner set --org example-org \
  --type labeled --labels self-hosted,linux
```

The `set` commands change live settings. Replace `example-org` with your
organization and review the [usage guide](docs/usage.md#copilot-settings-policy-set)
for permissions, enterprise policies, and runner inheritance.

### hook

Automatically approve selected Copilot CLI tools using rules in
`~/.config/gh-copilot-attendant/config.yaml`:

```yaml
hooks:
  permissionRequest:
    allow:
      tools: ["list_repositories"]
      scopedTools:
        - tools: ["view", "rg", "grep", "glob"]
          paths: ["/absolute/path/to/trusted/repo"]
```

Replace the example path with an existing directory to approve those read
tools only within that directory.

```sh
gh copilot-attendant hook install
```

The installer selects `preToolUse` for these rules or the gh, Git, and shell
display presets, and `permissionRequest` for ordinary command-only rules.
Mixed rules use `preToolUse` for all decisions. Other
`permissionRequest` hooks can prevent `preToolUse` approvals. Replacing an
existing hook requires `--force`; restart the session after installation.
Hook approvals can override native CLI denials, and scoped rules are not a
filesystem sandbox. See the
[usage guide](docs/usage.md#hook) for both modes and safety notes.

Unmatched rules produce no decision. Configuration or input errors emit a
progress warning and fall through to normal permission handling; this can also
skip explicitly configured deny rules. See the usage guide for failure limits.

Set `hooks.permissionRequest.allow.ghRead: true` to opt in to the built-in
`gh` read preset, with or without leading assignments. It checks supported
operations, flags, and explicit hosts without requiring individual command
rules. Explicit command rules remain independent and may be broader; see the
[usage guide](docs/usage.md#hook) for the preset's limits.

Set `hooks.permissionRequest.allow.gitRead: true` to opt in to supported Git
read operations. Only recognized command and option forms match; other forms
fall through. Git configuration and inherited environment can still run
programs or cause side effects, even for read operations.

Set `hooks.permissionRequest.allow.shellDisplay: true` to opt in to selected
forms of `basename`, `cat`, `cut`, `dirname`, `echo`, `find`, `head`, `jq`,
`ls`, `nl`, `printf`, `pwd`, `sed`, `sort`, `stat`, `tail`, `tr`, `uniq`, and
`wc`. File operands must resolve inside the session working directory or a
configured `allow.scopedTools.paths` root. `sort` may create temporary files
outside these roots. The preset is not a filesystem sandbox; see the
[usage guide](docs/usage.md#hook) for supported forms and limitations.

Leading Bash assignments can be ignored when matching existing command rules
for allowed commands. Assignment-bearing requests still restrict direct `gh`
commands to a conservative set of built-in read-only operations. The default
environment-variable allowlist permits GitHub token and display variables;
the optional denylist mode is broader and excludes common shell, Git, Go, proxy,
and runtime-control variables. `PATH` is never allowed. The preset and
assignment-bearing requests check explicit `gh` host targets against trusted
hosts, and assignment values are redacted from hook logs.
The denylist is not a safety boundary: unknown variables, allowed command
arguments, inherited environment, and subprocesses can change execution.
The hook does not expand variable references or inspect `gh` configuration,
credentials, proxies, or redirects; see the
[usage guide](docs/usage.md#hook) before enabling this policy.

On Linux and on macOS builds with cgo ACL support, calculated allow and deny
hook decisions are logged locally by default. Calls without a decision are not
logged; older no-decision records remain on disk but are excluded from
statistics. Set `hooks.logging.disabled: true` to turn logging off; see the
usage guide for privacy and storage details.

```sh
gh copilot-attendant hook status     # show the installed hook and config state
gh copilot-attendant hook disable    # ignore all rules without uninstalling
gh copilot-attendant hook enable     # apply the rules again
gh copilot-attendant hook uninstall  # remove the hook config file
```

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
gh copilot-attendant session stats --period 7d --permission-mode assisted \
  --decision-source human_response --result approved --kind shell
gh copilot-attendant session stats --period 7d --sandbox applied
```

Inspect the last seven days of local Copilot CLI activity in the current git
worktree (the default scope). Add `--all` to include every working directory,
or `--format json` for structured output. The second command filters permission
requests by their recorded Agent mode and limits usage to sessions with a
matching request. Filter permission outcomes by exact result with `--result`;
the dashboard also filters permission requests by clicking Results or
Read-only/read-write rows. `--sandbox` filters by the recorded
sandbox outcome (`applied`, `not-applied`, or `unknown`).
The assisted example selects human-approved shell permission requests while
Assisted Permissions was enabled; it does not prove a safety review ran.
Command breakdowns contain identifiers, not full command lines.
Hook activity is reported separately from permission requests, including
recorded allow and deny decisions, output states, and coverage gaps. The
dashboard shows the tool shares for each decision and starts with its detail
tables collapsed. A missing log does not prove that no hooks ran; see the
[usage guide](docs/usage.md#session-stats) for scope and limitations.
The dashboard also shows sessions with recorded events by local date and hour;
this is not a continuous-activity measurement. When `--agent-mode` is selected,
the heatmap counts matching permission requests by request time. With `--sandbox`,
it counts completed executions by completion time instead. With both filters,
the execution must be linked to a request in a selected Agent mode.

### vscode

```sh
gh copilot-attendant vscode stats --period 7d
gh copilot-attendant vscode stats --period 7d --sandbox applied
```

Inspect VS Code Copilot Chat Local harness activity for the current git worktree
over the last seven days. This log source supports stable VS Code on macOS only.
Both commands report how many terminal executions ran in a sandbox.
Locally recorded Copilot harness (Agent Host) sessions use `session stats` instead.

## Documentation

| Guide | What you will find |
| --- | --- |
| [Usage guide](docs/usage.md) | Full command usage, optional flags, defaults, filtering, and accounting details |
| [Dashboard guide](.github/extensions/attendant-dashboard/README.md) | Canvas requirements, queries, filters, and agent actions |
| [Agent skill](skills/gh-copilot-attendant/SKILL.md) | Guidance for agents using this extension |
| [Development](docs/development.md) | Local checks and updating the bundled dashboard matcher |
