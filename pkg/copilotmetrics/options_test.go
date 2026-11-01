package copilotmetrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSections(t *testing.T) {
	set, err := ParseSections(nil)
	require.NoError(t, err)
	assert.Equal(t, DefaultSections(), set)
	assert.False(t, set.Has(SectionUser))

	set, err = ParseSections([]string{"kpi, trend", "Team"})
	require.NoError(t, err)
	assert.Equal(t, []Section{SectionKPI, SectionTrend, SectionTeam}, set.Ordered())
	assert.True(t, set.NeedsOrganizationLevel())
	assert.True(t, set.NeedsUsers())
	assert.True(t, set.NeedsUserTeams())
	assert.False(t, set.NeedsRepositories())

	set, err = ParseSections([]string{"all"})
	require.NoError(t, err)
	assert.Equal(t, AllSections, set.Ordered())

	_, err = ParseSections([]string{"kpi,bogus"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
}

func TestResolvePeriod(t *testing.T) {
	// parser.ParseTime reads YYYY-MM-DD in the local time zone, as does the command.
	now := time.Date(2030, 3, 10, 15, 0, 0, 0, time.Local)

	p, err := ResolvePeriod("", "", "", now)
	require.NoError(t, err)
	assert.True(t, p.Latest)

	p, err = ResolvePeriod("2030-03-01", "", "", now)
	require.NoError(t, err)
	assert.Equal(t, []string{"2030-03-01"}, p.Days)

	_, err = ResolvePeriod("2030-03-10", "", "", now)
	require.Error(t, err)

	_, err = ResolvePeriod("2030/03/01", "", "", now)
	require.Error(t, err)

	_, err = ResolvePeriod("2030-03-01", "2030-03-01", "", now)
	require.Error(t, err)

	_, err = ResolvePeriod("", "", "2030-03-05", now)
	require.Error(t, err)

	p, err = ResolvePeriod("", "2030-03-07", "", now)
	require.NoError(t, err)
	assert.Equal(t, []string{"2030-03-07", "2030-03-08", "2030-03-09"}, p.Days)

	p, err = ResolvePeriod("", "2030-03-07", "2030-03-20", now)
	require.NoError(t, err)
	assert.Equal(t, "2030-03-09", p.Days[len(p.Days)-1], "until is capped at yesterday")

	p, err = ResolvePeriod("", "2030-02-27", "2030-03-01", now)
	require.NoError(t, err)
	assert.Equal(t, []string{"2030-02-27", "2030-02-28", "2030-03-01"}, p.Days)

	_, err = ResolvePeriod("", "2030-03-12", "", now)
	require.Error(t, err)
}

func TestDayRange(t *testing.T) {
	assert.Equal(t, []string{"2030-12-31", "2031-01-01"}, DayRange("2030-12-31", "2031-01-01"))
	assert.Nil(t, DayRange("bad", "2031-01-01"))
	assert.Nil(t, DayRange("2031-01-02", "2031-01-01"))
}

func TestParseFormats(t *testing.T) {
	f, err := ParseFormats(nil)
	require.NoError(t, err)
	assert.Equal(t, []Format{FormatMarkdown}, f)

	f, err = ParseFormats([]string{"html,markdown", "md", "HTML"})
	require.NoError(t, err)
	assert.Equal(t, []Format{FormatHTML, FormatMarkdown}, f)

	_, err = ParseFormats([]string{"pdf"})
	require.Error(t, err)
}

func TestPublishOptionsValidate(t *testing.T) {
	tests := []struct {
		name    string
		opts    PublishOptions
		wantErr bool
	}{
		{"disabled", PublishOptions{}, false},
		{"create", PublishOptions{CreateIssue: true, Title: "t", Labels: []string{"a"}}, false},
		{"existing", PublishOptions{Issue: 3, Repo: "octo/reports", AllowPublic: true}, false},
		{"both", PublishOptions{CreateIssue: true, Issue: 3}, true},
		{"negative", PublishOptions{Issue: -1}, true},
		{"title without create", PublishOptions{Issue: 3, Title: "t"}, true},
		{"label without create", PublishOptions{Labels: []string{"a"}}, true},
		{"repo without publish", PublishOptions{Repo: "octo/reports"}, true},
		{"allow-public without publish", PublishOptions{AllowPublic: true}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOutputOptionsValidate(t *testing.T) {
	both := []Format{FormatMarkdown, FormatHTML}
	assert.Error(t, OutputOptions{Formats: both}.Validate(PublishOptions{}))
	assert.NoError(t, OutputOptions{Formats: both, OutputDir: "out"}.Validate(PublishOptions{}))
	assert.NoError(t, OutputOptions{Formats: both}.Validate(PublishOptions{CreateIssue: true}))
	assert.ErrorContains(t, OutputOptions{Formats: []Format{FormatMarkdown}, Rich: true}.Validate(PublishOptions{}), "--rich requires --format html")
	assert.NoError(t, OutputOptions{Formats: both, Rich: true, OutputDir: "out"}.Validate(PublishOptions{}))
	assert.True(t, OutputOptions{Formats: []Format{FormatMarkdown}}.WritesStdout(PublishOptions{}))
	assert.False(t, OutputOptions{Formats: []Format{FormatMarkdown}}.WritesStdout(PublishOptions{Issue: 1}))
	assert.False(t, OutputOptions{OutputDir: "out"}.WritesStdout(PublishOptions{}))
}
