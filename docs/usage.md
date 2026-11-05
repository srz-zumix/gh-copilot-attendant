# Usage guide

See the [project overview and installation instructions](../README.md) to get started.
This guide covers command arguments, defaults, output semantics, and dashboard behavior.

## Contents

- [Copilot extensions](#copilot)
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
gh copilot-attendant copilot extension install [name...] [--scope <user|repo>] [--prefix <dir>] [--ref <ref>] [--dry-run] [--force]
```

Downloads and installs the bundled GitHub Copilot CLI canvas extensions
(currently `attendant-dashboard`) from this repository's GitHub sources.
For install, status, uninstall, and update, `[name...]` is optional; omitting
it targets every bundled extension. `--scope` selects the installation scope:
`user` (default) installs under `$COPILOT_HOME/extensions` (`~/.copilot/extensions` when
unset) and `repo` installs into the current repository's `.github/extensions`. `--prefix`
overrides the install directory and takes precedence over `--scope`. `--ref` overrides the
default git ref (`main`). `--dry-run` prints what would be installed without writing files.
`--force` overwrites an existing destination directory that is not managed by this command.

#### copilot extension list

```sh
gh copilot-attendant copilot extension list
```

Lists the bundled extensions with their source URL and default ref.

#### copilot extension status

```sh
gh copilot-attendant copilot extension status [name...] [--scope <user|repo>] [--prefix <dir>]
```

Shows whether the given extensions are installed and which ref and commit they were installed
from. Only the local filesystem is inspected. `--scope` defaults to `user`.

#### copilot extension uninstall

```sh
gh copilot-attendant copilot extension uninstall [name...] [--scope <user|repo>] [--prefix <dir>] [--dry-run] [--force]
```

Removes the given extensions. `--scope` defaults to `user`. `--dry-run` prints what would be
removed. `--force` removes a destination directory even if it is not managed by this command.

#### copilot extension update

```sh
gh copilot-attendant copilot extension update [name...] [--scope <user|repo>] [--prefix <dir>] [--ref <ref>] [--dry-run] [--force]
```

Re-installs the given installed extensions when their ref resolves to a different commit than
the installed one. Fails if an extension is not installed yet. `--scope` defaults to `user`.
`--ref` overrides the default git ref (`main`). `--dry-run` prints what would be updated.
`--force` re-installs even when already up to date, or overwrites an unmanaged directory.

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
gh copilot-attendant session stats [--all | --cwd <path> | --worktree <path> | -W <path>] [--session <id>] [--since <time> | --period <period>] [--until <time>] [--operation <read|write>]... [--kind <kind>]... [--decision-source <source>]... [--command <identifier>]... [--path <pattern>]... [--url <pattern>]... [--top <n>] [--format <format>] [--jq <expression>] [--template <template>]
```

Scans local Copilot CLI session history under `~/.copilot/session-state`
(or `$COPILOT_HOME/session-state`) and reports how often each tool, command, file
path, and URL was requested, broken down by whether it was approved or denied, as well as
by the CLI's own `decisionSource` for that outcome (e.g. `human_response`; `unknown` when
no decision source was recorded, including sessions from older CLI versions and unresolved
requests without a recorded outcome). It also reports each session's recorded usage totals
(premium requests, AIU, and token counts), including unfinished sessions, summed
overall and broken down by working directory; usage is counted independently of the
permission-request filters below, since it is not recorded per permission request.
Request-level `session.usage_record` events and cumulative `session.usage_checkpoint` and
`session.shutdown` snapshots are combined without double counting: each snapshot replaces
the counters it covers, and subsequent request records add new usage. Replayed request
records are deduplicated using their source-session accounting identity.
When request records or `modelMetrics` are available, `MODEL_USAGE` also reports model names, API
request counts (not permission requests), premium requests, AIU, and token counts per
model. JSON output includes this breakdown in `ByModelUsage`. Checkpoints can record AIU
and premium totals without model, request, or token details; those totals are included
without inventing missing details. `UsageIncompleteSessions` marks incomplete breakdowns,
and request/token counts for those sessions are recorded subtotals.
`AGENT_USAGE` (`ByRoleUsage` in JSON) separates `main`, `subagent`, and `unknown` usage
using recorded request metadata, not model names. Older or partially recorded usage stays
`unknown`. This role breakdown is never truncated by `--top`; it reports API requests,
premium requests, AIU, and tokens, not API duration. API request totals are also exported
as `UsageRequests`. Model usage follows the same session scope, time bounds, and `--top`
limit as working-directory usage. `--all`,
`--cwd`, and `--worktree`/`-W` are mutually exclusive and all optional; when none are given,
only sessions whose recorded working directory is inside the current git worktree are
counted. `--session` restricts to a single session ID. `--since` and `--until` accept
RFC3339 timestamps, `YYYY-MM-DD` dates, or a relative duration such as `7d`/`24h`/`30m`, and
default to no bound. Usage updates are selected by their recorded timestamps,
with an inclusive lower and exclusive upper bound. Earlier recorded totals are
subtracted from cumulative snapshots; a snapshot delta may include usage before
the selected period. `--period` is shorthand for `--since` using a window such
as `7d`/`3w`/`6m`/`1y`, and is mutually exclusive with `--since`. `--operation`
restricts to non-mutating (`read`) or mutating (`write`) requests. `--kind` restricts to an
exact tool kind (e.g. `shell`, `read`, `write`); `--decision-source` restricts to permission
outcomes with an exact decision source (e.g. `human_response`, `unattended_fallback`, or `unknown`
when no decision source was recorded, including older CLI versions and unresolved requests); `--command` restricts to requests that include an
exact command identifier; `--path` and `--url` restrict to requests with a recorded path or
URL matching the given regular expression (a plain substring is also a valid, unanchored
regular expression). `--operation`, `--kind`, `--decision-source`, `--command`, `--path`, and
`--url` may each be repeated to match any of multiple values. `--top` keeps only the top N
entries per statistic (default `10`; `0` keeps every entry).
`--format`, `--jq`, and `--template` follow the standard `gh` JSON export flags; without
`--format json`, results are printed as tables.

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

`ASK_USER` (`AskUser` in JSON) reports sessions with questions, question counts, and
answered/cancelled/failed/unresolved/unknown outcomes from paired `ask_user` tool events.
Answered questions with valid timestamps contribute to total, average, and maximum
wait in milliseconds; `TimedAnswers` and `UntimedAnswers` expose timing coverage.
Average and maximum are `null` (`n/a` in tables) without timed answers. Unknown completions
are not inferred as answers. Wait runs from tool start to completion and includes
UI/transport overhead; overlapping per-question intervals are summed, not deduplicated
as wall-clock idle time. Session/directory scope applies; time bounds select question
starts (inclusive `--since`, exclusive `--until`), while later completions still resolve
selected questions. Permission filters and `--top` do not affect this summary.
No question or answer text is exported. Older logs without recorded events cannot
reconstruct missing questions.

`PERMISSION_WAIT` (`PermissionWait` in JSON) reports request-to-completion elapsed time
separately from `ASK_USER`. `HumanResponses` includes only recorded `human_response`
decisions; `NonHumanDecisions` and `UnknownSourceDecisions` retain other and missing-source
decision times separately, without inferring human wait from an approval or a long delay.
Each group reports completed/timed/untimed counts and total/average/maximum milliseconds.
Zero-length intervals are valid; missing or negative timing is untimed and excluded.
Average/maximum are `null` (`n/a` in tables) without timed decisions. Unresolved requests
have no inferred duration. Replay duplicates count once, paired by session/agent/request
identity. All permission filters and directory/session scope apply; time bounds select
request starts, while later completions still resolve them. `--top` does not truncate
these totals. Intervals include UI/transport overhead; overlapping waits are summed,
not unique wall-clock idle time. Unlogged rule-based approvals cannot be reconstructed.

### skills

`gh copilot-attendant skills` manages the agent skills bundled with this extension. Run
`gh copilot-attendant skills --help` for its subcommands.

### vscode

#### vscode stats

```sh
gh copilot-attendant vscode stats [--all | --cwd <path> | --worktree <path> | -W <path>] [--session <id>] [--since <time> | --period <period>] [--until <time>] [--tool <pattern>]... [--model <pattern>]... [--agent <pattern>]... [--include-log-stats] [--top <n>] [--format <format>] [--jq <expression>] [--template <template>]
```

Scans local VS Code Copilot Chat debug logs from the Local harness under
`~/Library/Application Support/Code/User/workspaceStorage` and reports tool
usage, LLM token and usage totals, turn counts, and subagent invocations.
Only the stable, non-Insiders VS Code installation on macOS is supported.
Subagent log files referenced by each session
are included recursively, with each file counted once. Their model requests, tokens,
AIU, tool calls, and turns are attributed to the parent workspace; session counts still
refer to parent chat sessions. Internal
title-generation child logs and unreferenced files are excluded. Time bounds apply to
each event, including child activity whose invocation predates `--since`.
When `--agent` is specified, all activity is scoped to matching subagents' own child logs:
model requests, AIU, tokens, tool calls, turns, and workspace totals. Main-agent usage is
excluded, and differently named descendants are excluded unless they also match an agent
pattern. Without `--agent`, parent and referenced child usage is included as before.
Tool and model filters apply independently within that scope. Missing child logs produce
a warning, while their recorded matching invocations remain counted.

VS Code Copilot harness (Agent Host) sessions recorded locally under
`~/.copilot/session-state` use CLI-format logs and are reported by
`gh copilot-attendant session stats` instead. The source depends on the harness,
not the VS Code version.

`--all`, `--cwd`, and `--worktree`/`-W` are mutually exclusive and all optional; when none
are given, only sessions belonging to the
current git worktree are counted. `--session` restricts to a single session ID. `--since`
and `--until` accept RFC3339 timestamps, `YYYY-MM-DD` dates, or a relative duration such as
`7d`/`24h`/`30m`, and default to no bound. `--period` is shorthand for `--since` using a
coarser window such as `7d`/`3w`/`6m`/`1y`, and is mutually exclusive with `--since`.
`--tool` and `--model` restrict tool calls and LLM requests; `--agent` scopes activity by
subagent name. All three are optional and default to no filter. They match the given
regular expression (a plain substring is also a valid, unanchored regular expression)
and may each be repeated to match any of multiple values.
`--include-log-stats` is optional (default `false`) and adds versioned `LogDetails` to JSON
output. Details contain each parent chat's identity and workspace, unique log identities,
main/child classification, all referencing agent names, normalized own-activity metrics,
and invocation identities in traversal order. These details are not truncated by `--top`
and contain no raw events or conversation content. They support local agent filtering
without double-counting shared logs or parent chats; positive TTFT sample counts and sums
are retained for accurate averaging. This flag cannot be combined with `--agent`.
`--top` keeps only the top N entries per statistic (default `10`; `0` keeps every entry).
`--format`, `--jq`, and `--template` follow the standard `gh` JSON export flags; without
`--format json`, results are printed as tables.

For example, report only Explore agents' own usage across all workspaces over the last
seven days:

```sh
gh copilot-attendant vscode stats --all --period 7d --agent '^Explore$'
```

`USER_QUESTIONS` (`UserQuestions` in JSON) reports completed `vscode_askQuestions`
calls, sessions with calls, and answered/skipped/cancelled/failed/unknown outcomes.
Each multi-question batch counts as one call; a partially skipped batch with at least
one recorded answer counts as answered, while a fully skipped batch counts as skipped.
Only answered calls with valid recorded `dur` contribute to total/average/maximum
wait in milliseconds. `TimedAnswers` and `UntimedAnswers` expose timing coverage;
average/maximum are `null` (`n/a` in tables) without timed answers.
Time bounds select call starts, even when completion falls after `--until`.
Session/directory/agent/tool filters apply; model filters and `--top` do not.
Duration includes tool/UI/transport overhead; overlapping intervals are summed,
not deduplicated as wall-clock idle time. In-flight questions cannot be reconstructed
from completed-call logs. No question or answer text is exported. Log details include
these untruncated per-log metrics for exact local filtering and parent-session counting.

## Copilot app dashboard

This repository ships a canvas extension for the GitHub Copilot app in
[`.github/extensions/attendant-dashboard`](../.github/extensions/attendant-dashboard/README.md).
When a Copilot session runs inside this repository, ask the agent to open the
"Copilot attendant dashboard" to browse `session stats` and `vscode stats` results as
interactive charts, filter by clicking entries, and hand the data back to the agent.
The VS Code tab always explains that it reads logs from the Local harness.
Locally recorded VS Code Copilot harness (Agent Host) sessions are included in the
Copilot CLI tab. This distinction depends on the harness, not the VS Code version.
The VS Code canvas fetches log details once and processes Agent and Top changes locally,
without re-running gh. Refresh, scope/time/session changes, and tool/model filter changes
fetch a new dataset. Filtered display, summaries, and Ask agent use the same projected data.
On the Copilot CLI tab, **Fusion usage only** restricts the dashboard to usage
records explicitly tagged as Fusion, excluding permission data, resolution
events, and checkpoint-only usage. The switch applies locally to matching
cached data; scope, time, session, and Top changes fetch new data. Metric
coverage distinguishes missing values from zero, and record counts are not
necessarily distinct API calls. The fetch command shown in this mode returns
the unfiltered CLI result; filtering is applied by the dashboard. Use an
updated CLI or the canvas `source` runner if Fusion-only details are unavailable.
Filterable panels have a Clear button that removes only that panel's conditions.
Workspaces and Working directories clear the directory scope back to all workspaces.
Other filters, the time window, session selection, and Top remain unchanged.
Both tabs show a User questions panel with answer outcomes and total/average/maximum
wait; VS Code counts completed tool calls, while CLI pairs recorded question events.
User questions follows Workspaces on both tabs and precedes Agent usage on CLI.
The CLI Permissions summary uses the same bordered panel style as the other panels.
It also shows confirmed human permission wait and separate other/unknown-source
decision timing. VS Code Local harness debug logs do not independently identify
ordinary permission request/response waits; confirmation-tool or execution durations
are not substituted.
