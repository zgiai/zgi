package digitalhuman

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"github.com/zgiai/zgi/api/internal/migrations"
	"github.com/zgiai/zgi/api/internal/modules/runtimeworkspace"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestRuntimeWorkspaceEndUserErasurePostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run the end-user erasure PostgreSQL integration")
	}
	t.Setenv("ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_ENABLED", "true")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.RunWithDB(db); err != nil {
		t.Fatalf("apply repository migrations: %v", err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	exec := func(query string, args ...any) {
		t.Helper()
		if err := tx.Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	count := func(query string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := tx.Raw(query, args...).Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	ctx := context.Background()
	organizationID, workspaceID, accountID := uuid.New(), uuid.New(), uuid.New()
	exec("INSERT INTO organizations (id, name) VALUES (?, 'Erasure')", organizationID)
	exec("INSERT INTO accounts (id, name, email) VALUES (?, 'Owner', ?)", accountID, uuid.NewString()+"@example.invalid")
	exec("INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, 'Erasure workspace')", workspaceID, organizationID)

	service := NewService(NewRepository(tx))
	scope := Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: accountID}
	detail, err := service.Create(ctx, scope, CreateRequest{Name: "Erasable", Persona: "Helps.", Rules: []string{"Be brief."}, DefaultModel: "test-model"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = service.Publish(ctx, scope, detail.Definition.ID, PublishRequest{ExpectedRevision: detail.Definition.Revision, ExpectedDraftLockVersion: detail.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}
	digitalHumanID := detail.Definition.ID
	state, err := service.UpdateMemoryState(ctx, scope, digitalHumanID, UpdateMemoryStateRequest{ExpectedMemoryEpoch: 0, AutoExtractionEnabled: true})
	if err != nil {
		t.Fatal(err)
	}

	exec("INSERT INTO runtime_workspaces (workspace_id, organization_id, managed_by_account_id, status, mode) VALUES (?, ?, ?, 'active', 'attached')", workspaceID, organizationID, accountID)
	var rawRuntimeID string
	if err := tx.Raw("SELECT runtime_id::text FROM runtime_workspaces WHERE workspace_id = ?", workspaceID).Scan(&rawRuntimeID).Error; err != nil {
		t.Fatal(err)
	}
	runtimeID := uuid.MustParse(rawRuntimeID)
	endUser := func(user string) (uuid.UUID, uuid.UUID) {
		t.Helper()
		subjectID, err := runtimeworkspace.EndUserSubject(runtimeID, user)
		if err != nil {
			t.Fatal(err)
		}
		exec("INSERT INTO user_subjects (id, kind, issuer_type, issuer_id, created_at, updated_at) VALUES (?, 'api_end_user', ?, ?, now(), now())", subjectID, runtimeworkspace.SubjectIssuerType, runtimeID)
		exec("INSERT INTO runtime_workspace_end_users (subject_id, runtime_workspace_id, external_user_id) VALUES (?, ?, ?)", subjectID, runtimeID, user)
		instanceID := uuid.New()
		exec(`INSERT INTO digital_human_end_user_instances (id, organization_id, workspace_id, digital_human_id, subject_kind, subject_id, issuer_type, issuer_id, status, label, quota_policy, first_seen_at, last_active_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'api_end_user', ?, 'runtime_workspace', ?, 'active', 'Label', '{}', now(), now(), now(), now())`, instanceID, organizationID, workspaceID, digitalHumanID, subjectID, runtimeID)
		conversationID := uuid.New()
		exec("INSERT INTO chat_runtime_conversations (id, organization_id, workspace_id, account_id, title, status, source, caller_type, caller_id, created_at, updated_at) VALUES (?, ?, ?, ?, 'Hello', 'normal', 'external-api', 'digital_human', ?, now(), now())",
			conversationID, organizationID, workspaceID, subjectID, digitalHumanID)
		exec("INSERT INTO chat_runtime_messages (id, conversation_id, query, answer, status, model_name, created_at, updated_at) VALUES (?, ?, 'What is two plus two', 'Four.', 'normal', 'test-model', now(), now())", uuid.New(), conversationID)
		runID := uuid.New()
		exec(`INSERT INTO digital_human_runs (id, organization_id, workspace_id, digital_human_id, digital_human_version_id, idempotency_key, invocation_event, run_spec, run_spec_revision, execution_snapshot, execution_snapshot_revision, execution_trace, query, status, conversation_id, user_subject_kind, user_subject_id, surface, end_user_instance_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, '{}', '{}', 'sha256:x', '{}', 'sha256:x', '{}', 'What is two plus two', 'succeeded', ?, 'api_end_user', ?, 'digital_human_api', ?, now(), now())`,
			runID, organizationID, workspaceID, digitalHumanID, detail.Published.ID, "erase-"+runID.String(), conversationID, subjectID, instanceID)
		propose := func(memoryScope *memoryruntime.MemoryScopeRef, class memoryruntime.MemoryClass, key, content string) {
			t.Helper()
			if _, err := service.ApplyMemoryProposal(ctx, scope, digitalHumanID, MemoryProposalRequest{
				OperationID: uuid.New(), Key: key, Action: memoryruntime.MutationActionUpsert, MemoryClass: class, MemoryType: memoryruntime.MemoryTypeFact,
				Scope: memoryScope, Content: content, ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch, RunID: &runID,
				Evidence: []memoryruntime.MemoryEvidence{{Type: memoryruntime.EvidenceTypeManual, ID: "note"}}, Visibility: MemoryVisibilityWorkspace,
				Confidence: 1, OccurredAt: time.Now().UTC(),
			}); err != nil {
				t.Fatal(err)
			}
		}
		propose(&memoryruntime.MemoryScopeRef{Type: memoryruntime.MemoryScopeInstance, ID: instanceID.String()}, memoryruntime.MemoryClassBusiness, "pet-"+user, user+" has a cat.")
		propose(nil, memoryruntime.MemoryClassSelf, "lesson-"+user, "Answer arithmetic briefly.")
		taskID := uuid.New()
		exec(`INSERT INTO automation_tasks (id, organization_id, workspace_id, name, task_kind, subject_type, subject_id, owner_kind, owner_subject_kind, owner_subject_id, origin_surface, status, schedule_type, timezone, schedule_config, source_type, created_by, updated_by, created_at, updated_at)
			VALUES (?, ?, ?, 'Daily', 'digital_human', 'digital_human', ?, 'user_subject', 'api_end_user', ?, 'digital_human_api', 'active', 'manual', 'UTC', '{}', 'manual', ?, ?, now(), now())`,
			taskID, organizationID, workspaceID, digitalHumanID.String(), subjectID, accountID, accountID)
		exec("INSERT INTO inbox_notifications (id, organization_id, workspace_id, recipient_kind, recipient_id, event, source_type, source_id, title, summary, link, context, created_at) VALUES (?, ?, ?, 'api_end_user', ?, 'task.completed', 'task', ?, 'Done', 'Report ready', '', '{}', now())",
			uuid.New(), organizationID, workspaceID, subjectID, taskID)
		return subjectID, instanceID
	}
	alice, aliceInstance := endUser("alice")
	bob, _ := endUser("bob")

	runtimeworkspace.UseEndUserDataOwner(NewEndUserData())
	rw := runtimeworkspace.NewService(tx)
	actor := runtimeworkspace.EndUserActor{Kind: runtimeworkspace.EndUserActorAccount, ID: accountID}
	export, err := rw.ExportEndUser(ctx, workspaceID, alice, actor)
	if err != nil {
		t.Fatal(err)
	}
	dh, _ := export.Apps["digital_humans"].(*EndUserDataExport)
	if len(export.Conversations) != 1 || len(export.Conversations[0].Messages) != 1 || export.Conversations[0].Messages[0].Answer != "Four." ||
		dh == nil || len(dh.Instances) != 1 || len(dh.Memory) != 1 || len(dh.Runs) != 1 || len(dh.Tasks) != 1 || len(dh.Notifications) != 1 {
		t.Fatalf("alice export = %+v / %+v", export, dh)
	}

	if err := rw.EraseEndUser(ctx, workspaceID, alice, actor); err != nil {
		t.Fatal(err)
	}
	for name, n := range map[string]int64{
		"conversations":   count("SELECT count(*) FROM chat_runtime_conversations WHERE account_id = ?", alice),
		"runs":            count("SELECT count(*) FROM digital_human_runs WHERE user_subject_id = ?", alice),
		"instance memory": count("SELECT count(*) FROM digital_human_memories WHERE scope_type = 'instance' AND scope_id = ?", aliceInstance.String()),
		"tasks":           count("SELECT count(*) FROM automation_tasks WHERE owner_subject_id = ?", alice),
		"notifications":   count("SELECT count(*) FROM inbox_notifications WHERE recipient_id = ?", alice),
	} {
		if n != 0 {
			t.Fatalf("alice kept %d %s", n, name)
		}
	}
	if n := count("SELECT count(*) FROM digital_human_memories WHERE memory_key = 'lesson-alice' AND status = 'active'"); n != 1 {
		t.Fatalf("the DigitalHuman's own memory from alice's run is gone: %d", n)
	}
	if n := count("SELECT count(*) FROM digital_human_memory_events event JOIN digital_human_memories memory ON memory.id = event.memory_id WHERE memory.memory_key = 'lesson-alice' AND event.run_id IS NULL"); n == 0 {
		t.Fatal("the DigitalHuman's own memory events still point at alice's run")
	}
	var status, external, instanceStatus, label string
	if err := tx.Raw("SELECT status, external_user_id FROM runtime_workspace_end_users WHERE subject_id = ?", alice).Row().Scan(&status, &external); err != nil || status != runtimeworkspace.EndUserErased || external != "" {
		t.Fatalf("alice tombstone = %q %q %v", status, external, err)
	}
	if err := tx.Raw("SELECT status, label FROM digital_human_end_user_instances WHERE id = ?", aliceInstance).Row().Scan(&instanceStatus, &label); err != nil || instanceStatus != "erased" || label != "" {
		t.Fatalf("alice instance tombstone = %q %q %v", instanceStatus, label, err)
	}
	if _, err := rw.ExportEndUser(ctx, workspaceID, alice, actor); !errors.Is(err, runtimeworkspace.ErrEndUserNotFound) {
		t.Fatalf("an erased end user exported: %v", err)
	}
	if err := rw.EraseEndUser(ctx, workspaceID, alice, actor); err != nil {
		t.Fatalf("erasing twice = %v", err)
	}
	events, err := rw.ListEndUserEvents(ctx, workspaceID, alice)
	if err != nil || len(events) != 2 {
		t.Fatalf("alice events = %+v %v", events, err)
	}
	if n := count("SELECT count(*) FROM digital_human_runs WHERE user_subject_id = ?", bob); n != 1 {
		t.Fatalf("bob lost his runs: %d", n)
	}
	if n := count("SELECT count(*) FROM digital_human_memory_events WHERE run_id IS NOT NULL AND run_id IN (SELECT id FROM digital_human_runs WHERE user_subject_id = ?)", bob); n == 0 {
		t.Fatal("bob's memory events lost their runs")
	}
	// Outside an erasure the memory events stay immutable.
	if err := tx.Exec("SAVEPOINT immutable").Error; err != nil {
		t.Fatal(err)
	}
	exec("SELECT set_config('zgi.digital_human_end_user_erase', '', true)")
	if err := tx.Exec("DELETE FROM digital_human_memory_events WHERE digital_human_id = ?", digitalHumanID).Error; err == nil {
		t.Fatal("memory events were deleted outside an erasure")
	}
	exec("ROLLBACK TO SAVEPOINT immutable")

	// A teardown purges what is left, tombstones included.
	exec("UPDATE runtime_workspaces SET status = 'disabled' WHERE workspace_id = ?", workspaceID)
	if _, err := runtimeworkspace.TeardownAttachedWorkspace(ctx, tx, workspaceID, time.Now().UTC()); err != nil {
		t.Fatalf("teardown with DigitalHuman end users = %v", err)
	}
	if n := count("SELECT count(*) FROM digital_human_end_user_instances WHERE subject_id IN (?, ?)", alice, bob); n != 0 {
		t.Fatalf("teardown kept %d instances", n)
	}
	if n := count("SELECT count(*) FROM user_subjects WHERE id IN (?, ?)", alice, bob); n != 0 {
		t.Fatalf("teardown kept %d subjects", n)
	}
}
