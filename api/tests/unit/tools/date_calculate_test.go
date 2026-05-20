package tools_test

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/tools"
	timepkg "github.com/zgiai/zgi/api/internal/modules/tools/builtin/time"
)

func TestDateCalculateTool_ShiftDate(t *testing.T) {
	tool := timepkg.NewDateCalculateTool("tenant-1")
	tests := []struct {
		name       string
		params     map[string]interface{}
		wantResult string
	}{
		{
			name: "add days",
			params: map[string]interface{}{
				"operation": "add",
				"base_date": "2026-05-16",
				"amount":    3,
				"unit":      "day",
				"timezone":  "UTC",
			},
			wantResult: "2026-05-19",
		},
		{
			name: "subtract weeks",
			params: map[string]interface{}{
				"operation": "subtract",
				"base_date": "2026-05-16",
				"amount":    2,
				"unit":      "week",
				"timezone":  "UTC",
			},
			wantResult: "2026-05-02",
		},
		{
			name: "add months",
			params: map[string]interface{}{
				"operation": "add",
				"base_date": "2026-05-16",
				"amount":    1,
				"unit":      "month",
				"timezone":  "UTC",
			},
			wantResult: "2026-06-16",
		},
		{
			name: "add years",
			params: map[string]interface{}{
				"operation": "add",
				"base_date": "2026-05-16",
				"amount":    1,
				"unit":      "year",
				"timezone":  "UTC",
			},
			wantResult: "2027-05-16",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := tool.Invoke(context.Background(), "user-1", tt.params, nil, nil, nil)
			if err != nil {
				t.Fatalf("Invoke() error = %v", err)
			}
			if len(messages) != 1 || messages[0].Type != tools.ToolInvokeMessageTypeJSON {
				t.Fatalf("messages = %#v, want one json message", messages)
			}
			if messages[0].Data["result_date"] != tt.wantResult {
				t.Fatalf("result_date = %v, want %s", messages[0].Data["result_date"], tt.wantResult)
			}
		})
	}
}

func TestDateCalculateTool_MetadataIncludesLocalizedHumanText(t *testing.T) {
	entity := timepkg.NewDateCalculateTool("tenant-1").GetEntity()
	if got := entity.Identity.Label.Get("zh_Hans"); got != "日期计算" {
		t.Fatalf("zh_Hans label = %q, want localized text", got)
	}
	if got := entity.Description.Human.Get("zh_Hans"); got == "" || got == entity.Description.Human.Get("en_US") {
		t.Fatalf("zh_Hans description = %q, want localized text", got)
	}
	for _, param := range entity.Parameters {
		if got := param.Label.Get("zh_Hans"); got == "" || got == param.Label.Get("en_US") {
			t.Fatalf("parameter %s zh_Hans label = %q, want localized text", param.Name, got)
		}
		if got := param.HumanDescription.Get("zh_Hans"); got == "" || got == param.HumanDescription.Get("en_US") {
			t.Fatalf("parameter %s zh_Hans human description = %q, want localized text", param.Name, got)
		}
		if got := param.Placeholder.Get("zh_Hans"); param.Type != tools.ToolParameterTypeSelect && got == "" {
			t.Fatalf("parameter %s zh_Hans placeholder is empty", param.Name)
		}
		for _, option := range param.Options {
			if got := option.Label.Get("zh_Hans"); got == "" || got == option.Label.Get("en_US") {
				t.Fatalf("parameter %s option %s zh_Hans label = %q, want localized text", param.Name, option.Value, got)
			}
		}
	}
}

func TestDateCalculateTool_DiffDays(t *testing.T) {
	tool := timepkg.NewDateCalculateTool("tenant-1")
	tests := []struct {
		name       string
		baseDate   string
		targetDate string
		timezone   string
		wantDays   int
		wantAbs    int
	}{
		{
			name:       "utc range",
			baseDate:   "2026-05-16",
			targetDate: "2026-06-01",
			timezone:   "UTC",
			wantDays:   16,
			wantAbs:    16,
		},
		{
			name:       "dst spring forward natural day",
			baseDate:   "2026-03-08",
			targetDate: "2026-03-09",
			timezone:   "America/New_York",
			wantDays:   1,
			wantAbs:    1,
		},
		{
			name:       "dst fall back natural day",
			baseDate:   "2026-11-01",
			targetDate: "2026-11-02",
			timezone:   "America/New_York",
			wantDays:   1,
			wantAbs:    1,
		},
		{
			name:       "reverse range",
			baseDate:   "2026-06-01",
			targetDate: "2026-05-16",
			timezone:   "UTC",
			wantDays:   -16,
			wantAbs:    16,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, err := tool.Invoke(context.Background(), "user-1", map[string]interface{}{
				"operation":   "diff",
				"base_date":   tt.baseDate,
				"target_date": tt.targetDate,
				"timezone":    tt.timezone,
			}, nil, nil, nil)
			if err != nil {
				t.Fatalf("Invoke() error = %v", err)
			}
			if messages[0].Data["days"] != tt.wantDays {
				t.Fatalf("days = %v, want %d", messages[0].Data["days"], tt.wantDays)
			}
			if messages[0].Data["abs_days"] != tt.wantAbs {
				t.Fatalf("abs_days = %v, want %d", messages[0].Data["abs_days"], tt.wantAbs)
			}
		})
	}
}

func TestDateCalculateTool_InvalidInput(t *testing.T) {
	tool := timepkg.NewDateCalculateTool("tenant-1")
	tests := []struct {
		name   string
		params map[string]interface{}
	}{
		{
			name: "invalid timezone",
			params: map[string]interface{}{
				"operation": "add",
				"timezone":  "Invalid/Zone",
			},
		},
		{
			name: "invalid unit",
			params: map[string]interface{}{
				"operation": "add",
				"base_date": "2026-05-16",
				"amount":    1,
				"unit":      "century",
			},
		},
		{
			name: "invalid operation",
			params: map[string]interface{}{
				"operation": "multiply",
				"base_date": "2026-05-16",
			},
		},
		{
			name: "invalid date",
			params: map[string]interface{}{
				"operation": "add",
				"base_date": "not-a-date",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tool.Invoke(context.Background(), "user-1", tt.params, nil, nil, nil)
			if err == nil {
				t.Fatalf("Invoke() error = nil, want error")
			}
		})
	}
}
