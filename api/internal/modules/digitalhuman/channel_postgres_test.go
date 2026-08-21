package digitalhuman

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runspec"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/integrations/adapters/feishu"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestDigitalHumanChannelPostgresConcurrentReplayAndDeliveryIntent(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("ZGI_MIGRATION_TEST_DSN"))
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run DigitalHuman channel PostgreSQL integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(16)

	organizationID, workspaceID, actorID := uuid.New(), uuid.New(), uuid.New()
	t.Cleanup(func() { cleanupChannelPostgresFixture(db, organizationID, actorID) })
	if err := db.Exec("INSERT INTO accounts (id, name, email) VALUES (?, ?, ?)", actorID, "Channel actor", "channel-"+actorID.String()+"@example.invalid").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO organizations (id, name) VALUES (?, ?)", organizationID, "DigitalHuman channel integration").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, ?)", workspaceID, organizationID, "Channel integration").Error; err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 19, 11, 0, 0, 0, time.UTC)
	cipher, err := integrations.NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	encryptedCredentials, err := cipher.EncryptCredentials(map[string]string{
		"app_id": "cli_channel_test", "app_secret": "test-only-secret",
		"verification_token": "test-only-token", "encrypt_key": "test-only-encrypt-key",
	}, integrations.CredentialAAD{
		OrganizationID: organizationID, ConnectionID: connectionIDForChannelFixture(organizationID),
		IntegrationID: feishu.IntegrationID, CredentialVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	connectionID := connectionIDForChannelFixture(organizationID)
	connection := &integrations.IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: feishu.IntegrationID, DriverID: feishu.DriverID,
		Name: "Channel integration", CredentialSource: integrations.ConnectionCredentialSourceOrganization,
		AuthType: integrations.ConnectionAuthTypeServiceAccount, AuthMethodID: feishu.TenantAppAuthMethodID,
		EncryptedCredentials: &encryptedCredentials,
		Status:               integrations.ConnectionStatusActive, CreatedBy: &actorID, UpdatedBy: &actorID, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(connection).Error; err != nil {
		t.Fatal(err)
	}

	repository := NewRepository(db)
	lifecycle := NewService(repository)
	lifecycle.now = func() time.Time { return now }
	scope := Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: actorID}
	detail, err := lifecycle.Create(t.Context(), scope, CreateRequest{
		Name: "Channel subject", Persona: "Respond as a channel teammate.", Rules: []string{"Never disclose credentials."},
		DefaultModel: "test-model", MemoryPolicy: map[string]any{"automatic_extraction": true}, TriggerPolicy: map[string]any{"channel": true},
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

	installation := &channelruntime.Installation{
		ID: uuid.New(), OrganizationID: organizationID, WorkspaceID: workspaceID,
		Provider: feishu.IntegrationID, ConnectionID: connection.ID, Name: "Feishu test",
		ExternalTenantID: "tenant-test", ExternalBotID: "ou_bot_test", Status: channelruntime.InstallationStatusActive,
		Revision: 1, CreatedBy: actorID, UpdatedBy: actorID, CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.CreateChannelInstallation(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	mapping := &channelruntime.ConversationMapping{
		ID: uuid.New(), OrganizationID: organizationID, WorkspaceID: workspaceID, InstallationID: installation.ID,
		ExternalConversationID: "oc_concurrent_test", ConversationType: channelruntime.ConversationTypeGroup,
		CreatedAt: now, UpdatedAt: now,
	}
	binding := &channelruntime.Binding{
		ID: uuid.New(), OrganizationID: organizationID, WorkspaceID: workspaceID, InstallationID: installation.ID,
		ConversationMappingID: mapping.ID, SubjectType: runtimeidentity.SubjectTypeDigitalHuman, SubjectID: detail.Definition.ID,
		WakePolicy: channelruntime.WakePolicyDirectAndMention, Enabled: true, Revision: 1,
		CreatedBy: actorID, UpdatedBy: actorID, CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.CreateChannelBinding(t.Context(), mapping, binding); err != nil {
		t.Fatal(err)
	}

	const replayCount = 10
	var createdEvents atomic.Int32
	eventIDs := make(chan uuid.UUID, replayCount)
	errorsCh := make(chan error, replayCount)
	var wait sync.WaitGroup
	for index := 0; index < replayCount; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			stored, created, recordErr := repository.RecordInboundEvent(context.Background(), &channelruntime.InboundEvent{
				ID: uuid.New(), OrganizationID: organizationID, WorkspaceID: workspaceID,
				InstallationID: installation.ID, BindingID: &binding.ID,
				ProviderEventID: "feishu-event-concurrent", ProviderMessageID: "om_concurrent",
				ExternalConversationID: mapping.ExternalConversationID, SenderExternalID: "ou_sender",
				EventType: "channel.mention", PayloadDigest: "sha256:" + strings.Repeat("a", 64),
				Status: channelruntime.InboundEventReceived, ReceivedAt: now, CreatedAt: now, UpdatedAt: now,
			})
			if recordErr != nil {
				errorsCh <- recordErr
				return
			}
			if created {
				createdEvents.Add(1)
			}
			eventIDs <- stored.ID
		}()
	}
	wait.Wait()
	close(errorsCh)
	for recordErr := range errorsCh {
		t.Fatalf("RecordInboundEvent() error = %v", recordErr)
	}
	close(eventIDs)
	var inboundID uuid.UUID
	for id := range eventIDs {
		if inboundID == uuid.Nil {
			inboundID = id
		} else if inboundID != id {
			t.Fatalf("replay returned different inbound IDs: %s and %s", inboundID, id)
		}
	}
	if createdEvents.Load() != 1 {
		t.Fatalf("created inbound events = %d, want 1", createdEvents.Load())
	}
	replayedByMessage, createdByMessage, err := repository.RecordInboundEvent(t.Context(), &channelruntime.InboundEvent{
		ID: uuid.New(), OrganizationID: organizationID, WorkspaceID: workspaceID,
		InstallationID: installation.ID, BindingID: &binding.ID,
		ProviderEventID: "feishu-event-concurrent-retry", ProviderMessageID: "om_concurrent",
		ExternalConversationID: mapping.ExternalConversationID, SenderExternalID: "ou_sender",
		EventType: "channel.mention", PayloadDigest: "sha256:" + strings.Repeat("a", 64),
		Status: channelruntime.InboundEventReceived, ReceivedAt: now, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil || createdByMessage || replayedByMessage.ID != inboundID {
		t.Fatalf("message replay = (%#v, %t, %v), want existing inbound %s", replayedByMessage, createdByMessage, err, inboundID)
	}

	invocationEvent := invocation.Event{
		ID: "feishu-event-concurrent", Type: invocation.EventTypeChannelMention,
		Origin: invocation.Origin{Kind: invocation.OriginChannel, Provider: feishu.IntegrationID, InstallationID: installation.ID.String()},
		Target: runtimeidentity.SubjectRef{
			Type: runtimeidentity.SubjectTypeDigitalHuman, ID: detail.Definition.ID,
			OrganizationID: organizationID, WorkspaceID: workspaceID,
		},
		Sender: invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: "ou_sender"}, OccurredAt: now,
		Context: map[string]any{
			"channel_inbound_event_id": inboundID.String(), "channel_binding_id": binding.ID.String(),
			"provider_message_id": "om_concurrent", "external_conversation_id": mapping.ExternalConversationID,
		},
		ReplyTo: &invocation.ReplyAddress{
			Provider: feishu.IntegrationID, InstallationID: installation.ID.String(), ConversationID: mapping.ExternalConversationID,
		},
	}
	runtime := NewRuntimeService(
		repository,
		channelPostgresResolver{versionID: detail.Published.ID, now: now},
		fakeBrainAdapter{}, &fakeDigitalHumanChat{}, channelPostgresDispatcher{},
	)
	runtime.now = func() time.Time { return now }
	var createdRuns atomic.Int32
	runIDs := make(chan uuid.UUID, replayCount)
	runErrors := make(chan error, replayCount)
	for index := 0; index < replayCount; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			run, created, submitErr := runtime.SubmitInvocation(context.Background(), scope, invocationEvent, "请回复", nil)
			if submitErr != nil {
				runErrors <- submitErr
				return
			}
			if created {
				createdRuns.Add(1)
			}
			runIDs <- run.ID
		}()
	}
	wait.Wait()
	close(runErrors)
	for submitErr := range runErrors {
		t.Fatalf("SubmitInvocation() error = %v", submitErr)
	}
	close(runIDs)
	var runID uuid.UUID
	for id := range runIDs {
		if runID == uuid.Nil {
			runID = id
		} else if runID != id {
			t.Fatalf("replay returned different run IDs: %s and %s", runID, id)
		}
	}
	if createdRuns.Load() != 1 {
		t.Fatalf("created runs = %d, want 1", createdRuns.Load())
	}
	var runCount, outboxCount int64
	if err := db.Model(&Run{}).Where("organization_id = ? AND digital_human_id = ?", organizationID, detail.Definition.ID).Count(&runCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&RunOutbox{}).Where("organization_id = ?", organizationID).Count(&outboxCount).Error; err != nil {
		t.Fatal(err)
	}
	if runCount != 1 || outboxCount != 1 {
		t.Fatalf("durable run state: runs=%d, run_outbox=%d", runCount, outboxCount)
	}

	if err := repository.MarkInboundEvent(t.Context(), inboundID, channelruntime.InboundEventAccepted, "", &runID, now); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&Run{}).Where("id = ?", runID).Updates(map[string]any{"status": RunStatusRunning, "started_at": now, "updated_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteRunWithDelivery(t.Context(), runID, "最终回复", datatypes.JSON(`{}`), now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteRunWithDelivery(t.Context(), runID, "不得重复", datatypes.JSON(`{}`), now.Add(2*time.Second)); !errors.Is(err, ErrRunConflict) {
		t.Fatalf("second completion error = %v, want ErrRunConflict", err)
	}
	var delivery channelruntime.Delivery
	if err := db.Where("run_id = ?", runID).Take(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	if delivery.ID != channelDeliveryID(runID) {
		t.Fatalf("delivery ID = %s, want deterministic %s", delivery.ID, channelDeliveryID(runID))
	}
	var deliveryCount int64
	if err := db.Model(&channelruntime.Delivery{}).Where("run_id = ?", runID).Count(&deliveryCount).Error; err != nil {
		t.Fatal(err)
	}
	if deliveryCount != 1 || delivery.Status != channelruntime.DeliveryPending {
		t.Fatalf("delivery intents = %d, delivery = %#v", deliveryCount, delivery)
	}
	deliveryAdapter := &channelAdapterStub{deliveryErr: errors.New("temporary provider failure")}
	channelRegistry, err := channelruntime.NewRegistry(deliveryAdapter)
	if err != nil {
		t.Fatal(err)
	}
	deliveryService := NewChannelService(repository, repository, runtime, channelRegistry, channelCredentialStub{}, &channelTransientDispatcherStub{})
	deliveryNow := now.Add(3 * time.Second)
	deliveryService.now = func() time.Time { return deliveryNow }
	if err := deliveryService.DispatchDeliveries(t.Context(), 10); err == nil {
		t.Fatal("DispatchDeliveries() temporary error = nil")
	}
	if err := db.Where("id = ?", delivery.ID).Take(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	if delivery.Status != channelruntime.DeliveryPending || delivery.AttemptCount != 1 {
		t.Fatalf("delivery after provider failure = %#v", delivery)
	}
	deliveryAdapter.deliveryErr = nil
	deliveryAdapter.receipt = &channelruntime.DeliveryReceipt{ProviderMessageID: "om_recovered", ProviderRequestID: "request-recovered"}
	deliveryNow = deliveryNow.Add(time.Minute)
	if err := deliveryService.DispatchDeliveries(t.Context(), 10); err != nil {
		t.Fatalf("DispatchDeliveries(recovery) error = %v", err)
	}
	if err := db.Where("id = ?", delivery.ID).Take(&delivery).Error; err != nil {
		t.Fatal(err)
	}
	if delivery.Status != channelruntime.DeliverySucceeded || delivery.AttemptCount != 2 || delivery.ProviderMessageID != "om_recovered" {
		t.Fatalf("recovered delivery = %#v", delivery)
	}
	var runCountAfterRecovery, runOutboxCountAfterRecovery int64
	if err := db.Model(&Run{}).Where("organization_id = ? AND digital_human_id = ?", organizationID, detail.Definition.ID).Count(&runCountAfterRecovery).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&RunOutbox{}).Where("organization_id = ?", organizationID).Count(&runOutboxCountAfterRecovery).Error; err != nil {
		t.Fatal(err)
	}
	if runCountAfterRecovery != runCount || runOutboxCountAfterRecovery != outboxCount || deliveryAdapter.deliveryCalls != 2 {
		t.Fatalf(
			"delivery recovery reran model state: runs=%d/%d, outbox=%d/%d, delivery calls=%d",
			runCountAfterRecovery, runCount, runOutboxCountAfterRecovery, outboxCount, deliveryAdapter.deliveryCalls,
		)
	}
	if err := db.Model(&channelruntime.Delivery{}).Where("id = ?", delivery.ID).Update("provider", "forged").Error; err == nil {
		t.Fatal("database accepted a delivery provider that does not match its installation")
	}
	localConversationID := uuid.New()
	lastEventAt := now.Add(4 * time.Second)
	if err := db.Model(&channelruntime.ConversationMapping{}).Where("id = ?", mapping.ID).Updates(map[string]any{
		"local_conversation_id": localConversationID, "last_event_at": lastEventAt,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repository.DeleteChannelBinding(t.Context(), scope, detail.Definition.ID, binding.ID); err != nil {
		t.Fatal(err)
	}
	var resetMapping channelruntime.ConversationMapping
	if err := db.Where("id = ?", mapping.ID).Take(&resetMapping).Error; err != nil {
		t.Fatal(err)
	}
	if resetMapping.LocalConversationID != nil || resetMapping.LastEventAt != nil {
		t.Fatalf("conversation mapping retained prior subject state: %#v", resetMapping)
	}
	if err := integrations.NewGormConnectionRepository(db).DeleteAs(t.Context(), organizationID, connection.ID, &actorID); !errors.Is(err, integrations.ErrConnectionInUse) {
		t.Fatalf("DeleteAs(bound channel connection) error = %v", err)
	}

	cascadeBinding := &channelruntime.Binding{
		ID: uuid.New(), OrganizationID: organizationID, WorkspaceID: workspaceID, InstallationID: installation.ID,
		ConversationMappingID: mapping.ID, SubjectType: runtimeidentity.SubjectTypeDigitalHuman, SubjectID: detail.Definition.ID,
		WakePolicy: channelruntime.WakePolicyDirectAndMention, Enabled: true, Revision: 1,
		CreatedBy: actorID, UpdatedBy: actorID, CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.CreateChannelBinding(t.Context(), mapping, cascadeBinding); err != nil {
		t.Fatalf("CreateChannelBinding(cascade fixture) error = %v", err)
	}
	if err := repository.DeleteChannelInstallation(t.Context(), scope, installation.ID, DeleteChannelInstallationRequest{}); !errors.Is(err, ErrChannelInstallationInUse) {
		t.Fatalf("DeleteChannelInstallation(in use) error = %v, want ErrChannelInstallationInUse", err)
	}
	if err := repository.DeleteChannelInstallation(t.Context(), scope, installation.ID, DeleteChannelInstallationRequest{RemoveBindings: true}); err != nil {
		t.Fatalf("DeleteChannelInstallation(remove bindings) error = %v", err)
	}
	var deletedInstallation channelruntime.Installation
	if err := db.Unscoped().Where("id = ?", installation.ID).Take(&deletedInstallation).Error; err != nil {
		t.Fatalf("load deleted channel installation: %v", err)
	}
	if !deletedInstallation.DeletedAt.Valid {
		t.Fatal("channel installation deleted_at is not set")
	}
	var deletedBinding channelruntime.Binding
	if err := db.Unscoped().Where("id = ?", cascadeBinding.ID).Take(&deletedBinding).Error; err != nil {
		t.Fatalf("load cascade-deleted channel binding: %v", err)
	}
	if !deletedBinding.DeletedAt.Valid {
		t.Fatal("cascade-deleted channel binding deleted_at is not set")
	}
	var retainedEvents, retainedDeliveries int64
	if err := db.Model(&channelruntime.InboundEvent{}).Where("installation_id = ?", installation.ID).Count(&retainedEvents).Error; err != nil {
		t.Fatalf("count retained inbound events: %v", err)
	}
	if err := db.Model(&channelruntime.Delivery{}).Where("installation_id = ?", installation.ID).Count(&retainedDeliveries).Error; err != nil {
		t.Fatalf("count retained channel deliveries: %v", err)
	}
	if retainedEvents == 0 || retainedDeliveries == 0 {
		t.Fatalf("retained channel history: events=%d, deliveries=%d, want both non-zero", retainedEvents, retainedDeliveries)
	}
}

type channelPostgresResolver struct {
	versionID uuid.UUID
	now       time.Time
}

func (resolver channelPostgresResolver) Resolve(_ context.Context, event invocation.Event, _ *uuid.UUID) (*ResolvedExecution, error) {
	snapshot := ExecutionSnapshot{
		SchemaVersion: executionSnapshotSchemaV1, Subject: event.Target, DigitalHumanVersion: resolver.versionID,
		Name: "Channel subject", Persona: "Respond as a channel teammate.", Rules: []string{"Never disclose credentials."},
		Model: "test-model", ResolvedAt: resolver.now,
	}
	var err error
	snapshot.Revision, err = executionSnapshotRevision(snapshot)
	if err != nil {
		return nil, err
	}
	spec, err := runspec.Seal(runspec.Spec{
		InvocationID: event.ID, Subject: event.Target,
		Identity: runspec.VersionRef{Kind: "identity", ID: resolver.versionID.String(), Revision: snapshot.Revision},
		Model:    runspec.VersionRef{Kind: "model", ID: snapshot.Model, Revision: snapshot.Revision},
		Policies: []runspec.VersionRef{{Kind: "execution_snapshot", ID: resolver.versionID.String(), Revision: snapshot.Revision}},
		Budget:   runspec.Budget{MaxDurationSeconds: 300}, ResolvedAt: resolver.now,
	})
	return &ResolvedExecution{Spec: spec, Snapshot: snapshot}, err
}

type channelPostgresDispatcher struct{}

func (channelPostgresDispatcher) EnqueueRun(context.Context, uuid.UUID) error { return nil }
func (channelPostgresDispatcher) CancelRun(context.Context, uuid.UUID) error  { return nil }

func cleanupChannelPostgresFixture(db *gorm.DB, organizationID, actorID uuid.UUID) {
	if db == nil {
		return
	}
	for _, table := range []string{
		"channel_delivery_outbox", "channel_inbound_events", "channel_bindings", "external_conversation_mappings", "channel_installations",
		"digital_human_run_outbox", "digital_human_memory_jobs", "digital_human_action_ledger", "digital_human_memory_undo_records",
		"digital_human_memory_events", "digital_human_memories", "digital_human_memory_states", "digital_human_resource_bindings", "digital_human_role_assignments",
		"digital_human_runs", "digital_human_versions", "digital_humans", "integration_connections", "workspaces", "organizations",
	} {
		column := "organization_id"
		if table == "organizations" {
			column = "id"
		}
		_ = db.Exec("DELETE FROM "+table+" WHERE "+column+" = ?", organizationID).Error
	}
	_ = db.Exec("DELETE FROM accounts WHERE id = ?", actorID).Error
}

// Stable only inside this test fixture so the credential AAD can be created
// before the model value without retaining plaintext credential material.
func connectionIDForChannelFixture(organizationID uuid.UUID) uuid.UUID {
	return uuid.NewSHA1(organizationID, []byte("digital-human-channel-postgres-fixture"))
}
