package copilotmetrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalendarMonthDays(t *testing.T) {
	tests := []struct {
		end   string
		start string
		days  int
	}{
		{end: "2030-01-01", start: "2030-01-01", days: 1},
		{end: "2030-01-31", start: "2030-01-01", days: 31},
		{end: "2032-02-29", start: "2032-02-01", days: 29},
		{end: "2030-03-01", start: "2030-03-01", days: 1},
	}
	for _, tt := range tests {
		t.Run(tt.end, func(t *testing.T) {
			days := calendarMonthDays(tt.end)
			require.Len(t, days, tt.days)
			assert.Equal(t, tt.start, days[0])
			assert.Equal(t, tt.end, days[len(days)-1])
		})
	}
	assert.Empty(t, calendarMonthDays("2031-02-29"))
}

func TestIDEActiveUserEstimateDeduplicatesWithinCalendarMonth(t *testing.T) {
	ds := &Dataset{
		Days: DayRange("2030-01-03", "2030-01-05"),
		Users: []UserDay{
			{UserID: 1, Day: "2030-01-03", IDEs: []string{"Editor A", "Editor B"}},
			{UserID: 1, Day: "2030-01-05", IDEs: []string{"Editor B"}},
			{UserID: 2, Day: "2030-01-05", IDEs: []string{"Editor A"}},
			{UserID: 3, Day: "2030-01-04", IDEs: []string{"Editor A"}},
			{UserID: 4, Day: "2029-12-31", IDEs: []string{"Editor A"}},
			{UserID: 5, Day: "2030-01-05"},
			{UserID: 6, Day: "2030-01-05", IDEs: []string{""}},
		},
		Missing: map[Family][]string{FamilyUsers: {"2030-01-04"}},
		ideMonthSupplement: &ideMonthSupplement{
			days:        []string{"2030-01-01", "2030-01-02"},
			missingDays: []string{"2030-01-02"},
			users: []ideMonthUserDay{
				{day: "2030-01-01", userID: 1},
				{day: "2030-01-02", userID: 7},
			},
		},
	}

	estimate := buildIDEActiveUserEstimate(ds)
	require.NotNil(t, estimate)
	assert.Equal(t, "2030-01-01", estimate.StartDay)
	assert.Equal(t, "2030-01-05", estimate.EndDay)
	assert.Equal(t, Counter{Value: 2, Valid: true}, estimate.Users)
	assert.Equal(t, []string{"2030-01-02", "2030-01-04"}, estimate.MissingDays)

	summary := BuildSummary(ds, SummaryOptions{Sections: sections(SectionIDE), IncludeRichMetrics: true})
	assert.Equal(t, estimate, summary.Rich.IDEActiveUserMonth)
	withoutIDE := BuildSummary(ds, SummaryOptions{Sections: sections(SectionTrend), IncludeRichMetrics: true})
	assert.Nil(t, withoutIDE.Rich.IDEActiveUserMonth)
}

func TestIDEActiveUserEstimateAvailabilityAndMonthBoundary(t *testing.T) {
	t.Run("available empty reports are zero", func(t *testing.T) {
		ds := &Dataset{
			Days: DayRange("2030-01-02", "2030-01-03"),
			ideMonthSupplement: &ideMonthSupplement{
				days: []string{"2030-01-01"},
			},
		}
		estimate := buildIDEActiveUserEstimate(ds)
		require.NotNil(t, estimate)
		assert.Equal(t, Counter{Value: 0, Valid: true}, estimate.Users)
		assert.Empty(t, estimate.MissingDays)
	})
	t.Run("all missing reports are unavailable", func(t *testing.T) {
		ds := &Dataset{
			Days:    []string{"2030-01-03"},
			Users:   []UserDay{{UserID: 1, Day: "2030-01-03", IDEs: []string{"Editor A"}}},
			Missing: map[Family][]string{FamilyUsers: {"2030-01-03"}},
		}
		estimate := buildIDEActiveUserEstimate(ds)
		require.NotNil(t, estimate)
		assert.Equal(t, Counter{}, estimate.Users)
		assert.Equal(t, DayRange("2030-01-01", "2030-01-03"), estimate.MissingDays)
	})
	t.Run("period spanning months excludes earlier month", func(t *testing.T) {
		ds := &Dataset{
			Days: DayRange("2029-12-30", "2030-01-02"),
			Users: []UserDay{
				{UserID: 1, Day: "2029-12-31", IDEs: []string{"Editor A"}},
				{UserID: 2, Day: "2030-01-01", IDEs: []string{"Editor B"}},
				{UserID: 3, Day: "2030-01-02", IDEs: []string{"Editor A"}},
			},
		}
		estimate := buildIDEActiveUserEstimate(ds)
		require.NotNil(t, estimate)
		assert.Equal(t, Counter{Value: 2, Valid: true}, estimate.Users)
		assert.Equal(t, "2030-01-01", estimate.StartDay)
		assert.Empty(t, estimate.MissingDays)
	})
}
