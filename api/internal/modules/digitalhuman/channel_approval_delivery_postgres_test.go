package digitalhuman

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providers/feishu"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	credentialcrypto "github.com/zgiai/zgi/api/internal/infra/credentialcrypto"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TestChannelWorkflowApprovalDeliveryPostgres covers the production path a
// channel approval uses to reply after the original run completed: the
// RuntimeService resolves the pending message's run and appends a durable
// follow-up final through GormRepository.
func TestChannelWorkflowApprovalDeliveryPostgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("ZGI_MIGRATION_TEST_DSN"))
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run DigitalHuman channel approval delivery PostgreSQL integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	organizationID, workspaceID, actorID := uuid.New(), uuid.New(), uuid.New()
	t.Cleanup(func() { cleanupChannelPostgresFixture(db, organizationID, actorID) })
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO accounts (id, name, email) VALUES (?, ?, ?)", []any{actorID, "Channel approval actor", "channel-approval-" + actorID.String() + "@example.invalid"}},
		{"INSERT INTO organizations (id, name) VALUES (?, ?)", []any{organizationID, "DigitalHuman channel approval"}},
		{"INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, ?)", []any{workspaceID, organizationID, "Channel approval"}},
	} {
		if err := db.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}

	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	connectionID := connectionIDForChannelFixture(organizationID)
	cipher, err := credentialcrypto.NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	encryptedCredentials, err := cipher.EncryptCredentials(map[string]string{
		"app_id": "cli_channel_approval", "app_secret": "test-only-secret",
		"verification_token": "test-only-token", "encrypt_key": "test-only-encrypt-key",
	}, credentialcrypto.CredentialAAD{
		OrganizationID: organizationID, ConnectionID: connectionID,
		IntegrationID: feishu.IntegrationID, CredentialVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	repository := NewRepository(db)
	lifecycle := NewService(repository)
	lifecycle.now = func() time.Time { return now }
	scope := Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: actorID}
	detail, err := lifecycle.Create(t.Context(), scope, CreateRequest{
		Name: "Channel approval human", IdentityName: "Channel teammate", Persona: "Respond as a channel teammate.",
		DefaultModel: "test-model", TriggerPolicy: map[string]any{"channel": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	detail, err = lifecycle.Publish(t.Context(), scope, detail.Definition.ID, PublishRequest{
		ExpectedRevision: detail.Definition.Revision, ExpectedDraftLockVersion: detail.Draft.LockVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	digitalHumanID := detail.Definition.ID

	installation := &channelruntime.Installation{
		ID: connectionID, OrganizationID: organizationID, WorkspaceID: workspaceID,
		Provider: feishu.IntegrationID, ConnectionID: connectionID, Name: "Feishu approval",
		ExternalTenantID: "tenant-approval", ExternalBotID: "ou_bot_approval", Status: channelruntime.InstallationStatusActive,
		Revision: 1, CreatedBy: actorID, UpdatedBy: actorID, CreatedAt: now, UpdatedAt: now,
	}
	defaultSubjectType := runtimeidentity.SubjectTypeDigitalHuman
	installation.DefaultSubjectType = &defaultSubjectType
	installation.DefaultSubjectID = &digitalHumanID
	if err := repository.CreateChannelInstallationWithCredential(t.Context(), installation, &ChannelInstallationCredential{
		InstallationID: installation.ID, OrganizationID: organizationID, WorkspaceID: workspaceID, Provider: feishu.IntegrationID,
		EncryptedCredentials: encryptedCredentials, CredentialVersion: 1, CreatedBy: actorID, UpdatedBy: actorID, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	mapping := &channelruntime.ConversationMapping{
		ID: uuid.New(), OrganizationID: organizationID, WorkspaceID: workspaceID, InstallationID: installation.ID,
		ExternalConversationID: "oc_channel_approval", ConversationType: channelruntime.ConversationTypeGroup,
		CreatedAt: now, UpdatedAt: now,
	}
	binding := &channelruntime.Binding{
		ID: uuid.New(), OrganizationID: organizationID, WorkspaceID: workspaceID, InstallationID: installation.ID,
		ConversationMappingID: mapping.ID, SubjectType: runtimeidentity.SubjectTypeDigitalHuman, SubjectID: digitalHumanID,
		WakePolicy: channelruntime.WakePolicyDirectAndMention, Enabled: true, Revision: 1,
		CreatedBy: actorID, UpdatedBy: actorID, CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.CreateChannelBinding(t.Context(), mapping, binding); err != nil {
		t.Fatal(err)
	}
	inbound, _, err := repository.RecordInboundEvent(t.Context(), &channelruntime.InboundEvent{
		ID: uuid.New(), OrganizationID: organizationID, WorkspaceID: workspaceID,
		InstallationID: installation.ID, BindingID: &binding.ID,
		ProviderEventID: "feishu-event-approval", ProviderMessageID: "om_approval",
		ExternalConversationID: mapping.ExternalConversationID, SenderExternalID: "ou_sender",
		EventType: "channel.mention", PayloadDigest: "sha256:" + strings.Repeat("c", 64),
		Status: channelruntime.InboundEventReceived, ReceivedAt: now, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}

	runtime := NewRuntimeService(
		repository,
		channelPostgresResolver{versionID: detail.Published.ID, now: now},
		fakeBrainAdapter{}, &fakeDigitalHumanChat{}, channelPostgresDispatcher{},
	)
	runtime.now = func() time.Time { return now }
	run, _, err := runtime.SubmitInvocation(t.Context(), scope, invocation.Event{
		ID: "feishu-event-approval", Type: invocation.EventTypeChannelMention,
		Origin: invocation.Origin{Kind: invocation.OriginChannel, Provider: feishu.IntegrationID, InstallationID: installation.ID.String()},
		Target: runtimeidentity.SubjectRef{
			Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID,
			OrganizationID: organizationID, WorkspaceID: workspaceID,
		},
		Sender: invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "ou_sender"}, OccurredAt: now,
		Context: map[string]any{
			"channel_inbound_event_id": inbound.ID.String(), "channel_binding_id": binding.ID.String(),
			"provider_message_id": "om_approval", "external_conversation_id": mapping.ExternalConversationID,
		},
		ReplyTo: &invocation.ReplyAddress{
			Provider: feishu.IntegrationID, InstallationID: installation.ID.String(), ConversationID: mapping.ExternalConversationID,
		},
	}, "运行需要审批的工作流", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkInboundEvent(t.Context(), inbound.ID, channelruntime.InboundEventAccepted, "", &run.ID, now); err != nil {
		t.Fatal(err)
	}
	messageID := uuid.New()
	if err := db.Model(&Run{}).Where("id = ?", run.ID).Updates(map[string]any{
		"status": RunStatusRunning, "started_at": now, "updated_at": now,
		"message_id": messageID, "conversation_id": uuid.New(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	// The run ends while the workflow waits for approval; its proposal is the
	// original final delivery.
	if err := repository.CompleteRunWithDelivery(t.Context(), run.ID, "工作流需要审批", datatypes.JSON(`{}`), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var original channelruntime.Delivery
	if err := db.Where("id = ?", channelDeliveryID(run.ID)).Take(&original).Error; err != nil {
		t.Fatal(err)
	}

	pending := &runtimemodel.Message{ID: messageID}
	for attempt := 0; attempt < 2; attempt++ {
		if err := runtime.deliverChannelWorkflowApprovalMessage(t.Context(), scope, digitalHumanID, pending, "approval:workflow-run-approval", "审批已通过，结果如下"); err != nil {
			t.Fatalf("deliver approval answer (attempt %d) error = %v", attempt, err)
		}
	}
	if err := runtime.deliverChannelWorkflowApprovalMessage(t.Context(), scope, digitalHumanID, pending, "approval-wait:"+uuid.NewString(), channelWorkflowApprovalWaitingNotice); err != nil {
		t.Fatalf("deliver waiting notice error = %v", err)
	}

	var deliveries []channelruntime.Delivery
	if err := db.Where("run_id = ?", run.ID).Order("created_at, delivery_key").Find(&deliveries).Error; err != nil {
		t.Fatal(err)
	}
	if len(deliveries) != 3 {
		t.Fatalf("deliveries = %d, want original final, one idempotent answer and one notice", len(deliveries))
	}
	for _, delivery := range deliveries {
		if delivery.Kind != channelruntime.DeliveryKindFinal || delivery.InstallationID != installation.ID ||
			delivery.BindingID != binding.ID || delivery.ExternalConversationID != mapping.ExternalConversationID {
			t.Fatalf("delivery = %#v, want a final on the original reply target", delivery)
		}
		if delivery.ID != original.ID && delivery.Status != channelruntime.DeliveryPending {
			t.Fatalf("follow-up delivery = %#v, want pending", delivery)
		}
	}

	// Completion is the execution-of-record: a channel target that disappears
	// while the model runs must not leave the run looking like it never ended.
	orphan, _, err := runtime.SubmitInvocation(t.Context(), scope, invocation.Event{
		ID: "scheduled-orphaned-target", Type: invocation.EventTypeSchedule,
		Origin: invocation.Origin{Kind: invocation.OriginSchedule},
		Target: runtimeidentity.SubjectRef{
			Type: runtimeidentity.SubjectTypeDigitalHuman, ID: digitalHumanID,
			OrganizationID: organizationID, WorkspaceID: workspaceID,
		},
		Sender:     invocation.PrincipalRef{Type: invocation.PrincipalSystem, ID: "digital-human-task-scheduler"},
		OccurredAt: now,
		ReplyTo: &invocation.ReplyAddress{
			Provider: feishu.IntegrationID, InstallationID: installation.ID.String(), ConversationID: mapping.ExternalConversationID,
		},
	}, "安装被停用前开始的任务", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&Run{}).Where("id = ?", orphan.ID).Updates(map[string]any{
		"status": RunStatusRunning, "started_at": now, "updated_at": now,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&channelruntime.Installation{}).Where("id = ?", installation.ID).
		Update("status", channelruntime.InstallationStatusDisabled).Error; err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteRunWithDelivery(t.Context(), orphan.ID, "任务已完成", datatypes.JSON(`{}`), now.Add(2*time.Second)); err != nil {
		t.Fatalf("CompleteRunWithDelivery() with an unavailable channel target error = %v", err)
	}
	var completed Run
	if err := db.Where("id = ?", orphan.ID).Take(&completed).Error; err != nil {
		t.Fatal(err)
	}
	if completed.Status != RunStatusSucceeded {
		t.Fatalf("run status = %s, want the completion committed without a delivery target", completed.Status)
	}
	var orphanDeliveries int64
	if err := db.Model(&channelruntime.Delivery{}).Where("run_id = ?", orphan.ID).Count(&orphanDeliveries).Error; err != nil {
		t.Fatal(err)
	}
	if orphanDeliveries != 0 {
		t.Fatalf("deliveries for a run without a channel target = %d, want 0", orphanDeliveries)
	}
}
