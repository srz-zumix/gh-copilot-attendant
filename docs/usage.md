# Usage guide

See the [project overview and installation instructions](../README.md) to get
started. This guide covers command arguments, defaults, output semantics, and
dashboard behavior.

## Contents

- [Copilot extensions and code review settings](#copilot)
- [Organization and enterprise metrics](#metrics)
- [Local Copilot CLI sessions](#session)
- [Agent skills](#skills)
- [VS Code Copilot Chat](#vscode)
- [Copilot app dashboard](#copilot-app-dashboard)

Square brackets in usage lines indicate optional arguments or flags; parentheses
with `|` indicate alternatives. Defaults and repeatable flags are described
below each command. For shell completion setup, run
`gh copilot-attendant completion --help`.

## Commands

### copilot

#### copilot extension install

```sh
gh copilot-attendant copilot extension install [name...] \
  [--scope <user|repo>] [--prefix <dir>] [--ref <ref>] [--dry-run] [--force]
```

Downloads and installs the bundled GitHub Copilot CLI canvas extensions
(currently `attendant-dashboard`) from this repository's GitHub sources. For
install, status, uninstall, and update, `[name...]` is optional; omitting it
targets every bundled extension. `--scope` selects the installation scope:
`user` (default) installs under `$COPILOT_HOME/extensions`
(`~/.copilot/extensions` when unset) and `repo` installs into the current
repository's `.github/extensions`. `--prefix` overrides the install directory
and takes precedence over `--scope`. `--ref` overrides the default git ref
(`main`). `--dry-run` prints what would be installed without writing files.
`--force` overwrites an existing destination directory that is not managed by
this command.

#### copilot extension list

```sh
gh copilot-attendant copilot extension list
```

Lists the bundled extensions with their source URL and default ref.

#### copilot extension status

```sh
gh copilot-attendant copilot extension status [name...] \
  [--scope <user|repo>] [--prefix <dir>]
```

Shows whether the given extensions are installed and which ref and commit they
were installed from. Only the local filesystem is inspected. `--scope` defaults
to `user`.

#### copilot extension uninstall

```sh
gh copilot-attendant copilot extension uninstall [name...] \
  [--scope <user|repo>] [--prefix <dir>] [--dry-run] [--force]
```

Removes the given extensions. `--scope` defaults to `user`. `--dry-run` prints
what would be removed. `--force` removes a destination directory even if it is
not managed by this command.

#### copilot extension update

```sh
gh copilot-attendant copilot extension update [name...] \
  [--scope <user|repo>] [--prefix <dir>] [--ref <ref>] [--dry-run] [--force]
```

Re-installs the given installed extensions when their ref resolves to a
different commit than the installed one. Fails if an extension is not installed
yet. `--scope` defaults to `user`. `--ref` overrides the default git ref
(`main`). `--dry-run` prints what would be updated. `--force` re-installs even
when already up to date, or overwrites an unmanaged directory.

#### copilot settings policy set

```sh
gh copilot-attendant copilot settings policy set --state <state> \
  [--enterprise [HOST/]ENTERPRISE | --org [HOST/]ORG \
   | --repo [HOST/]OWNER/REPO]
```

Sets an organization's or enterprise's Copilot code review policy. Target
selectors are optional and mutually exclusive; explicitly empty selectors are
invalid. Without one, the current repository's owner is used as the
organization; `GH_REPO` can override that repository. Unless a host is
specified, the existing GitHub host resolution is used. Options other than
`--labels` take the last value when repeated; `--labels` combines values as
described under
[runner set](#copilot-settings-runner-set).

- `--enterprise`: select an enterprise as `[HOST/]ENTERPRISE` (optional).
- `--org`: select an organization as `[HOST/]ORG` (optional).
- `--repo` / `-R`: select the organization owning `[HOST/]OWNER/REPO`
  (optional). This does not update repository-level settings.
- `--state` / `-s`: required policy state. For an organization, use `enabled`
  or `disabled`. For an enterprise, use `enabled_for_all_organizations`,
  `disabled_for_all_organizations`, or `configured_by_organization_admins`.
  The last value is unavailable for standalone enterprises; the API reports
  an error without a separate preflight request.

The update is sent even if the requested state matches the current state.
The API returns no policy object; a successful enterprise update is accepted
for asynchronous propagation. If an enterprise manages the organization policy,
the API may return `409 POLICY_MANAGED_BY_ENTERPRISE` with policy details.

For all four settings commands, organization operations require an organization
owner and a classic PAT or OAuth token with `read:org` or
`manage_billing:copilot` for reads, or `admin:org` or `manage_billing:copilot`
for writes. Enterprise policy operations require an enterprise owner or the
corresponding `read_enterprise_ai_controls` or `write_enterprise_ai_controls`
permission, together with the respective `read:enterprise` or
`admin:enterprise` scope. These APIs do not support GitHub App tokens or
fine-grained PATs. Check the returned details for 403 errors, which can have
causes other than token permissions.

#### copilot settings policy view

```sh
gh copilot-attendant copilot settings policy view \
  [--enterprise [HOST/]ENTERPRISE | --org [HOST/]ORG \
   | --repo [HOST/]OWNER/REPO] [--format json] [--jq <expression>] \
  [--template <template>]
```

Views an organization's or enterprise's Copilot code review policy. Target
selection and authentication follow [policy set](#copilot-settings-policy-set).
Human-readable output is the default. Organization output includes
`configured_state`, `effective_state`, `editable`, and `managed_by`; enterprise
output includes `policy_state`. A null state remains `null`, not `disabled`.
An organization response missing the required `effective_state` is an error.

- `--format`: output a single API-shaped JSON object when set to `json`
  (optional; default is human-readable output).
- `--jq` / `-q`: filter JSON with a jq expression (optional; requires
  `--format json`).
- `--template` / `-t`: format JSON with a Go template (optional; requires
  `--format json`). If both expressions are nonempty, `--jq` takes precedence.

#### copilot settings runner set

```sh
gh copilot-attendant copilot settings runner set --type <standard|labeled> \
  [--allow-repository-customization[=true|false]] [--format json] \
  [--group <name>] [--jq <expression>] [--labels <label[,label...]>]... \
  [--org [HOST/]ORG | --repo [HOST/]OWNER/REPO] [--template <template>]
```

Sets an organization's Copilot code review runner. Select the organization
with `--org` or `--repo`, or use the current repository owner by default, as
described under [policy set](#copilot-settings-policy-set). Enterprise runner
selection is not supported. The update is sent even when the values match the
current configuration; no confirmation GET is made.

- `--allow-repository-customization`: allow repositories to use their own
  runner configuration. The bare flag and `=true` enable it; `=false` disables
  it. When omitted, the current effective value is retained.
- `--format`: use `json` for a single API-shaped object (optional; default is
  human-readable output).
- `--group`: runner group name (optional; must not be empty or whitespace).
- `--jq` / `-q`: filter JSON with a jq expression (optional; requires
  `--format json`).
- `--labels`: comma-separated runner labels (optional; may be repeated).
  Every label must be nonempty and contain more than whitespace.
- `--org`: select an organization as `[HOST/]ORG` (optional).
- `--repo` / `-R`: select the organization owning `[HOST/]OWNER/REPO`
  (optional; mutually exclusive with `--org`).
- `--template` / `-t`: format JSON with a Go template (optional; requires
  `--format json`). A nonempty `--jq` expression takes precedence.
- `--type`: required `standard` or `labeled`. A labeled runner requires at
  least one label or a group; a standard runner accepts neither option.
  `--type` has no short flag because `-t` selects `--template`.

Before a code review-specific runner is configured, the effective coding agent
runner is inherited. A successful update creates a code review-specific
configuration, and output includes the resulting `configured_by` value. If
the successful response cannot be decoded, lacks `runner_type`, or cannot be
rendered, the error reports that the update succeeded so that it is not
mistaken for a failed PUT.

#### copilot settings runner view

```sh
gh copilot-attendant copilot settings runner view \
  [--org [HOST/]ORG | --repo [HOST/]OWNER/REPO] [--format json] \
  [--jq <expression>] [--template <template>]
```

Views the effective runner configuration for an organization. Target selection,
authentication, and output flags follow
[runner set](#copilot-settings-runner-set). An unconfigured code review runner
inherits the coding agent configuration. The output shows `runner_type`,
`runner_labels`, `runner_group_name`, `allow_repository_customization`, and
`configured_by` when reported. JSON, jq, and template output use one
API-shaped object and preserve null values.

### metrics

#### metrics report

```sh
gh copilot-attendant metrics report \
  (--org [HOST/]ORG | --enterprise [HOST/]ENTERPRISE) \
  [--allow-public] [--create-issue | --issue <number> | --pages] \
  [--day <YYYY-MM-DD> | --since <time> [--until <time>]] \
  [--format <md|html>]... [--label <label>]... [--output-dir <dir>] \
  [--repo [HOST/]OWNER/REPO | -R [HOST/]OWNER/REPO] [--rich] \
  [--section <section>]... [--strict] [--title <title>] [--top <n>]
```

Reads the Copilot usage metrics reports of an organization or enterprise and
renders a summary that can be shared with people who cannot view the usage
metrics dashboard. The caller needs access to the usage metrics API (for example
an organization owner, or an enterprise owner or billing manager), and the
"Copilot usage metrics" policy must be enabled. The report covers key metrics,
the daily trend, and breakdowns by feature, language, model, IDE, repository,
team, and user. Active-user metrics are never summed across days: daily active
users are averaged and peaked, and weekly/monthly active users are taken from
the last day with data. Counters are summed and rates are derived from the
summed values. Values missing from the reports are shown as `n/a`, and days
without a report are listed as partial data.

The model and language sections include Model usage per chat mode and Model
usage per language detail tables in both Markdown and HTML, with or without
`--rich`. They list all reported model/chat-mode and language/model pairs, even
when `--top` limits the main tables. Counts sum available values; percentages
use the total of valid, nonnegative counts in the corresponding detail
breakdown. Missing counts and percentages with a zero denominator are `n/a`.
Empty names appear as `(not reported)`, and Other and Unknown names remain
separate in the tables.

Exactly one of `--org` or `--enterprise` is required.

- `--allow-public`: allow publishing to a public repository (optional, default
  `false`). Publishing to a public repository fails without it.
- `--create-issue`: create an issue in `--repo` whose body is a short overview,
  and attach the rendered reports to it as issue artifacts (optional, default
  `false`). Mutually exclusive with `--issue`.
- `--day`: report a single day from its 1-day report (optional, `YYYY-MM-DD`).
  Mutually exclusive with `--since` and `--until`.
- `--enterprise`: enterprise to report on, as `[HOST/]ENTERPRISE`.
- `--format`: `md` (Markdown with mermaid charts) or `html` (a self-contained
  page with inline CSS, SVG charts, collapsible sections and tables)
  (optional, default `md`). JavaScript enables table sorting and numeric heat
  colors. Tables are expanded by default, or collapsed with `--rich`; sections
  are expanded in both modes.
  In either HTML mode, hovering or focusing a day in a time-series chart
  reveals its available points and tooltip; missing days remain empty.
  Stacked-area points mark cumulative boundaries, while tooltips show each
  series' share. If JavaScript is unavailable, browsers supporting CSS `:has()`
  show a simplified tooltip on hover or focus, placed next to the target where
  CSS anchor positioning is supported; it can be scrolled with the pointer.
  Each chart's **Chart values** disclosure lists its full values.
  With JavaScript enabled, printing temporarily expands all sections and tables
  and then restores their screen state. May be repeated or comma separated.
  Multiple formats require `--output-dir`, `--create-issue`, or `--issue`;
  with `--pages`, they require `--output-dir`.
- `--issue`: attach the rendered reports as artifacts to this existing issue
  (optional).
- `--label`: label to add to the created issue (optional, may be repeated).
  Requires `--create-issue`.
- `--org`: organization to report on, as `[HOST/]ORG`.
- `--pages`: publish the HTML report and a report-history index to a
  dedicated private GitHub Pages site in `--repo` (optional, default `false`).
  Requires `--format html`. The destination must be an organization-owned
  private or internal repository on GitHub Enterprise Cloud with an initial
  commit. If the repository has no Pages site, the dedicated
  `copilot-metrics-pages` branch is created (or reused when it already holds a
  managed history) and the site is initialized; metrics are not fetched until
  privacy is verified, and existing unrelated sites or branches are never
  overwritten. Mutually exclusive with `--create-issue`, `--issue`, and
  `--allow-public`. Source updates do not confirm deployment: Pages deploys
  asynchronously, and commits made with a workflow's `GITHUB_TOKEN` do not
  trigger branch builds.
- `--output-dir`: write `report.md` and/or `report.html` to this directory
  (optional). Without it, a single format is written to stdout unless the report
  is published.
- `--repo` / `-R`: repository to publish to, as `[HOST/]OWNER/REPO` (optional,
  defaults to the current repository). Requires `--create-issue`, `--issue`, or
  `--pages`.
  It may be on a different host than the metrics source.
- `--rich`: add dashboard-style KPI cards and breakdown charts to the HTML
  report (optional, default `false`). KPI cards sit side by side when space
  allows; paired pie and stacked charts appear side by side or stack vertically
  on narrow screens. Hover over chart areas, points, or bars to see their labels
  and values in a tooltip; keyboard focus also shows the tooltip. Daily chart
  tooltips list all series for the selected day, including `n/a` for missing
  values. Stacked-bar tooltips list every displayed series and, when complete,
  the bar total with counts and shares; missing values are `n/a`. Tooltips omit
  chart titles. With JavaScript, tooltips align numeric columns, and long
  tooltips can be scrolled over the chart or with arrow keys while focused.
  Requires `--format html`; it also applies when Markdown and HTML are both
  selected. The per-user section, including login names, is included only
  with `--section user` or `--section all`, in either mode. User-level reports
  are also fetched when selected team data or aggregate IDE active-user counts
  require them. The IDE active users card is an estimate: it counts distinct
  users with a reported IDE from the first day of the calendar month through
  the report end day, deduplicating across days and IDEs. It can differ from
  GitHub's dashboard when IDE details are unavailable. Daily user reports
  outside the selected period are fetched when needed for this card only; they
  do not change the user or team tables. Missing days make the estimate
  partial, or `n/a` if no day is available; `--strict` fails if these reports
  are missing. Dashboard charts include active-user trends, requests per chat
  mode, code-generation and acceptance activity, inline code completions
  (suggested/accepted), their acceptance rates, IDE daily and rolling seven-day
  users, model interactions by feature, and code-generation activity by
  language and model. Requests per chat mode appears below Average chat
  requests per active user as a daily stacked bar chart of request counts,
  using the `user_initiated_interaction_count` values for `chat_panel_*_mode`
  in each day's `totals_by_feature`, including Plan. It uses a green theme with
  gray for Other. Missing counts remain `n/a`; incomplete days are marked with
  `*` and show a subtotal of known counts (or `n/a` if none), without total
  shares. The chart is omitted when no chat-mode feature is reported. When
  both have data, the activity and completion charts each appear beside their
  acceptance-rate chart; they stack vertically on narrow screens.
  Model usage per chat mode uses the `chat_panel_*_mode` feature entries in
  the model/feature breakdown: the
  horizontal axis is model, segments are chat modes, and the vertical axis is
  percentage of interactions across all chat modes. Inline chat and non-mode
  features are excluded; unknown chat mode is Other. The chart retains the top
  five models by chat-mode interaction total plus Other, beside the Chat model
  usage (interactions) pie chart (vertically on narrow screens). Language/model
  charts are code-generation activity, not chat requests. Rolling IDE counts
  are unavailable for the first six days and for any window containing a
  missing user report. Language usage is shown as a pie chart with percentages,
  grouping languages below 1% into Other (exactly 1% stays separate); pie
  tooltips retain activity counts. Model usage per day and Language usage per
  day are 100% stacked area charts, showing shares of each day's reported model
  interactions or language activities. Their tooltips show percentages;
  zero-total days have no area. Boundaries are smoothly interpolated between
  days with data. A missing series tapers to no area at its day, while its
  tooltip remains `n/a`. These area charts do not use the section theme; named
  series are colored by a hue gradient from blue toward red and purple, ordered
  by total over the report period, with gray for Other. Model usage per
  language is a vertical
  stacked bar chart with languages on the horizontal axis and models as
  segments. Its title is "Model usage per language (code-generation
  activities)"; the vertical axis shows each segment's percentage of
  code-generation activities across all languages, not a per-language
  normalization. The vertical-axis maximum follows the largest language total
  percentage, rounded up to the next whole percentage point (minimum 1%).
  Other, Others, and Unknown (including the spelling Unkown) are combined as
  Other in these charts, with the Other language bar placed rightmost. Chat
  model usage (interactions) is a percentage pie chart based on model
  interactions; shares below 1% and unknown models are grouped into Other,
  with counts in tooltips. Additive activity
  charts show at most seven categories (the palette size), including Other: the
  largest activity totals are retained and remaining categories are combined;
  stacked charts limit the breakdown segments separately. IDE active-user
  series remain separate because their users can overlap. The model section
  uses a blue theme and the language section an orange theme, with gray for
  Other. A model or language keeps one color in HTML table keys; pie charts
  use darker shades for larger categories.
  Each stacked chat mode or model keeps one color in every bar and in the
  detail-table keys.
  Blue numeric table backgrounds
  independently indicate relative values within each column. Model usage per
  language retains the top five languages by activity total and combines the
  rest into Other. It appears beside the language usage pie chart, stacking
  vertically on narrow screens. Line charts use compact fonts and height.
  Reports use shared typography sizes for headings, chart titles, KPI labels,
  tables, legends, and tooltips; paired SVG charts compensate for their smaller
  width. By team includes paired Team usage pie charts for user-initiated
  interactions and code-generation activities, stacking vertically on narrow
  screens. Each pie independently uses all reported teams regardless of
  `--top`, groups shares below 1% and excess categories into Other, and shows
  at most seven categories including Other. Tooltips show reported counts and
  shares of team-attributed activity, joined using same-day membership.
  Multi-team users count in each team, so shares are not an organization-wide
  breakdown. Ambiguous team labels include team IDs. Missing and zero counts
  have no slices; charts with no positive counts are omitted, and a single
  available chart uses the full width.
  By user includes two rose-themed chart pairs, both using all reported users
  regardless of `--top`. Interaction per active days ranks the top seven
  users by reported interactions divided by Active days, including days with
  a user record but zero interactions. Missing interaction values are excluded
  from totals; users without reported interactions are omitted. Users by
  surface shows vertical bars of users who used each surface; users can count
  toward multiple surfaces. Reported days distribution groups users by days
  with a user record (`1`, `2–3`, `4–7`, `8–14`, `15+`). Acceptance rate
  distribution groups users with code generation by accepted divided by
  generated activities on matching days (`<20%` to `≥80%`; the last bucket
  includes rates above 100%). Empty buckets have no slices. A pair uses half
  widths only when both charts are available.
  By repository includes horizontal stacked bars for reviewed, created,
  and merged pull requests. The reviewed chart compares PRs reviewed with
  Copilot to those without a Copilot review; created and merged charts compare
  Copilot-created PRs with other-created PRs. Their tooltips show counts and
  shares. Copilot review suggestions by repository compares generated and
  applied counts in separate bars, not as an acceptance rate: the events may
  concern different suggestions. When both are available, the reviewed and
  suggestions charts appear side by side, stacking vertically on narrow
  screens. Partial period totals are marked and show only known counts.
  Copilot reviews per day by repository stacks daily counts
  by repository. Available days without repository activity count as zero;
  missing reports or values remain `n/a`. Incomplete days are marked, and
  their tooltips show a displayed subtotal without shares. Each chart ranks
  repositories separately and shows the top seven plus a combined Other
  category, using all reported repositories regardless of `--top`. Review
  counts sum daily reports, so a PR reviewed on multiple days can be counted
  more than once.
- `--section`: `kpi`, `trend`, `feature`, `language`, `model`, `ide`,
  `repository`, `team`, `user`, or `all` (optional, default every section
  except `user`). May be repeated or comma separated. The per-user ranking is
  opt-in.
- `--since`: first day of a range of 1-day reports (optional, `YYYY-MM-DD`,
  RFC3339, or a relative duration such as `7d`). Without `--day`, `--since`,
  and `--until`, the latest 28-day report is used.
- `--strict`: fail when a selected report is missing for any day or the report
  windows differ, instead of reporting partial data (optional, default `false`).
- `--title`: title of the created issue (optional, defaults to the source and
  window). Requires `--create-issue`.
- `--top`: keep only the top N rows of ranked tables (optional, default `20`;
  `0` keeps every row). It does not truncate the model/chat-mode or
  language/model detail tables, or the data behind `--rich` charts.
- `--until`: last day of the range (optional, defaults to and is capped at
  yesterday). Requires `--since`.

Each report is attached as an issue artifact (Markdown as `generic`, HTML as
`embedded_html`) named after the source, window, and format, so re-running the
report for the same window updates the existing artifacts instead of adding new
ones. Issue artifacts are a public preview feature of GitHub. If attaching fails
after the issue was created, the error shows the issue so that the command can
be retried with `--issue`.
Artifact names replace unsupported characters (including colons, parentheses,
and host/owner separators) with spaces to meet GitHub's naming restrictions.
Issue titles retain their original punctuation.

```sh
# Print the latest 28-day report of an organization as Markdown
gh copilot-attendant metrics report --org my-org

# Write a rich HTML dashboard and Markdown report for a date range
gh copilot-attendant metrics report --org my-org \
  --since 2025-06-01 --until 2025-06-30 --format md,html --rich \
  --output-dir ./copilot-metrics

# Publish the latest enterprise report to a new issue in a private repository
gh copilot-attendant metrics report --enterprise my-enterprise \
  --format md --format html --create-issue --repo my-org/copilot-reports \
  --label copilot-metrics
```

### session

#### session stats

```sh
gh copilot-attendant session stats \
  [--all | --cwd <path> | --worktree <path> | -W <path>] [--session <id>] \
  [--since <time> | --period <period>] [--until <time>] \
  [--operation <read|write>]... [--agent-mode <mode>]... \
  [--permission-mode <mode>]... [--result <value>]... \
  [--command-access <read-only|read-write>]... [--kind <kind>]... \
  [--decision-source <source>]... [--command <identifier>]... \
  [--path <pattern>]... [--sandbox <applied|not-applied|unknown>]... \
  [--url <pattern>]... [--top <n>] \
  [--format <format>] [--jq <expression>] [--template <template>]
```

Scans local Copilot CLI session history under `~/.copilot/session-state` (or
`$COPILOT_HOME/session-state`) and reports how often each tool, command, file
path, and URL was requested, broken down by whether it was approved or denied,
as well as by the CLI's own `decisionSource` for that outcome (e.g.
`human_response`; `unknown` when no decision source was recorded, including
sessions from older CLI versions and unresolved requests without a recorded
outcome). It also reports each session's recorded usage totals (premium
requests, AIU, and token counts), including unfinished sessions, summed overall
and broken down by working directory. Usage is not recorded per permission
request: `--agent-mode` selects sessions through matching requests, while other
permission filters do not affect usage totals.

`AGENT_MODE` (`ByAgentMode` in JSON) groups permission requests by the
`agentMode` recorded when permission evaluation began. It includes
approved, approved-for-location, denied, and unresolved outcomes. Each
logical request counts once, using its first recorded snapshot; modes
can differ within one session. Missing, null, empty, or malformed modes
are grouped as `unknown`; nonempty future mode names remain distinct.
Modes are not inferred from `session.mode_changed`, defaults, model names,
or main/subagent roles. This is not a mode-duration or usage breakdown,
and unlogged requests cannot be reconstructed. All directory, session,
time, and permission filters apply. Completion events after `--until`
still resolve selected requests. Rows are sorted by request total
descending, then mode name ascending; `--top` limits the rows, not
overall request totals.

`PERMISSION_MODE` (`ByPermissionMode` in JSON) groups requests by the
`permissionMode` recorded when permission evaluation began, independently of
Agent mode. It uses the first request snapshot and the same outcome, sorting,
time-bound, filter, and `--top` rules as `AGENT_MODE`. Missing, null, empty, or
malformed modes are `unknown`; other nonempty values remain distinct. Modes are
not inferred from session defaults or assessment events. An `assisted` snapshot
means Assisted Permissions was enabled, not that a safety review actually ran.

To select human decisions made while Assisted Permissions was enabled:

```sh
gh copilot-attendant session stats --period 7d \
  --permission-mode assisted --decision-source human_response
```

Human decisions include denials. Add `--result approved --kind shell` to select
approved shell permission requests. The result filter is exact: other approved
variants, including `approved-for-location`, require their own `--result` value.
The `COMMAND` / `ByCommand` breakdown counts command identifiers once per
request, not full command lines or executed-command history.

Request-level `session.usage_record` events and cumulative
`session.usage_checkpoint` and `session.shutdown` snapshots are combined without
double counting: each snapshot replaces the counters it covers, and subsequent
request records add new usage. Replayed request records are deduplicated using
their source-session accounting identity. When request records or `modelMetrics`
are available, `MODEL_USAGE` also reports model names, API request counts (not
permission requests), premium requests, AIU, and token counts per model. JSON
output includes this breakdown in `ByModelUsage`. Checkpoints can record AIU and
premium totals without model, request, or token details; those totals are
included without inventing missing details. `UsageIncompleteSessions` marks
incomplete breakdowns, and request/token counts for those sessions are recorded
subtotals. `AGENT_USAGE` (`ByRoleUsage` in JSON) separates `main`, `subagent`,
and `unknown` usage using recorded request metadata, not model names. Older or
partially recorded usage stays `unknown`. This role breakdown is never truncated
by `--top`; it reports API requests, premium requests, AIU, and tokens, not API
duration. API request totals are also exported as `UsageRequests`. Model usage
follows the same session scope, time bounds, and `--top` limit as
working-directory usage. `--all`, `--cwd`, and `--worktree`/`-W` are mutually
exclusive and all optional; when none are given, only sessions whose recorded
working directory is inside the current git worktree are counted. `--session`
restricts to a single session ID. `--since` and `--until` accept RFC3339
timestamps, `YYYY-MM-DD` dates, or a relative duration such as `7d`/`24h`/`30m`,
and default to no bound. Usage updates are selected by their recorded
timestamps, with an inclusive lower and exclusive upper bound. Earlier recorded
totals are subtracted from cumulative snapshots; a snapshot delta may include
usage before the selected period. `--period` is shorthand for `--since` using a
window such as `7d`/`3w`/`6m`/`1y`, and is mutually exclusive with `--since`.
`--operation` restricts requests to non-mutating (`read`) or mutating (`write`)
classification. `--result` matches an exact recorded result value, including
specific denied variants. `--command-access` selects requests containing at
least one command with the selected `read-only` or `read-write` attribute; a
mixed request can match both values and its commands can appear in both rows.
`--agent-mode` matches the recorded Agent mode exactly and case-sensitively; use
`--permission-mode` to match the recorded permission mode in the same way.
By default, neither mode is filtered. `unknown` includes requests whose
corresponding mode is missing or invalid. Empty mode filter values are rejected.
`--kind` restricts to an exact tool kind (e.g. `shell`, `read`, `write`);
`--decision-source` restricts to permission outcomes
with an exact decision source (e.g. `human_response`, `unattended_fallback`, or
`unknown` when no decision source was recorded, including older CLI versions and
unresolved requests). `--command` restricts to requests that include an exact
command identifier; `--path` and `--url` restrict to requests with a recorded
path or URL matching the given regular expression (a plain substring is also a
valid, unanchored regular expression). `--sandbox` restricts to requests whose
matched tool execution recorded `sandboxApplied` as `applied` or `not-applied`,
or `unknown` when no outcome was recorded. `--operation`, `--agent-mode`,
`--permission-mode`, `--result`, `--command-access`, `--kind`,
`--decision-source`, `--command`, `--path`, `--sandbox`, and `--url` may each
be repeated to match any of multiple values; different filter types are combined.
`--command-access` describes
command-level attributes and is distinct from `--operation`, which classifies
the request as a whole. `--agent-mode` also restricts usage totals and the
working directory, model, and agent usage breakdowns to sessions with at least
one matching permission request in the selected time window. Usage is not
recorded per mode, so every usage update of a matching session is counted,
including usage from other modes in that session. Other permission filters do
not affect usage, and permission filters do not affect Fusion or user-question
statistics. JSON output reports the Agent modes applied to usage in
`UsageAgentModes`. `--top` keeps only the top N entries per statistic (default
`10`; `0` keeps every entry). `--format`, `--jq`, and `--template` follow the
standard `gh` JSON export flags; without `--format json`, results are printed as
tables.

`SANDBOX_REQUEST` (`BySandbox` in JSON) groups permission requests by the
`sandboxApplied` value recorded in the telemetry of the matching
`tool.execution_complete` event. `SANDBOX` and `SANDBOX_TOOL` (`Sandbox` in
JSON) count completed shell executions and other completed tools with a usable
`sandboxApplied` outcome, including search tools such as `rg`, `glob`, and
`grep`. Shell completions without an outcome are `unknown`. Executions that
requested a sandbox opt-out are counted separately in `OptOutRequested`.
Execution statistics follow the session scope, completion-time bounds, and
`--sandbox`; other permission filters do not apply. Unknown outcomes are never
inferred, and unfinished calls are excluded.

JSON output includes versioned `SessionActivity` data for the CLI dashboard.
By default, each local date has 24 hourly counts of distinct sessions with at
least one timestamped event in that hour. Multiple events and repeated clock
hours count a session once per cell; the result does not infer continuous
activity or idle time. `Days` contains dates with matching events, while
`StartDate` and `EndDate` define the display range. Dates and hours use the CLI
host's local time zone, identified by `TimeZone`, not the browser's time zone.
`Coverage` reports `full`, `partial`, `outside`, or `nonexistent` for affected
hours on boundary or clock-transition dates; dates omitted from `Coverage` have
full coverage. A partial cell includes only events within the selected window,
including when a repeated hour is only partly selected.

Directory and session scope apply; timestamps are selected with inclusive
`--since` and exclusive `--until`. With `--agent-mode` but no `--sandbox`, cells
count matching `permission.requested` events by their request timestamps. With
`--sandbox`, cells instead count completed shell executions and other executions
with recognized sandbox telemetry, using their completion timestamps. Selected
sandbox states are alternatives; `unknown` means no usable outcome was recorded.
Unfinished calls are excluded. When both filters are selected, an execution must
be linked to at least one permission request in a selected Agent mode. Exact
agent/tool-call identity is used, or a tool-call ID unique among started and
completed executions in the same session; ambiguous or unlinked calls do not
match an Agent mode. Only the completion time must be in the selected window,
not the request time. Other
permission filters and `--top` do not affect these counts. `AgentModes` and
`SandboxStates` record the sorted, deduplicated filters applied to the activity.
`UntimedEvents` counts matching events or executions with no usable timestamp;
`MalformedEvents` counts malformed event envelopes. `UntimedSessions` counts
scoped sessions with untimed matching records. These diagnostics cover the
scoped logs, including when a time window is selected. Missing timestamps cannot
be assigned to that window. `AsOf` is the
scan completion time, not an implicit upper bound; active logs may change
while scanning, and recorded future timestamps can extend the displayed range.
With no lower bound, the display is anchored at the earliest dated event and
includes local dates reached after it. With no upper bound, it ends on the scan
date or the latest recorded event date. A fully
bounded empty period is displayed with zero counts; without a known start date,
when a future `--since` has no dated events, or when `--since` is not earlier
than `--until`, no range is displayed.

`FUSION` (`Fusion` in JSON) reports Copilot Fusion activity from recorded
events. `FUSION_PATTERN` counts distinct resolved turns by pattern;
`FUSION_PHASE` groups deduplicated usage records by pattern, phase, role, and
the recorded usage model. Sessions and identified turns and phases are counted
distinctly; a phase requires both a Fusion ID and a phase ID. Resolutions and
usage records lacking these identifiers are reported separately. A turn or
phase can appear in more than one phase/model row. AIU is a subtotal from usage
records with a recorded `totalNanoAiu`; the record count with AIU exposes
coverage, and partial subtotals are marked `>=` in tables. Missing AIU is
`n/a`, not zero. Checkpoints, planned phases, and missing Fusion metadata are
not attributed or inferred, so older or incomplete logs cannot prove that
Fusion was not used. Usage record counts are not necessarily distinct API calls.
JSON also includes the versioned `Fusion.Usage` dataset: totals and independent
model, working-directory, and main/subagent/unknown-role breakdowns use only
explicitly tagged usage records. Each metric records its observed, complete,
and excluded record counts, so missing values are not treated as zero.
Premium requests use cost multipliers for user or unspecified initiators, as
in the shared collector; other initiators are excluded from that metric.
Unknown models, working directories, and agent provenance remain unidentified.

Directory and session scope apply to Fusion; `--since` and `--until` select
each resolution or usage record by its own timestamp, with an inclusive lower
and exclusive upper bound. Events without timestamps are excluded when a time
bound is set. Replayed records are deduplicated before time filtering.
Permission filters do not apply. `--top` limits each Fusion
breakdown but not its totals. Active sessions can be updated while scanning,
so shared and Fusion totals are not an atomic snapshot.

`ASK_USER` (`AskUser` in JSON) reports sessions with questions, question counts,
and answered/cancelled/failed/unresolved/unknown outcomes from paired `ask_user`
tool events. Answered questions with valid timestamps contribute to total,
average, and maximum wait in milliseconds; `TimedAnswers` and `UntimedAnswers`
expose timing coverage. Average and maximum are `null` (`n/a` in tables) without
timed answers. Unknown completions are not inferred as answers. Wait runs from
tool start to completion and includes UI/transport overhead; overlapping
per-question intervals are summed, not deduplicated as wall-clock idle time.
Session/directory scope applies; time bounds select question starts (inclusive
`--since`, exclusive `--until`), while later completions still resolve selected
questions. Permission filters and `--top` do not affect this summary. No
question or answer text is exported. Older logs without recorded events cannot
reconstruct missing questions.

`PERMISSION_WAIT` (`PermissionWait` in JSON) reports request-to-completion
elapsed time separately from `ASK_USER`. `HumanResponses` includes only recorded
`human_response` decisions; `NonHumanDecisions` and `UnknownSourceDecisions`
retain other and missing-source decision times separately, without inferring
human wait from an approval or a long delay. Each group reports
completed/timed/untimed counts and total/average/maximum milliseconds.
Zero-length intervals are valid; missing or negative timing is untimed and
excluded. Average/maximum are `null` (`n/a` in tables) without timed decisions.
Unresolved requests have no inferred duration. Replay duplicates count once,
paired by session/agent/request identity. All permission filters and
directory/session scope apply; time bounds select request starts, while later
completions still resolve them. `--top` does not truncate these totals.
Intervals include UI/transport overhead; overlapping waits are summed, not
unique wall-clock idle time. Unlogged rule-based approvals cannot be
reconstructed.

### skills

`gh copilot-attendant skills` manages the agent skills bundled with this
extension. Run `gh copilot-attendant skills --help` for its subcommands.

### vscode

#### vscode stats

```sh
gh copilot-attendant vscode stats \
  [--all | --cwd <path> | --worktree <path> | -W <path>] [--session <id>] \
  [--since <time> | --period <period>] [--until <time>] \
  [--tool <pattern>]... [--model <pattern>]... [--agent <pattern>]... \
  [--sandbox <applied|not-applied|unknown>]... \
  [--include-log-stats] [--top <n>] [--format <format>] \
  [--jq <expression>] [--template <template>]
```

Scans local VS Code Copilot Chat debug logs from the Local harness under
`~/Library/Application Support/Code/User/workspaceStorage` and reports tool
usage, LLM token and usage totals, turn counts, and subagent invocations. Only
the stable, non-Insiders VS Code installation on macOS is supported. Subagent
log files referenced by each session are included recursively, with each file
counted once. Their model requests, tokens, AIU, tool calls, and turns are
attributed to the parent workspace; session counts still refer to parent chat
sessions. Internal title-generation child logs and unreferenced files are
excluded. Time bounds apply to each event, including child activity whose
invocation predates `--since`. When `--agent` is specified, all activity is
scoped to matching subagents' own child logs: model requests, AIU, tokens, tool
calls, turns, and workspace totals. Main-agent usage is excluded, and
differently named descendants are excluded unless they also match an agent
pattern. Without `--agent`, parent and referenced child usage is included as
before. Tool and model filters apply independently within that scope. Missing
child logs produce a warning, while their recorded matching invocations remain
counted.

VS Code Copilot harness (Agent Host) sessions recorded locally under
`~/.copilot/session-state` use CLI-format logs and are reported by
`gh copilot-attendant session stats` instead. The source depends on the harness,
not the VS Code version.

`--all`, `--cwd`, and `--worktree`/`-W` are mutually exclusive and all optional;
when none are given, only sessions belonging to the current git worktree are
counted. `--session` restricts to a single session ID. `--since` and `--until`
accept RFC3339 timestamps, `YYYY-MM-DD` dates, or a relative duration such as
`7d`/`24h`/`30m`, and default to no bound. `--period` is shorthand for `--since`
using a coarser window such as `7d`/`3w`/`6m`/`1y`, and is mutually exclusive
with `--since`. `--tool` and `--model` restrict tool calls and LLM requests;
`--agent` scopes activity by subagent name. All three are optional and default
to no filter. They match the given regular expression (a plain substring is also
a valid, unanchored regular expression) and may each be repeated to match any of
multiple values. `--sandbox` is optional and repeatable; it restricts only the
sandbox statistics described below. `--include-log-stats` is optional (default
`false`) and adds versioned `LogDetails` to JSON output. Details contain each
parent chat's identity and workspace, unique log identities, main/child
classification, all referencing agent names, normalized own-activity metrics,
and invocation identities in traversal order. These details are not truncated by
`--top` and contain no raw events or conversation content. They support local
agent filtering without double-counting shared logs or parent chats; positive
TTFT sample counts and sums are retained for accurate averaging. This flag
cannot be combined with `--agent`. `--top` keeps only the top N entries per
statistic (default `10`; `0` keeps every entry). `--format`, `--jq`, and
`--template` follow the standard `gh` JSON export flags; without `--format
json`, results are printed as tables.

For example, report only Explore agents' own usage across all workspaces over
the last seven days:

```sh
gh copilot-attendant vscode stats --all --period 7d --agent '^Explore$'
```

`SANDBOX` and `SANDBOX_TOOL` (`Sandbox` in JSON) report terminal tool executions
by the `isSandboxWrapped` value recorded in chat session logs
(`chatSessions/*.jsonl`): `applied`, `not-applied`, or `unknown` when nothing
was recorded. Each tool call counts once. Totals, sessions, and a per-tool
breakdown are reported; tool IDs are shown as recorded, including localized
names. Session, directory, and time filters apply; `--tool`, `--model`, and
`--agent` do not. `--top` limits only the per-tool rows.

`USER_QUESTIONS` (`UserQuestions` in JSON) reports completed
`vscode_askQuestions` calls, sessions with calls, and
answered/skipped/cancelled/failed/unknown outcomes. Each multi-question batch
counts as one call; a partially skipped batch with at least one recorded answer
counts as answered, while a fully skipped batch counts as skipped. Only answered
calls with valid recorded `dur` contribute to total/average/maximum wait in
milliseconds. `TimedAnswers` and `UntimedAnswers` expose timing coverage;
average/maximum are `null` (`n/a` in tables) without timed answers. Time bounds
select call starts, even when completion falls after `--until`.
Session/directory/agent/tool filters apply; model filters and `--top` do not.
Duration includes tool/UI/transport overhead; overlapping intervals are summed,
not deduplicated as wall-clock idle time. In-flight questions cannot be
reconstructed from completed-call logs. No question or answer text is exported.
Log details include these untruncated per-log metrics for exact local filtering
and parent-session counting.

## Copilot app dashboard

This repository ships a canvas extension for the GitHub Copilot app in
[`.github/extensions/attendant-dashboard`](../.github/extensions/attendant-dashboard/README.md).
When a Copilot session runs inside this repository, ask the agent to open the
"Copilot attendant dashboard" to browse `session stats` and `vscode stats`
results as interactive charts, filter by clicking entries, and hand the data
back to the agent. The VS Code tab always explains that it reads logs from the
Local harness. Locally recorded VS Code Copilot harness (Agent Host) sessions
are included in the Copilot CLI tab. This distinction depends on the harness,
not the VS Code version. The VS Code canvas fetches log details once and
processes Agent and Top changes locally, without re-running gh. Refresh,
scope/time/session changes, and tool/model filter changes fetch a new dataset.
Filtered display, summaries, and Ask agent use the same projected data. On the
Copilot CLI tab, **Fusion usage only** restricts the dashboard to usage records
explicitly tagged as Fusion, excluding permission data, resolution events, and
checkpoint-only usage. The switch applies locally to matching cached data;
scope, time, session, and Top changes fetch new data. Metric coverage
distinguishes missing values from zero, and record counts are not necessarily
distinct API calls. The fetch command shown in this mode returns the unfiltered
CLI result; filtering is applied by the dashboard. Use an updated CLI or the
canvas `source` runner if Fusion-only details are unavailable. Filterable panels
have a Clear button that removes only that panel's conditions. Workspaces and
Working directories clear the directory scope back to all workspaces. Other
filters, the time window, session selection, and Top remain unchanged. Both tabs
show a User questions panel with answer outcomes and total/average/maximum wait;
VS Code counts completed tool calls, while CLI pairs recorded question events.
User questions follows Workspaces on both tabs and precedes Agent usage on CLI.
The normal Copilot CLI view also shows a date/hour Session activity heatmap,
with 30-day navigation, host-local time, and explicit boundary and missing-hour
states. Without Agent mode or Sandbox filters, cells count all recorded events.
With Agent mode alone they count matching requests by request time; with Sandbox
they count matching completed executions by completion time. When both are
selected, the execution must link to a matching Agent mode request. Other
permission filters and `--top` do not apply to the heatmap. Older or mismatched
CLI results need an updated CLI or the canvas `source` runner. Total and Average
rows summarize each hour on the displayed page. Totals add daily counts, so a
session can count on multiple days. Averages
include zeros and partial hours once per covered day; outside and nonexistent
hours are excluded. A summary asterisk means it includes partial hours. Footer
cell colors use the heatmap scale relative to the largest value in each summary
row. Fusion-only and VS Code views do not show this full-session activity. The
dashboard groups CLI permission panels into a collapsible Permissions category;
its Summary panel uses the same bordered panel style as the other panels and
also shows confirmed human permission wait and separate other/unknown-source
decision timing. Select `assisted` in Permission modes and `human_response` in
Decision sources to filter human decisions while Assisted Permissions was
enabled; optionally select `approved` in Results and `shell` in Tool kinds.
The Permission modes filter does not restrict usage, session activity, Fusion,
sandbox execution, or user-question statistics. Each panel, including Models
and Workspaces, can be collapsed
independently of its category. Panel states persist across dashboard updates and
view changes; CLI and VS Code panels keep separate states, while matching CLI
panels share state with the Fusion-only view. Panel Clear buttons remain
available when collapsed. Option-clicking any category or panel heading expands
all included disclosures when any are collapsed, or collapses all when they are
already expanded. The Usage and Permissions categories are excluded; their inner
panels are still included. VS Code Local harness debug logs do not independently
identify ordinary permission request/response waits; confirmation-tool or
execution durations are not substituted.
