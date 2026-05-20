package time

import (
	"context"
	"fmt"
	"time"

	"github.com/zgiai/ginext/internal/modules/tools"
	"github.com/zgiai/ginext/internal/modules/tools/builtin"
)

// CurrentTimeTool is a builtin tool for getting the current time
type CurrentTimeTool struct {
	*builtin.BuiltinTool
}

// NewCurrentTimeTool creates a new CurrentTimeTool
func NewCurrentTimeTool(tenantID string) *CurrentTimeTool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "current_time",
			Author:   "System",
			Provider: "time",
			Label: tools.I18nText{
				"en_US":   "Current Time",
				"zh_Hans": "获取当前时间",
			},
			Icon: "clock",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US":   "Get the current system time with timezone support.",
				"zh_Hans": "获取指定时区的当前系统时间。",
			},
			LLM: "Get the current system time with timezone support.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name: "format",
				Label: tools.I18nText{
					"en_US":   "Format",
					"zh_Hans": "格式",
				},
				HumanDescription: tools.I18nText{
					"en_US":   "Time format using strftime syntax, e.g., %Y-%m-%d %H:%M:%S",
					"zh_Hans": "strftime 标准的时间格式，如 %Y-%m-%d %H:%M:%S",
				},
				Type:            tools.ToolParameterTypeString,
				Form:            tools.ToolParameterFormForm,
				Required:        false,
				Default:         "%Y-%m-%d %H:%M:%S",
				SupportVariable: false, // Format is typically a fixed pattern, no variable needed
			},
			{
				Name: "timezone",
				Label: tools.I18nText{
					"en_US":   "Timezone",
					"zh_Hans": "时区",
				},
				HumanDescription: tools.I18nText{
					"en_US":   "Timezone for the current time",
					"zh_Hans": "当前时间的时区",
				},
				Type:            tools.ToolParameterTypeSelect,
				Form:            tools.ToolParameterFormForm,
				Required:        false,
				Default:         "UTC",
				SupportVariable: true, // Allow variable input for timezone
				Options: []tools.ToolParameterOption{
					{Value: "UTC", Label: tools.I18nText{"en_US": "UTC", "zh_Hans": "UTC"}},
					{Value: "Asia/Shanghai", Label: tools.I18nText{"en_US": "Asia/Shanghai", "zh_Hans": "亚洲/上海"}},
					{Value: "Asia/Tokyo", Label: tools.I18nText{"en_US": "Asia/Tokyo", "zh_Hans": "亚洲/东京"}},
					{Value: "Asia/Singapore", Label: tools.I18nText{"en_US": "Asia/Singapore", "zh_Hans": "亚洲/新加坡"}},
					{Value: "Asia/Hong_Kong", Label: tools.I18nText{"en_US": "Asia/Hong_Kong", "zh_Hans": "亚洲/香港"}},
					{Value: "Asia/Seoul", Label: tools.I18nText{"en_US": "Asia/Seoul", "zh_Hans": "亚洲/首尔"}},
					{Value: "Asia/Dubai", Label: tools.I18nText{"en_US": "Asia/Dubai", "zh_Hans": "亚洲/迪拜"}},
					{Value: "America/New_York", Label: tools.I18nText{"en_US": "America/New_York", "zh_Hans": "美洲/纽约"}},
					{Value: "America/Los_Angeles", Label: tools.I18nText{"en_US": "America/Los_Angeles", "zh_Hans": "美洲/洛杉矶"}},
					{Value: "America/Chicago", Label: tools.I18nText{"en_US": "America/Chicago", "zh_Hans": "美洲/芝加哥"}},
					{Value: "America/Sao_Paulo", Label: tools.I18nText{"en_US": "America/Sao_Paulo", "zh_Hans": "美洲/圣保罗"}},
					{Value: "Europe/London", Label: tools.I18nText{"en_US": "Europe/London", "zh_Hans": "欧洲/伦敦"}},
					{Value: "Europe/Paris", Label: tools.I18nText{"en_US": "Europe/Paris", "zh_Hans": "欧洲/巴黎"}},
					{Value: "Europe/Berlin", Label: tools.I18nText{"en_US": "Europe/Berlin", "zh_Hans": "欧洲/柏林"}},
					{Value: "Europe/Moscow", Label: tools.I18nText{"en_US": "Europe/Moscow", "zh_Hans": "欧洲/莫斯科"}},
					{Value: "Australia/Sydney", Label: tools.I18nText{"en_US": "Australia/Sydney", "zh_Hans": "澳洲/悉尼"}},
					{Value: "Pacific/Auckland", Label: tools.I18nText{"en_US": "Pacific/Auckland", "zh_Hans": "太平洋/奥克兰"}},
					{Value: "Africa/Cairo", Label: tools.I18nText{"en_US": "Africa/Cairo", "zh_Hans": "非洲/开罗"}},
				},
			},
		},
		OutputType: "text",
		Tags:       []string{"utilities"},
	}

	return &CurrentTimeTool{
		BuiltinTool: builtin.NewBuiltinTool(entity, tenantID),
	}
}

// Invoke executes the current time tool
func (t *CurrentTimeTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	// Get timezone parameter
	timezone := "UTC"
	if tz, ok := toolParameters["timezone"].(string); ok && tz != "" {
		timezone = tz
	}

	// Get format parameter (strftime syntax from user)
	strftimeFormat := "%Y-%m-%d %H:%M:%S"
	if fm, ok := toolParameters["format"].(string); ok && fm != "" {
		strftimeFormat = fm
	}

	// Convert strftime format to Go format
	goFormat := strftimeToGoFormat(strftimeFormat)

	// Load timezone location
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return []tools.ToolInvokeMessage{
			builtin.CreateTextMessage(fmt.Sprintf("Invalid timezone: %s, error: %v", timezone, err)),
		}, nil
	}

	// Get current time in the specified timezone
	now := time.Now().In(loc)
	formattedTime := now.Format(goFormat)

	return []tools.ToolInvokeMessage{
		builtin.CreateTextMessage(formattedTime),
	}, nil
}

// strftimeToGoFormat converts strftime format specifiers to Go time layout
func strftimeToGoFormat(strftime string) string {
	// Map of strftime specifiers to Go layout components
	replacements := map[string]string{
		"%Y": "2006",    // 4-digit year
		"%y": "06",      // 2-digit year
		"%m": "01",      // Month as zero-padded decimal
		"%d": "02",      // Day as zero-padded decimal
		"%H": "15",      // Hour (24-hour) as zero-padded decimal
		"%I": "03",      // Hour (12-hour) as zero-padded decimal
		"%M": "04",      // Minute as zero-padded decimal
		"%S": "05",      // Second as zero-padded decimal
		"%p": "PM",      // AM/PM
		"%P": "pm",      // am/pm
		"%b": "Jan",     // Abbreviated month name
		"%B": "January", // Full month name
		"%a": "Mon",     // Abbreviated weekday name
		"%A": "Monday",  // Full weekday name
		"%j": "002",     // Day of year as zero-padded decimal
		"%Z": "MST",     // Timezone abbreviation
		"%z": "-0700",   // UTC offset
		"%%": "%",       // Literal %
	}

	result := strftime
	for strfmt, gofmt := range replacements {
		result = replaceAll(result, strfmt, gofmt)
	}

	return result
}

// replaceAll replaces all occurrences of old with new in s
func replaceAll(s, old, new string) string {
	result := s
	for {
		idx := indexOf(result, old)
		if idx == -1 {
			break
		}
		result = result[:idx] + new + result[idx+len(old):]
	}
	return result
}

// indexOf finds the index of substr in s, returns -1 if not found
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// ForkToolRuntime creates a copy of the tool with new runtime
func (t *CurrentTimeTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &CurrentTimeTool{
		BuiltinTool: t.BuiltinTool.ForkToolRuntime(runtime),
	}
}

// Ensure CurrentTimeTool implements tools.Tool interface
var _ tools.Tool = (*CurrentTimeTool)(nil)
