package definition

import (
	"testing"
	"time"

	automationmodel "github.com/zgiai/ginext/internal/modules/automation/model"
)

func TestCompileNextRunAtUsesCronTimezone(t *testing.T) {
	now := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)

	nextRunAt, err := compileNextRunAtFrom(
		automationmodel.AutomationScheduleTypeCron,
		"Asia/Shanghai",
		map[string]interface{}{"cron_expr": "50 19 * * *"},
		now,
	)
	if err != nil {
		t.Fatalf("compile next run at: %v", err)
	}

	expected := time.Date(2026, 5, 17, 11, 50, 0, 0, time.UTC)
	if !nextRunAt.Equal(expected) {
		t.Fatalf("expected next run %s, got %s", expected, nextRunAt.UTC())
	}
}
