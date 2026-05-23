package time

import (
	"context"
	"fmt"
	"math"
	"strings"
	stdtime "time"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

const defaultDateFormat = "2006-01-02"

var dateInputLayouts = []string{
	defaultDateFormat,
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
}

// DateCalculateTool performs small date arithmetic and difference calculations.
type DateCalculateTool struct {
	*builtin.BuiltinTool
}

func NewDateCalculateTool(tenantID string) *DateCalculateTool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     "date_calculate",
			Author:   "System",
			Provider: "time",
			Label: tools.I18nText{
				"en_US":   "Date Calculate",
				"zh_Hans": "日期计算",
			},
			Icon: "calendar",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US":   "Add, subtract, or compare dates.",
				"zh_Hans": "对日期进行加减或计算两个日期之间的天数。",
			},
			LLM: "Add or subtract day/week/month/year intervals from a date, or calculate the number of days between two dates.",
		},
		Parameters: []tools.ToolParameter{
			{
				Name: "operation",
				Label: tools.I18nText{
					"en_US":   "Operation",
					"zh_Hans": "操作",
				},
				HumanDescription: tools.I18nText{
					"en_US":   "Choose add, subtract, or diff. Diff returns the day interval between base_date and target_date.",
					"zh_Hans": "选择增加、减少或计算日期差。日期差会返回 base_date 与 target_date 之间的天数。",
				},
				LLMDescription:  "Operation to perform: add, subtract, or diff.",
				Type:            tools.ToolParameterTypeSelect,
				Form:            tools.ToolParameterFormLLM,
				Required:        true,
				SupportVariable: true,
				Options: []tools.ToolParameterOption{
					{Value: "add", Label: tools.I18nText{"en_US": "Add", "zh_Hans": "增加"}},
					{Value: "subtract", Label: tools.I18nText{"en_US": "Subtract", "zh_Hans": "减少"}},
					{Value: "diff", Label: tools.I18nText{"en_US": "Difference", "zh_Hans": "日期差"}},
				},
			},
			{
				Name:             "base_date",
				Label:            tools.I18nText{"en_US": "Base Date", "zh_Hans": "基准日期"},
				HumanDescription: tools.I18nText{"en_US": "Base date in YYYY-MM-DD format. Leave empty, today, or now to use the current date in the selected timezone.", "zh_Hans": "YYYY-MM-DD 格式的基准日期。留空，或填写 today、now 时，会使用所选时区的当前日期。"},
				LLMDescription:   "Base date in YYYY-MM-DD format. Use today when omitted.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Placeholder:      tools.I18nText{"en_US": "2026-05-19", "zh_Hans": "例如：2026-05-19"},
				SupportVariable:  true,
			},
			{
				Name:             "amount",
				Label:            tools.I18nText{"en_US": "Amount", "zh_Hans": "数量"},
				HumanDescription: tools.I18nText{"en_US": "Interval amount used by add or subtract. Ignored when operation is diff.", "zh_Hans": "add 或 subtract 操作使用的间隔数量。operation 为 diff 时会忽略该参数。"},
				LLMDescription:   "Interval amount for add or subtract operations.",
				Type:             tools.ToolParameterTypeNumber,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Default:          1,
				Placeholder:      tools.I18nText{"en_US": "3", "zh_Hans": "例如：3"},
				SupportVariable:  true,
			},
			{
				Name:             "unit",
				Label:            tools.I18nText{"en_US": "Unit", "zh_Hans": "单位"},
				HumanDescription: tools.I18nText{"en_US": "Interval unit used by add or subtract. Ignored when operation is diff.", "zh_Hans": "add 或 subtract 操作使用的间隔单位。operation 为 diff 时会忽略该参数。"},
				LLMDescription:   "Interval unit for add or subtract: day, week, month, or year.",
				Type:             tools.ToolParameterTypeSelect,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Default:          "day",
				SupportVariable:  true,
				Options: []tools.ToolParameterOption{
					{Value: "day", Label: tools.I18nText{"en_US": "Day", "zh_Hans": "天"}},
					{Value: "week", Label: tools.I18nText{"en_US": "Week", "zh_Hans": "周"}},
					{Value: "month", Label: tools.I18nText{"en_US": "Month", "zh_Hans": "月"}},
					{Value: "year", Label: tools.I18nText{"en_US": "Year", "zh_Hans": "年"}},
				},
			},
			{
				Name:             "target_date",
				Label:            tools.I18nText{"en_US": "Target Date", "zh_Hans": "目标日期"},
				HumanDescription: tools.I18nText{"en_US": "Target date in YYYY-MM-DD format. Required only when operation is diff.", "zh_Hans": "YYYY-MM-DD 格式的目标日期。仅 operation 为 diff 时必填。"},
				LLMDescription:   "Target date in YYYY-MM-DD format for diff operations.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Placeholder:      tools.I18nText{"en_US": "2026-06-01", "zh_Hans": "例如：2026-06-01"},
				SupportVariable:  true,
			},
			{
				Name:             "timezone",
				Label:            tools.I18nText{"en_US": "Timezone", "zh_Hans": "时区"},
				HumanDescription: tools.I18nText{"en_US": "IANA timezone used when base_date is empty, today, or now.", "zh_Hans": "当 base_date 为空、today 或 now 时使用的 IANA 时区。"},
				LLMDescription:   "Timezone used when base_date is omitted.",
				Type:             tools.ToolParameterTypeString,
				Form:             tools.ToolParameterFormLLM,
				Required:         false,
				Default:          "UTC",
				Placeholder:      tools.I18nText{"en_US": "Asia/Shanghai", "zh_Hans": "例如：Asia/Shanghai"},
				SupportVariable:  true,
			},
		},
		OutputType: "json",
		Tags:       []string{"utilities", "date"},
	}
	return &DateCalculateTool{BuiltinTool: builtin.NewBuiltinTool(entity, tenantID)}
}

func (t *DateCalculateTool) Invoke(
	ctx context.Context,
	userID string,
	toolParameters map[string]interface{},
	conversationID *string,
	appID *string,
	messageID *string,
) ([]tools.ToolInvokeMessage, error) {
	_ = ctx
	_ = userID
	_ = conversationID
	_ = appID
	_ = messageID

	loc, err := resolveLocation(toolParameters)
	if err != nil {
		return nil, err
	}
	operation := strings.ToLower(strings.TrimSpace(stringParam(toolParameters, "operation", "")))
	switch operation {
	case "add", "subtract":
		return t.invokeShift(toolParameters, loc, operation)
	case "diff":
		return t.invokeDiff(toolParameters, loc)
	default:
		return nil, fmt.Errorf("invalid operation: %s", operation)
	}
}

func (t *DateCalculateTool) invokeShift(params map[string]interface{}, loc *stdtime.Location, operation string) ([]tools.ToolInvokeMessage, error) {
	base, err := parseDateParam(params, "base_date", loc)
	if err != nil {
		return nil, err
	}
	amount, err := intParam(params, "amount", 1)
	if err != nil {
		return nil, err
	}
	if operation == "subtract" {
		amount = -amount
	}
	unit := strings.ToLower(strings.TrimSpace(stringParam(params, "unit", "day")))
	result, err := shiftDate(base, amount, unit)
	if err != nil {
		return nil, err
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(map[string]interface{}{
		"operation":   operation,
		"base_date":   base.Format(defaultDateFormat),
		"amount":      int(math.Abs(float64(amount))),
		"unit":        unit,
		"result_date": result.Format(defaultDateFormat),
		"weekday":     result.Weekday().String(),
		"timezone":    loc.String(),
	})}, nil
}

func (t *DateCalculateTool) invokeDiff(params map[string]interface{}, loc *stdtime.Location) ([]tools.ToolInvokeMessage, error) {
	base, err := parseDateParam(params, "base_date", loc)
	if err != nil {
		return nil, err
	}
	targetRaw := strings.TrimSpace(stringParam(params, "target_date", ""))
	if targetRaw == "" {
		return nil, fmt.Errorf("target_date is required for diff operation")
	}
	target, err := parseDate(targetRaw, loc)
	if err != nil {
		return nil, fmt.Errorf("invalid target_date: %w", err)
	}
	days := calendarDayDiff(base, target)
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(map[string]interface{}{
		"operation":   "diff",
		"base_date":   base.Format(defaultDateFormat),
		"target_date": target.Format(defaultDateFormat),
		"days":        days,
		"abs_days":    int(math.Abs(float64(days))),
		"timezone":    loc.String(),
	})}, nil
}

func calendarDayDiff(base stdtime.Time, target stdtime.Time) int {
	baseDate := stdtime.Date(base.Year(), base.Month(), base.Day(), 0, 0, 0, 0, stdtime.UTC)
	targetDate := stdtime.Date(target.Year(), target.Month(), target.Day(), 0, 0, 0, 0, stdtime.UTC)
	return int(targetDate.Sub(baseDate).Hours() / 24)
}

func resolveLocation(params map[string]interface{}) (*stdtime.Location, error) {
	timezone := stringParam(params, "timezone", "UTC")
	loc, err := stdtime.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("invalid timezone: %s", timezone)
	}
	return loc, nil
}

func parseDateParam(params map[string]interface{}, key string, loc *stdtime.Location) (stdtime.Time, error) {
	raw := strings.TrimSpace(stringParam(params, key, ""))
	if raw == "" || strings.EqualFold(raw, "today") || strings.EqualFold(raw, "now") {
		now := stdtime.Now().In(loc)
		return stdtime.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc), nil
	}
	parsed, err := parseDate(raw, loc)
	if err != nil {
		return stdtime.Time{}, fmt.Errorf("invalid %s: %w", key, err)
	}
	return parsed, nil
}

func parseDate(raw string, loc *stdtime.Location) (stdtime.Time, error) {
	for _, layout := range dateInputLayouts {
		if parsed, err := stdtime.ParseInLocation(layout, raw, loc); err == nil {
			return stdtime.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, loc), nil
		}
	}
	parsed, err := stdtime.Parse(stdtime.RFC3339, raw)
	if err != nil {
		return stdtime.Time{}, err
	}
	inLoc := parsed.In(loc)
	return stdtime.Date(inLoc.Year(), inLoc.Month(), inLoc.Day(), 0, 0, 0, 0, loc), nil
}

func shiftDate(base stdtime.Time, amount int, unit string) (stdtime.Time, error) {
	switch unit {
	case "day", "days":
		return base.AddDate(0, 0, amount), nil
	case "week", "weeks":
		return base.AddDate(0, 0, amount*7), nil
	case "month", "months":
		return base.AddDate(0, amount, 0), nil
	case "year", "years":
		return base.AddDate(amount, 0, 0), nil
	default:
		return stdtime.Time{}, fmt.Errorf("invalid unit: %s", unit)
	}
}

func stringParam(params map[string]interface{}, key string, fallback string) string {
	if params == nil {
		return fallback
	}
	value, ok := params[key]
	if !ok || value == nil {
		return fallback
	}
	if text, ok := value.(string); ok {
		text = strings.TrimSpace(text)
		if text != "" {
			return text
		}
	}
	return fallback
}

func intParam(params map[string]interface{}, key string, fallback int) (int, error) {
	if params == nil {
		return fallback, nil
	}
	value, ok := params[key]
	if !ok || value == nil {
		return fallback, nil
	}
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case int32:
		return int(typed), nil
	case float64:
		if typed != math.Trunc(typed) {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
		return int(typed), nil
	case float32:
		if typed != float32(math.Trunc(float64(typed))) {
			return 0, fmt.Errorf("%s must be an integer", key)
		}
		return int(typed), nil
	default:
		return 0, fmt.Errorf("%s must be an integer", key)
	}
}

func (t *DateCalculateTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &DateCalculateTool{
		BuiltinTool: t.BuiltinTool.ForkToolRuntime(runtime),
	}
}

var _ tools.Tool = (*DateCalculateTool)(nil)
