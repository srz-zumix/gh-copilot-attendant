package copilotmetrics

// ChatModeForFeature returns the chat mode represented by a feature value.
func ChatModeForFeature(feature string) (string, bool) {
	switch feature {
	case "chat_panel_agent_mode":
		return "Agent", true
	case "chat_panel_ask_mode":
		return "Ask", true
	case "chat_panel_custom_mode":
		return "Custom", true
	case "chat_panel_edit_mode":
		return "Edit", true
	case "chat_panel_plan_mode":
		return "Plan", true
	case "chat_panel_unknown_mode":
		return "Unknown", true
	default:
		return "", false
	}
}
