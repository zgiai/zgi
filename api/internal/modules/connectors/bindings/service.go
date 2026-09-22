package bindings

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

const (
	bindingIdempotencyKeyMinBytes = 16
	bindingIdempotencyKeyMaxBytes = 200
)

var nativeTargetBindingNamespace = uuid.MustParse("742481c0-34f6-5c0d-ae6a-5bed0bb8fb5b")

// TargetIdentity identifies the editable host resource. A Workflow binding
// instance identifies a placement inside this resource and deliberately does
// not participate in the host authorization lookup.
type TargetIdentity struct {
	Type      core.TargetType
	ID        string
	Scope     core.TargetScope
	VersionID string
}

func (target TargetIdentity) Validate() error {
	ref := core.TargetRef{Type: target.Type, ID: target.ID, Scope: target.Scope, VersionID: target.VersionID}
	if target.Type == core.TargetTypeWorkflow {
		ref.BindingInstanceID = "authority"
	}
	if ref.Validate() != nil || !validExactText(target.ID) {
		return core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	return nil
}

// TargetAuthority is the only host-specific dependency of the Connector
// binding lifecycle. Implementations must check that actingPrincipalID is
// currently allowed to edit the concrete target inside organizationID. They
// must not infer Connection ownership or mutate legacy bindings.
type TargetReadAuthority interface {
	RequireTargetRead(context.Context, uuid.UUID, uuid.UUID, TargetIdentity) error
}

type TargetAuthority interface {
	RequireTargetEdit(context.Context, uuid.UUID, uuid.UUID, TargetIdentity) error
}

// ConnectionReader resolves only an organization-and-owner-scoped Personal
// Connection. The Connector connection service and repository both satisfy
// this narrow contract; the binding lifecycle never asks for credentials.
type ConnectionReader interface {
	GetConnection(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (core.PersonalConnection, error)
}

// DefinitionCatalog supplies the server-owned contract digest captured when
// a binding explicitly opts into direct execution.
type DefinitionCatalog interface {
	Get(string) (catalog.Definition, bool)
}

type UpsertBindingCommand struct {
	OrganizationID    uuid.UUID
	ActingPrincipalID uuid.UUID
	ConnectionID      uuid.UUID
	IdempotencyKey    string `json:"-"`
	Target            core.TargetRef
	ExpectedRevision  int
	AuthorizationMode string
}

type UpsertBindingResult struct {
	Binding core.TargetBinding
	Created bool
}

// DetachBindingCommand disables one Connector-native Target Binding. The row
// and its stable UUID are retained as non-secret lifecycle evidence and so a
// later re-attach can reuse the same logical identity.
type DetachBindingCommand struct {
	OrganizationID    uuid.UUID
	ActingPrincipalID uuid.UUID
	BindingID         uuid.UUID
	ExpectedRevision  int
}

// UpdateBindingAuthorizationCommand changes only the durable authorization
// mode of an active owner-scoped binding. It is a CAS mutation so a stale
// picker cannot silently grant or revoke direct execution.
type UpdateBindingAuthorizationCommand struct {
	OrganizationID    uuid.UUID
	ActingPrincipalID uuid.UUID
	BindingID         uuid.UUID
	ExpectedRevision  int
	AuthorizationMode string
}

// RemovalImpact is the Connector-native binding impact for a Personal
// Connection. Legacy read-only projections remain available through target
// listing, but they never block or participate in the native removal lifecycle.
type RemovalImpact struct {
	ConnectionID         uuid.UUID
	CanRemove            bool
	BlockingBindingCount int
	AffectedBindings     []core.TargetBinding
}

type Service struct {
	repository  Repository
	connections ConnectionReader
	targets     TargetAuthority
	catalog     DefinitionCatalog
	now         func() time.Time
}

func NewService(repository Repository, connections ConnectionReader, targets TargetAuthority, catalogs ...DefinitionCatalog) (*Service, error) {
	if repository == nil || connections == nil || targets == nil {
		return nil, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	var definitionCatalog DefinitionCatalog
	if len(catalogs) > 0 {
		definitionCatalog = catalogs[0]
	}
	return &Service{
		repository:  repository,
		connections: connections,
		targets:     targets,
		catalog:     definitionCatalog,
		now:         func() time.Time { return time.Now().UTC() },
	}, nil
}

// UpsertBinding attaches a ready Personal Connection owned by the Acting
// Principal. The logical binding key is organization + owner + target +
// Connector + placement; replay returns the first stored UUID, and replacing
// the Connection under an expected revision keeps that UUID.
func (service *Service) UpsertBinding(ctx context.Context, command UpsertBindingCommand) (UpsertBindingResult, error) {
	if err := service.available(); err != nil {
		return UpsertBindingResult{}, err
	}
	if err := validateUpsertCommand(command); err != nil {
		return UpsertBindingResult{}, err
	}
	if command.Target.Type != core.TargetTypeChat {
		return UpsertBindingResult{}, core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	connection, err := service.ownedConnection(
		ctx, command.OrganizationID, command.ActingPrincipalID, command.ConnectionID,
	)
	if err != nil {
		return UpsertBindingResult{}, err
	}
	if connection.State != core.ReadinessReady {
		return UpsertBindingResult{}, core.NewError(core.ErrorCodeConnectionNotReady, false, nil)
	}
	targetIdentity := TargetIdentity{Type: command.Target.Type, ID: command.Target.ID, Scope: command.Target.Scope, VersionID: command.Target.VersionID}
	if err := service.requireTargetEdit(ctx, command.OrganizationID, command.ActingPrincipalID, targetIdentity); err != nil {
		return UpsertBindingResult{}, err
	}

	now := service.now().UTC()
	revision := 1
	if command.ExpectedRevision > 0 {
		revision = command.ExpectedRevision
	}
	binding := core.TargetBinding{
		Constraint:     core.OwnerConstraint(command.ActingPrincipalID, connection.UsableOperationIDs),
		ID:             stableNativeBindingID(command.OrganizationID, command.ActingPrincipalID, connection.ConnectorID, command.Target),
		OrganizationID: command.OrganizationID,
		OwnerAccountID: command.ActingPrincipalID,
		ConnectionID:   connection.ID,
		ConnectorID:    connection.ConnectorID,
		Target:         command.Target,
		Status:         core.BindingStatusActive,

		Revision:  revision,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if command.AuthorizationMode != "" {
		binding.Constraint.AuthorizationMode = command.AuthorizationMode
		if command.AuthorizationMode == core.AuthorizationModeDirect {
			binding.Constraint.AuthorizationConnectionRevision = connection.Revision
			binding.Constraint.AuthorizationCredentialVersion = connection.CredentialVersion
			contractDigest, digestErr := service.authorizationContractDigest(connection.ConnectorID)
			if digestErr != nil {
				return UpsertBindingResult{}, digestErr
			}
			binding.Constraint.AuthorizationContractDigest = contractDigest
		}
	}
	mutationRepository, ok := service.repository.(LifecycleMutationRepository)
	if !ok {
		return UpsertBindingResult{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	stored, created, err := mutationRepository.UpsertBindingWithLifecycle(
		ctx, command.OrganizationID, command.ActingPrincipalID, binding, command.ExpectedRevision,
		BindingLifecycleMutation{
			ID:            uuid.New(),
			MutationType:  MutationBindingUpserted,
			CorrelationID: bindingUpsertCorrelation(command.IdempotencyKey),
			CreatedAt:     now,
		},
	)
	if err != nil {
		return UpsertBindingResult{}, safeBindingDependencyError(err)
	}
	if err := validateStoredNativeBinding(command, connection, stored); err != nil {
		return UpsertBindingResult{}, err
	}
	return UpsertBindingResult{Binding: stored, Created: created}, nil
}

// ListTargetBindings returns native canonical bindings for
// an editable target. Owner scope is retained even when the target itself is
// shared with other accounts.
func (service *Service) ListTargetBindings(
	ctx context.Context,
	organizationID, actingPrincipalID uuid.UUID,
	target TargetIdentity,
) ([]core.TargetBinding, error) {
	if err := service.available(); err != nil {
		return nil, err
	}
	if !validOwnerScope(organizationID, actingPrincipalID) {
		return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	if err := target.Validate(); err != nil {
		return nil, err
	}
	if target.Scope == core.ScopePublished {
		reader, ok := service.targets.(TargetReadAuthority)
		if !ok {
			return nil, core.NewError(core.ErrorCodeAccessDenied, false, nil)
		}
		if err := reader.RequireTargetRead(ctx, organizationID, actingPrincipalID, target); err != nil {
			return nil, err
		}
	} else if err := service.requireTargetEdit(ctx, organizationID, actingPrincipalID, target); err != nil {
		return nil, err
	}
	items, err := service.repository.ListBindingsByTarget(
		ctx, organizationID, actingPrincipalID, target.Type, target.ID, ListOptions{Scope: (core.TargetRef{Type: target.Type, Scope: target.Scope}).EffectiveScope(), VersionID: target.VersionID},
	)
	if err != nil {
		return nil, safeBindingDependencyError(err)
	}
	if err := validateListedTargetBindings(organizationID, actingPrincipalID, target, items); err != nil {
		return nil, err
	}
	return items, nil
}

// DetachBinding removes a native Connection from an editable target without
// deleting the binding identity. Runtime readers admit active rows only, so a
// successful detach is effective before the next provider submission. A
// replay carrying the immediately preceding revision is idempotent once the
// same row is already disabled.
func (service *Service) DetachBinding(ctx context.Context, command DetachBindingCommand) (core.TargetBinding, error) {
	if err := service.available(); err != nil {
		return core.TargetBinding{}, err
	}
	if !validOwnerScope(command.OrganizationID, command.ActingPrincipalID) ||
		command.BindingID == uuid.Nil || command.ExpectedRevision < 1 {
		return core.TargetBinding{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	binding, err := service.repository.GetBinding(
		ctx, command.OrganizationID, command.ActingPrincipalID, command.BindingID,
	)
	if err != nil {
		return core.TargetBinding{}, safeBindingDependencyError(err)
	}
	if binding.OrganizationID != command.OrganizationID || binding.OwnerAccountID != command.ActingPrincipalID ||
		binding.ID != command.BindingID {
		return core.TargetBinding{}, core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	if binding.Target.Type != core.TargetTypeChat {
		return core.TargetBinding{}, core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	target := TargetIdentity{Type: binding.Target.Type, ID: binding.Target.ID, Scope: binding.Target.Scope, VersionID: binding.Target.VersionID}
	if err := service.requireTargetEdit(ctx, command.OrganizationID, command.ActingPrincipalID, target); err != nil {
		return core.TargetBinding{}, err
	}
	if binding.Status == core.BindingStatusDisabled {
		if command.ExpectedRevision == binding.Revision || command.ExpectedRevision == binding.Revision-1 {
			return binding, nil
		}
		return core.TargetBinding{}, core.NewError(core.ErrorCodeConflict, false, nil)
	}
	if binding.Revision != command.ExpectedRevision {
		return core.TargetBinding{}, core.NewError(core.ErrorCodeConflict, false, nil)
	}
	mutationRepository, ok := service.repository.(LifecycleMutationRepository)
	if !ok {
		return core.TargetBinding{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	detached, err := mutationRepository.SetBindingStatusWithLifecycle(
		ctx,
		command.OrganizationID,
		command.ActingPrincipalID,
		command.BindingID,
		core.BindingStatusDisabled,
		command.ExpectedRevision,
		BindingLifecycleMutation{
			ID:            uuid.New(),
			MutationType:  MutationBindingDetached,
			CorrelationID: bindingDetachCorrelation(command.BindingID, command.ExpectedRevision),
			CreatedAt:     service.now().UTC(),
		},
	)
	if err != nil {
		return core.TargetBinding{}, safeBindingDependencyError(err)
	}
	if detached.ID != binding.ID || detached.OrganizationID != binding.OrganizationID ||
		detached.OwnerAccountID != binding.OwnerAccountID || detached.ConnectionID != binding.ConnectionID ||
		detached.ConnectorID != binding.ConnectorID || !detached.Target.SameIdentity(binding.Target) ||
		detached.Status != core.BindingStatusDisabled ||
		detached.Revision != binding.Revision+1 {
		return core.TargetBinding{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	return detached, nil
}

// UpdateBindingAuthorization persists the explicit ask_each_time/direct
// choice without replacing the connection or target identity.
func (service *Service) UpdateBindingAuthorization(ctx context.Context, command UpdateBindingAuthorizationCommand) (core.TargetBinding, error) {
	if err := service.available(); err != nil {
		return core.TargetBinding{}, err
	}
	if !validOwnerScope(command.OrganizationID, command.ActingPrincipalID) ||
		command.BindingID == uuid.Nil || command.ExpectedRevision < 1 ||
		(command.AuthorizationMode != core.AuthorizationModeAsk && command.AuthorizationMode != core.AuthorizationModeDirect) {
		return core.TargetBinding{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	binding, err := service.repository.GetBinding(ctx, command.OrganizationID, command.ActingPrincipalID, command.BindingID)
	if err != nil {
		return core.TargetBinding{}, safeBindingDependencyError(err)
	}
	if binding.OrganizationID != command.OrganizationID || binding.OwnerAccountID != command.ActingPrincipalID ||
		binding.ID != command.BindingID || binding.Target.Type != core.TargetTypeChat {
		return core.TargetBinding{}, core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	target := TargetIdentity{Type: binding.Target.Type, ID: binding.Target.ID, Scope: binding.Target.Scope, VersionID: binding.Target.VersionID}
	if err := service.requireTargetEdit(ctx, command.OrganizationID, command.ActingPrincipalID, target); err != nil {
		return core.TargetBinding{}, err
	}
	if binding.Status != core.BindingStatusActive || binding.Revision != command.ExpectedRevision {
		return core.TargetBinding{}, core.NewError(core.ErrorCodeConflict, false, nil)
	}
	if binding.Constraint == nil || binding.Constraint.Validate() != nil {
		return core.TargetBinding{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	constraint := *binding.Constraint
	constraint.AuthorizationMode = command.AuthorizationMode
	constraint.AuthorizationConnectionRevision = 0
	constraint.AuthorizationCredentialVersion = 0
	constraint.AuthorizationContractDigest = ""
	if command.AuthorizationMode == core.AuthorizationModeDirect {
		connection, connectionErr := service.ownedConnection(ctx, command.OrganizationID, command.ActingPrincipalID, binding.ConnectionID)
		if connectionErr != nil {
			return core.TargetBinding{}, connectionErr
		}
		if connection.State != core.ReadinessReady {
			return core.TargetBinding{}, core.NewError(core.ErrorCodeConnectionNotReady, false, nil)
		}
		constraint.AuthorizationConnectionRevision = connection.Revision
		constraint.AuthorizationCredentialVersion = connection.CredentialVersion
		contractDigest, digestErr := service.authorizationContractDigest(connection.ConnectorID)
		if digestErr != nil {
			return core.TargetBinding{}, digestErr
		}
		constraint.AuthorizationContractDigest = contractDigest
	}
	mutationRepository, ok := service.repository.(AuthorizationMutationRepository)
	if !ok {
		return core.TargetBinding{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	updated, err := mutationRepository.UpdateBindingAuthorizationWithLifecycle(
		ctx, command.OrganizationID, command.ActingPrincipalID, command.BindingID, &constraint, command.ExpectedRevision,
		BindingLifecycleMutation{
			ID: uuid.New(), MutationType: MutationBindingUpserted,
			CorrelationID: bindingAuthorizationCorrelation(command.BindingID, command.ExpectedRevision, command.AuthorizationMode),
			CreatedAt:     service.now().UTC(),
		},
	)
	if err != nil {
		return core.TargetBinding{}, safeBindingDependencyError(err)
	}
	if updated.ID != binding.ID || updated.Revision != binding.Revision+1 || updated.Constraint == nil ||
		updated.Constraint.AuthorizationModeValue() != command.AuthorizationMode {
		return core.TargetBinding{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	return updated, nil
}

// GetRemovalImpact does not require current edit authority on every referenced
// target: an owner must still see stale or published
// references before removal. It never mutates a binding.
func (service *Service) GetRemovalImpact(
	ctx context.Context,
	organizationID, actingPrincipalID, connectionID uuid.UUID,
) (RemovalImpact, error) {
	if err := service.available(); err != nil {
		return RemovalImpact{}, err
	}
	if !validOwnerScope(organizationID, actingPrincipalID) || connectionID == uuid.Nil {
		return RemovalImpact{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	if _, err := service.ownedConnection(ctx, organizationID, actingPrincipalID, connectionID); err != nil {
		return RemovalImpact{}, err
	}
	items, err := service.repository.ListBindingsByConnection(
		ctx, organizationID, actingPrincipalID, connectionID,
		ListOptions{},
	)
	if err != nil {
		return RemovalImpact{}, safeBindingDependencyError(err)
	}
	blocking := 0
	for _, item := range items {
		if item.OrganizationID != organizationID || item.OwnerAccountID != actingPrincipalID || item.ConnectionID != connectionID {
			return RemovalImpact{}, core.NewError(core.ErrorCodeInternal, false, nil)
		}
		if err := item.Validate(); err != nil {
			return RemovalImpact{}, core.NewError(core.ErrorCodeInternal, false, nil)
		}
		if item.Status == core.BindingStatusActive {
			blocking++
		}
	}
	return RemovalImpact{
		ConnectionID: connectionID, CanRemove: blocking == 0,
		BlockingBindingCount: blocking, AffectedBindings: items,
	}, nil
}

func (service *Service) ownedConnection(
	ctx context.Context,
	organizationID, actingPrincipalID, connectionID uuid.UUID,
) (core.PersonalConnection, error) {
	connection, err := service.connections.GetConnection(ctx, organizationID, actingPrincipalID, connectionID)
	if err != nil {
		return core.PersonalConnection{}, safeBindingDependencyError(err)
	}
	if connection.OrganizationID != organizationID || connection.OwnerAccountID != actingPrincipalID {
		return core.PersonalConnection{}, core.NewError(core.ErrorCodeAccessDenied, false, nil)
	}
	if connection.ID != connectionID || !connection.ConnectorID.Valid() {
		return core.PersonalConnection{}, core.NewError(core.ErrorCodeInternal, false, nil)
	}
	return connection, nil
}

func (service *Service) authorizationContractDigest(connectorID core.Identifier) (string, error) {
	if service.catalog == nil {
		// Isolated callers that do not inject the catalog retain the legacy
		// compatibility path; production wiring always supplies it.
		return "", nil
	}
	definition, ok := service.catalog.Get(connectorID.String())
	if !ok {
		return "", core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
	}
	digest, err := catalog.DefinitionContractDigest(definition)
	if err != nil || digest == "" {
		return "", core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
	}
	return digest, nil
}

func (service *Service) requireTargetEdit(
	ctx context.Context,
	organizationID, actingPrincipalID uuid.UUID,
	target TargetIdentity,
) error {
	if err := service.targets.RequireTargetEdit(ctx, organizationID, actingPrincipalID, target); err != nil {
		return safeBindingDependencyError(err)
	}
	return nil
}

func (service *Service) available() error {
	if service == nil || service.repository == nil || service.connections == nil || service.targets == nil || service.now == nil {
		return core.NewError(core.ErrorCodeInternal, false, nil)
	}
	return nil
}

func validateUpsertCommand(command UpsertBindingCommand) error {
	if !validOwnerScope(command.OrganizationID, command.ActingPrincipalID) || command.ConnectionID == uuid.Nil ||
		len(command.IdempotencyKey) < bindingIdempotencyKeyMinBytes || len(command.IdempotencyKey) > bindingIdempotencyKeyMaxBytes ||
		command.ExpectedRevision < 0 || !validExactTarget(command.Target) ||
		(command.AuthorizationMode != "" && command.AuthorizationMode != core.AuthorizationModeAsk && command.AuthorizationMode != core.AuthorizationModeDirect) {
		return core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	return nil
}

func validateStoredNativeBinding(
	command UpsertBindingCommand,
	connection core.PersonalConnection,
	binding core.TargetBinding,
) error {
	if err := binding.Validate(); err != nil ||
		binding.OrganizationID != command.OrganizationID || binding.OwnerAccountID != command.ActingPrincipalID ||
		binding.ConnectionID != connection.ID || binding.ConnectorID != connection.ConnectorID ||
		!binding.Target.SameIdentity(command.Target) || binding.Status != core.BindingStatusActive {
		return core.NewError(core.ErrorCodeInternal, false, nil)
	}
	return nil
}

func validateListedTargetBindings(
	organizationID, actingPrincipalID uuid.UUID,
	target TargetIdentity,
	items []core.TargetBinding,
) error {
	for _, item := range items {
		if item.OrganizationID != organizationID || item.OwnerAccountID != actingPrincipalID ||
			item.Target.Type != target.Type || item.Target.ID != target.ID {
			return core.NewError(core.ErrorCodeInternal, false, nil)
		}
		if err := item.Validate(); err != nil {
			return core.NewError(core.ErrorCodeInternal, false, nil)
		}
	}
	return nil
}

func stableNativeBindingID(
	organizationID, ownerAccountID uuid.UUID,
	connectorID core.Identifier,
	target core.TargetRef,
) uuid.UUID {
	payload, _ := json.Marshal(struct {
		OrganizationID    string           `json:"organization_id"`
		OwnerAccountID    string           `json:"owner_account_id"`
		ConnectorID       string           `json:"connector_id"`
		TargetType        string           `json:"target_type"`
		TargetID          string           `json:"target_id"`
		BindingInstanceID string           `json:"binding_instance_id"`
		TargetScope       core.TargetScope `json:"target_scope"`
		TargetVersionID   string           `json:"target_version_id"`
	}{
		OrganizationID: organizationID.String(), OwnerAccountID: ownerAccountID.String(),
		ConnectorID: connectorID.String(), TargetType: string(target.Type), TargetID: target.ID,
		BindingInstanceID: target.BindingInstanceID, TargetScope: target.EffectiveScope(), TargetVersionID: target.VersionID,
	})
	return uuid.NewSHA1(nativeTargetBindingNamespace, payload)
}

func bindingUpsertCorrelation(idempotencyKey string) string {
	digest := sha256.Sum256([]byte(idempotencyKey))
	return "binding-upsert:" + hex.EncodeToString(digest[:])
}

func bindingDetachCorrelation(bindingID uuid.UUID, expectedRevision int) string {
	return "binding-detach:" + bindingID.String() + ":" + strconv.Itoa(expectedRevision)
}

func bindingAuthorizationCorrelation(bindingID uuid.UUID, expectedRevision int, mode string) string {
	return "binding-authorization:" + bindingID.String() + ":" + strconv.Itoa(expectedRevision) + ":" + mode
}

func safeBindingDependencyError(err error) error {
	if err == nil {
		return nil
	}
	var connectorError *core.Error
	if errors.As(err, &connectorError) && connectorError != nil && connectorError.Code.Valid() {
		return connectorError
	}
	return core.NewError(core.ErrorCodeInternal, false, nil)
}
