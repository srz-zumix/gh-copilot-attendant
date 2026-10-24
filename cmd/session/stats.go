package session

import (
	"fmt"
	"time"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
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

Use --operation to restrict to non-mutating ("read") or mutating ("write") requests, and
--kind, --command, --path, or --url to restrict to requests matching that tool kind,
command, path, or URL. --path and --url match as regular expressions (a plain substring is
also a valid, unanchored regular expression). --decision-source restricts to permission
outcomes whose recorded decision source matches exactly (e.g. human_response,
unattended_fallback, or unknown when no decision source was recorded, including older CLI
versions and unresolved requests without a recorded outcome). Each of these
flags may be repeated to match any of multiple values. --since, --until, and --period also
bound which sessions' lifetime usage totals are counted, by their latest recorded usage
update time (not individual API request times). The role breakdown is never truncated by
--top. Missing breakdowns are marked as incomplete;
--operation, --kind, --decision-source, --command, --path, and --url do not affect usage
totals, since usage is not recorded per permission request.`,
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
			opts.Kind = kinds
			opts.DecisionSource = decisionSources
			opts.Command = commands
			opts.Path = paths
			opts.URL = urls

			stats, err := copilotext.CollectPermissionStats(opts)
			if err != nil {
				return fmt.Errorf("collect permission stats: %w", err)
			}

			r := render.NewRenderer(exporter)
			if err := r.RenderCopilotPermissionStats(stats); err != nil {
				return fmt.Errorf("render permission stats: %w", err)
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
