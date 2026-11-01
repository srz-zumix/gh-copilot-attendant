package copilotmetrics

import (
	"sort"

	"github.com/google/go-github/v93/github"
)

// RichSeries contains a named daily metric series.
type RichSeries struct {
	Name   string
	Values []Counter
}

// RichValue is one aggregate metric dimension.
type RichValue struct {
	Name  string
	Group string
	Value Counter
}

// IDEActiveUsers contains daily and rolling seven-day active-user counts for one IDE.
type IDEActiveUsers struct {
	IDE    string
	Daily  []Counter
	Weekly []Counter
}

// RichRepository contains repository-level daily reviews and suggestion totals.
// Incomplete suggestion totals retain the known subtotal.
type RichRepository struct {
	ID                         int64
	Name                       string
	Reviews                    []Counter
	Suggestions                Counter
	SuggestionsComplete        bool
	AppliedSuggestions         Counter
	AppliedSuggestionsComplete bool
}

// RichMetrics contains dashboard-oriented metrics derived from usage reports.
// User identifiers are used only while building IDE active-user counts and are not retained.
type RichMetrics struct {
	Days                             []string
	DailyActiveUsers                 []Counter
	WeeklyActiveUsers                []Counter
	MonthlyActiveUsers               []Counter
	MonthlyActiveAgentUsers          []Counter
	AverageChatRequestsPerActiveUser []FloatCounter
	ChatModeRequests                 []RichSeries
	CodeGeneration                   []Counter
	CodeAcceptance                   []Counter
	AcceptanceRate                   []FloatCounter
	CodeCompletionsSuggested         []Counter
	CodeCompletionsAccepted          []Counter
	CodeCompletionsAcceptanceRate    []FloatCounter
	ModelRequestsByDay               []RichSeries
	ModelRequests                    []RichValue
	ModelRequestsByFeature           []RichValue
	LanguageCodeGenerationByDay      []RichSeries
	LanguageCodeGeneration           []RichValue
	ModelCodeGenerationByLanguage    []RichValue
	IDEActiveUsers                   []IDEActiveUsers
	IDEActiveUserTotalDaily          []Counter
	IDEActiveUserTotalWeekly         []Counter
	IDEActiveUserMonth               *IDEActiveUserEstimate
	Repositories                     []RichRepository
}

type richDimension struct {
	name  string
	group string
}

func buildRichMetrics(ds *Dataset, includeIDEUsers bool, modelFeatures, modelLanguages []RichValue) *RichMetrics {
	rich := &RichMetrics{
		Days:                             append([]string(nil), ds.Days...),
		AverageChatRequestsPerActiveUser: make([]FloatCounter, len(ds.Days)),
		AcceptanceRate:                   make([]FloatCounter, len(ds.Days)),
		CodeCompletionsSuggested:         make([]Counter, len(ds.Days)),
		CodeCompletionsAccepted:          make([]Counter, len(ds.Days)),
		CodeCompletionsAcceptanceRate:    make([]FloatCounter, len(ds.Days)),
		ModelRequestsByFeature:           modelFeatures,
		ModelCodeGenerationByLanguage:    modelLanguages,
	}
	dayIndex := make(map[string]int, len(ds.Days))
	for i, day := range ds.Days {
		dayIndex[day] = i
	}
	orgByDay := make(map[string]*github.CopilotDailyMetrics, len(ds.OrgDays))
	for _, day := range ds.OrgDays {
		if day != nil {
			orgByDay[day.Day] = day
		}
	}

	rich.DailyActiveUsers = make([]Counter, len(ds.Days))
	rich.WeeklyActiveUsers = make([]Counter, len(ds.Days))
	rich.MonthlyActiveUsers = make([]Counter, len(ds.Days))
	rich.MonthlyActiveAgentUsers = make([]Counter, len(ds.Days))
	rich.CodeGeneration = make([]Counter, len(ds.Days))
	rich.CodeAcceptance = make([]Counter, len(ds.Days))

	modeDayValues := map[string][]Counter{}
	modelDayValues := map[string][]Counter{}
	languageDayValues := map[string][]Counter{}
	modelTotals := map[string]Counter{}
	languageTotals := map[string]Counter{}
	var modelsSeen = map[string]bool{}
	var languagesSeen = map[string]bool{}

	for _, day := range ds.Days {
		i := dayIndex[day]
		record, found := orgByDay[day]
		if !found {
			continue
		}
		rich.DailyActiveUsers[i].Add(record.DailyActiveUsers)
		rich.WeeklyActiveUsers[i].Add(record.WeeklyActiveUsers)
		rich.MonthlyActiveUsers[i].Add(record.MonthlyActiveUsers)
		rich.MonthlyActiveAgentUsers[i].Add(record.MonthlyActiveAgentUsers)
		rich.CodeGeneration[i].Add(record.CodeGenerationActivityCount)
		rich.CodeAcceptance[i].Add(record.CodeAcceptanceActivityCount)
		chatRequests := record.UserInitiatedInteractionCount
		if chatRequests != nil && record.DailyActiveUsers != nil && *record.DailyActiveUsers > 0 {
			rich.AverageChatRequestsPerActiveUser[i] = FloatCounter{
				Value: float64(*chatRequests) / float64(*record.DailyActiveUsers), Valid: true,
			}
		}
		if rich.CodeGeneration[i].Valid && rich.CodeAcceptance[i].Valid {
			if rate, ok := Rate(rich.CodeAcceptance[i], rich.CodeGeneration[i]); ok {
				rich.AcceptanceRate[i] = FloatCounter{Value: rate, Valid: true}
			}
		}
		for _, feature := range record.TotalsByFeature {
			if feature == nil {
				continue
			}
			if feature.Feature == "code_completion" {
				rich.CodeCompletionsSuggested[i].Add(feature.CodeGenerationActivityCount)
				rich.CodeCompletionsAccepted[i].Add(feature.CodeAcceptanceActivityCount)
			}
			if name, ok := ChatModeForFeature(feature.Feature); ok {
				if modeDayValues[name] == nil {
					modeDayValues[name] = make([]Counter, len(ds.Days))
				}
				modeDayValues[name][i].Add(feature.UserInitiatedInteractionCount)
			}
		}
		if rate, ok := Rate(rich.CodeCompletionsAccepted[i], rich.CodeCompletionsSuggested[i]); ok {
			rich.CodeCompletionsAcceptanceRate[i] = FloatCounter{Value: rate, Valid: true}
		}
		for _, model := range record.TotalsByModelFeature {
			if model == nil {
				continue
			}
			name := model.Model
			if name == "" {
				name = "unknown"
			}
			modelsSeen[name] = true
			if modelDayValues[name] == nil {
				modelDayValues[name] = make([]Counter, len(ds.Days))
			}
			modelDayValues[name][i].Add(model.UserInitiatedInteractionCount)
			addRichCounter(modelTotals, name, counterFromInt(model.UserInitiatedInteractionCount))
		}
		for _, language := range record.TotalsByLanguageFeature {
			if language == nil {
				continue
			}
			name := language.Language
			if name == "" {
				name = "unknown"
			}
			languagesSeen[name] = true
			if languageDayValues[name] == nil {
				languageDayValues[name] = make([]Counter, len(ds.Days))
			}
			languageDayValues[name][i].Add(language.CodeGenerationActivityCount)
			addRichCounter(languageTotals, name, counterFromInt(language.CodeGenerationActivityCount))
		}
	}

	rich.ChatModeRequests = richSeriesFromMap(modeDayValues)
	rich.ModelRequestsByDay = richSeriesFromMap(modelDayValues)
	rich.LanguageCodeGenerationByDay = richSeriesFromMap(languageDayValues)
	rich.ModelRequests = richValuesFromMap(modelTotals)
	rich.LanguageCodeGeneration = richValuesFromMap(languageTotals)
	if includeIDEUsers {
		rich.IDEActiveUsers, rich.IDEActiveUserTotalDaily, rich.IDEActiveUserTotalWeekly = buildIDEActiveUsers(ds, dayIndex)
		rich.IDEActiveUserMonth = buildIDEActiveUserEstimate(ds)
	}
	return rich
}

func counterFromInt(value *int) Counter {
	var counter Counter
	counter.Add(value)
	return counter
}

func addRichCounter(values map[string]Counter, key string, value Counter) {
	counter := values[key]
	counter.AddCounter(value)
	values[key] = counter
}

func addRichDimension(values map[richDimension]Counter, key richDimension, value Counter) {
	counter := values[key]
	counter.AddCounter(value)
	values[key] = counter
}

func richSeriesFromMap(values map[string][]Counter) []RichSeries {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	series := make([]RichSeries, 0, len(names))
	for _, name := range names {
		series = append(series, RichSeries{Name: name, Values: values[name]})
	}
	return series
}

func richValuesFromMap(values map[string]Counter) []RichValue {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]RichValue, 0, len(names))
	for _, name := range names {
		result = append(result, RichValue{Name: name, Value: values[name]})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Value.Value != result[j].Value.Value {
			return result[i].Value.Value > result[j].Value.Value
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func richDimensionsFromMap(values map[richDimension]Counter) []RichValue {
	result := make([]RichValue, 0, len(values))
	for dimension, value := range values {
		result = append(result, RichValue{Name: dimension.name, Group: dimension.group, Value: value})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Value.Value != result[j].Value.Value {
			return result[i].Value.Value > result[j].Value.Value
		}
		if result[i].Group != result[j].Group {
			return result[i].Group < result[j].Group
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func buildIDEActiveUsers(ds *Dataset, dayIndex map[string]int) ([]IDEActiveUsers, []Counter, []Counter) {
	type usersByIDE map[int64]struct{}
	dayIDEUsers := make([]map[string]usersByIDE, len(ds.Days))
	ideNames := map[string]bool{}
	for i := range dayIDEUsers {
		dayIDEUsers[i] = map[string]usersByIDE{}
	}
	for _, user := range ds.Users {
		i, ok := dayIndex[user.Day]
		if !ok {
			continue
		}
		for _, ide := range user.IDEs {
			if ide == "" {
				continue
			}
			ideNames[ide] = true
			if dayIDEUsers[i][ide] == nil {
				dayIDEUsers[i][ide] = usersByIDE{}
			}
			dayIDEUsers[i][ide][user.UserID] = struct{}{}
		}
	}
	names := make([]string, 0, len(ideNames))
	for name := range ideNames {
		names = append(names, name)
	}
	sort.Strings(names)
	missing := map[string]bool{}
	for _, day := range ds.Missing[FamilyUsers] {
		missing[day] = true
	}
	series := make([]IDEActiveUsers, 0, len(names))
	totalDaily := make([]Counter, len(ds.Days))
	totalWeekly := make([]Counter, len(ds.Days))
	for _, ide := range names {
		item := IDEActiveUsers{IDE: ide, Daily: make([]Counter, len(ds.Days)), Weekly: make([]Counter, len(ds.Days))}
		for dayIndex, day := range ds.Days {
			if !missing[day] {
				item.Daily[dayIndex] = Counter{Value: int64(len(dayIDEUsers[dayIndex][ide])), Valid: true}
			}
			if dayIndex < 6 {
				continue
			}
			windowComplete := true
			users := usersByIDE{}
			for i := dayIndex - 6; i <= dayIndex; i++ {
				if missing[ds.Days[i]] {
					windowComplete = false
					break
				}
				for id := range dayIDEUsers[i][ide] {
					users[id] = struct{}{}
				}
			}
			if windowComplete {
				item.Weekly[dayIndex] = Counter{Value: int64(len(users)), Valid: true}
			}
		}
		series = append(series, item)
	}
	for dayIndex := range ds.Days {
		if missing[ds.Days[dayIndex]] {
			continue
		}
		dailyUsers := map[int64]struct{}{}
		for _, ideUsers := range dayIDEUsers[dayIndex] {
			for id := range ideUsers {
				dailyUsers[id] = struct{}{}
			}
		}
		totalDaily[dayIndex] = Counter{Value: int64(len(dailyUsers)), Valid: true}
		if dayIndex < 6 {
			continue
		}
		windowComplete := true
		weeklyUsers := map[int64]struct{}{}
		for i := dayIndex - 6; i <= dayIndex; i++ {
			if missing[ds.Days[i]] {
				windowComplete = false
				break
			}
			for _, ideUsers := range dayIDEUsers[i] {
				for id := range ideUsers {
					weeklyUsers[id] = struct{}{}
				}
			}
		}
		if windowComplete {
			totalWeekly[dayIndex] = Counter{Value: int64(len(weeklyUsers)), Valid: true}
		}
	}
	return series, totalDaily, totalWeekly
}
