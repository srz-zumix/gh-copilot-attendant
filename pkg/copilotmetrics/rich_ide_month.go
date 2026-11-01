package copilotmetrics

import "time"

// IDEActiveUserEstimate counts users with a reported IDE during a calendar month.
// It can differ from GitHub's dashboard even when all daily reports are available.
type IDEActiveUserEstimate struct {
	StartDay    string
	EndDay      string
	Users       Counter
	MissingDays []string
}

type ideMonthUserDay struct {
	day    string
	userID int64
}

type ideMonthSupplement struct {
	days        []string
	users       []ideMonthUserDay
	missingDays []string
}

func calendarMonthDays(end string) []string {
	day, err := time.Parse(dayLayout, end)
	if err != nil {
		return nil
	}
	start := day.AddDate(0, 0, 1-day.Day()).Format(dayLayout)
	return DayRange(start, end)
}

func hasIDE(ides []string) bool {
	for _, ide := range ides {
		if ide != "" {
			return true
		}
	}
	return false
}

func buildIDEActiveUserEstimate(ds *Dataset) *IDEActiveUserEstimate {
	monthDays := calendarMonthDays(ds.EndDay())
	if len(monthDays) == 0 {
		return nil
	}
	result := &IDEActiveUserEstimate{StartDay: monthDays[0], EndDay: monthDays[len(monthDays)-1]}
	available := daySet(ds.Days)
	for _, day := range ds.Missing[FamilyUsers] {
		delete(available, day)
	}
	if supplement := ds.ideMonthSupplement; supplement != nil {
		for _, day := range supplement.days {
			available[day] = true
		}
		for _, day := range supplement.missingDays {
			delete(available, day)
		}
	}

	month := daySet(monthDays)
	for _, day := range monthDays {
		if !available[day] {
			result.MissingDays = append(result.MissingDays, day)
		}
	}
	ids := map[int64]struct{}{}
	add := func(day string, id int64) {
		if month[day] && available[day] {
			ids[id] = struct{}{}
		}
	}
	for _, user := range ds.Users {
		if hasIDE(user.IDEs) {
			add(user.Day, user.UserID)
		}
	}
	if supplement := ds.ideMonthSupplement; supplement != nil {
		for _, user := range supplement.users {
			add(user.day, user.userID)
		}
	}
	result.Users = Counter{Value: int64(len(ids)), Valid: len(result.MissingDays) < len(monthDays)}
	return result
}
