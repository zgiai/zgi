package skills

import (
	"strings"
	"time"
)

// timeSkillDefaultFormat names the zone of the time it shows, so a time is
// never read as another zone's.
const timeSkillDefaultFormat = "%Y-%m-%d %H:%M:%S %Z (UTC%z)"

// withTimeSkillDefaults fills the time Skill's timezone with the caller's,
// when the model names none, and shows the zone in the current time.
func withTimeSkillDefaults(toolName string, arguments map[string]interface{}, timezone string) map[string]interface{} {
	result := copyStringAnyMap(arguments)
	if result == nil {
		result = map[string]interface{}{}
	}
	timezone = strings.TrimSpace(timezone)
	if current, _ := result["timezone"].(string); strings.TrimSpace(current) == "" && timezone != "" {
		if _, err := time.LoadLocation(timezone); err == nil {
			result["timezone"] = timezone
		}
	}
	if toolName == "current_time" {
		if format, _ := result["format"].(string); strings.TrimSpace(format) == "" {
			result["format"] = timeSkillDefaultFormat
		}
	}
	return result
}
