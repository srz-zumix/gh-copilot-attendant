package session

import (
	"fmt"
	"time"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
	localrender "github.com/srz-zumix/gh-copilot-attendant/pkg/render"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/sessionstats"
	"github.com/srz-zumix/go-gh-extension/pkg/copilotext"
	"github.com/srz-zumix/go-gh-extension/pkg/parser"
	"github.com/srz-zumix/go-gh-extension/pkg/render"
)

// NewStatsCmd creates the "stats" command.
func NewStatsCmd() *cobra.Command {
	var all bool
	var cwd string
	var worktree string
	var sessionID string
	var since string
	var until string
	var period string
	var operations []string
	var agentModes []string
	var permissionModes []string
	var results []string
	var commandAccess []string
	var sandboxes []string
	var kinds []string
	var decisionSources []string
	var commands []string
	var paths []string
	var urls []string
	var top int
	var exporter cmdutil.Exporter

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Report permission, usage, and user-question statistics from local Copilot CLI history",
		Long: `Scan the GitHub Copilot CLI's local session history and report how often each
tool, command, file path, and URL was requested, and whether it was approved or denied. It
also reports recorded usage (premium requests, AIU, and token counts), including unfinished
sessions, by working directory, model, and main/subagent/unknown role.
Request-level "session.usage_record" events and cumulative "session.usage_checkpoint"
and "session.shutdown" snapshots are combined without double counting. Model breakdowns
use request records or "modelMetrics"; checkpoint-only usage may have no model or token
breakdown. Unattributed usage is reported as unknown, never inferred from model names.
Model request counts are API requests, not permission requests.

Agent mode counts group permission requests by their recorded "agentMode" at evaluation
start, with approval/denial outcomes. Missing modes are unknown; session mode changes and
main/subagent roles are not used to infer them. All permission filters and --top apply.

Permission modes group requests by their recorded "permissionMode" at evaluation start,
separately from Agent modes. Missing or invalid modes are unknown. Use --permission-mode
with --decision-source human_response to select human decisions made while assisted
permissions were enabled; the mode does not imply a safety review ran.

Fusion statistics count recorded resolved turns by pattern and recorded usage by pattern,
phase, role, and model. They include only explicitly attributed usage records, not planned
phases or checkpoint-only usage. Missing AIU and identifiers are shown as incomplete.
Time bounds select individual Fusion events; permission filters do not apply. --top limits
each Fusion breakdown, but not its totals. Phase and model rows can share turns or phases.
Active sessions may change while scanning; shared and Fusion totals are not an atomic snapshot.

Sandbox statistics count completed shell tool executions and other completed tools with a
recorded "sandboxApplied" outcome: applied, not-applied, or unknown for shell executions
without an outcome. Unfinished calls are excluded. Requested sandbox opt-outs are reported
separately. Session scope, time bounds (by completion time), and --sandbox apply; other
permission filters do not. Permission requests are also grouped by the sandbox state of
their tool call, and --sandbox restricts permission requests to tool calls in the selected states.

JSON output includes SessionActivity: distinct sessions with timestamped events by local
date and hour, not continuous active time. Repeated clock hours are combined. Directory
and session scope and event-time bounds apply; --agent-mode selects permission.requested
events by their recorded mode and request timestamp. With --sandbox, it counts matching
completed tool executions by completion time instead. If both filters are set, executions
must link to a permission request in a selected Agent mode. Other permission filters and
--top do not apply. Records without usable timestamps and malformed envelopes are counted
as coverage gaps.

Permission request-to-completion intervals report total, average, and maximum wait.
Confirmed human_response waits are separate from non-human and unknown-source decisions;
older logs without decisionSource are not inferred to be human responses. Invalid timing
and unresolved requests are excluded from durations. All permission filters apply;
time bounds select request starts, while completions after --until still resolve them.
--top does not truncate wait totals. Intervals include UI/transport overhead, and
overlapping waits are summed rather than deduplicated as wall-clock idle time.

Recorded ask_user starts and completions also report questions, answered, cancelled,
failed, unresolved, and unknown outcomes, plus total, average, and maximum answer wait.
Only answered questions with valid paired timestamps contribute to wait metrics.
These intervals include tool/UI/transport overhead, not only user thinking time.
Time bounds select questions by their start time; completions after --until still resolve
selected questions. Permission filters and --top do not affect question statistics.

By default only sessions whose recorded working directory is inside the current git
worktree are counted. Use --worktree to scope to a different worktree, --cwd to match a
single working directory exactly, or --all to include every recorded session.

Use --period as shorthand for --since when you only need a time window back from now
(e.g. 7d, 3w, 6m, 1y).

Use --operation to restrict requests to non-mutating ("read") or mutating ("write")
requests. Use --result to match an exact recorded result, --command-access to match
requests containing commands with the selected read-only/read-write attribute, and
--agent-mode, --permission-mode, --kind, --sandbox, --command, --path, or --url to
restrict requests matching that recorded mode, tool kind, command, path, or URL.
--agent-mode, --permission-mode, and --result match exactly. For --agent-mode and
--permission-mode, use unknown for missing or invalid recorded modes. --path and --url match as regular expressions (a
plain substring is also a valid, unanchored regular expression). --decision-source restricts
to permission outcomes whose recorded decision source matches exactly (e.g. human_response,
unattended_fallback, or unknown when no decision source was recorded, including older CLI
versions and unresolved requests without a recorded outcome). Each of these
flags may be repeated to match any of multiple values; different filters are combined.
--since, --until, and --period also
select usage updates by their recorded timestamps. Earlier recorded totals are subtracted
from cumulative snapshots; a snapshot delta may include usage before the selected period.
The role breakdown is never truncated by --top. Missing breakdowns are marked as incomplete;
--agent-mode also restricts usage totals and breakdowns to sessions with at least one
matching permission request in the selected time window; all usage of those sessions is
counted because usage is not recorded per mode. --operation, --permission-mode, --kind, --decision-source,
--result, --command-access, --sandbox, --command, --path, and --url do not affect usage totals,
since usage is not recorded per permission request. JSON output lists the modes applied
to usage in UsageAgentModes.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			scope, err := copilotext.ResolveSessionScope(ctx, all, cwd, worktree)
			if err != nil {
				return fmt.Errorf("resolve session scope: %w", err)
			}

			opts := copilotext.PermissionStatsOptions{
				Scope:     scope,
				SessionID: sessionID,
				Top:       top,
			}
			if since != "" {
				t, err := parser.ParseTime(since)
				if err != nil {
					return fmt.Errorf("parse --since: %w", err)
				}
				opts.Since = t
			}
			if until != "" {
				t, err := parser.ParseTime(until)
				if err != nil {
					return fmt.Errorf("parse --until: %w", err)
				}
				opts.Until = t
			}
			if period != "" {
				d, err := parser.ParsePeriod(period)
				if err != nil {
					return fmt.Errorf("parse --period: %w", err)
				}
				opts.Since = time.Now().Add(-d)
			}
			opts.Operations = operations
			opts.AgentMode = agentModes
			opts.PermissionMode = permissionModes
			opts.Results = results
			opts.CommandAccess = commandAccess
			opts.Sandbox = sandboxes
			opts.Kind = kinds
			opts.DecisionSource = decisionSources
			opts.Command = commands
			opts.Path = paths
			opts.URL = urls

			stats, err := sessionstats.Collect(opts)
			if err != nil {
				return fmt.Errorf("collect session stats: %w", err)
			}

			r := render.NewRenderer(exporter)
			if err := localrender.RenderCopilotSessionStats(r, stats); err != nil {
				return fmt.Errorf("render session stats: %w", err)
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.BoolVar(&all, "all", false, "include every recorded session, ignoring the current directory")
	f.StringVar(&cwd, "cwd", "", "match only sessions whose recorded working directory equals this path exactly")
	f.StringVarP(&worktree, "worktree", "W", "", "match sessions under this git worktree's root (defaults to the current directory's worktree)")
	f.StringVar(&sessionID, "session", "", "match only the session with this ID")
	f.StringVar(&since, "since", "", "only count requests made at or after this time (RFC3339, YYYY-MM-DD, or a relative duration like 2w/7d/24h/30m)")
	f.StringVar(&until, "until", "", "only count requests made before this time (RFC3339, YYYY-MM-DD, or a relative duration like 2w/7d/24h/30m)")
	f.StringVar(&period, "period", "", "only count requests made within this period back from now (e.g. 7d, 3w, 6m, 1y); shorthand for --since")
	cmdutil.StringSliceEnumFlag(cmd, &operations, "operation", "", nil, []string{"read", "write"}, "only count requests classified as this operation; may be repeated to match multiple operations")
	f.StringArrayVar(&agentModes, "agent-mode", nil, "only count requests with this exact recorded Agent mode (use unknown for missing modes); may be repeated")
	f.StringArrayVar(&permissionModes, "permission-mode", nil, "only count requests with this exact recorded permission mode (use unknown for missing modes); may be repeated")
	f.StringArrayVar(&results, "result", nil, "only count requests with this exact recorded result; may be repeated")
	cmdutil.StringSliceEnumFlag(cmd, &commandAccess, "command-access", "", nil, []string{"read-only", "read-write"}, "only count requests containing commands with this access mode; may be repeated")
	cmdutil.StringSliceEnumFlag(cmd, &sandboxes, "sandbox", "", nil, copilotext.SandboxStates, "only count tool calls whose recorded sandbox state matches; may be repeated")
	f.StringArrayVar(&kinds, "kind", nil, "only count requests of this exact tool kind (e.g. shell, read, write); may be repeated to match multiple kinds")
	f.StringArrayVar(&decisionSources, "decision-source", nil, "only count permission outcomes with this exact decision source (e.g. human_response, unattended_fallback, unknown); may be repeated to match multiple values")
	f.StringArrayVar(&commands, "command", nil, "only count requests that include this exact command identifier; may be repeated to match multiple commands")
	f.StringArrayVar(&paths, "path", nil, "only count requests with a recorded path matching this regular expression; may be repeated to match any of multiple patterns")
	f.StringArrayVar(&urls, "url", nil, "only count requests with a recorded URL matching this regular expression; may be repeated to match any of multiple patterns")
	f.IntVar(&top, "top", 10, "keep only the top N entries per statistic (0 keeps every entry)")
	cmd.MarkFlagsMutuallyExclusive("all", "cwd", "worktree")
	cmd.MarkFlagsMutuallyExclusive("since", "period")
	cmdutil.AddFormatFlags(cmd, &exporter)

	return cmd
}
