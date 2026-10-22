package digitalhumantask

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/gorm"
)

func TestReadOnlyCheckKeepsDatabaseErrors(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_TASK_VISIBILITY_ENABLED", "true")
	fixture := newVisibilityFixture(t)
	outage := errors.New("visibility probe timed out")
	// Fail the visibility count that follows a change outside the caller's
	// scope, as a transient database error would.
	if err := fixture.service.db.Callback().Query().Before("gorm:query").Register("test:fail_visibility_count", func(tx *gorm.DB) {
		if _, ok := tx.Statement.Dest.(*int64); ok {
			_ = tx.AddError(outage)
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, err := fixture.service.ApplyTaskAction(t.Context(), fixture.memberA, fixture.shared.ID, digitalhuman.TaskActionPause,
		digitalhuman.ApplyTaskActionRequest{ExpectedRevision: fixture.shared.Revision}, digitalhuman.TaskMutationContext{OperationID: uuid.New()})
	if !errors.Is(err, outage) || errors.Is(err, digitalhuman.ErrTaskNotFound) {
		t.Fatalf("pause(shared) during an outage error = %v, want the database error", err)
	}
}
