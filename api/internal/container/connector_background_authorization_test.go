package container

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"github.com/zgiai/zgi/api/internal/modules/connectors/backgroundauth"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	socialtools "github.com/zgiai/zgi/api/internal/modules/tools/builtin/socialoperations"
	"gorm.io/datatypes"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type backgroundAccessStub struct {
	calls int
	err   error
	run   *digitalhuman.Run
	event invocation.Event
}

func (s *backgroundAccessStub) AuthorizeBackgroundConnectorExecution(_ context.Context, run *digitalhuman.Run, event invocation.Event) error {
	s.calls++
	s.run, s.event = run, event
	return s.err
}

type backgroundGrantStoreStub struct {
	consumeRequest backgroundauth.ConsumeRequest
	consumeReceipt backgroundauth.Receipt
	consumeErr     error
	issueRequest   backgroundauth.IssueRequest
	issueGrant     backgroundauth.Grant
	issueErr       error
	list           []backgroundauth.Grant
	listErr        error
	revoked        uuid.UUID
	revokeErr      error
}

func (s *backgroundGrantStoreStub) Consume(_ context.Context, request backgroundauth.ConsumeRequest) (backgroundauth.Receipt, error) {
	s.consumeRequest = request
	return s.consumeReceipt, s.consumeErr
}
func (s *backgroundGrantStoreStub) Issue(_ context.Context, request backgroundauth.IssueRequest) (backgroundauth.Grant, error) {
	s.issueRequest = request
	return s.issueGrant, s.issueErr
}
func (s *backgroundGrantStoreStub) List(_ context.Context, _, _ uuid.UUID) ([]backgroundauth.Grant, error) {
	return s.list, s.listErr
}
func (s *backgroundGrantStoreStub) Revoke(_ context.Context, _, _ uuid.UUID, grantID uuid.UUID) error {
	s.revoked = grantID
	return s.revokeErr
}

type backgroundBindingReaderStub struct{ binding connectorcore.TargetBinding }

func (s backgroundBindingReaderStub) GetBinding(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (connectorcore.TargetBinding, error) {
	return s.binding, nil
}

type backgroundConnectionReaderStub struct {
	connection connectorcore.PersonalConnection
}

func (s backgroundConnectionReaderStub) GetConnection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (connectorcore.PersonalConnection, error) {
	return s.connection, nil
}

type backgroundEmployeeReaderStub struct{ detail *digitalhuman.Detail }

func (s backgroundEmployeeReaderStub) Get(context.Context, digitalhuman.Scope, uuid.UUID) (*digitalhuman.Detail, error) {
	return s.detail, nil
}

type backgroundCatalogStub struct{ definition catalog.Definition }

func (s backgroundCatalogStub) Get(string) (catalog.Definition, bool) { return s.definition, true }

func TestBackgroundTaskRunResolverRestoresDurableTaskOwnerAndEvent(t *testing.T) {
	db := backgroundAuthorizationTestDB(t)
	organizationID, workspaceID, employeeID, versionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	conversationID, messageID, runID, taskID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	ownerID, actionID, taskRunID := uuid.New(), uuid.New(), uuid.New()
	event := backgroundTaskEvent(t, organizationID, workspaceID, employeeID, taskID, taskRunID, actionID)
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	run := digitalhuman.Run{ID: runID, OrganizationID: organizationID, WorkspaceID: workspaceID, DigitalHumanID: employeeID,
		DigitalHumanVersionID: versionID, Surface: digitalhuman.SurfaceTask, Status: digitalhuman.RunStatusRunning,
		ConversationID: &conversationID, MessageID: &messageID, InvocationEvent: raw, RunSpec: datatypes.JSON(`{}`), ExecutionSnapshot: datatypes.JSON(`{}`), ExecutionTrace: datatypes.JSON(`{}`)}
	kind := automationmodel.AutomationTaskOwnerDigitalHuman
	task := automationmodel.AutomationTask{ID: taskID.String(), OrganizationID: organizationID.String(), WorkspaceID: workspaceID.String(),
		SubjectID: stringPointer(employeeID.String()), TaskKind: automationmodel.AutomationTaskKindDigitalHuman,
		OwnerKind: &kind, CreatedBy: ownerID.String(), UpdatedBy: ownerID.String(), Status: automationmodel.AutomationTaskStatusActive}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	access := &backgroundAccessStub{}
	resolver := NewBackgroundTaskRunResolver(db, access)
	restored, err := resolver.Restore(t.Context(), ActiveBackgroundTaskRunRequest{OrganizationID: organizationID, ConversationID: conversationID,
		MessageID: &messageID, EmployeeID: employeeID, VersionID: versionID})
	if err != nil {
		t.Fatal(err)
	}
	if restored.Run.ID != runID || restored.OwnerID != ownerID || restored.WorkspaceID != workspaceID || restored.Event.ID != actionID.String() || access.calls != 1 {
		t.Fatalf("restored task = %#v, access=%#v", restored, access)
	}
	socialRuntime := &socialOperationsRuntime{container: &ServiceContainer{db: db, backgroundTaskRunResolver: resolver}}
	if _, _, err = socialRuntime.restoreSocialOperationsRun(t.Context(), socialtools.InvocationIdentity{Runtime: &tools.ToolRuntime{TenantID: organizationID.String(), InvokeFrom: tools.ToolInvokeFromDigitalHuman}, ActorID: ownerID.String(), ConversationID: conversationID.String(), EmployeeID: employeeID.String(), MessageID: messageID.String()}); err == nil {
		t.Fatal("ordinary task bypassed required SOP setup")
	}

	if err := db.Model(&digitalhuman.Run{}).Where("id = ?", runID).Update("cancel_requested_at", time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.Restore(t.Context(), ActiveBackgroundTaskRunRequest{OrganizationID: organizationID, ConversationID: conversationID,
		EmployeeID: employeeID, VersionID: versionID}); err == nil {
		t.Fatal("cancelled run restored")
	}
}

func TestBackgroundTaskRunResolverRejectsStaleSocialOperationsConfiguration(t *testing.T) {
	db := backgroundAuthorizationTestDB(t)
	organizationID, workspaceID, ownerID, employeeID, versionID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	recordID, artifactID := uuid.New(), uuid.New()
	record := socialops.Record{ID: recordID, Scope: socialops.Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, OwnerID: ownerID, EmployeeID: employeeID},
		ScopeKey: strings.Repeat("c", 64), Kind: socialops.CurrentConfigurationRecordKind, Key: socialops.CurrentConfigurationRecordKey,
		Revision: 4, ArtifactID: artifactID, UpdatedAt: time.Now().UTC()}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	metadata := digitalhuman.SocialOperationsTaskMetadata{Scope: digitalhuman.SocialOperationsTaskScope{
		OrganizationID: organizationID, WorkspaceID: workspaceID, OwnerID: ownerID, EmployeeID: employeeID,
	}, EmployeeVersionID: versionID, ConfigurationRevision: 4, ConfigurationRecordID: recordID, ConfigurationArtifactID: artifactID, TaskKey: "daily_content",
		Budget: digitalhuman.SocialOperationsTaskBudget{MaxConnectorCalls: 80}}
	source := "social_operations"
	task := automationmodel.AutomationTask{SourceRef: &source, SourceSnapshot: map[string]any{"social_operations": metadata}}
	run := digitalhuman.Run{OrganizationID: organizationID, WorkspaceID: workspaceID, DigitalHumanID: employeeID, DigitalHumanVersionID: versionID}
	resolver := &BackgroundTaskRunResolver{db: db}
	if err := resolver.validateSocialOperationsTaskSnapshot(t.Context(), task, run, ownerID); err != nil {
		t.Fatalf("current configuration rejected: %v", err)
	}
	if err := db.Model(&socialops.Record{}).Where("id = ?", recordID).Updates(map[string]any{"revision": 5, "artifact_id": uuid.New()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := resolver.validateSocialOperationsTaskSnapshot(t.Context(), task, run, ownerID); !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeAccessDenied) {
		t.Fatalf("stale configuration error = %v", err)
	}
}

func TestConnectorBackgroundAuthorizerUsesOnlyRestoredTaskIdentityAndLiveSnapshots(t *testing.T) {
	db := backgroundAuthorizationTestDB(t)
	organizationID, workspaceID, employeeID, versionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	conversationID, messageID, runID, taskID, taskRunID, actionID, ownerID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	event := backgroundTaskEvent(t, organizationID, workspaceID, employeeID, taskID, taskRunID, actionID)
	raw, _ := json.Marshal(event)
	principal := ownerID
	run := digitalhuman.Run{ID: runID, OrganizationID: organizationID, WorkspaceID: workspaceID, DigitalHumanID: employeeID,
		DigitalHumanVersionID: versionID, PrincipalID: &principal, PrincipalType: string(invocation.PrincipalSystem), Surface: digitalhuman.SurfaceTask,
		Status: digitalhuman.RunStatusRunning, ConversationID: &conversationID, MessageID: &messageID, InvocationEvent: raw, RunSpec: datatypes.JSON(`{}`), ExecutionSnapshot: datatypes.JSON(`{}`), ExecutionTrace: datatypes.JSON(`{}`)}
	kind := automationmodel.AutomationTaskOwnerDigitalHuman
	task := automationmodel.AutomationTask{ID: taskID.String(), OrganizationID: organizationID.String(), WorkspaceID: workspaceID.String(), SubjectID: stringPointer(employeeID.String()),
		TaskKind: automationmodel.AutomationTaskKindDigitalHuman, OwnerKind: &kind, CreatedBy: ownerID.String(), UpdatedBy: ownerID.String(), Status: automationmodel.AutomationTaskStatusActive,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	access := &backgroundAccessStub{}
	grants := &backgroundGrantStoreStub{consumeReceipt: backgroundauth.Receipt{GrantID: uuid.New(), ConsumptionID: uuid.New()}}
	authorizer := &ConnectorBackgroundAuthorization{resolver: NewBackgroundTaskRunResolver(db, access), grants: grants}
	bindingID, connectionID := uuid.New(), uuid.New()
	target := connectorcore.TargetRef{Type: connectorcore.TargetTypeDigitalHuman, ID: employeeID.String(), Scope: connectorcore.ScopePublished,
		VersionID: versionID.String(), BindingInstanceID: uuid.NewString()}
	binding := connectorcore.TargetBinding{ID: bindingID, OrganizationID: organizationID, OwnerAccountID: ownerID, ConnectionID: connectionID,
		ConnectorID: "x-social", Target: target, Status: connectorcore.BindingStatusActive, Revision: 7}
	connection := connectorcore.PersonalConnection{ID: connectionID, OrganizationID: organizationID, OwnerAccountID: ownerID, ConnectorID: "x-social",
		CredentialVersion: 3, Revision: 5}
	request := connectorexecution.BackgroundAuthorizationRequest{Request: connectorexecution.ExecutionRequest{ConversationID: conversationID.String(),
		OrganizationID: organizationID, OwnerAccountID: ownerID, ActingPrincipalID: ownerID, BindingID: bindingID, ConnectionID: connectionID,
		ConnectorID: "x-social", Target: target, OperationID: "x-social.account.get", LogicalOperationID: "publication-1",
		Input: map[string]any{"account_id": "account-1", "employee_id": uuid.NewString()}}, Binding: binding, Connection: connection,
		Operation: catalog.Operation{ID: "x-social.account.get"}, ContractDigest: strings.Repeat("a", 64)}
	if allowed, err := authorizer.AuthorizeBackground(t.Context(), request); err == nil || allowed {
		t.Fatalf("missing background identity = %v, %v", allowed, err)
	}
	if allowed, err := authorizer.AuthorizeBackground(connectorexecution.WithBackgroundIdentity(t.Context(), uuid.New(), messageID), request); err == nil || allowed {
		t.Fatalf("wrong run identity = %v, %v", allowed, err)
	}
	ctx := connectorexecution.WithBackgroundIdentity(t.Context(), runID, messageID)
	allowed, err := authorizer.AuthorizeBackground(ctx, request)
	if err != nil || !allowed {
		t.Fatalf("AuthorizeBackground() = %v, %v", allowed, err)
	}
	want := backgroundauth.Scope{OrganizationID: organizationID, OwnerID: ownerID, WorkspaceID: workspaceID, EmployeeID: employeeID,
		VersionID: versionID, BindingID: bindingID, ConnectionID: connectionID, BindingRevision: 7, ConnectionRevision: 5,
		CredentialVersion: 3, ContractDigest: strings.Repeat("a", 64)}
	if grants.consumeRequest.Scope != want || grants.consumeRequest.OperationID != "x-social.account.get" || grants.consumeRequest.Input["account_id"] != "account-1" {
		t.Fatalf("consume request = %#v", grants.consumeRequest)
	}

	grants.consumeErr = connectorcore.NewError(connectorcore.ErrorCodeAccessDenied, false, nil)
	if allowed, err = authorizer.AuthorizeBackground(ctx, request); err != nil || allowed {
		t.Fatalf("missing grant = %v, %v", allowed, err)
	}
	grants.consumeErr = errors.New("database unavailable")
	if allowed, err = authorizer.AuthorizeBackground(ctx, request); err == nil || allowed {
		t.Fatalf("infrastructure error did not fail closed: %v, %v", allowed, err)
	}
	request.Connection.OwnerAccountID = uuid.New()
	if allowed, err = authorizer.AuthorizeBackground(ctx, request); err == nil || allowed {
		t.Fatalf("caller identity overrode connection owner: %v, %v", allowed, err)
	}
}

func TestConnectorBackgroundAuthorizerRequiresPublicationClaimForSocialWrites(t *testing.T) {
	db := backgroundAuthorizationTestDB(t)
	organizationID, workspaceID, employeeID, versionID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	conversationID, messageID, runID, taskID, taskRunID, actionID, ownerID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	event := backgroundTaskEvent(t, organizationID, workspaceID, employeeID, taskID, taskRunID, actionID)
	raw, _ := json.Marshal(event)
	run := digitalhuman.Run{ID: runID, OrganizationID: organizationID, WorkspaceID: workspaceID, DigitalHumanID: employeeID, DigitalHumanVersionID: versionID,
		Surface: digitalhuman.SurfaceTask, Status: digitalhuman.RunStatusRunning, ConversationID: &conversationID, MessageID: &messageID, InvocationEvent: raw,
		RunSpec: datatypes.JSON(`{}`), ExecutionSnapshot: datatypes.JSON(`{}`), ExecutionTrace: datatypes.JSON(`{}`)}
	kind := automationmodel.AutomationTaskOwnerDigitalHuman
	configurationRecordID, configurationArtifactID := uuid.New(), uuid.New()
	configurationRecord := socialops.Record{ID: configurationRecordID,
		Scope:    socialops.Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, OwnerID: ownerID, EmployeeID: employeeID},
		ScopeKey: strings.Repeat("e", 64), Kind: socialops.CurrentConfigurationRecordKind, Key: socialops.CurrentConfigurationRecordKey,
		Revision: 1, ArtifactID: configurationArtifactID, UpdatedAt: time.Now().UTC()}
	if err := db.Create(&configurationRecord).Error; err != nil {
		t.Fatal(err)
	}
	metadata := digitalhuman.SocialOperationsTaskMetadata{Scope: digitalhuman.SocialOperationsTaskScope{
		OrganizationID: organizationID, WorkspaceID: workspaceID, OwnerID: ownerID, EmployeeID: employeeID,
	}, EmployeeVersionID: versionID, ConfigurationRevision: 1, ConfigurationRecordID: configurationRecordID,
		ConfigurationArtifactID: configurationArtifactID, TaskKey: "frequent_inbox",
		Budget: digitalhuman.SocialOperationsTaskBudget{MaxConnectorCalls: 80}}
	source := "social_operations"
	task := automationmodel.AutomationTask{ID: taskID.String(), OrganizationID: organizationID.String(), WorkspaceID: workspaceID.String(), SubjectID: stringPointer(employeeID.String()),
		TaskKind: automationmodel.AutomationTaskKindDigitalHuman, OwnerKind: &kind, CreatedBy: ownerID.String(), UpdatedBy: ownerID.String(), Status: automationmodel.AutomationTaskStatusActive,
		SourceRef: &source, SourceSnapshot: map[string]any{"social_operations": metadata}}
	if err := db.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&task).Error; err != nil {
		t.Fatal(err)
	}
	publicationID, connectionID, bindingID := uuid.New(), uuid.New(), uuid.New()
	publication := socialops.Publication{ID: publicationID, Scope: socialops.Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, OwnerID: ownerID, EmployeeID: employeeID},
		ScopeKey: strings.Repeat("b", 64), ConnectionID: connectionID, AccountID: "account-1", ContentKey: "content", Platform: "x", State: "dispatching", Revision: 7,
		MediaJSON: "[]", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Create(&publication).Error; err != nil {
		t.Fatal(err)
	}
	grants := &backgroundGrantStoreStub{consumeReceipt: backgroundauth.Receipt{GrantID: uuid.New(), ConsumptionID: uuid.New()}}
	authorizer := &ConnectorBackgroundAuthorization{resolver: NewBackgroundTaskRunResolver(db, &backgroundAccessStub{}), grants: grants}
	target := connectorcore.TargetRef{Type: connectorcore.TargetTypeDigitalHuman, ID: employeeID.String(), Scope: connectorcore.ScopePublished, VersionID: versionID.String(), BindingInstanceID: uuid.NewString()}
	binding := connectorcore.TargetBinding{ID: bindingID, OrganizationID: organizationID, OwnerAccountID: ownerID, ConnectionID: connectionID, ConnectorID: "x-social", Target: target, Status: connectorcore.BindingStatusActive, Revision: 1}
	connection := connectorcore.PersonalConnection{ID: connectionID, OrganizationID: organizationID, OwnerAccountID: ownerID, ConnectorID: "x-social", CredentialVersion: 1, Revision: 1}
	input := map[string]any{"account_id": "account-1", "text": "approved"}
	request := connectorexecution.BackgroundAuthorizationRequest{Request: connectorexecution.ExecutionRequest{ConversationID: conversationID.String(), OrganizationID: organizationID,
		OwnerAccountID: ownerID, ActingPrincipalID: ownerID, BindingID: bindingID, ConnectionID: connectionID, ConnectorID: "x-social", Target: target,
		OperationID: "x-social.post.create", LogicalOperationID: "publication-write", Input: input}, Binding: binding, Connection: connection,
		Operation: catalog.Operation{ID: "x-social.post.create"}, ContractDigest: strings.Repeat("a", 64)}
	ctx := connectorexecution.WithBackgroundIdentity(t.Context(), runID, messageID)
	for name, claimCtx := range map[string]context.Context{
		"missing":        ctx,
		"wrong revision": withSocialPublicationClaim(ctx, publicationID, publication.Revision+1, input),
		"wrong input":    withSocialPublicationClaim(ctx, publicationID, publication.Revision, map[string]any{"account_id": "account-1", "text": "other"}),
	} {
		if allowed, err := authorizer.AuthorizeBackground(claimCtx, request); err == nil || allowed {
			t.Fatalf("%s claim = %v, %v", name, allowed, err)
		}
	}
	allowed, err := authorizer.AuthorizeBackground(withSocialPublicationClaim(ctx, publicationID, publication.Revision, input), request)
	if err != nil || !allowed {
		t.Fatalf("valid publication claim = %v, %v", allowed, err)
	}
	if grants.consumeRequest.RunID != runID || grants.consumeRequest.MaxRunInvocations != 80 {
		t.Fatalf("restored run budget = (%s, %d)", grants.consumeRequest.RunID, grants.consumeRequest.MaxRunInvocations)
	}
	outbound := socialops.Outbound{ID: uuid.New(), Scope: publication.Scope, ScopeKey: strings.Repeat("c", 64), WorkKey: "dm-work", ConfigurationRevision: 1,
		Kind: "dm", ConnectorID: "x-social", ConnectionID: connectionID, OperationID: "x-social.message.send", AccountID: "account-1", ConversationID: "conversation-1",
		BodyArtifactID: uuid.New(), ReviewedDigest: strings.Repeat("d", 64), State: "dispatching", Revision: 3, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := db.Create(&outbound).Error; err != nil {
		t.Fatal(err)
	}
	outInput := map[string]any{"account_id": "account-1", "conversation_id": "conversation-1", "text": "reviewed"}
	request.Request.OperationID, request.Request.LogicalOperationID, request.Request.Input = "x-social.message.send", "outbound-write", outInput
	request.Operation = catalog.Operation{ID: "x-social.message.send", Effect: catalog.OperationEffectExternalSend}
	if allowed, err = authorizer.AuthorizeBackground(ctx, request); err == nil || allowed {
		t.Fatalf("raw outbound write = %v, %v", allowed, err)
	}
	if allowed, err = authorizer.AuthorizeBackground(withSocialOutboundClaim(ctx, outbound, map[string]any{"text": "other"}), request); err == nil || allowed {
		t.Fatalf("wrong outbound input = %v, %v", allowed, err)
	}
	if allowed, err = authorizer.AuthorizeBackground(withSocialOutboundClaim(ctx, outbound, outInput), request); err != nil || !allowed {
		t.Fatalf("valid outbound claim = %v, %v", allowed, err)
	}
	request.Request.ConnectorID, request.Binding.ConnectorID, request.Connection.ConnectorID = "web-search", "web-search", "web-search"
	request.Request.OperationID, request.Request.LogicalOperationID, request.Request.Input = "web.search", "research-read", map[string]any{"query": "current source"}
	request.Operation = catalog.Operation{ID: "web.search"}
	if allowed, err = authorizer.AuthorizeBackground(ctx, request); err == nil || allowed {
		t.Fatal("unconfigured research connection admitted")
	}
	metadata.ResearchConnectorID, metadata.ResearchConnectionID = "web-search", connectionID
	snapshot, _ := json.Marshal(map[string]any{"social_operations": metadata})
	if err = db.Model(&automationmodel.AutomationTask{}).Where("id = ?", task.ID).Update("source_snapshot", datatypes.JSON(snapshot)).Error; err != nil {
		t.Fatal(err)
	}
	if allowed, err = authorizer.AuthorizeBackground(ctx, request); err != nil || !allowed {
		t.Fatal("configured research connection denied", err)
	}
	otherResearch := uuid.New()
	request.Request.ConnectionID, request.Connection.ID, request.Binding.ConnectionID = otherResearch, otherResearch, otherResearch
	if allowed, err = authorizer.AuthorizeBackground(ctx, request); err == nil || allowed {
		t.Fatal("research switched connections")
	}
}

func TestIssueConnectorBackgroundGrantRestoresCurrentPublishedBindingContract(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	organizationID, workspaceID, ownerID, employeeID, versionID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	bindingID, connectionID := uuid.New(), uuid.New()
	target := connectorcore.TargetRef{Type: connectorcore.TargetTypeDigitalHuman, ID: employeeID.String(), Scope: connectorcore.ScopePublished,
		VersionID: versionID.String(), BindingInstanceID: uuid.NewString()}
	operations := []connectorcore.Identifier{"x-social.post.create", "x-social.comment.create"}
	constraint := connectorcore.OwnerConstraint(ownerID, operations)
	constraint.TargetBoundaries = []connectorcore.TargetBoundary{{Type: target.Type, ID: target.ID, Scope: string(target.Scope), VersionID: target.VersionID}}
	binding := connectorcore.TargetBinding{Constraint: constraint, ID: bindingID, OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectionID: connectionID, ConnectorID: "x-social", Target: target, Status: connectorcore.BindingStatusActive, Revision: 4}
	connectionConstraint := connectorcore.OwnerConstraint(ownerID, operations)
	connection := connectorcore.PersonalConnection{Constraint: connectionConstraint, ID: connectionID, OrganizationID: organizationID, OwnerAccountID: ownerID,
		ConnectorID: "x-social", ProviderID: "x-social", DriverID: "mcp", AuthMethodID: "oauth2", CredentialVersion: 2,
		State: connectorcore.ReadinessReady, UsableOperationIDs: operations, UnavailableOperations: map[connectorcore.Identifier]string{}, Revision: 6, LastVerifiedAt: now}
	definition := catalog.SocialMCPDefinitions()[0]
	grants := &backgroundGrantStoreStub{issueGrant: backgroundauth.Grant{ID: uuid.New()}}
	service := &ConnectorBackgroundAuthorization{grants: grants, bindings: backgroundBindingReaderStub{binding}, connections: backgroundConnectionReaderStub{connection},
		employees: backgroundEmployeeReaderStub{&digitalhuman.Detail{Definition: &digitalhuman.Definition{ID: employeeID, OrganizationID: organizationID,
			WorkspaceID: workspaceID, Status: digitalhuman.StatusActive, PublishedVersionID: &versionID}, Published: &digitalhuman.Version{ID: versionID,
			DigitalHumanID: employeeID, OrganizationID: organizationID, WorkspaceID: workspaceID, State: digitalhuman.VersionStatePublished}}},
		catalog: backgroundCatalogStub{definition}, now: func() time.Time { return now }}
	grant, err := service.IssueGrant(t.Context(), organizationID, workspaceID, ownerID, bindingID, []string{string(operations[0])},
		map[string]string{"account_id": "account-1"}, now.Add(time.Hour), 8)
	if err != nil || grant.ID == uuid.Nil {
		t.Fatalf("IssueGrant() = %#v, %v", grant, err)
	}
	digest, err := catalog.DefinitionContractDigest(definition)
	if err != nil {
		t.Fatal(err)
	}
	wantScope := backgroundauth.Scope{OrganizationID: organizationID, OwnerID: ownerID, WorkspaceID: workspaceID, EmployeeID: employeeID,
		VersionID: versionID, BindingID: bindingID, ConnectionID: connectionID, BindingRevision: 4, ConnectionRevision: 6,
		CredentialVersion: 2, ContractDigest: digest}
	if grants.issueRequest.Scope != wantScope || len(grants.issueRequest.Operations) != 1 || grants.issueRequest.Destinations["account_id"] != "account-1" {
		t.Fatalf("issue request = %#v", grants.issueRequest)
	}

	service.employees = backgroundEmployeeReaderStub{&digitalhuman.Detail{Definition: &digitalhuman.Definition{ID: employeeID, OrganizationID: organizationID,
		WorkspaceID: workspaceID, Status: digitalhuman.StatusActive, PublishedVersionID: uuidPointer(uuid.New())}, Published: &digitalhuman.Version{ID: uuid.New()}}}
	if _, err := service.IssueGrant(t.Context(), organizationID, workspaceID, ownerID, bindingID, []string{string(operations[0])}, nil, now.Add(time.Hour), 1); err == nil {
		t.Fatal("stale binding version issued a grant")
	}
}

func TestListAndRevokeConnectorBackgroundGrantsStayInAuthenticatedWorkspace(t *testing.T) {
	db := backgroundAuthorizationTestDB(t)
	if err := db.AutoMigrate(&backgroundauth.Grant{}); err != nil {
		t.Fatal(err)
	}
	organizationID, ownerID, workspaceID := uuid.New(), uuid.New(), uuid.New()
	target := backgroundauth.Grant{ID: uuid.New(), Scope: backgroundauth.Scope{OrganizationID: organizationID, OwnerID: ownerID, WorkspaceID: workspaceID,
		EmployeeID: uuid.New(), VersionID: uuid.New(), BindingID: uuid.New(), ConnectionID: uuid.New(), BindingRevision: 1, ConnectionRevision: 1,
		CredentialVersion: 1, ContractDigest: strings.Repeat("a", 64)}, OperationsJSON: `["x-social.post.create"]`, DestinationsJSON: `{}`,
		ExpiresAt: time.Now().Add(time.Hour), MaxInvocations: 1, CreatedAt: time.Now()}
	foreign := target
	foreign.ID, foreign.WorkspaceID = uuid.New(), uuid.New()
	if err := db.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&foreign).Error; err != nil {
		t.Fatal(err)
	}
	grants := &backgroundGrantStoreStub{}
	service := &ConnectorBackgroundAuthorization{resolver: &BackgroundTaskRunResolver{db: db}, grants: grants}
	listed, err := service.ListGrants(t.Context(), organizationID, workspaceID, ownerID)
	if err != nil || len(listed) != 1 || listed[0].ID != target.ID {
		t.Fatalf("ListGrants() = %#v, %v", listed, err)
	}
	if err := service.RevokeGrant(t.Context(), organizationID, workspaceID, ownerID, foreign.ID); err == nil {
		t.Fatal("grant from another workspace revoked")
	}
	if err := service.RevokeGrant(t.Context(), organizationID, workspaceID, ownerID, target.ID); err != nil || grants.revoked != target.ID {
		t.Fatalf("RevokeGrant() = %v, revoked=%s", err, grants.revoked)
	}
}

func backgroundAuthorizationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&digitalhuman.Run{}, &automationmodel.AutomationTask{}, &socialops.Publication{}, &socialops.Outbound{}, &socialops.Record{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func backgroundTaskEvent(t *testing.T, organizationID, workspaceID, employeeID, taskID, taskRunID, actionID uuid.UUID) invocation.Event {
	t.Helper()
	event, err := invocation.ValidateEvent(invocation.Event{ID: actionID.String(), Type: invocation.EventTypeSchedule,
		Origin: invocation.Origin{Kind: invocation.OriginSchedule}, Target: runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman,
			ID: employeeID, OrganizationID: organizationID, WorkspaceID: workspaceID}, Sender: invocation.PrincipalRef{Type: invocation.PrincipalSystem,
			ID: "digital-human-task-scheduler"}, OccurredAt: time.Now().UTC(), Context: map[string]any{"surface": digitalhuman.SurfaceTask,
			"linux_execution_entry": "task", "task_id": taskID.String(), "task_run_id": taskRunID.String(), "action_run_id": actionID.String(),
			"task_owner_kind": string(automationmodel.AutomationTaskOwnerDigitalHuman)}})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func stringPointer(value string) *string     { return &value }
func uuidPointer(value uuid.UUID) *uuid.UUID { return &value }

func TestBackgroundGrantsForSSHOnlyCoverInspection(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	ownerID := uuid.New()
	target := connectorcore.TargetRef{Type: connectorcore.TargetTypeDigitalHuman, ID: uuid.NewString(), Scope: connectorcore.ScopePublished,
		VersionID: uuid.NewString(), BindingInstanceID: uuid.NewString()}
	definition := catalog.ServerOpsSSHDefinition()
	operations := []connectorcore.Identifier{}
	for _, operation := range definition.Operations {
		operations = append(operations, connectorcore.Identifier(operation.ID))
	}
	constraint := connectorcore.OwnerConstraint(ownerID, operations)
	constraint.TargetBoundaries = []connectorcore.TargetBoundary{{Type: target.Type, ID: target.ID, Scope: string(target.Scope), VersionID: target.VersionID}}
	binding := connectorcore.TargetBinding{Constraint: constraint, Target: target}
	connection := connectorcore.PersonalConnection{Constraint: connectorcore.OwnerConstraint(ownerID, operations), UsableOperationIDs: operations}
	if !backgroundOperationsAllowed([]string{"server.system.snapshot", "server.journal.tail", "server.command.run"}, definition, binding, connection, ownerID, now) {
		t.Fatal("an inspection grant was refused")
	}
	for _, write := range []string{"server.service.restart", "server.service.start", "server.service.stop", "server.service.reload"} {
		if backgroundOperationsAllowed([]string{"server.system.snapshot", write}, definition, binding, connection, ownerID, now) {
			t.Fatalf("a task grant covered %s", write)
		}
	}
}
