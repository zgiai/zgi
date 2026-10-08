package digitalhuman

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func platformWaitingRunFixture(t *testing.T) (*Run, *runtimemodel.Message) {
	t.Helper()
	cid, mid, subject := uuid.New(), uuid.New(), uuid.New()
	run := &Run{ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(),
		DigitalHumanID: uuid.New(), DigitalHumanVersionID: uuid.New(), UserSubjectKind: "api_end_user",
		UserSubjectID: &subject, Surface: SurfaceAPI, ConversationID: &cid, MessageID: &mid, Status: RunStatusRunning}
	origin := &toolgovernance.ExecutionOrigin{OrganizationID: run.OrganizationID.String(), WorkspaceID: run.WorkspaceID.String(),
		ApplicationType: "digital_human", ApplicationID: run.DigitalHumanID.String(), PublicationRef: run.DigitalHumanVersionID.String(),
		Entry: "external-api", SubjectKind: run.UserSubjectKind, SubjectID: subject.String(), ConversationID: cid.String(), MessageID: mid.String()}
	frozen := toolgovernance.NewFrozenInvocation(toolgovernance.FrozenInvocationRequest{
		ExecutionOrigin: origin, CorrelationID: "pending", SkillID: "chart-generator", ToolName: "generate_chart",
		Arguments: map[string]interface{}{"output_filename": "test-only"}, Now: time.Now(),
	})
	metadata := map[string]interface{}{"tool_governance_continuation": map[string]interface{}{"correlation_id": "pending"},
		"tool_governance_decisions": []interface{}{map[string]interface{}{"correlation_id": "pending",
			"approval_event": map[string]interface{}{"frozen_invocation": frozen}}}}
	// Match the map/slice shape restored by the JSONB serializer.
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		t.Fatal(err)
	}
	return run, &runtimemodel.Message{ID: mid, ConversationID: cid, Status: runtimemodel.MessageStatusWaitingApproval, Metadata: metadata}
}

func TestPlatformGovernanceMessagePinsRunOwner(t *testing.T) {
	mutations := map[string]func(*Run, *runtimemodel.Message){
		"organization":              func(r *Run, _ *runtimemodel.Message) { r.OrganizationID = uuid.New() },
		"workspace":                 func(r *Run, _ *runtimemodel.Message) { r.WorkspaceID = uuid.New() },
		"application":               func(r *Run, _ *runtimemodel.Message) { r.DigitalHumanID = uuid.New() },
		"publication":               func(r *Run, _ *runtimemodel.Message) { r.DigitalHumanVersionID = uuid.New() },
		"subject":                   func(r *Run, _ *runtimemodel.Message) { r.UserSubjectID = uuidPointer(uuid.New()) },
		"entry":                     func(r *Run, _ *runtimemodel.Message) { r.Surface = SurfaceWebApp },
		"message":                   func(_ *Run, m *runtimemodel.Message) { m.ID = uuid.New() },
		"conversation":              func(_ *Run, m *runtimemodel.Message) { m.ConversationID = uuid.New() },
		"missing frozen invocation": func(_ *Run, m *runtimemodel.Message) { delete(m.Metadata, "tool_governance_decisions") },
		"correlation": func(_ *Run, m *runtimemodel.Message) {
			m.Metadata["tool_governance_continuation"] = map[string]interface{}{"correlation_id": "other"}
		},
		"tampered hash": func(_ *Run, m *runtimemodel.Message) {
			event := m.Metadata["tool_governance_decisions"].([]interface{})[0].(map[string]interface{})
			approval := event["approval_event"].(map[string]interface{})
			approval["frozen_invocation"].(map[string]interface{})["hash"] = "sha256:invalid"
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			run, message := platformWaitingRunFixture(t)
			if !platformGovernanceMessageMatchesRun(run, message) {
				t.Fatal("valid sealed platform approval was rejected")
			}
			mutate(run, message)
			if platformGovernanceMessageMatchesRun(run, message) {
				t.Fatal("mismatched platform approval was accepted")
			}
		})
	}
}

func TestPlatformGovernanceRunLifecycle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&Run{}, &runtimemodel.Message{}); err != nil {
		t.Fatal(err)
	}
	testPlatformGovernanceRunLifecycle(t, db)
}

func TestPlatformGovernanceRunLifecyclePostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("dedicated PostgreSQL DSN not configured")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{DisableForeignKeyConstraintWhenMigrating: true})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	// Transaction-local tables shadow production names; no application data changes.
	for _, ddl := range []string{
		`CREATE TEMP TABLE digital_human_runs (id uuid PRIMARY KEY, organization_id uuid, workspace_id uuid, digital_human_id uuid, digital_human_version_id uuid, user_subject_kind text, user_subject_id uuid, surface text, conversation_id uuid, message_id uuid, status text, waiting_reason text, waiting_receipt uuid, cancel_requested_at timestamptz, result_summary text, execution_trace jsonb, completed_at timestamptz, updated_at timestamptz) ON COMMIT DROP`,
		`CREATE TEMP TABLE chat_runtime_messages (id uuid PRIMARY KEY, conversation_id uuid, status text, metadata jsonb, deleted_at timestamptz) ON COMMIT DROP`,
	} {
		if err := tx.Exec(ddl).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, column := range []string{"waiting_approval_origin varchar(32) NOT NULL DEFAULT ''", "idempotency_key text", "invocation_event jsonb", "run_spec jsonb", "run_spec_revision text", "execution_snapshot jsonb", "execution_snapshot_revision text", "query text", "result_summary text", "error_code text", "error_message text", "created_at timestamptz"} {
		if err := tx.Exec("ALTER TABLE digital_human_runs ADD COLUMN IF NOT EXISTS " + column).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, column := range []string{"query text", "model_name text", "model_parameters jsonb DEFAULT '{}'", "created_at timestamptz DEFAULT CURRENT_TIMESTAMP", "updated_at timestamptz"} {
		if err := tx.Exec("ALTER TABLE chat_runtime_messages ADD COLUMN " + column).Error; err != nil {
			t.Fatal(err)
		}
	}
	testPlatformGovernanceRunLifecycle(t, tx)
}

func testPlatformGovernanceRunLifecycle(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Exec(`CREATE TEMP TABLE connector_confirmation_continuations (origin_version integer, origin_surface text, run_id text, digital_human_id text, state text, route_receipt text)`).Error; err != nil {
		t.Fatal(err)
	}
	run, message := platformWaitingRunFixture(t)
	run.IdempotencyKey = run.ID.String()
	run.InvocationEvent, run.RunSpec, run.ExecutionSnapshot, run.ExecutionTrace = datatypes.JSON(`{}`), datatypes.JSON(`{}`), datatypes.JSON(`{}`), datatypes.JSON(`{}`)
	if err := db.Select("id", "organization_id", "workspace_id", "digital_human_id", "digital_human_version_id", "user_subject_kind", "user_subject_id", "surface", "conversation_id", "message_id", "status", "idempotency_key", "invocation_event", "run_spec", "run_spec_revision", "execution_snapshot", "execution_snapshot_revision", "execution_trace", "query", "result_summary", "error_code", "error_message", "created_at", "updated_at").Create(run).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Select("id", "conversation_id", "status", "metadata", "query", "model_name", "updated_at").Create(message).Error; err != nil {
		t.Fatal(err)
	}
	repo, now := NewRepository(db), time.Now().UTC()
	if err := db.Model(message).Update("metadata", datatypes.JSON(`{}`)).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkRunWaiting(t.Context(), run.ID, runtimemodel.MessageStatusWaitingApproval, now); err == nil {
		t.Fatal("approval without a sealed invocation or Connector receipt was accepted")
	}
	encoded, err := json.Marshal(message.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(message).Update("metadata", datatypes.JSON(encoded)).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkRunWaiting(t.Context(), run.ID, runtimemodel.MessageStatusWaitingApproval, now); err != nil {
		t.Fatalf("mark platform waiting without Connector receipt: %v", err)
	}
	var waiting Run
	if err := db.First(&waiting, "id = ?", run.ID).Error; err != nil || waiting.WaitingApprovalOrigin != platformCLIApprovalOrigin || waiting.WaitingReceipt != nil {
		t.Fatalf("platform approval origin not persisted: %#v, %v", waiting, err)
	}
	if err := repo.CompleteRun(t.Context(), run.ID, "premature", datatypes.JSON(`{}`), now); err == nil {
		t.Fatal("pending approval completed its host run")
	}
	if err := db.Model(message).Update("status", runtimemodel.MessageStatusCompleted).Error; err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := repo.CompleteRun(t.Context(), run.ID, "done", datatypes.JSON(`{}`), now); err != nil {
			t.Fatalf("complete or repeat platform result: %v", err)
		}
	}
	var stored Run
	if err := db.First(&stored, "id = ?", run.ID).Error; err != nil || stored.Status != RunStatusSucceeded || stored.WaitingReason != "" || stored.WaitingReceipt != nil || stored.WaitingApprovalOrigin != "" {
		t.Fatalf("stored host = %#v, err %v; want succeeded without Connector receipt", stored, err)
	}
	for _, action := range []string{"cancel", "fail"} {
		if err := db.Model(&Run{}).Where("id = ?", run.ID).Updates(map[string]any{
			"status": RunStatusWaiting, "waiting_reason": runtimemodel.MessageStatusWaitingApproval,
			"waiting_approval_origin": platformCLIApprovalOrigin, "cancel_requested_at": nil,
		}).Error; err != nil {
			t.Fatal(err)
		}
		want := RunStatusFailed
		if action == "cancel" {
			want = RunStatusCanceled
			_, err = repo.RequestRunCancel(t.Context(), Scope{OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID}, run.DigitalHumanID, run.ID, now)
		} else {
			err = repo.FailRun(t.Context(), run.ID, "test_failure", "test only", now)
		}
		if err != nil {
			t.Fatalf("%s waiting approval: %v", action, err)
		}
		var terminal Run
		if err := db.First(&terminal, "id = ?", run.ID).Error; err != nil || terminal.Status != want || terminal.WaitingReason != "" || terminal.WaitingReceipt != nil || terminal.WaitingApprovalOrigin != "" {
			t.Fatalf("%s did not clear waiting state: %#v, %v", action, terminal, err)
		}
	}
}
