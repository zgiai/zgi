package service

import (
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Run only against a dedicated disposable PostgreSQL database.
func TestPlatformApprovalAtomicClaimPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_PLATFORM_APPROVAL_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("dedicated platform approval PostgreSQL DSN not configured")
	}
	root, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	rootDB, err := root.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rootDB.Close() })
	// Every run owns a fresh schema, so tests do not depend on tables or
	// migration metadata left by a previous acceptance run.
	schema := "platform_approval_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := root.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Error(err)
		}
	})
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.RuntimeParams["search_path"] = schema
	sqlDB := stdlib.OpenDB(*config)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&runtimemodel.Conversation{}, &runtimemodel.Message{}); err != nil {
		t.Fatal(err)
	}
	for _, appType := range []string{"agent", "digital_human"} {
		for _, entry := range []string{"console", "webapp", "external-api"} {
			t.Run(appType+"/"+entry, func(t *testing.T) {
				org, ws, user, app, cid, mid := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
				scope := Scope{OrganizationID: org, WorkspaceID: &ws, AccountID: user, SkipAccessCheck: true}
				caller := Caller{Type: appType, ID: &app, Source: entry}
				conversation := &runtimemodel.Conversation{ID: cid, OrganizationID: org, WorkspaceID: &ws, AccountID: user, CallerType: appType, CallerID: &app, Source: entry,
					Status: runtimemodel.ConversationStatusNormal, RuntimeStatus: runtimemodel.ConversationRuntimeStatusIdle}
				if entry == "webapp" {
					id := uuid.New()
					caller.SourceWebAppID, conversation.SourceWebAppID = &id, &id
				}
				if appType == "agent" && entry == "external-api" {
					id := uuid.New()
					scope.ExternalUserID, conversation.ExternalUserID = &id, &id
				}
				metadata := pendingToolGovernanceDecisionMetadata("atomic")
				event, _ := toolGovernanceDecisionEventFromMetadata(metadata, "atomic")
				event["approval_status"] = "approved"
				metadata = mergeToolGovernanceDecisionMetadata(metadata, event)
				message := &runtimemodel.Message{ID: mid, ConversationID: cid, Status: runtimemodel.MessageStatusWaitingApproval, Metadata: metadata}
				if err := db.Create(conversation).Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Create(message).Error; err != nil {
					t.Fatal(err)
				}
				repos := repository.NewRepositories(db)
				repos.Access = toolGovernanceStreamAccessRepo{}
				svc := NewService(repos, nil).(*service)
				var winners atomic.Int32
				var wg sync.WaitGroup
				start := make(chan struct{})
				for range 12 {
					wg.Go(func() {
						<-start
						_, err := svc.beginToolGovernanceContinuationForCaller(t.Context(), scope, caller, cid, mid, "atomic")
						if err == nil {
							winners.Add(1)
						} else if !IsContinuationAlreadyRunningError(err) {
							t.Errorf("claim: %v", err)
						}
					})
				}
				close(start)
				wg.Wait()
				if winners.Load() != 1 {
					t.Fatalf("claimed %d times", winners.Load())
				}
				other := scope
				other.AccountID = uuid.New()
				if _, err := svc.beginToolGovernanceContinuationForCaller(t.Context(), other, caller, cid, mid, "atomic"); err == nil {
					t.Fatal("cross-user claim accepted")
				}
			})
		}
	}
}
