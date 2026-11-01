package copilotmetrics

import (
	"github.com/google/go-github/v93/github"
)

// UserDay is a normalized single user-day record from either a 1-day or a 28-day users report.
type UserDay struct {
	UserID int64
	Login  string
	Day    string

	AICreditsUsed               *float64
	UserInitiatedInteractions   *int
	CodeGenerationActivities    *int
	CodeAcceptanceActivities    *int
	LOCAdded                    *int
	LOCDeleted                  *int
	IDEs                        []string
	UsedAgent                   bool
	UsedChat                    bool
	UsedCLI                     bool
	UsedCopilotApp              bool
	UsedCopilotCloudAgent       bool
	UsedCopilotCodeReviewActive bool
	UsedCopilotCodingAgent      bool
}

func userIDEs(ides []*github.CopilotUserMetricsIDE) []string {
	var out []string
	for _, ide := range ides {
		if ide != nil && ide.IDE != "" {
			out = append(out, ide.IDE)
		}
	}
	return out
}

func boolValue(b *bool) bool {
	return b != nil && *b
}

// UserDayFromDaily normalizes a 1-day users report record.
func UserDayFromDaily(r *github.CopilotUserDailyMetrics) UserDay {
	return UserDay{
		UserID:                      int64(r.UserID),
		Login:                       r.UserLogin,
		Day:                         r.Day,
		AICreditsUsed:               r.AICreditsUsed,
		UserInitiatedInteractions:   r.UserInitiatedInteractionCount,
		CodeGenerationActivities:    r.CodeGenerationActivityCount,
		CodeAcceptanceActivities:    r.CodeAcceptanceActivityCount,
		LOCAdded:                    r.LOCAddedSum,
		LOCDeleted:                  r.LOCDeletedSum,
		IDEs:                        userIDEs(r.TotalsByIDE),
		UsedAgent:                   boolValue(r.UsedAgent),
		UsedChat:                    boolValue(r.UsedChat),
		UsedCLI:                     boolValue(r.UsedCLI),
		UsedCopilotApp:              boolValue(r.UsedCopilotApp),
		UsedCopilotCloudAgent:       boolValue(r.UsedCopilotCloudAgent),
		UsedCopilotCodeReviewActive: boolValue(r.UsedCopilotCodeReviewActive),
		UsedCopilotCodingAgent:      boolValue(r.UsedCopilotCodingAgent),
	}
}

// UserDayFromPeriodic normalizes a 28-day users report record, which is also a single user-day.
func UserDayFromPeriodic(r *github.CopilotUserPeriodicMetrics) UserDay {
	return UserDay{
		UserID:                      int64(r.UserID),
		Login:                       r.UserLogin,
		Day:                         r.Day,
		AICreditsUsed:               r.AICreditsUsed,
		UserInitiatedInteractions:   r.UserInitiatedInteractionCount,
		CodeGenerationActivities:    r.CodeGenerationActivityCount,
		CodeAcceptanceActivities:    r.CodeAcceptanceActivityCount,
		LOCAdded:                    r.LOCAddedSum,
		LOCDeleted:                  r.LOCDeletedSum,
		IDEs:                        userIDEs(r.TotalsByIDE),
		UsedAgent:                   boolValue(r.UsedAgent),
		UsedChat:                    boolValue(r.UsedChat),
		UsedCLI:                     boolValue(r.UsedCLI),
		UsedCopilotApp:              boolValue(r.UsedCopilotApp),
		UsedCopilotCloudAgent:       boolValue(r.UsedCopilotCloudAgent),
		UsedCopilotCodeReviewActive: boolValue(r.UsedCopilotCodeReviewActive),
		UsedCopilotCodingAgent:      boolValue(r.UsedCopilotCodingAgent),
	}
}
