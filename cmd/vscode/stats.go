package vscode

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
	var tools []string
	var models []string
	var agents []string
	var includeLogStats bool
	var top int
	var exporter cmdutil.Exporter

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Report tool, model, subagent, and user-question statistics from local VS Code Copilot Chat history",
		Long: `Scan the VS Code GitHub Copilot Chat extension's local debug logs and report tool
usage, LLM token/usage totals, turn counts, and subagent invocations.

Completed vscode_askQuestions calls report answered, skipped, cancelled, failed, and
unknown outcomes, plus total/average/maximum answer wait from recorded tool duration.
Only answered calls with valid duration contribute to timing. Each multi-question batch
counts once; partially skipped batches with an answer count as answered. In-flight questions
are unavailable. Time bounds select call starts. Session, directory, agent, and tool filters
apply; model filters and --top do not affect question totals.

Referenced subagent logs are included once each, with activity attributed to the parent
workspace. Session counts refer to parent chats, not individual subagent logs.

By default only sessions belonging to the current git worktree are counted. Use
--worktree to scope to a different worktree, --cwd to match a single working directory
exactly, or --all to include every recorded session.

Use --period as shorthand for --since when you only need a time window back from now
(e.g. 7d, 3w, 6m, 1y).

Use --agent to scope all usage to matching subagents' own child logs and invocations.
Main-agent usage is excluded. Differently named descendants are excluded unless they
also match an agent pattern. Without --agent, parent and referenced child usage is included.
Session counts still refer to parent chats. Missing child logs produce warnings without
dropping recorded invocations.

Within that scope, --tool and --model independently restrict tool calls and LLM requests.
Each name filter matches as a regular expression (a plain substring is also a valid,
unanchored regular expression) and may be repeated to match any of multiple values.

Use --include-log-stats with JSON output to export untruncated normalized log metrics,
referencing agent names, parent chat identities, and invocation identities for local
filtering. It is optional (default false) and cannot be combined with --agent.

Only the stable, non-Insiders VS Code installation on macOS is supported.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			scope, err := copilotext.ResolveSessionScope(ctx, all, cwd, worktree)
			if err != nil {
				return fmt.Errorf("resolve session scope: %w", err)
			}

			opts := copilotext.VSCodeStatsOptions{
				Scope:           scope,
				SessionID:       sessionID,
				Tool:            tools,
				Model:           models,
				Agent:           agents,
				IncludeLogStats: includeLogStats,
				Top:             top,
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

			stats, err := copilotext.CollectVSCodeStats(opts)
			if err != nil {
				return fmt.Errorf("collect vscode stats: %w", err)
			}

			r := render.NewRenderer(exporter)
			if err := r.RenderVSCodeStats(stats); err != nil {
				return fmt.Errorf("render vscode stats: %w", err)
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.BoolVar(&all, "all", false, "include every recorded session, ignoring the current directory")
	f.StringVar(&cwd, "cwd", "", "match only sessions whose recorded workspace folder equals this path exactly")
	f.StringVarP(&worktree, "worktree", "W", "", "match sessions under this git worktree's root (defaults to the current directory's worktree)")
	f.StringVar(&sessionID, "session", "", "match only the session with this ID")
	f.StringVar(&since, "since", "", "only count events at or after this time (RFC3339, YYYY-MM-DD, or a relative duration like 2w/7d/24h/30m)")
	f.StringVar(&until, "until", "", "only count events before this time (RFC3339, YYYY-MM-DD, or a relative duration like 2w/7d/24h/30m)")
	f.StringVar(&period, "period", "", "only count events within this period back from now (e.g. 7d, 3w, 6m, 1y); shorthand for --since")
	f.StringArrayVar(&tools, "tool", nil, "only count tool calls matching this regular expression; may be repeated to match any of multiple patterns")
	f.StringArrayVar(&models, "model", nil, "only count LLM requests matching this model regular expression; may be repeated to match any of multiple patterns")
	f.StringArrayVar(&agents, "agent", nil, "scope usage and invocations to subagents matching this regular expression; may be repeated to match any of multiple patterns")
	f.BoolVar(&includeLogStats, "include-log-stats", false, "export untruncated normalized log details for local filtering (JSON; incompatible with --agent)")
	f.IntVar(&top, "top", 10, "keep only the top N entries per statistic (0 keeps every entry)")
	cmd.MarkFlagsMutuallyExclusive("all", "cwd", "worktree")
	cmd.MarkFlagsMutuallyExclusive("since", "period")
	cmd.MarkFlagsMutuallyExclusive("agent", "include-log-stats")
	cmdutil.AddFormatFlags(cmd, &exporter)

	return cmd
}
