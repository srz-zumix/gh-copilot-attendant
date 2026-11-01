package copilotmetrics

import (
	"fmt"
	"sort"
	"strings"
)

type richRepositoryAccumulator struct {
	repository   RichRepository
	nameDay      string
	reportedDays map[int]bool
}

func buildRichRepositories(ds *Dataset) []RichRepository {
	dayIndex := make(map[string]int, len(ds.Days))
	missing := daySet(ds.Missing[FamilyRepositories])
	available := make([]bool, len(ds.Days))
	availableDays := 0
	for index, day := range ds.Days {
		dayIndex[day] = index
		available[index] = !missing[day]
		if available[index] {
			availableDays++
		}
	}
	allAvailable := availableDays == len(ds.Days)
	repositories := map[int64]*richRepositoryAccumulator{}
	for _, record := range ds.Repositories {
		if record == nil {
			continue
		}
		index, inWindow := dayIndex[record.Day]
		if !inWindow || !available[index] {
			continue
		}
		acc := repositories[record.RepoID]
		if acc == nil {
			acc = &richRepositoryAccumulator{
				repository: RichRepository{
					ID:                         record.RepoID,
					Name:                       repositoryName(record),
					Reviews:                    make([]Counter, len(ds.Days)),
					SuggestionsComplete:        allAvailable,
					AppliedSuggestionsComplete: allAvailable,
				},
				nameDay:      record.Day,
				reportedDays: map[int]bool{},
			}
			for day, present := range available {
				acc.repository.Reviews[day].Valid = present
			}
			repositories[record.RepoID] = acc
		}
		if acc.reportedDays[index] {
			continue
		}
		acc.reportedDays[index] = true
		name := repositoryName(record)
		if record.Day < acc.nameDay || (record.Day == acc.nameDay && name < acc.repository.Name) {
			acc.nameDay = record.Day
			acc.repository.Name = name
		}
		acc.repository.Reviews[index] = Counter{}
		if record.PullRequests == nil {
			acc.repository.SuggestionsComplete = false
			acc.repository.AppliedSuggestionsComplete = false
			continue
		}
		pr := record.PullRequests
		if pr.TotalReviewedByCopilot != nil && *pr.TotalReviewedByCopilot >= 0 {
			acc.repository.Reviews[index].Add(pr.TotalReviewedByCopilot)
		}
		if pr.TotalCopilotSuggestions != nil && *pr.TotalCopilotSuggestions >= 0 {
			acc.repository.Suggestions.Add(pr.TotalCopilotSuggestions)
		} else {
			acc.repository.SuggestionsComplete = false
		}
		if pr.TotalCopilotAppliedSuggestions != nil && *pr.TotalCopilotAppliedSuggestions >= 0 {
			acc.repository.AppliedSuggestions.Add(pr.TotalCopilotAppliedSuggestions)
		} else {
			acc.repository.AppliedSuggestionsComplete = false
		}
	}

	names := map[string]int{}
	for _, acc := range repositories {
		if availableDays > len(acc.reportedDays) {
			acc.repository.Suggestions.Valid = true
			acc.repository.AppliedSuggestions.Valid = true
		}
		names[acc.repository.Name]++
	}
	result := make([]RichRepository, 0, len(repositories))
	for _, acc := range repositories {
		name := acc.repository.Name
		if name == "" {
			name = "(not reported)"
		}
		if names[acc.repository.Name] > 1 || reservedRepositoryName(name) {
			name = fmt.Sprintf("%s (repository %d)", name, acc.repository.ID)
		}
		acc.repository.Name = name
		result = append(result, acc.repository)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].ID < result[j].ID
	})
	return result
}

func reservedRepositoryName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "other", "others", "unknown", "unkown":
		return true
	default:
		return false
	}
}
