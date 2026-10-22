package digitalhuman

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// A run left waiting after its conversation moved on is found for repair;
// one whose question is still open, or that settles elsewhere, is not.
func TestAnsweredWaitingRunsAreListedForRepair(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&Run{}, &runtimemodel.Message{}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	add := func(surface, messageStatus string) uuid.UUID {
		messageID := uuid.New()
		if err := db.Create(&runtimemodel.Message{ID: messageID, ConversationID: uuid.New(), Query: "q", Status: messageStatus, CreatedAt: old, UpdatedAt: old}).Error; err != nil {
			t.Fatal(err)
		}
		run := &Run{ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(), DigitalHumanID: uuid.New(), DigitalHumanVersionID: uuid.New(),
			IdempotencyKey: uuid.NewString(), InvocationEvent: datatypes.JSON(`{}`), RunSpec: datatypes.JSON(`{}`), ExecutionSnapshot: datatypes.JSON(`{}`),
			ExecutionTrace: datatypes.JSON(`{}`), Query: "q", Status: RunStatusWaiting, WaitingReason: runtimemodel.MessageStatusWaitingQuestion,
			Surface: surface, MessageID: &messageID, CreatedAt: old, UpdatedAt: old}
		if err := db.Create(run).Error; err != nil {
			t.Fatal(err)
		}
		return run.ID
	}
	answered := add(SurfaceWebApp, runtimemodel.MessageStatusCompleted)
	add(SurfaceWebApp, runtimemodel.MessageStatusWaitingQuestion)
	add(SurfaceTask, runtimemodel.MessageStatusCompleted)

	runs, err := (&GormRepository{db: db}).ListAnsweredWaitingRuns(context.Background(), time.Now(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != answered {
		t.Fatalf("answered waiting runs = %#v", runs)
	}
}

func TestRunConversationCallerNamesTheWebApp(t *testing.T) {
	webAppID := uuid.New()
	run := &Run{ID: uuid.New(), DigitalHumanID: uuid.New(), Surface: SurfaceWebApp,
		InvocationEvent: datatypes.JSON(`{"context":{"web_app_id":"` + webAppID.String() + `"}}`)}
	caller := runConversationCaller(run)
	if caller.SourceWebAppID == nil || *caller.SourceWebAppID != webAppID || caller.RunID != nil {
		t.Fatalf("caller = %#v", caller)
	}
	if caller.Source != digitalHumanWebAppConversationSource {
		t.Fatalf("caller source = %q", caller.Source)
	}
}
