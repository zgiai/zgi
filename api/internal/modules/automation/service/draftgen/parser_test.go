package draftgen

import (
	"testing"

	automationmodel "github.com/zgiai/ginext/internal/modules/automation/model"
)

func TestParseDraftEmailCron(t *testing.T) {
	raw := `{
		"name": "Weekly digest",
		"description": "Send a weekly operations summary.",
		"schedule": {
			"type": "cron",
			"cron_expr": "0 9 * * 1",
			"timezone": "Europe/Dublin"
		},
		"actions": [
			{
				"type": "send_notification",
				"channel": "email",
				"to": ["ops@example.com"],
				"subject": "Weekly digest",
				"body": "Here is the weekly digest.",
				"body_type": "text/plain"
			}
		],
		"missing_fields": [],
		"warnings": [],
		"summary": "Created a weekly email draft."
	}`

	result, err := ParseDraft(raw, "Asia/Tokyo")
	if err != nil {
		t.Fatalf("ParseDraft returned error: %v", err)
	}
	if result.Draft.Name != "Weekly digest" {
		t.Fatalf("unexpected draft name: %s", result.Draft.Name)
	}
	if result.Draft.ScheduleType != automationmodel.AutomationScheduleTypeCron {
		t.Fatalf("unexpected schedule type: %s", result.Draft.ScheduleType)
	}
	if result.Draft.Timezone != "Europe/Dublin" {
		t.Fatalf("unexpected timezone: %s", result.Draft.Timezone)
	}
	if result.Draft.ScheduleConfig["cron_expr"] != "0 9 * * 1" {
		t.Fatalf("unexpected cron expr: %v", result.Draft.ScheduleConfig["cron_expr"])
	}
	if len(result.Draft.Actions) != 1 {
		t.Fatalf("expected one action, got %d", len(result.Draft.Actions))
	}
	config := result.Draft.Actions[0].Config
	if config["channel_type"] != "email" {
		t.Fatalf("unexpected channel type: %v", config["channel_type"])
	}
	if len(result.MissingFields) != 0 {
		t.Fatalf("unexpected missing fields: %#v", result.MissingFields)
	}
}

func TestParseDraftMarksMissingWorkflowID(t *testing.T) {
	raw := "```json\n" + `{
		"name": "Run workflow",
		"schedule": {
			"type": "once",
			"run_at": "2026-05-18T09:00:00Z"
		},
		"actions": [
			{
				"type": "run_workflow",
				"workflow_inputs": {"query": "check status"}
			}
		]
	}` + "\n```"

	result, err := ParseDraft(raw, "Asia/Tokyo")
	if err != nil {
		t.Fatalf("ParseDraft returned error: %v", err)
	}
	if result.Draft.ScheduleType != automationmodel.AutomationScheduleTypeOnce {
		t.Fatalf("unexpected schedule type: %s", result.Draft.ScheduleType)
	}
	if result.Draft.Timezone != "Asia/Tokyo" {
		t.Fatalf("unexpected fallback timezone: %s", result.Draft.Timezone)
	}
	if len(result.MissingFields) == 0 {
		t.Fatal("expected missing workflow agent id")
	}
	if result.MissingFields[0] != "actions.1.workflow_agent_id" {
		t.Fatalf("unexpected missing fields: %#v", result.MissingFields)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected workflow selection warning")
	}
}

func TestParseDraftMarksMissingCronTimezone(t *testing.T) {
	raw := `{
		"name": "Daily report",
		"schedule": {
			"type": "cron",
			"cron_expr": "0 9 * * *"
		},
		"actions": [
			{
				"type": "send_notification",
				"channel": "email",
				"to": ["ops@example.com"],
				"subject": "Daily report",
				"body": "Send the daily report."
			}
		]
	}`

	result, err := ParseDraft(raw, "")
	if err != nil {
		t.Fatalf("ParseDraft returned error: %v", err)
	}
	if !containsString(result.MissingFields, "schedule.timezone") {
		t.Fatalf("expected missing timezone, got %#v", result.MissingFields)
	}
}

func TestParseDraftSmsMarksRequiredFields(t *testing.T) {
	raw := `{
		"name": "SMS reminder",
		"schedule": {
			"type": "once",
			"run_at": "2026-05-18T09:00:00Z"
		},
		"actions": [
			{
				"type": "send_notification",
				"channel": "sms"
			}
		]
	}`

	result, err := ParseDraft(raw, "Asia/Tokyo")
	if err != nil {
		t.Fatalf("ParseDraft returned error: %v", err)
	}
	for _, field := range []string{
		"actions.1.to",
		"actions.1.notification_title",
		"actions.1.sms_link_code",
	} {
		if !containsString(result.MissingFields, field) {
			t.Fatalf("expected missing field %s, got %#v", field, result.MissingFields)
		}
	}
}

func TestParseDraftIgnoresUnsupportedModelMissingFields(t *testing.T) {
	raw := `{
		"name": "Customer follow-up",
		"schedule": {
			"type": "once",
			"run_at": "2026-05-18T09:00:00Z"
		},
		"actions": [
			{
				"type": "send_notification",
				"channel": "email",
				"to": [],
				"subject": "Follow up",
				"body": "Please check the customer follow-up."
			}
		],
		"missing_fields": ["待补充信息", "actions.1.to", "操作 1 收件人"]
	}`

	result, err := ParseDraft(raw, "Asia/Tokyo")
	if err != nil {
		t.Fatalf("ParseDraft returned error: %v", err)
	}
	if len(result.MissingFields) != 1 || result.MissingFields[0] != "actions.1.to" {
		t.Fatalf("expected only canonical missing field, got %#v", result.MissingFields)
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
