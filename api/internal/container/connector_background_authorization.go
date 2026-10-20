package container

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
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
	"gorm.io/gorm"
)

type socialPublicationClaimContextKey struct{}
type socialPublicationClaim struct {
	Kind                  string
	RecordID              uuid.UUID
	Revision              int64
	ConfigurationRevision int64
	ReviewedDigest        string
	InputDigest           string
}

func withSocialPublicationClaim(ctx context.Context, publicationID uuid.UUID, revision int64, input any) context.Context {
	digest, err := backgroundInputDigest(input)
	if err != nil {
		return ctx
	}
	return context.WithValue(ctx, socialPublicationClaimContextKey{}, socialPublicationClaim{Kind: "publication", RecordID: publicationID, Revision: revision, InputDigest: digest})
}

func withSocialOutboundClaim(ctx context.Context, outbound socialops.Outbound, input any) context.Context {
	digest, err := backgroundInputDigest(input)
	if err != nil {
		return ctx
	}
	claim := socialPublicationClaim{Kind: "outbound", RecordID: outbound.ID, Revision: outbound.Revision, ConfigurationRevision: outbound.ConfigurationRevision,
		ReviewedDigest: outbound.ReviewedDigest, InputDigest: digest}
	return context.WithValue(ctx, socialPublicationClaimContextKey{}, claim)
}

func backgroundInputDigest(input any) (string, error) {
	data, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

type backgroundConnectorAccess interface {
	AuthorizeBackgroundConnectorExecution(context.Context, *digitalhuman.Run, invocation.Event) error
}

type backgroundGrantStore interface {
	Issue(context.Context, backgroundauth.IssueRequest) (backgroundauth.Grant, error)
	Consume(context.Context, backgroundauth.ConsumeRequest) (backgroundauth.Receipt, error)
	Revoke(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
	List(context.Context, uuid.UUID, uuid.UUID) ([]backgroundauth.Grant, error)
}

type backgroundEmployeeReader interface {
	Get(context.Context, digitalhuman.Scope, uuid.UUID) (*digitalhuman.Detail, error)
}

type backgroundDefinitionCatalog interface {
	Get(string) (catalog.Definition, bool)
}

// ActiveBackgroundTaskRunRequest identifies one durable scheduled employee run.
// Workspace and owner are deliberately absent: both are restored from server
// records, never from tool arguments.
type ActiveBackgroundTaskRunRequest struct {
	OrganizationID uuid.UUID
	ConversationID uuid.UUID
	MessageID      *uuid.UUID
	EmployeeID     uuid.UUID
	VersionID      uuid.UUID
}

// ActiveBackgroundTaskRun is the server-restored identity shared by connector
// background authorization and social-operations tools.
type ActiveBackgroundTaskRun struct {
	Run                  digitalhuman.Run
	Event                invocation.Event
	WorkspaceID          uuid.UUID
	OwnerID              uuid.UUID
	SocialOperations     bool
	MaxConnectorCalls    int
	ResearchConnectorID  string
	ResearchConnectionID uuid.UUID
}

// BackgroundTaskRunResolver restores and reauthorizes active task runs.
type BackgroundTaskRunResolver struct {
	db     *gorm.DB
	access backgroundConnectorAccess
}

func NewBackgroundTaskRunResolver(db *gorm.DB, access backgroundConnectorAccess) *BackgroundTaskRunResolver {
	return &BackgroundTaskRunResolver{db: db, access: access}
}

func (r *BackgroundTaskRunResolver) Restore(ctx context.Context, request ActiveBackgroundTaskRunRequest) (ActiveBackgroundTaskRun, error) {
	if r == nil || r.db == nil || r.access == nil || request.OrganizationID == uuid.Nil || request.ConversationID == uuid.Nil ||
		request.EmployeeID == uuid.Nil || request.VersionID == uuid.Nil || request.MessageID != nil && *request.MessageID == uuid.Nil {
		return ActiveBackgroundTaskRun{}, backgroundDenied(nil)
	}
	query := r.db.WithContext(ctx).Where(
		"organization_id = ? AND conversation_id = ? AND digital_human_id = ? AND digital_human_version_id = ? AND surface = ? AND status = ? AND cancel_requested_at IS NULL",
		request.OrganizationID, request.ConversationID, request.EmployeeID, request.VersionID, digitalhuman.SurfaceTask, digitalhuman.RunStatusRunning,
	)
	if request.MessageID != nil {
		query = query.Where("message_id = ?", *request.MessageID)
	}
	var runs []digitalhuman.Run
	if err := query.Order("created_at DESC, id DESC").Limit(2).Find(&runs).Error; err != nil {
		return ActiveBackgroundTaskRun{}, backgroundInternal("restore active DigitalHuman task run", err)
	}
	if len(runs) != 1 {
		return ActiveBackgroundTaskRun{}, backgroundDenied(nil)
	}
	run := runs[0]
	var event invocation.Event
	if len(run.InvocationEvent) == 0 || json.Unmarshal(run.InvocationEvent, &event) != nil {
		return ActiveBackgroundTaskRun{}, backgroundDenied(nil)
	}
	event, err := invocation.ValidateEvent(event)
	if err != nil || event.Target.Type != runtimeidentity.SubjectTypeDigitalHuman || event.Target.ID != run.DigitalHumanID ||
		event.Target.OrganizationID != run.OrganizationID || event.Target.WorkspaceID != run.WorkspaceID {
		return ActiveBackgroundTaskRun{}, backgroundDenied(nil)
	}
	taskID, ok := backgroundEventUUID(event.Context, "task_id")
	if !ok {
		return ActiveBackgroundTaskRun{}, backgroundDenied(nil)
	}
	var task automationmodel.AutomationTask
	if err := r.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND workspace_id = ? AND subject_id = ? AND task_kind = ?",
		taskID, run.OrganizationID, run.WorkspaceID, run.DigitalHumanID, automationmodel.AutomationTaskKindDigitalHuman).Take(&task).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ActiveBackgroundTaskRun{}, backgroundDenied(nil)
		}
		return ActiveBackgroundTaskRun{}, backgroundInternal("restore Automation task owner", err)
	}
	ownerID, err := uuid.Parse(strings.TrimSpace(task.CreatedBy))
	if err != nil || ownerID == uuid.Nil || ownerID.String() != task.CreatedBy {
		return ActiveBackgroundTaskRun{}, backgroundDenied(nil)
	}
	if err := r.validateSocialOperationsTaskSnapshot(ctx, task, run, ownerID); err != nil {
		return ActiveBackgroundTaskRun{}, err
	}
	if err := r.access.AuthorizeBackgroundConnectorExecution(ctx, &run, event); err != nil {
		return ActiveBackgroundTaskRun{}, backgroundDenied(err)
	}
	metadata, socialOperations := socialOperationsTaskMetadata(task)
	maxConnectorCalls := 0
	if socialOperations {
		maxConnectorCalls = metadata.Budget.MaxConnectorCalls
	}
	return ActiveBackgroundTaskRun{Run: run, Event: event, WorkspaceID: run.WorkspaceID, OwnerID: ownerID,
		SocialOperations: socialOperations, MaxConnectorCalls: maxConnectorCalls, ResearchConnectorID: metadata.ResearchConnectorID, ResearchConnectionID: metadata.ResearchConnectionID}, nil
}

func (r *BackgroundTaskRunResolver) validateSocialOperationsTaskSnapshot(ctx context.Context, task automationmodel.AutomationTask, run digitalhuman.Run, ownerID uuid.UUID) error {
	metadata, exists := socialOperationsTaskMetadata(task)
	if !exists {
		if _, malformed := task.SourceSnapshot["social_operations"]; malformed {
			return backgroundDenied(nil)
		}
		return nil
	}
	if task.SourceRef == nil || *task.SourceRef != "social_operations" {
		return backgroundDenied(nil)
	}
	validTaskKey := map[string]bool{
		"daily_content": true, "frequent_inbox": true,
		"daily_seven_day_samples": true, "weekly_review": true,
	}[metadata.TaskKey]
	if !validTaskKey || metadata.Scope.OrganizationID != run.OrganizationID || metadata.Scope.WorkspaceID != run.WorkspaceID ||
		metadata.Scope.OwnerID != ownerID || metadata.Scope.EmployeeID != run.DigitalHumanID || metadata.EmployeeVersionID != run.DigitalHumanVersionID ||
		metadata.ConfigurationRevision < 1 || metadata.ConfigurationRecordID == uuid.Nil || metadata.ConfigurationArtifactID == uuid.Nil ||
		metadata.Budget.MaxConnectorCalls < 1 || metadata.Budget.MaxConnectorCalls > 100000 {
		return backgroundDenied(nil)
	}
	var record socialops.Record
	err := r.db.WithContext(ctx).Where(
		"id = ? AND organization_id = ? AND workspace_id = ? AND owner_id = ? AND employee_id = ? AND kind = ? AND record_key = ? AND revision = ? AND artifact_id = ?",
		metadata.ConfigurationRecordID, metadata.Scope.OrganizationID, metadata.Scope.WorkspaceID, metadata.Scope.OwnerID, metadata.Scope.EmployeeID,
		socialops.CurrentConfigurationRecordKind, socialops.CurrentConfigurationRecordKey, metadata.ConfigurationRevision, metadata.ConfigurationArtifactID,
	).Take(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return backgroundDenied(nil)
		}
		return backgroundInternal("restore social operations task configuration", err)
	}
	return nil
}

func socialOperationsTaskMetadata(task automationmodel.AutomationTask) (digitalhuman.SocialOperationsTaskMetadata, bool) {
	raw, exists := task.SourceSnapshot["social_operations"]
	if !exists {
		return digitalhuman.SocialOperationsTaskMetadata{}, false
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return digitalhuman.SocialOperationsTaskMetadata{}, false
	}
	var metadata digitalhuman.SocialOperationsTaskMetadata
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&metadata); err != nil {
		return digitalhuman.SocialOperationsTaskMetadata{}, false
	}
	var trailing any
	if err = decoder.Decode(&trailing); err != io.EOF {
		return digitalhuman.SocialOperationsTaskMetadata{}, false
	}
	return metadata, true
}

func backgroundEventUUID(values map[string]any, key string) (uuid.UUID, bool) {
	raw, ok := values[key].(string)
	raw = strings.TrimSpace(raw)
	id, err := uuid.Parse(raw)
	return id, ok && err == nil && id != uuid.Nil && id.String() == raw
}

// ConnectorBackgroundAuthorization is both the execution hook and the owner
// management service. The hook restores task identity before consuming a grant;
// management restores all grant scope from live server records.
type ConnectorBackgroundAuthorization struct {
	resolver    *BackgroundTaskRunResolver
	grants      backgroundGrantStore
	bindings    connectorexecution.BindingReader
	connections connectorexecution.ConnectionReader
	employees   backgroundEmployeeReader
	catalog     backgroundDefinitionCatalog
	now         func() time.Time
}

// connectorBackgroundAuthorizationProxy keeps container assembly lazy. Some
// constructor tests intentionally have no database; the grant service is only
// required when a published noninteractive employee invocation reaches the
// background authorization hook.
type connectorBackgroundAuthorizationProxy struct{ container *ServiceContainer }

func (p connectorBackgroundAuthorizationProxy) AuthorizeBackground(ctx context.Context, request connectorexecution.BackgroundAuthorizationRequest) (bool, error) {
	if p.container == nil {
		return false, backgroundInternal("authorize Connector background invocation", nil)
	}
	return p.container.GetConnectorBackgroundAuthorization().AuthorizeBackground(ctx, request)
}

func NewConnectorBackgroundAuthorization(
	resolver *BackgroundTaskRunResolver,
	grants backgroundGrantStore,
	bindings connectorexecution.BindingReader,
	connections connectorexecution.ConnectionReader,
	employees backgroundEmployeeReader,
	definitions backgroundDefinitionCatalog,
) (*ConnectorBackgroundAuthorization, error) {
	if resolver == nil || grants == nil || bindings == nil || connections == nil || employees == nil || definitions == nil {
		return nil, backgroundInternal("initialize Connector background authorization", nil)
	}
	return &ConnectorBackgroundAuthorization{resolver: resolver, grants: grants, bindings: bindings, connections: connections,
		employees: employees, catalog: definitions, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (s *ConnectorBackgroundAuthorization) AuthorizeBackground(ctx context.Context, request connectorexecution.BackgroundAuthorizationRequest) (bool, error) {
	if s == nil || s.resolver == nil || s.grants == nil {
		return false, backgroundInternal("authorize Connector background invocation", nil)
	}
	conversationID, conversationErr := uuid.Parse(strings.TrimSpace(request.Request.ConversationID))
	employeeID, employeeErr := uuid.Parse(strings.TrimSpace(request.Request.Target.ID))
	versionID, versionErr := uuid.Parse(strings.TrimSpace(request.Request.Target.VersionID))
	r := request.Request
	if conversationErr != nil || employeeErr != nil || versionErr != nil || conversationID == uuid.Nil || employeeID == uuid.Nil || versionID == uuid.Nil ||
		r.Target.Type != connectorcore.TargetTypeDigitalHuman || r.Target.EffectiveScope() != connectorcore.ScopePublished ||
		r.OrganizationID == uuid.Nil || r.OwnerAccountID == uuid.Nil || r.ActingPrincipalID != r.OwnerAccountID ||
		request.Binding.ID != r.BindingID || request.Connection.ID != r.ConnectionID || request.Binding.ConnectionID != request.Connection.ID ||
		request.Binding.OrganizationID != r.OrganizationID || request.Connection.OrganizationID != r.OrganizationID ||
		request.Binding.OwnerAccountID != request.Connection.OwnerAccountID || request.Binding.OwnerAccountID != r.OwnerAccountID ||
		!request.Binding.Target.SameIdentity(r.Target) || request.Binding.Revision < 1 || request.Connection.Revision < 1 ||
		request.Connection.CredentialVersion < 1 || request.Operation.ID != string(r.OperationID) || strings.TrimSpace(request.ContractDigest) == "" {
		return false, backgroundDenied(nil)
	}
	backgroundIdentity, ok := connectorexecution.BackgroundIdentityFromContext(ctx)
	if !ok {
		return false, backgroundDenied(nil)
	}
	restored, err := s.resolver.Restore(ctx, ActiveBackgroundTaskRunRequest{OrganizationID: r.OrganizationID, ConversationID: conversationID,
		MessageID: &backgroundIdentity.MessageID, EmployeeID: employeeID, VersionID: versionID})
	if err != nil {
		return false, err
	}
	if restored.Run.ID != backgroundIdentity.RunID || restored.Run.MessageID == nil || *restored.Run.MessageID != backgroundIdentity.MessageID {
		return false, backgroundDenied(nil)
	}
	if restored.OwnerID != request.Connection.OwnerAccountID {
		return false, backgroundDenied(nil)
	}
	input, _ := r.Input.(map[string]any)
	if restored.SocialOperations && (r.OperationID == "web.search" || r.OperationID == "web.fetch" || strings.HasSuffix(r.OperationID.String(), ".web.search") || strings.HasSuffix(r.OperationID.String(), ".web.fetch")) &&
		(restored.ResearchConnectorID != r.ConnectorID.String() || restored.ResearchConnectionID != request.Connection.ID) {
		return false, backgroundDenied(nil)
	}
	if socialBackgroundWrite(r.ConnectorID, r.OperationID) {
		if err := s.authorizeSocialPublicationClaim(ctx, restored, request, input); err != nil {
			return false, err
		}
	} else if request.Operation.Effect == catalog.OperationEffectExternalSend && restored.SocialOperations {
		if err := s.authorizeSocialOutboundClaim(ctx, restored, request, input); err != nil {
			return false, err
		}
	}
	consume := backgroundauth.ConsumeRequest{Scope: backgroundauth.Scope{
		OrganizationID: r.OrganizationID, OwnerID: restored.OwnerID, WorkspaceID: restored.WorkspaceID,
		EmployeeID: employeeID, VersionID: versionID, BindingID: request.Binding.ID, ConnectionID: request.Connection.ID,
		BindingRevision: request.Binding.Revision, ConnectionRevision: request.Connection.Revision,
		CredentialVersion: request.Connection.CredentialVersion, ContractDigest: request.ContractDigest,
	}, OperationID: string(r.OperationID), LogicalOperationID: r.LogicalOperationID, Input: input}
	if restored.SocialOperations {
		consume.RunID, consume.MaxRunInvocations = restored.Run.ID, restored.MaxConnectorCalls
	}
	_, err = s.grants.Consume(ctx, consume)
	if err == nil {
		return true, nil
	}
	if connectorcore.IsErrorCode(err, connectorcore.ErrorCodeAccessDenied) {
		return false, nil
	}
	return false, err
}

func socialBackgroundWrite(connector connectorcore.Identifier, operation connectorcore.Identifier) bool {
	if connector != "x-social" && connector != "linkedin-social" && connector != "facebook-social" {
		return false
	}
	for _, suffix := range []string{".post.create", ".comment.create", ".post.quote", ".post.repost"} {
		if string(operation) == string(connector)+suffix {
			return true
		}
	}
	return false
}

func (s *ConnectorBackgroundAuthorization) authorizeSocialPublicationClaim(ctx context.Context, restored ActiveBackgroundTaskRun, request connectorexecution.BackgroundAuthorizationRequest, input map[string]any) error {
	claim, ok := ctx.Value(socialPublicationClaimContextKey{}).(socialPublicationClaim)
	if !ok || claim.Kind != "publication" || claim.RecordID == uuid.Nil || claim.Revision < 1 || s.resolver == nil || s.resolver.db == nil {
		return backgroundDenied(nil)
	}
	digest, err := backgroundInputDigest(input)
	if err != nil || digest != claim.InputDigest {
		return backgroundDenied(err)
	}
	var publication socialops.Publication
	err = s.resolver.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND workspace_id = ? AND owner_id = ? AND employee_id = ?", claim.RecordID,
		restored.Run.OrganizationID, restored.WorkspaceID, restored.OwnerID, restored.Run.DigitalHumanID).Take(&publication).Error
	if err != nil || publication.State != "dispatching" || publication.Revision != claim.Revision || publication.ConnectionID != request.Connection.ID ||
		connectorForPlatform(publication.Platform) != request.Request.ConnectorID {
		return backgroundDenied(err)
	}
	return nil
}

func (s *ConnectorBackgroundAuthorization) authorizeSocialOutboundClaim(ctx context.Context, restored ActiveBackgroundTaskRun, request connectorexecution.BackgroundAuthorizationRequest, input map[string]any) error {
	claim, ok := ctx.Value(socialPublicationClaimContextKey{}).(socialPublicationClaim)
	if !ok || claim.Kind != "outbound" || claim.RecordID == uuid.Nil || claim.Revision < 1 || claim.ConfigurationRevision < 1 || claim.ReviewedDigest == "" || s.resolver == nil || s.resolver.db == nil {
		return backgroundDenied(nil)
	}
	digest, err := backgroundInputDigest(input)
	if err != nil || digest != claim.InputDigest {
		return backgroundDenied(err)
	}
	var outbound socialops.Outbound
	err = s.resolver.db.WithContext(ctx).Where("id = ? AND organization_id = ? AND workspace_id = ? AND owner_id = ? AND employee_id = ?", claim.RecordID,
		restored.Run.OrganizationID, restored.WorkspaceID, restored.OwnerID, restored.Run.DigitalHumanID).Take(&outbound).Error
	if err != nil || outbound.State != "dispatching" || outbound.Revision != claim.Revision || outbound.ConfigurationRevision != claim.ConfigurationRevision ||
		outbound.ReviewedDigest != claim.ReviewedDigest || outbound.ConnectionID != request.Connection.ID || outbound.ConnectorID != request.Request.ConnectorID.String() ||
		outbound.OperationID != request.Request.OperationID.String() {
		return backgroundDenied(err)
	}
	return nil
}

func (s *ConnectorBackgroundAuthorization) IssueGrant(
	ctx context.Context,
	organizationID, workspaceID, ownerID, bindingID uuid.UUID,
	operations []string,
	destinations map[string]string,
	expiresAt time.Time,
	maxInvocations int,
) (backgroundauth.Grant, error) {
	if s == nil || s.grants == nil || s.bindings == nil || s.connections == nil || s.employees == nil || s.catalog == nil ||
		organizationID == uuid.Nil || workspaceID == uuid.Nil || ownerID == uuid.Nil || bindingID == uuid.Nil {
		return backgroundauth.Grant{}, backgroundDenied(nil)
	}
	binding, err := s.bindings.GetBinding(ctx, organizationID, ownerID, bindingID)
	if err != nil {
		return backgroundauth.Grant{}, backgroundDenied(err)
	}
	if binding.ID != bindingID || binding.OrganizationID != organizationID || binding.OwnerAccountID != ownerID ||
		binding.Status != connectorcore.BindingStatusActive || binding.Target.Type != connectorcore.TargetTypeDigitalHuman ||
		binding.Target.EffectiveScope() != connectorcore.ScopePublished || binding.Revision < 1 {
		return backgroundauth.Grant{}, backgroundDenied(nil)
	}
	employeeID, employeeErr := uuid.Parse(binding.Target.ID)
	versionID, versionErr := uuid.Parse(binding.Target.VersionID)
	if employeeErr != nil || versionErr != nil || employeeID == uuid.Nil || versionID == uuid.Nil {
		return backgroundauth.Grant{}, backgroundDenied(nil)
	}
	detail, err := s.employees.Get(ctx, digitalhuman.Scope{OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: ownerID}, employeeID)
	if err != nil || detail == nil || detail.Definition == nil || detail.Published == nil || detail.Definition.Status != digitalhuman.StatusActive ||
		detail.Definition.OrganizationID != organizationID || detail.Definition.WorkspaceID != workspaceID || detail.Definition.PublishedVersionID == nil ||
		*detail.Definition.PublishedVersionID != versionID || detail.Published.ID != versionID || detail.Published.DigitalHumanID != employeeID ||
		detail.Published.OrganizationID != organizationID || detail.Published.WorkspaceID != workspaceID || detail.Published.State != digitalhuman.VersionStatePublished {
		return backgroundauth.Grant{}, backgroundDenied(err)
	}
	connection, err := s.connections.GetConnection(ctx, organizationID, ownerID, binding.ConnectionID)
	if err != nil {
		return backgroundauth.Grant{}, backgroundDenied(err)
	}
	if connection.ID != binding.ConnectionID || connection.OrganizationID != organizationID || connection.OwnerAccountID != ownerID ||
		connection.ConnectorID != binding.ConnectorID || connection.State != connectorcore.ReadinessReady || connection.Revision < 1 || connection.CredentialVersion < 1 {
		return backgroundauth.Grant{}, backgroundDenied(nil)
	}
	definition, ok := s.catalog.Get(string(binding.ConnectorID))
	if !ok || definition.ID != string(binding.ConnectorID) {
		return backgroundauth.Grant{}, connectorcore.NewError(connectorcore.ErrorCodeConnectorUnavailable, false, nil)
	}
	contractDigest, err := catalog.DefinitionContractDigest(definition)
	if err != nil {
		return backgroundauth.Grant{}, backgroundInternal("freeze Connector background contract", err)
	}
	if !backgroundOperationsAllowed(operations, definition, binding, connection, ownerID, s.currentTime()) {
		return backgroundauth.Grant{}, backgroundDenied(nil)
	}
	return s.grants.Issue(ctx, backgroundauth.IssueRequest{Scope: backgroundauth.Scope{
		OrganizationID: organizationID, OwnerID: ownerID, WorkspaceID: workspaceID, EmployeeID: employeeID, VersionID: versionID,
		BindingID: binding.ID, ConnectionID: connection.ID, BindingRevision: binding.Revision, ConnectionRevision: connection.Revision,
		CredentialVersion: connection.CredentialVersion, ContractDigest: contractDigest,
	}, Operations: append([]string{}, operations...), Destinations: cloneBackgroundDestinations(destinations), ExpiresAt: expiresAt, MaxInvocations: maxInvocations})
}

func backgroundOperationsAllowed(operations []string, definition catalog.Definition, binding connectorcore.TargetBinding, connection connectorcore.PersonalConnection, owner uuid.UUID, now time.Time) bool {
	if len(operations) == 0 {
		return false
	}
	defined := make(map[string]bool, len(definition.Operations))
	for _, operation := range definition.Operations {
		defined[operation.ID] = true
	}
	usable := make(map[string]bool, len(connection.UsableOperationIDs))
	for _, operation := range connection.UsableOperationIDs {
		usable[string(operation)] = true
	}
	seen := make(map[string]bool, len(operations))
	for _, operation := range operations {
		id := connectorcore.Identifier(operation)
		if !id.Valid() || seen[operation] || !defined[operation] || !usable[operation] ||
			!connectorcore.ConstraintsAllow(connection.Constraint, binding.Constraint, owner, id, binding.Target, now) {
			return false
		}
		seen[operation] = true
	}
	return true
}

func cloneBackgroundDestinations(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func (s *ConnectorBackgroundAuthorization) ListGrants(ctx context.Context, organizationID, workspaceID, ownerID uuid.UUID) ([]backgroundauth.Grant, error) {
	if s == nil || s.grants == nil || organizationID == uuid.Nil || workspaceID == uuid.Nil || ownerID == uuid.Nil {
		return nil, backgroundDenied(nil)
	}
	if s.resolver != nil && s.resolver.db != nil {
		var grants []backgroundauth.Grant
		if err := s.resolver.db.WithContext(ctx).Where("organization_id = ? AND workspace_id = ? AND owner_id = ?", organizationID, workspaceID, ownerID).
			Order("created_at DESC, id DESC").Limit(100).Find(&grants).Error; err != nil {
			return nil, backgroundInternal("list Connector background grants", err)
		}
		return grants, nil
	}
	grants, err := s.grants.List(ctx, organizationID, ownerID)
	if err != nil {
		return nil, err
	}
	filtered := make([]backgroundauth.Grant, 0, len(grants))
	for _, grant := range grants {
		if grant.WorkspaceID == workspaceID {
			filtered = append(filtered, grant)
		}
	}
	return filtered, nil
}

func (s *ConnectorBackgroundAuthorization) RevokeGrant(ctx context.Context, organizationID, workspaceID, ownerID, grantID uuid.UUID) error {
	if s == nil || s.grants == nil || organizationID == uuid.Nil || workspaceID == uuid.Nil || ownerID == uuid.Nil || grantID == uuid.Nil {
		return backgroundDenied(nil)
	}
	if s.resolver != nil && s.resolver.db != nil {
		var count int64
		if err := s.resolver.db.WithContext(ctx).Model(&backgroundauth.Grant{}).Where(
			"id = ? AND organization_id = ? AND workspace_id = ? AND owner_id = ?", grantID, organizationID, workspaceID, ownerID,
		).Count(&count).Error; err != nil {
			return backgroundInternal("load Connector background grant for revocation", err)
		}
		if count != 1 {
			return backgroundDenied(nil)
		}
		return s.grants.Revoke(ctx, organizationID, ownerID, grantID)
	}
	grants, err := s.ListGrants(ctx, organizationID, workspaceID, ownerID)
	if err != nil {
		return err
	}
	for _, grant := range grants {
		if grant.ID == grantID {
			return s.grants.Revoke(ctx, organizationID, ownerID, grantID)
		}
	}
	return backgroundDenied(nil)
}

func (s *ConnectorBackgroundAuthorization) currentTime() time.Time {
	if s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

func backgroundDenied(cause error) error {
	return connectorcore.NewError(connectorcore.ErrorCodeAccessDenied, false, cause)
}

func backgroundInternal(operation string, cause error) error {
	if cause == nil {
		cause = errors.New(operation)
	}
	return connectorcore.NewError(connectorcore.ErrorCodeInternal, false, fmt.Errorf("%s: %w", operation, cause))
}

func (c *ServiceContainer) GetConnectorBackgroundGrantService() *backgroundauth.Service {
	if c.connectorBackgroundGrantService == nil {
		service, err := backgroundauth.New(c.db, nil)
		if err != nil {
			panic(fmt.Sprintf("initialize Connector background grant service: %v", err))
		}
		c.connectorBackgroundGrantService = service
	}
	return c.connectorBackgroundGrantService
}

func (c *ServiceContainer) GetBackgroundTaskRunResolver() *BackgroundTaskRunResolver {
	if c.backgroundTaskRunResolver == nil {
		c.backgroundTaskRunResolver = NewBackgroundTaskRunResolver(c.db, digitalhuman.NewAccessService(c.db))
	}
	return c.backgroundTaskRunResolver
}

func (c *ServiceContainer) GetConnectorBackgroundAuthorization() *ConnectorBackgroundAuthorization {
	if c.connectorBackgroundAuthorization == nil {
		service, err := NewConnectorBackgroundAuthorization(
			c.GetBackgroundTaskRunResolver(), c.GetConnectorBackgroundGrantService(), c.GetConnectorBindingRepository(),
			c.GetConnectorConnectionRepository(), c.GetDigitalHumanService(), c.GetConnectorPluginManager(),
		)
		if err != nil {
			panic(fmt.Sprintf("initialize Connector background authorization: %v", err))
		}
		c.connectorBackgroundAuthorization = service
	}
	return c.connectorBackgroundAuthorization
}
