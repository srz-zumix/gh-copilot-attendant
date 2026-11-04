package metrics

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/copilotmetrics"
	"github.com/srz-zumix/gh-copilot-attendant/pkg/render"
	"github.com/srz-zumix/go-gh-extension/pkg/gh"
	"github.com/srz-zumix/go-gh-extension/pkg/parser"
)

// NewReportCmd creates the "report" command.
func NewReportCmd() *cobra.Command {
	var org string
	var enterprise string
	var day string
	var since string
	var until string
	var sections []string
	var formats []string
	var outputDir string
	var top int
	var strict bool
	var rich bool
	var repo string
	var createIssue bool
	var issue int
	var title string
	var labels []string
	var allowPublic bool
	var pages bool

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Build a shareable summary of Copilot usage metrics",
		Long: `Read the GitHub Copilot usage metrics reports of an organization (--org) or enterprise
(--enterprise) and render a summary that can be shared with people who cannot view the
usage metrics dashboard. The caller needs access to the usage metrics API (for example an
organization owner, or an enterprise owner or billing manager) and the "Copilot usage
metrics" policy must be enabled.

By default the latest 28-day report is used. Use --day for a single day, or --since and
optionally --until for a range of 1-day reports (YYYY-MM-DD, RFC3339, or a relative
duration like 7d). --until defaults to and is capped at yesterday.

--section selects the parts of the report: kpi, trend, feature, language, model, ide,
repository, team, and user, or all. The per-user ranking (user) is opt-in; every other
section is included by default. Active-user metrics are never summed across days, counters
are summed, and rates are derived from the summed values. Values missing from the reports
are shown as n/a. Days without a report are listed in the summary; --strict fails instead.
--top limits ranked tables (0 keeps every row).

--format selects md (Markdown with mermaid charts) and/or html (a self-contained page with
inline CSS and SVG charts). --rich adds dashboard-style KPI cards and breakdown charts to
HTML and requires --format html. The per-user section, including login names, is included
only with --section user or --section all, in either mode. User-level reports are also
fetched when selected team data or aggregate IDE active-user counts require them.
The IDE active-users card estimates distinct users with a reported IDE from the first
day of the calendar month through the report end day. It may fetch additional daily user
reports; missing days make the estimate partial and --strict fails.
HTML tables are sortable and color-coded in either mode. A single format is written to
stdout; use --output-dir to write
report.md and report.html.

--create-issue creates an issue in --repo (defaults to the current repository) whose body
is a short overview, and --issue uses an existing issue instead. Each rendered report is
attached to the issue as an issue artifact (Markdown as generic, HTML as embedded_html).
An artifact with the same name, which identifies the source, window, and format, is
updated instead of duplicated. Publishing to a public repository requires --allow-public.

--pages publishes HTML and a report-history index to a dedicated private GitHub Pages
site in --repo and requires --format html. The destination must be an organization-owned
private/internal project repository on GitHub Enterprise Cloud with an initial commit.
If the repository has no Pages site, the dedicated copilot-metrics-pages branch is created
(or reused when it already holds a managed history) and the site is initialized. Only a
harmless placeholder may be public during setup, and metrics are not fetched until privacy
is verified. Existing unrelated sites or branches are never overwritten.
Pages cannot be combined with issue publication or --allow-public. Multiple formats with
Pages require --output-dir. Source updates do not confirm deployment: Pages deploys
asynchronously, and commits made with a workflow's GITHUB_TOKEN do not trigger branch builds.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			stderr := cmd.ErrOrStderr()

			sectionSet, err := copilotmetrics.ParseSections(sections)
			if err != nil {
				return fmt.Errorf("parse --section: %w", err)
			}
			formatList, err := copilotmetrics.ParseFormats(formats)
			if err != nil {
				return fmt.Errorf("parse --format: %w", err)
			}
			period, err := copilotmetrics.ResolvePeriod(day, since, until, time.Now())
			if err != nil {
				return fmt.Errorf("resolve report period: %w", err)
			}
			publishOpts := copilotmetrics.PublishOptions{
				Repo:        repo,
				CreateIssue: createIssue,
				Issue:       issue,
				Title:       title,
				Labels:      labels,
				AllowPublic: allowPublic,
				Pages:       pages,
			}
			if err := publishOpts.Validate(); err != nil {
				return fmt.Errorf("validate publish options: %w", err)
			}
			outputOpts := copilotmetrics.OutputOptions{Formats: formatList, OutputDir: outputDir, Rich: rich}
			if err := outputOpts.Validate(publishOpts); err != nil {
				return fmt.Errorf("validate output options: %w", err)
			}
			owner := org
			if enterprise != "" {
				owner = enterprise
			}
			sourceOwner, err := parser.Repository(parser.RepositoryOwnerWithHost(owner))
			if err != nil {
				return fmt.Errorf("parse metrics source %q: %w", owner, err)
			}
			source := gh.CopilotMetricsSource{Owner: sourceOwner, Enterprise: enterprise != ""}
			sourceClient, err := gh.NewGitHubClientWithRepo(sourceOwner)
			if err != nil {
				return fmt.Errorf("create GitHub client for %s: %w", source, err)
			}

			var publisher *copilotmetrics.GitHubIssuePublisher
			var pagesPublisher *copilotmetrics.PagesPublisher
			var pagesTarget *copilotmetrics.PagesTarget
			if publishOpts.Enabled() {
				destRepo, err := parser.Repository(parser.RepositoryInput(repo))
				if err != nil {
					return fmt.Errorf("resolve destination repository: %w", err)
				}
				destClient, err := gh.NewGitHubClientWithRepo(destRepo)
				if err != nil {
					return fmt.Errorf("create GitHub client for %s: %w", parser.GetRepositoryFullNameWithHost(destRepo), err)
				}
				if publishOpts.Pages {
					pagesPublisher = copilotmetrics.NewGitHubPagesPublisher(destClient, destRepo)
					pagesTarget, err = pagesPublisher.Prepare(ctx)
					if err != nil {
						return fmt.Errorf("prepare Pages destination %s: %w", parser.GetRepositoryFullName(destRepo), err)
					}
					if pagesTarget.Initialized() {
						if _, err := fmt.Fprintf(stderr, "Initialized private Pages site %s from branch %s\n", pagesTarget.URL(), pagesTarget.Branch()); err != nil {
							return fmt.Errorf("write progress message to stderr: %w", err)
						}
					}
				} else {
					publisher = copilotmetrics.NewGitHubIssuePublisher(destClient, destRepo)
					if err := copilotmetrics.CheckPublishTarget(ctx, publisher, publishOpts); err != nil {
						return fmt.Errorf("check destination %s: %w", parser.GetRepositoryFullName(destRepo), err)
					}
				}
			}

			dataset, err := copilotmetrics.Collect(ctx, copilotmetrics.NewGitHubFetcher(sourceClient, source), copilotmetrics.CollectOptions{
				Sections:              sectionSet,
				Period:                period,
				IncludeIDEActiveUsers: rich && sectionSet.Has(copilotmetrics.SectionIDE),
				Strict:                strict,
				Progress:              stderr,
			})
			if err != nil {
				return fmt.Errorf("collect usage metrics of %s: %w", source, err)
			}
			summary := copilotmetrics.BuildSummary(dataset, copilotmetrics.SummaryOptions{
				Source:             source.String(),
				Sections:           sectionSet,
				Top:                top,
				GeneratedAt:        time.Now(),
				IncludeRichMetrics: rich,
			})
			reports := map[copilotmetrics.Format]string{}
			var artifacts []copilotmetrics.Artifact
			for _, f := range formatList {
				body, err := render.RenderCopilotMetricsReportWithOptions(summary, f, render.CopilotMetricsReportOptions{Rich: rich})
				if err != nil {
					return fmt.Errorf("render %s report: %w", f, err)
				}
				reports[f] = body
				artifacts = append(artifacts, copilotmetrics.NewArtifact(summary, f, body))
			}

			if outputOpts.WritesStdout(publishOpts) {
				if _, err := fmt.Fprint(cmd.OutOrStdout(), reports[formatList[0]]); err != nil {
					return fmt.Errorf("write report to stdout: %w", err)
				}
			}
			if outputDir != "" {
				paths, err := copilotmetrics.WriteReports(outputDir, reports, formatList)
				if err != nil {
					return fmt.Errorf("write reports to %s: %w", outputDir, err)
				}
				for _, p := range paths {
					if _, err := fmt.Fprintf(stderr, "Wrote %s\n", p); err != nil {
						return fmt.Errorf("write progress message to stderr: %w", err)
					}
				}
			}

			if pagesPublisher != nil {
				result, err := pagesPublisher.Publish(ctx, pagesTarget, source, summary, reports[copilotmetrics.FormatHTML])
				if err != nil {
					return fmt.Errorf("publish usage metrics report to Pages: %w", err)
				}
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Pages source updated: %s\nBranch: %s\nCommit: %s\nDeployment: not verified (GitHub Pages deploys asynchronously)\n", result.URL, result.Branch, result.CommitSHA); err != nil {
					return fmt.Errorf("write Pages result to stdout: %w", err)
				}
				return nil
			}
			if publisher == nil {
				return nil
			}
			issueTitle := title
			if issueTitle == "" {
				issueTitle = copilotmetrics.DefaultIssueTitle(summary)
			}
			names := make([]string, 0, len(artifacts))
			for _, a := range artifacts {
				names = append(names, a.Name)
			}
			body := render.RenderCopilotMetricsIssueBody(summary, names)
			result, err := copilotmetrics.Publish(ctx, publisher, publishOpts, issueTitle, body, artifacts)
			if err != nil {
				return fmt.Errorf("publish usage metrics report: %w", err)
			}
			out := cmd.OutOrStdout()
			if _, err := fmt.Fprintf(out, "Issue: %s\n", result.IssueURL); err != nil {
				return fmt.Errorf("write issue URL to stdout: %w", err)
			}
			for _, a := range result.Artifacts {
				action := "Updated"
				if a.Created {
					action = "Created"
				}
				line := fmt.Sprintf("%s artifact: %s", action, a.Name)
				if a.URL != "" {
					line += fmt.Sprintf(" (%s)", a.URL)
				}
				if _, err := fmt.Fprintln(out, line); err != nil {
					return fmt.Errorf("write artifact %s result to stdout: %w", a.Name, err)
				}
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.BoolVar(&allowPublic, "allow-public", false, "allow publishing to a public repository")
	f.BoolVar(&createIssue, "create-issue", false, "create an issue and attach the rendered reports as issue artifacts")
	f.StringVar(&day, "day", "", "report a single day (YYYY-MM-DD) from its 1-day report")
	f.StringVar(&enterprise, "enterprise", "", "enterprise to report on, as [HOST/]ENTERPRISE")
	f.StringSliceVar(&formats, "format", nil, "output format: md or html (default md); may be repeated or comma separated")
	f.IntVar(&issue, "issue", 0, "attach the rendered reports as artifacts to this existing issue number")
	f.StringSliceVar(&labels, "label", nil, "label to add to the created issue; may be repeated; requires --create-issue")
	f.StringVar(&org, "org", "", "organization to report on, as [HOST/]ORG")
	f.StringVar(&outputDir, "output-dir", "", "write report.md and/or report.html to this directory instead of stdout")
	f.BoolVar(&pages, "pages", false, "publish HTML and report history to a dedicated private GitHub Pages site; requires --format html")
	f.StringVarP(&repo, "repo", "R", "", "repository to publish to, as [HOST/]OWNER/REPO (defaults to the current repository)")
	f.BoolVar(&rich, "rich", false, "add dashboard-style KPI cards and breakdown charts to HTML; requires --format html")
	f.StringSliceVar(&sections, "section", nil, "sections to include: "+strings.Join(copilotmetrics.SectionNames(), ", ")+", or all (default: all except user); may be repeated or comma separated")
	f.StringVar(&since, "since", "", "first day of a range of 1-day reports (YYYY-MM-DD, RFC3339, or a relative duration like 7d)")
	f.BoolVar(&strict, "strict", false, "fail when any selected report is missing for a day or the report windows differ")
	f.StringVar(&title, "title", "", "title of the created issue (defaults to the source and window); requires --create-issue")
	f.IntVar(&top, "top", 20, "keep only the top N rows of ranked tables (0 keeps every row)")
	f.StringVar(&until, "until", "", "last day of the range (defaults to and is capped at yesterday); requires --since")
	cmd.MarkFlagsMutuallyExclusive("org", "enterprise")
	cmd.MarkFlagsOneRequired("org", "enterprise")
	cmd.MarkFlagsMutuallyExclusive("create-issue", "issue")
	cmd.MarkFlagsMutuallyExclusive("pages", "create-issue")
	cmd.MarkFlagsMutuallyExclusive("pages", "issue")
	cmd.MarkFlagsMutuallyExclusive("pages", "allow-public")
	cmd.MarkFlagsMutuallyExclusive("day", "since")
	cmd.MarkFlagsMutuallyExclusive("day", "until")
	_ = cmd.RegisterFlagCompletionFunc("section", cobra.FixedCompletions(append(copilotmetrics.SectionNames(), "all"), cobra.ShellCompDirectiveNoFileComp))
	_ = cmd.RegisterFlagCompletionFunc("format", cobra.FixedCompletions([]string{"md", "html"}, cobra.ShellCompDirectiveNoFileComp))

	return cmd
}
