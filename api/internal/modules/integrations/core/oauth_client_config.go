package integrations

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	OAuthClientSourceOrganization = "organization"
	OAuthClientSourceDeployment   = "deployment"
)

type IntegrationOAuthClientConfig struct {
	ID                   uuid.UUID      `gorm:"type:uuid;primaryKey" json:"-"`
	OrganizationID       uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_integration_oauth_client_unique,priority:1" json:"-"`
	IntegrationID        string         `gorm:"size:64;not null;uniqueIndex:idx_integration_oauth_client_unique,priority:2" json:"-"`
	DriverID             string         `gorm:"size:64;not null" json:"-"`
	AuthMethodID         string         `gorm:"size:128;not null;uniqueIndex:idx_integration_oauth_client_unique,priority:3" json:"-"`
	EncryptedCredentials string         `gorm:"type:text;not null" json:"-"`
	Config               map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"-"`
	Enabled              bool           `gorm:"not null;default:true" json:"-"`
	CredentialVersion    int            `gorm:"not null;default:1" json:"-"`
	Revision             int            `gorm:"not null;default:1" json:"-"`
	CreatedBy            *uuid.UUID     `gorm:"type:uuid" json:"-"`
	UpdatedBy            *uuid.UUID     `gorm:"type:uuid" json:"-"`
	CreatedAt            time.Time      `json:"-"`
	UpdatedAt            time.Time      `json:"-"`
}

func (IntegrationOAuthClientConfig) TableName() string {
	return "integration_oauth_client_configs"
}

func (config *IntegrationOAuthClientConfig) BeforeCreate(_ *gorm.DB) error {
	if config.ID == uuid.Nil {
		config.ID = uuid.New()
	}
	if config.CredentialVersion < 1 {
		config.CredentialVersion = 1
	}
	if config.Revision < 1 {
		config.Revision = 1
	}
	if config.Config == nil {
		config.Config = map[string]any{}
	}
	return nil
}

type OAuthClientConfigRepository interface {
	Get(ctx context.Context, organizationID uuid.UUID, integrationID, authMethodID string) (*IntegrationOAuthClientConfig, error)
	Create(ctx context.Context, config *IntegrationOAuthClientConfig) error
	Update(ctx context.Context, config *IntegrationOAuthClientConfig, expectedRevision int) error
	Delete(ctx context.Context, organizationID uuid.UUID, integrationID, authMethodID string) error
}

type GormOAuthClientConfigRepository struct{ db *gorm.DB }

func NewGormOAuthClientConfigRepository(db *gorm.DB) *GormOAuthClientConfigRepository {
	return &GormOAuthClientConfigRepository{db: db}
}

func (repository *GormOAuthClientConfigRepository) Get(ctx context.Context, organizationID uuid.UUID, integrationID, authMethodID string) (*IntegrationOAuthClientConfig, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("integration OAuth client config repository is unavailable")
	}
	var config IntegrationOAuthClientConfig
	db, _ := oauthClientFlowDatabase(ctx, repository.db)
	err := db.
		Where("organization_id = ? AND integration_id = ? AND auth_method_id = ?", organizationID, normalizeOAuthIdentifier(integrationID), normalizeOAuthIdentifier(authMethodID)).
		First(&config).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrConnectionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get integration OAuth client config: %w", err)
	}
	return &config, nil
}

func (repository *GormOAuthClientConfigRepository) Create(ctx context.Context, config *IntegrationOAuthClientConfig) error {
	if repository == nil || repository.db == nil || config == nil {
		return fmt.Errorf("integration OAuth client config repository is unavailable")
	}
	db, _ := oauthClientFlowDatabase(ctx, repository.db)
	if err := db.Create(config).Error; err != nil {
		return fmt.Errorf("create integration OAuth client config: %w", err)
	}
	return nil
}

func (repository *GormOAuthClientConfigRepository) Update(ctx context.Context, config *IntegrationOAuthClientConfig, expectedRevision int) error {
	if repository == nil || repository.db == nil || config == nil || config.ID == uuid.Nil || expectedRevision < 1 {
		return fmt.Errorf("integration OAuth client config repository is unavailable")
	}
	encodedConfig, err := json.Marshal(config.Config)
	if err != nil {
		return fmt.Errorf("encode integration OAuth client config: %w", err)
	}
	db, _ := oauthClientFlowDatabase(ctx, repository.db)
	result := db.Model(&IntegrationOAuthClientConfig{}).
		Where("id = ? AND organization_id = ? AND revision = ?", config.ID, config.OrganizationID, expectedRevision).
		Updates(map[string]any{
			"driver_id":             config.DriverID,
			"encrypted_credentials": config.EncryptedCredentials,
			"config":                datatypes.JSON(encodedConfig),
			"enabled":               config.Enabled,
			"credential_version":    config.CredentialVersion,
			"revision":              expectedRevision + 1,
			"updated_by":            config.UpdatedBy,
			"updated_at":            gorm.Expr("CURRENT_TIMESTAMP"),
		})
	if result.Error != nil {
		return fmt.Errorf("update integration OAuth client config: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrConnectionChanged
	}
	config.Revision = expectedRevision + 1
	return nil
}

func (repository *GormOAuthClientConfigRepository) Delete(ctx context.Context, organizationID uuid.UUID, integrationID, authMethodID string) error {
	if repository == nil || repository.db == nil {
		return fmt.Errorf("integration OAuth client config repository is unavailable")
	}
	db, _ := oauthClientFlowDatabase(ctx, repository.db)
	result := db.
		Where("organization_id = ? AND integration_id = ? AND auth_method_id = ?", organizationID, normalizeOAuthIdentifier(integrationID), normalizeOAuthIdentifier(authMethodID)).
		Delete(&IntegrationOAuthClientConfig{})
	if result.Error != nil {
		return fmt.Errorf("delete integration OAuth client config: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrConnectionNotFound
	}
	return nil
}

type OAuthDeploymentClient struct {
	IntegrationID string
	DriverID      string
	AuthMethodID  string
	ClientID      string
	ClientSecret  string
	Config        map[string]any
}

type OAuthClientConfigView struct {
	IntegrationID  string         `json:"integration_id"`
	DriverID       string         `json:"driver_id"`
	AuthMethodID   string         `json:"auth_method_id"`
	Configured     bool           `json:"configured"`
	Source         string         `json:"source"`
	ClientIDMasked string         `json:"client_id_masked,omitempty"`
	ClientIDMask   string         `json:"client_id_mask,omitempty"`
	HasSecret      bool           `json:"has_client_secret"`
	CallbackURL    string         `json:"callback_url,omitempty"`
	Config         map[string]any `json:"config"`
	Revision       int            `json:"revision"`
	UpdatedAt      *time.Time     `json:"updated_at,omitempty"`
}

type PutOAuthClientConfigRequest struct {
	OrganizationID uuid.UUID
	IntegrationID  string
	DriverID       string
	AuthMethodID   string
	ClientID       string
	ClientSecret   string
	Config         map[string]any
	ActorID        *uuid.UUID
	Revision       int
}

type OAuthClientConfigService struct {
	repository  OAuthClientConfigRepository
	cipher      CredentialCipher
	registry    *Registry
	deployment  map[string]OAuthDeploymentClient
	connections ConnectionRepository
	flows       OAuthClientFlowImpactRepository
	recovery    OAuthRecoveryImpactRepository
	flowLocker  OAuthClientFlowLocker
}

func (service *OAuthClientConfigService) WithFlowImpactRepository(repository OAuthClientFlowImpactRepository) *OAuthClientConfigService {
	if service != nil {
		service.flows = repository
	}
	return service
}

func (service *OAuthClientConfigService) WithConnectionRepository(repository ConnectionRepository) *OAuthClientConfigService {
	if service != nil {
		service.connections = repository
	}
	return service
}

func (service *OAuthClientConfigService) WithRecoveryImpactRepository(repository OAuthRecoveryImpactRepository) *OAuthClientConfigService {
	if service != nil {
		service.recovery = repository
	}
	return service
}

func (service *OAuthClientConfigService) WithOAuthClientFlowLocker(locker OAuthClientFlowLocker) *OAuthClientConfigService {
	if service != nil {
		service.flowLocker = locker
	}
	return service
}

func NewOAuthClientConfigService(repository OAuthClientConfigRepository, cipher CredentialCipher, registry *Registry, deployment []OAuthDeploymentClient) *OAuthClientConfigService {
	clients := make(map[string]OAuthDeploymentClient, len(deployment))
	for _, client := range deployment {
		client.IntegrationID = normalizeOAuthIdentifier(client.IntegrationID)
		client.DriverID = normalizeOAuthIdentifier(client.DriverID)
		client.AuthMethodID = normalizeOAuthIdentifier(client.AuthMethodID)
		if registry != nil {
			if definition, ok := registry.ProviderDefinition(client.IntegrationID); ok {
				for _, method := range definition.AuthMethods {
					if method.Type == AuthMethodTypeOAuth2 && method.ID == client.AuthMethodID && method.OAuth != nil && method.OAuth.ClientConfigID != "" {
						client.AuthMethodID = method.OAuth.ClientConfigID
						break
					}
				}
			}
		}
		if client.IntegrationID == "" || client.DriverID == "" || client.AuthMethodID == "" || strings.TrimSpace(client.ClientID) == "" {
			continue
		}
		if validateConnectionConfig(client.Config) != nil {
			continue
		}
		client.Config = cloneAnyMap(client.Config)
		clients[oauthClientKey(client.IntegrationID, client.AuthMethodID)] = client
	}
	service := &OAuthClientConfigService{repository: repository, cipher: cipher, registry: registry, deployment: clients}
	if gormRepository, ok := repository.(*GormOAuthClientConfigRepository); ok && gormRepository != nil {
		service.flowLocker = newGormOAuthClientFlowLocker(gormRepository.db)
	}
	return service
}

func (service *OAuthClientConfigService) ResolveOAuthClient(ctx context.Context, request OAuthClientResolveRequest) (OAuthClient, error) {
	if service == nil || service.registry == nil {
		return OAuthClient{}, NewError(ErrorCodeDisabled, "integration OAuth client configuration is unavailable", nil)
	}
	var err error
	request, err = service.canonicalClientRequest(request)
	if err != nil {
		return OAuthClient{}, err
	}
	if service.repository != nil && service.cipher != nil {
		config, err := service.repository.Get(ctx, request.OrganizationID, request.IntegrationID, request.AuthMethodID)
		if err == nil && config.Enabled {
			credentials, decryptErr := service.cipher.DecryptCredentials(config.EncryptedCredentials, oauthClientCredentialAAD(config))
			if decryptErr != nil {
				return OAuthClient{}, NewError(ErrorCodeConnectionInvalid, "integration OAuth client configuration is unavailable", decryptErr)
			}
			client := OAuthClient{
				ClientID: strings.TrimSpace(credentials["client_id"]), ClientSecret: credentials["client_secret"],
				Config: cloneAnyMap(config.Config), Source: OAuthClientSourceOrganization,
			}
			destroyCredentialMap(credentials)
			if client.ClientID == "" {
				client.Destroy()
				return OAuthClient{}, NewError(ErrorCodeConnectionInvalid, "integration OAuth client configuration is incomplete", nil)
			}
			return client, nil
		}
		if err != nil && !errors.Is(err, ErrConnectionNotFound) {
			return OAuthClient{}, NewError(ErrorCodeConnectionInvalid, "integration OAuth client configuration could not be loaded", err)
		}
	}
	deployment, ok := service.deployment[oauthClientKey(request.IntegrationID, request.AuthMethodID)]
	if !ok || !strings.EqualFold(deployment.DriverID, request.DriverID) {
		return OAuthClient{}, NewError(ErrorCodeDisabled, "integration OAuth client is not configured", nil)
	}
	return OAuthClient{
		ClientID: deployment.ClientID, ClientSecret: deployment.ClientSecret,
		Config: cloneAnyMap(deployment.Config), Source: OAuthClientSourceDeployment,
	}, nil
}

func (service *OAuthClientConfigService) OAuthClientConfigured(ctx context.Context, request OAuthClientResolveRequest) bool {
	client, err := service.ResolveOAuthClient(ctx, request)
	if err != nil {
		return false
	}
	client.Destroy()
	return true
}

func (service *OAuthClientConfigService) GetView(ctx context.Context, request OAuthClientResolveRequest) (OAuthClientConfigView, error) {
	var err error
	request, err = service.canonicalClientRequest(request)
	if err != nil {
		return OAuthClientConfigView{}, err
	}
	if service.repository != nil {
		config, err := service.repository.Get(ctx, request.OrganizationID, request.IntegrationID, request.AuthMethodID)
		if err == nil {
			if service.cipher == nil {
				return OAuthClientConfigView{}, NewError(ErrorCodeConnectionInvalid, "integration OAuth client configuration is unavailable", nil)
			}
			view := OAuthClientConfigView{
				IntegrationID: config.IntegrationID, DriverID: config.DriverID, AuthMethodID: config.AuthMethodID,
				Configured: config.Enabled, Source: OAuthClientSourceOrganization,
				Config: cloneAnyMap(config.Config), Revision: config.Revision, UpdatedAt: cloneTimePointer(&config.UpdatedAt),
			}
			credentials, decryptErr := service.cipher.DecryptCredentials(config.EncryptedCredentials, oauthClientCredentialAAD(config))
			if decryptErr != nil {
				return OAuthClientConfigView{}, NewError(ErrorCodeConnectionInvalid, "integration OAuth client configuration is unavailable", decryptErr)
			}
			defer destroyCredentialMap(credentials)
			view.ClientIDMasked = maskOAuthClientID(credentials["client_id"])
			view.ClientIDMask = view.ClientIDMasked
			view.HasSecret = strings.TrimSpace(credentials["client_secret"]) != ""
			return view, nil
		}
		if !errors.Is(err, ErrConnectionNotFound) {
			return OAuthClientConfigView{}, err
		}
	}
	deployment, exists := service.deployment[oauthClientKey(request.IntegrationID, request.AuthMethodID)]
	return OAuthClientConfigView{
		IntegrationID: request.IntegrationID, DriverID: request.DriverID, AuthMethodID: request.AuthMethodID,
		Configured: exists, Source: OAuthClientSourceDeployment,
		ClientIDMasked: maskOAuthClientID(deployment.ClientID), ClientIDMask: maskOAuthClientID(deployment.ClientID),
		HasSecret: strings.TrimSpace(deployment.ClientSecret) != "", Config: map[string]any{},
	}, nil
}

func (service *OAuthClientConfigService) Put(ctx context.Context, request PutOAuthClientConfigRequest) (OAuthClientConfigView, error) {
	resolveRequest := OAuthClientResolveRequest{
		OrganizationID: request.OrganizationID, IntegrationID: normalizeOAuthIdentifier(request.IntegrationID),
		DriverID: normalizeOAuthIdentifier(request.DriverID), AuthMethodID: normalizeOAuthIdentifier(request.AuthMethodID),
	}
	var err error
	resolveRequest, err = service.canonicalClientRequest(resolveRequest)
	if err != nil {
		return OAuthClientConfigView{}, err
	}
	if service.repository == nil || service.cipher == nil {
		return OAuthClientConfigView{}, NewError(ErrorCodeDisabled, "organization OAuth client overrides are unavailable", nil)
	}
	if err := validateConnectionConfig(request.Config); err != nil {
		return OAuthClientConfigView{}, err
	}
	canonicalConfig, err := canonicalOAuthClientProviderConfig(request.Config)
	if err != nil {
		return OAuthClientConfigView{}, err
	}
	request.Config = canonicalConfig
	var view OAuthClientConfigView
	err = withOAuthClientFlowLock(
		ctx,
		service.flowLocker,
		resolveRequest.OrganizationID,
		resolveRequest.IntegrationID,
		resolveRequest.AuthMethodID,
		func(lockedContext context.Context) error {
			var putErr error
			view, putErr = service.putLocked(lockedContext, request, resolveRequest)
			return putErr
		},
	)
	return view, err
}

func (service *OAuthClientConfigService) putLocked(
	ctx context.Context,
	request PutOAuthClientConfigRequest,
	resolveRequest OAuthClientResolveRequest,
) (OAuthClientConfigView, error) {
	stored, err := service.repository.Get(ctx, request.OrganizationID, resolveRequest.IntegrationID, resolveRequest.AuthMethodID)
	if err != nil && !errors.Is(err, ErrConnectionNotFound) {
		return OAuthClientConfigView{}, err
	}
	create := errors.Is(err, ErrConnectionNotFound)
	if create && request.Revision != 0 {
		return OAuthClientConfigView{}, invalidInput("OAuth client configuration revision must be zero when creating", nil)
	}
	if !create && request.Revision < 1 {
		return OAuthClientConfigView{}, NewError(ErrorCodeConnectionConflict, "OAuth client configuration changed; reload it and retry", ErrConnectionChanged)
	}
	clientID := strings.TrimSpace(request.ClientID)
	secret := request.ClientSecret
	previousClientID := ""
	previousSecret := ""
	previousConfig := map[string]any{}
	previousMaterialExists := false
	if !create {
		previous, decryptErr := service.cipher.DecryptCredentials(stored.EncryptedCredentials, oauthClientCredentialAAD(stored))
		if decryptErr != nil {
			return OAuthClientConfigView{}, NewError(ErrorCodeConnectionInvalid, "integration OAuth client configuration is unavailable", decryptErr)
		}
		previousClientID = strings.TrimSpace(previous["client_id"])
		previousSecret = previous["client_secret"]
		previousConfig, decryptErr = canonicalOAuthClientProviderConfig(stored.Config)
		if decryptErr != nil {
			destroyCredentialMap(previous)
			return OAuthClientConfigView{}, NewError(ErrorCodeConnectionInvalid, "integration OAuth client configuration is invalid", decryptErr)
		}
		previousMaterialExists = true
		if clientID == "" {
			clientID = previousClientID
		}
		if secret == "" {
			secret = previousSecret
		}
		destroyCredentialMap(previous)
	} else if deployment, exists := service.deployment[oauthClientKey(resolveRequest.IntegrationID, resolveRequest.AuthMethodID)]; exists &&
		strings.EqualFold(deployment.DriverID, resolveRequest.DriverID) {
		previousClientID = strings.TrimSpace(deployment.ClientID)
		previousSecret = deployment.ClientSecret
		previousConfig, err = canonicalOAuthClientProviderConfig(deployment.Config)
		if err != nil {
			return OAuthClientConfigView{}, NewError(ErrorCodeConnectionInvalid, "deployment OAuth client configuration is invalid", err)
		}
		previousMaterialExists = true
	}
	if clientID == "" || len(clientID) > 1024 || len(secret) > 4096 {
		return OAuthClientConfigView{}, invalidInput("OAuth client credentials are invalid", nil)
	}
	if err := service.validateClientFields(resolveRequest, clientID, secret, request.Config); err != nil {
		return OAuthClientConfigView{}, err
	}
	materialChanged := true
	if previousMaterialExists {
		materialChanged, err = oauthClientMaterialChanged(
			previousClientID,
			previousSecret,
			previousConfig,
			clientID,
			secret,
			request.Config,
		)
		if err != nil {
			return OAuthClientConfigView{}, NewError(ErrorCodeConnectionInvalid, "integration OAuth client configuration could not be compared", err)
		}
	}
	if materialChanged {
		pendingFlows, pendingErr := service.countPendingOAuthClientFlows(ctx, resolveRequest)
		if pendingErr != nil {
			return OAuthClientConfigView{}, pendingErr
		}
		if pendingFlows > 0 {
			return OAuthClientConfigView{}, NewError(
				ErrorCodeConnectionInUse,
				"OAuth client configuration cannot change while authorization is pending",
				ErrConnectionInUse,
			)
		}
	}
	if !create && previousClientID != "" && clientID != previousClientID {
		impact, impactErr := service.Impact(ctx, resolveRequest)
		if impactErr != nil {
			return OAuthClientConfigView{}, impactErr
		}
		if impact.DependentConnections > 0 || impact.PendingFlows > 0 {
			return OAuthClientConfigView{}, NewError(ErrorCodeConnectionInUse, "OAuth client id cannot change while OAuth connections still depend on it", ErrConnectionInUse)
		}
	}
	if stored == nil {
		stored = &IntegrationOAuthClientConfig{
			ID: uuid.New(), OrganizationID: request.OrganizationID, IntegrationID: resolveRequest.IntegrationID,
			DriverID: resolveRequest.DriverID, AuthMethodID: resolveRequest.AuthMethodID, Config: cloneAnyMap(request.Config),
			Enabled: true, CredentialVersion: 1, Revision: 1, CreatedBy: cloneUUIDPointer(request.ActorID), UpdatedBy: cloneUUIDPointer(request.ActorID),
		}
	} else {
		if request.Revision > 0 && request.Revision != stored.Revision {
			return OAuthClientConfigView{}, NewError(ErrorCodeConnectionConflict, "integration OAuth client configuration changed; reload it and retry", ErrConnectionChanged)
		}
		stored.CredentialVersion++
		stored.DriverID = resolveRequest.DriverID
		stored.Config = cloneAnyMap(request.Config)
		stored.Enabled = true
		stored.UpdatedBy = cloneUUIDPointer(request.ActorID)
	}
	credentials := map[string]string{"client_id": clientID}
	if strings.TrimSpace(secret) != "" {
		credentials["client_secret"] = secret
	}
	envelope, encryptErr := service.cipher.EncryptCredentials(credentials, oauthClientCredentialAAD(stored))
	destroyCredentialMap(credentials)
	if encryptErr != nil {
		return OAuthClientConfigView{}, NewError(ErrorCodeConnectionInvalid, "integration OAuth client configuration could not be protected", encryptErr)
	}
	stored.EncryptedCredentials = envelope
	if create {
		if createErr := service.repository.Create(ctx, stored); createErr != nil {
			return OAuthClientConfigView{}, createErr
		}
	} else {
		expectedRevision := stored.Revision
		if updateErr := service.repository.Update(ctx, stored, expectedRevision); updateErr != nil {
			return OAuthClientConfigView{}, mapConnectionLookupError(updateErr)
		}
	}
	return service.GetView(ctx, resolveRequest)
}

func (service *OAuthClientConfigService) Delete(ctx context.Context, request OAuthClientResolveRequest) error {
	var err error
	request, err = service.canonicalClientRequest(request)
	if err != nil {
		return err
	}
	if service.repository == nil {
		return NewError(ErrorCodeDisabled, "organization OAuth client overrides are unavailable", nil)
	}
	return withOAuthClientFlowLock(
		ctx,
		service.flowLocker,
		request.OrganizationID,
		request.IntegrationID,
		request.AuthMethodID,
		func(lockedContext context.Context) error {
			return service.deleteLocked(lockedContext, request)
		},
	)
}

func (service *OAuthClientConfigService) deleteLocked(ctx context.Context, request OAuthClientResolveRequest) error {
	impact, err := service.Impact(ctx, request)
	if err != nil {
		return err
	}
	if impact.DependentConnections > 0 || impact.PendingFlows > 0 {
		return NewError(ErrorCodeConnectionInUse, "OAuth client configuration is still used by active connections", ErrConnectionInUse)
	}
	return mapConnectionLookupError(service.repository.Delete(ctx, request.OrganizationID, request.IntegrationID, request.AuthMethodID))
}

type OAuthClientConfigImpact struct {
	DependentConnections int  `json:"dependent_connections"`
	ActiveConnections    int  `json:"active_connections"`
	PendingFlows         int  `json:"pending_flows"`
	PendingRevocations   int  `json:"pending_revocations"`
	CanRemove            bool `json:"can_remove"`
}

func (service *OAuthClientConfigService) Impact(ctx context.Context, request OAuthClientResolveRequest) (OAuthClientConfigImpact, error) {
	if service == nil || service.connections == nil {
		return OAuthClientConfigImpact{}, NewError(ErrorCodeDisabled, "OAuth client dependency checking is unavailable", nil)
	}
	var err error
	request, err = service.canonicalClientRequest(request)
	if err != nil {
		return OAuthClientConfigImpact{}, err
	}
	connections, err := service.connections.List(ctx, request.OrganizationID, ConnectionListFilter{
		IntegrationID: request.IntegrationID,
	})
	if err != nil {
		return OAuthClientConfigImpact{}, NewError(ErrorCodeConnectionInvalid, "OAuth client dependencies could not be checked", err)
	}
	impact := OAuthClientConfigImpact{}
	for _, connection := range connections {
		if connection == nil || connection.AuthType != ConnectionAuthTypeOAuth2 ||
			!strings.EqualFold(connection.DriverID, request.DriverID) {
			continue
		}
		connectionRequest, canonicalErr := service.canonicalClientRequest(OAuthClientResolveRequest{
			OrganizationID: request.OrganizationID, IntegrationID: connection.IntegrationID,
			DriverID: connection.DriverID, AuthMethodID: connection.AuthMethodID,
		})
		if canonicalErr == nil && connectionRequest.AuthMethodID == request.AuthMethodID {
			impact.DependentConnections++
			if connection.Status != ConnectionStatusDisabled {
				impact.ActiveConnections++
			}
		}
	}
	if service.flows == nil {
		return OAuthClientConfigImpact{}, NewError(ErrorCodeDisabled, "OAuth flow dependency checking is unavailable", nil)
	}
	authMethodIDs := service.authMethodIDsForClientConfig(request.IntegrationID, request.AuthMethodID)
	pending, err := service.flows.CountPendingOAuthFlows(ctx, request.OrganizationID, request.IntegrationID, authMethodIDs)
	if err != nil {
		return OAuthClientConfigImpact{}, NewError(ErrorCodeConnectionInvalid, "OAuth flow dependencies could not be checked", err)
	}
	impact.PendingFlows = int(pending)
	if service.recovery != nil {
		pendingRevocations, recoveryErr := service.recovery.CountPendingRevocations(
			ctx,
			request.OrganizationID,
			request.IntegrationID,
			authMethodIDs,
		)
		if recoveryErr != nil {
			return OAuthClientConfigImpact{}, NewError(
				ErrorCodeConnectionInvalid,
				"OAuth revocation dependencies could not be checked",
				recoveryErr,
			)
		}
		impact.PendingRevocations = int(pendingRevocations)
	}
	impact.CanRemove = impact.DependentConnections == 0 && impact.PendingFlows == 0
	return impact, nil
}

func (service *OAuthClientConfigService) authMethodIDsForClientConfig(integrationID, clientConfigID string) []string {
	definition, ok := service.registry.ProviderDefinition(integrationID)
	if !ok {
		return nil
	}
	ids := make([]string, 0)
	for _, method := range definition.AuthMethods {
		if method.Type != AuthMethodTypeOAuth2 || method.OAuth == nil {
			continue
		}
		configID := method.OAuth.ClientConfigID
		if configID == "" {
			configID = method.ID
		}
		if configID == clientConfigID {
			ids = append(ids, method.ID)
		}
	}
	return normalizeCatalogStringList(ids, 64)
}

func (service *OAuthClientConfigService) countPendingOAuthClientFlows(ctx context.Context, request OAuthClientResolveRequest) (int64, error) {
	if service == nil || service.flows == nil {
		return 0, NewError(ErrorCodeDisabled, "OAuth flow dependency checking is unavailable", nil)
	}
	authMethodIDs := service.authMethodIDsForClientConfig(request.IntegrationID, request.AuthMethodID)
	pending, err := service.flows.CountPendingOAuthFlows(ctx, request.OrganizationID, request.IntegrationID, authMethodIDs)
	if err != nil {
		return 0, NewError(ErrorCodeConnectionInvalid, "OAuth flow dependencies could not be checked", err)
	}
	return pending, nil
}

func (service *OAuthClientConfigService) canonicalClientRequest(request OAuthClientResolveRequest) (OAuthClientResolveRequest, error) {
	request.IntegrationID = normalizeOAuthIdentifier(request.IntegrationID)
	request.DriverID = normalizeOAuthIdentifier(request.DriverID)
	request.AuthMethodID = normalizeOAuthIdentifier(request.AuthMethodID)
	if request.OrganizationID == uuid.Nil || !integrationIdentifierPattern.MatchString(request.IntegrationID) ||
		!integrationIdentifierPattern.MatchString(request.DriverID) || !integrationIdentifierPattern.MatchString(request.AuthMethodID) {
		return OAuthClientResolveRequest{}, invalidInput("OAuth client configuration identity is invalid", nil)
	}
	definition, ok := service.registry.ProviderDefinition(request.IntegrationID)
	if !ok || !strings.EqualFold(definition.DriverID, request.DriverID) {
		return OAuthClientResolveRequest{}, NewError(ErrorCodeDisabled, "integration OAuth provider is unavailable", nil)
	}
	for _, method := range definition.AuthMethods {
		if method.ID == request.AuthMethodID && method.Type == AuthMethodTypeOAuth2 {
			if method.OAuth != nil && method.OAuth.ClientConfigID != "" {
				request.AuthMethodID = method.OAuth.ClientConfigID
			}
			return request, nil
		}
		if method.Type == AuthMethodTypeOAuth2 && method.OAuth != nil && method.OAuth.ClientConfigID == request.AuthMethodID {
			return request, nil
		}
	}
	return OAuthClientResolveRequest{}, invalidInput("integration OAuth auth method is unsupported", nil)
}

func (service *OAuthClientConfigService) validateClientFields(request OAuthClientResolveRequest, clientID, clientSecret string, config map[string]any) error {
	definition, ok := service.registry.ProviderDefinition(request.IntegrationID)
	if !ok {
		return NewError(ErrorCodeDisabled, "integration OAuth provider is unavailable", nil)
	}
	var metadata *OAuthMethodMetadata
	for index := range definition.AuthMethods {
		if definition.AuthMethods[index].ID == request.AuthMethodID ||
			(definition.AuthMethods[index].OAuth != nil && definition.AuthMethods[index].OAuth.ClientConfigID == request.AuthMethodID) {
			metadata = definition.AuthMethods[index].OAuth
			break
		}
	}
	if metadata == nil {
		return invalidInput("integration OAuth auth method is unsupported", nil)
	}
	values := make(map[string]string, len(config)+2)
	values["client_id"] = strings.TrimSpace(clientID)
	values["client_secret"] = strings.TrimSpace(clientSecret)
	for key, raw := range config {
		value, isString := raw.(string)
		if !isString {
			return invalidInput("OAuth client config values must be strings", nil)
		}
		values[normalizeOAuthIdentifier(key)] = strings.TrimSpace(value)
	}
	allowed := make(map[string]CredentialFieldDefinition, len(metadata.ClientFields))
	for _, field := range metadata.ClientFields {
		allowed[field.Key] = field
	}
	for key, value := range values {
		field, exists := allowed[key]
		if !exists && value != "" {
			return invalidInput("OAuth client configuration contains an unknown field", nil)
		}
		if exists && key != "client_id" && key != "client_secret" && field.Secret && value != "" {
			return invalidInput("OAuth client secrets must use the write-only credential fields", nil)
		}
	}
	for key, field := range allowed {
		if field.Required && values[key] == "" {
			return invalidInput("OAuth client configuration is incomplete", nil)
		}
	}
	return nil
}

func canonicalOAuthClientProviderConfig(config map[string]any) (map[string]any, error) {
	canonical := make(map[string]any, len(config))
	for rawKey, rawValue := range config {
		key := normalizeOAuthIdentifier(rawKey)
		if key == "" {
			return nil, invalidInput("OAuth client configuration contains an invalid field", nil)
		}
		value, ok := rawValue.(string)
		if !ok {
			return nil, invalidInput("OAuth client config values must be strings", nil)
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if existing, exists := canonical[key]; exists && existing != value {
			return nil, invalidInput("OAuth client configuration contains conflicting fields", nil)
		}
		canonical[key] = value
	}
	return canonical, nil
}

func oauthClientMaterialChanged(
	previousClientID string,
	previousSecret string,
	previousConfig map[string]any,
	nextClientID string,
	nextSecret string,
	nextConfig map[string]any,
) (bool, error) {
	type oauthClientMaterial struct {
		ClientID     string         `json:"client_id"`
		ClientSecret string         `json:"client_secret"`
		Config       map[string]any `json:"config"`
	}
	previousJSON, err := json.Marshal(oauthClientMaterial{
		ClientID: strings.TrimSpace(previousClientID), ClientSecret: previousSecret, Config: previousConfig,
	})
	if err != nil {
		return false, fmt.Errorf("encode previous OAuth client material: %w", err)
	}
	defer clearBytes(previousJSON)
	nextJSON, err := json.Marshal(oauthClientMaterial{
		ClientID: strings.TrimSpace(nextClientID), ClientSecret: nextSecret, Config: nextConfig,
	})
	if err != nil {
		return false, fmt.Errorf("encode next OAuth client material: %w", err)
	}
	defer clearBytes(nextJSON)
	previousDigest := sha256.Sum256(previousJSON)
	nextDigest := sha256.Sum256(nextJSON)
	return subtle.ConstantTimeCompare(previousDigest[:], nextDigest[:]) != 1, nil
}

func clearBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

func oauthClientCredentialAAD(config *IntegrationOAuthClientConfig) CredentialAAD {
	return oauthClientCredentialAADWithVersion(config, config.CredentialVersion)
}

func oauthClientCredentialAADWithVersion(config *IntegrationOAuthClientConfig, version int) CredentialAAD {
	return CredentialAAD{
		OrganizationID: config.OrganizationID, ConnectionID: config.ID,
		IntegrationID: "oauth-client-" + config.IntegrationID + "-" + config.AuthMethodID, CredentialVersion: version,
	}
}

func oauthClientKey(integrationID, authMethodID string) string {
	return normalizeOAuthIdentifier(integrationID) + "\x00" + normalizeOAuthIdentifier(authMethodID)
}

func normalizeOAuthIdentifier(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func maskOAuthClientID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= 8 {
		return "••••"
	}
	return string(runes[:4]) + "••••" + string(runes[len(runes)-4:])
}
