package draftgen

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	automationdto "github.com/zgiai/zgi/api/internal/modules/automation/dto"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
)

const (
	defaultWorkflowTimeoutSeconds = 120
	maxTextLength                 = 4000
	warningWorkflowSelectionZH    = "\u5de5\u4f5c\u6d41\u64cd\u4f5c\u9700\u8981\u5148\u9009\u62e9\u5df2\u53d1\u5e03\u5de5\u4f5c\u6d41\uff0c\u4efb\u52a1\u624d\u80fd\u6b63\u5e38\u8fd0\u884c\u3002"
	warningSMSConfigZH            = "\u77ed\u4fe1\u64cd\u4f5c\u4f9d\u8d56\u5df2\u914d\u7f6e\u7684\u77ed\u4fe1\u670d\u52a1\u5546\u548c\u6a21\u677f\uff0c\u8bf7\u5728\u521b\u5efa\u4efb\u52a1\u524d\u68c0\u67e5\u3002"
)

var (
	jsonFencePattern    = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)\\s*```")
	missingFieldPattern = regexp.MustCompile(`^actions\.(\d+)\.(to|subject|body|notification_title|sms_link_suffix|workflow_agent_id)$`)
)

type modelOutput struct {
	Name          string        `json:"name"`
	Description   string        `json:"description"`
	Schedule      modelSchedule `json:"schedule"`
	Actions       []modelAction `json:"actions"`
	MissingFields []string      `json:"missing_fields"`
	Warnings      []string      `json:"warnings"`
	Summary       string        `json:"summary"`
}

type modelSchedule struct {
	Type     string `json:"type"`
	RunAt    string `json:"run_at"`
	CronExpr string `json:"cron_expr"`
	Timezone string `json:"timezone"`
}

type modelAction struct {
	Type                string                 `json:"type"`
	Channel             string                 `json:"channel"`
	To                  []string               `json:"to"`
	Subject             string                 `json:"subject"`
	Body                string                 `json:"body"`
	BodyType            string                 `json:"body_type"`
	WorkflowAgentID     string                 `json:"workflow_agent_id"`
	WorkflowVersionUUID string                 `json:"workflow_version_uuid"`
	WorkflowInputs      map[string]interface{} `json:"workflow_inputs"`
	WorkflowTimeout     int                    `json:"workflow_timeout_seconds"`
}

func ParseDraft(raw string, fallbackTimezone string, locales ...string) (*GenerateResult, error) {
	payload := extractJSONPayload(raw)
	var output modelOutput
	if err := json.Unmarshal([]byte(payload), &output); err != nil {
		return nil, fmt.Errorf("parse draft json: %w", err)
	}

	locale := ""
	if len(locales) > 0 {
		locale = locales[0]
	}
	missingFields := normalizeMissingFields(output.MissingFields)
	warnings := normalizeStringList(output.Warnings)
	draft := AutomationTaskDraft{
		Name:           cleanLongText(output.Name, 255),
		Description:    optionalString(cleanLongText(output.Description, 2000)),
		ScheduleType:   automationmodel.AutomationScheduleTypeOnce,
		Timezone:       firstNonEmpty(cleanShortText(output.Schedule.Timezone), cleanShortText(fallbackTimezone)),
		ScheduleConfig: map[string]interface{}{},
		Actions:        make([]automationdto.CreateTaskActionRequest, 0, len(output.Actions)),
	}

	switch strings.ToLower(strings.TrimSpace(output.Schedule.Type)) {
	case string(automationmodel.AutomationScheduleTypeCron):
		draft.ScheduleType = automationmodel.AutomationScheduleTypeCron
		cronExpr := cleanShortText(output.Schedule.CronExpr)
		draft.ScheduleConfig["cron_expr"] = cronExpr
		if cronExpr == "" {
			missingFields = appendMissing(missingFields, "schedule.cron_expr")
		}
		if draft.Timezone == "" {
			missingFields = appendMissing(missingFields, "schedule.timezone")
		}
	case string(automationmodel.AutomationScheduleTypeOnce), "":
		draft.ScheduleType = automationmodel.AutomationScheduleTypeOnce
		runAt := cleanShortText(output.Schedule.RunAt)
		draft.ScheduleConfig["run_at"] = runAt
		if runAt == "" {
			missingFields = appendMissing(missingFields, "schedule.run_at")
		}
	default:
		return nil, fmt.Errorf("unsupported schedule type %q", output.Schedule.Type)
	}

	if draft.Name == "" {
		missingFields = appendMissing(missingFields, "name")
	}
	if len(output.Actions) == 0 {
		missingFields = appendMissing(missingFields, "actions")
	}

	for index, action := range output.Actions {
		parsed, actionMissing, actionWarnings := parseAction(action, index+1, locale)
		missingFields = append(missingFields, actionMissing...)
		warnings = append(warnings, actionWarnings...)
		draft.Actions = append(draft.Actions, parsed)
	}

	return &GenerateResult{
		Draft:         draft,
		MissingFields: dedupeStrings(missingFields),
		Warnings:      dedupeStrings(warnings),
		Summary:       cleanLongText(output.Summary, 500),
	}, nil
}

func parseAction(action modelAction, order int, locale string) (automationdto.CreateTaskActionRequest, []string, []string) {
	missingFields := make([]string, 0)
	warnings := make([]string, 0)
	enabled := true
	actionType := strings.ToLower(strings.TrimSpace(action.Type))

	if actionType == string(automationmodel.AutomationActionTypeRunWorkflow) {
		workflowRef := map[string]interface{}{
			"agent_id":         cleanShortText(action.WorkflowAgentID),
			"version_strategy": "latest_published",
		}
		if versionUUID := cleanShortText(action.WorkflowVersionUUID); versionUUID != "" {
			workflowRef["version_strategy"] = "pinned"
			workflowRef["version_uuid"] = versionUUID
		}
		if workflowRef["agent_id"] == "" {
			missingFields = append(missingFields, fmt.Sprintf("actions.%d.workflow_agent_id", order))
			warnings = append(warnings, localizedWarning(locale, "workflow_selection"))
		}
		timeoutSeconds := action.WorkflowTimeout
		if timeoutSeconds <= 0 {
			timeoutSeconds = defaultWorkflowTimeoutSeconds
		}
		return automationdto.CreateTaskActionRequest{
			ActionType:  automationmodel.AutomationActionTypeRunWorkflow,
			ActionOrder: order,
			Enabled:     &enabled,
			Config: map[string]interface{}{
				"workflow_ref": workflowRef,
				"inputs":       normalizeObject(action.WorkflowInputs),
				"execution": map[string]interface{}{
					"timeout_seconds": timeoutSeconds,
				},
			},
		}, missingFields, warnings
	}

	channel := strings.ToLower(strings.TrimSpace(action.Channel))
	if channel == "sms" {
		recipients := normalizeRecipients(action.To)
		notificationTitle := cleanLongText(firstNonEmpty(action.Subject, action.Body), 200)
		if len(recipients) == 0 {
			missingFields = append(missingFields, fmt.Sprintf("actions.%d.to", order))
		}
		if notificationTitle == "" {
			missingFields = append(missingFields, fmt.Sprintf("actions.%d.notification_title", order))
		}
		missingFields = append(missingFields, fmt.Sprintf("actions.%d.sms_link_suffix", order))
		warnings = append(warnings, localizedWarning(locale, "sms_config"))
		return automationdto.CreateTaskActionRequest{
			ActionType:  automationmodel.AutomationActionTypeSendNotification,
			ActionOrder: order,
			Enabled:     &enabled,
			Config: map[string]interface{}{
				"channel_type": "sms",
				"to":           recipients,
				"template":     "pending_action_notification",
				"template_params": map[string]interface{}{
					"notification_title": notificationTitle,
					"link_suffix":        "",
				},
			},
		}, missingFields, warnings
	}

	recipients := normalizeRecipients(action.To)
	subject := cleanLongText(action.Subject, 255)
	body := cleanLongText(action.Body, maxTextLength)
	if len(recipients) == 0 {
		missingFields = append(missingFields, fmt.Sprintf("actions.%d.to", order))
	}
	if subject == "" {
		missingFields = append(missingFields, fmt.Sprintf("actions.%d.subject", order))
	}
	if body == "" {
		missingFields = append(missingFields, fmt.Sprintf("actions.%d.body", order))
	}

	bodyType := strings.ToLower(strings.TrimSpace(action.BodyType))
	if bodyType != "text/plain" && bodyType != "text/html" {
		bodyType = "text/html"
	}

	return automationdto.CreateTaskActionRequest{
		ActionType:  automationmodel.AutomationActionTypeSendNotification,
		ActionOrder: order,
		Enabled:     &enabled,
		Config: map[string]interface{}{
			"channel_type": "email",
			"to":           recipients,
			"subject":      subject,
			"body":         body,
			"body_type":    bodyType,
		},
	}, missingFields, warnings
}

func extractJSONPayload(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if match := jsonFencePattern.FindStringSubmatch(trimmed); len(match) == 2 {
		return strings.TrimSpace(match[1])
	}

	start := strings.Index(trimmed, "{")
	end := strings.LastIndex(trimmed, "}")
	if start >= 0 && end > start {
		return trimmed[start : end+1]
	}

	return trimmed
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func normalizeRecipients(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		cleaned := cleanShortText(value)
		if cleaned != "" {
			result = append(result, cleaned)
		}
	}
	return result
}

func normalizeObject(value map[string]interface{}) map[string]interface{} {
	if value == nil {
		return map[string]interface{}{}
	}
	return value
}

func normalizeStringList(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		cleaned := cleanLongText(value, 500)
		if cleaned != "" {
			result = append(result, cleaned)
		}
	}
	return dedupeStrings(result)
}

func normalizeMissingFields(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		cleaned := cleanShortText(value)
		if cleaned == "" || !isSupportedMissingField(cleaned) {
			continue
		}
		result = append(result, cleaned)
	}
	return dedupeStrings(result)
}

func isSupportedMissingField(value string) bool {
	switch value {
	case "name", "actions", "schedule.run_at", "schedule.cron_expr", "schedule.timezone":
		return true
	}

	match := missingFieldPattern.FindStringSubmatch(value)
	return len(match) == 3
}

func appendMissing(values []string, value string) []string {
	value = cleanShortText(value)
	if value == "" {
		return values
	}
	for _, item := range values {
		if item == value {
			return values
		}
	}
	return append(values, value)
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		cleaned := cleanLongText(value, 500)
		if cleaned == "" {
			continue
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		result = append(result, cleaned)
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func cleanShortText(value string) string {
	return cleanLongText(value, 255)
}

func cleanLongText(value string, limit int) string {
	cleaned := strings.TrimSpace(value)
	if limit <= 0 {
		return cleaned
	}
	runes := []rune(cleaned)
	if len(runes) <= limit {
		return cleaned
	}
	return string(runes[:limit])
}

func isChineseLocale(locale string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(locale)), "zh")
}

func localizedWarning(locale string, key string) string {
	zh := isChineseLocale(locale)
	switch key {
	case "workflow_selection":
		if zh {
			return warningWorkflowSelectionZH
		}
		return "Workflow actions need a published workflow selected before the task can run."
	case "sms_config":
		if zh {
			return warningSMSConfigZH
		}
		return "SMS actions require a configured SMS provider and template. Review the generated action before creating the task."
	default:
		return key
	}
}
