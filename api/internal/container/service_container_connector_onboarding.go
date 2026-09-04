package container

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/onboarding"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/messaging"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type connectorOnboardingAuthority struct{ container *ServiceContainer }

func (a *connectorOnboardingAuthority) Check(ctx context.Context, f onboarding.Flow) error {
	if a == nil || a.container == nil || f.OrganizationID == uuid.Nil || f.OwnerAccountID == uuid.Nil {
		return targetDenied()
	}
	c := a.container
	member, err := c.GetOrganizationService().IsOrganizationMember(ctx, f.OrganizationID.String(), f.OwnerAccountID.String())
	if err != nil || !member {
		return targetDenied()
	}
	d, ok := c.GetConnectorPluginManager().Get(f.ConnectorID)
	if !ok {
		return core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
	}
	if err := checkOnboardingDefinition(f, d); err != nil {
		return err
	}
	if f.ConnectionID != nil {
		connection, err := c.GetConnectorConnectionRepository().GetConnection(ctx, f.OrganizationID, f.OwnerAccountID, *f.ConnectionID)
		if err != nil {
			return err
		}
		if err = checkOnboardingConnection(f, connection); err != nil {
			return err
		}
	}
	return nil
}

func checkOnboardingDefinition(f onboarding.Flow, d catalog.Definition) error {
	if f.ProtocolVersion != 0 {
		compiled, ok := catalog.MessagingDefinition(f.ConnectorID)
		if !ok || f.ProtocolVersion != 1 || d.Setup == nil || f.Handler != compiled.Setup.Handler || catalog.Validate(d) != nil || !d.IsMessagingCredentials() {
			return core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
		}
		digest, err := catalog.DefinitionContractDigest(d)
		if err != nil || digest != f.AcceptedDefinitionDigest {
			return core.NewError(core.ErrorCodeConflict, false, nil)
		}
		return nil
	}
	// The registrar encodes the compiled template. A persisted older definition
	// must not start a flow whose frozen evidence describes different addons.
	compiled, err := catalog.FeishuDefinition()
	if err != nil || d.Setup == nil || d.Setup.TemplateID != f.TemplateID || d.Setup.TemplateDigest != f.TemplateDigest ||
		compiled.Setup.TemplateID != f.TemplateID || compiled.Setup.TemplateDigest != f.TemplateDigest {
		return core.NewError(core.ErrorCodeConnectorUnavailable, false, nil)
	}
	digest, err := catalog.DefinitionContractDigest(d)
	if err != nil || digest != f.AcceptedDefinitionDigest {
		return core.NewError(core.ErrorCodeConflict, false, nil)
	}
	return nil
}

func checkOnboardingConnection(f onboarding.Flow, c core.PersonalConnection) error {
	identity := ""
	if f.ProtocolVersion == 0 && f.ExpectedAppID != nil {
		identity = f.Region + ":" + *f.ExpectedAppID
	}
	if f.ProtocolVersion == 1 && f.ExpectedIdentity != nil {
		identity = *f.ExpectedIdentity
	}
	if f.ConnectionID == nil || identity == "" || f.ExpectedConnectionRevision == nil || f.ExpectedCredentialVersion == nil || c.ExternalAccountID == nil ||
		c.ID != *f.ConnectionID || c.OrganizationID != f.OrganizationID || c.OwnerAccountID != f.OwnerAccountID || c.ConnectorID.String() != f.ConnectorID ||
		c.Revision != *f.ExpectedConnectionRevision || c.CredentialVersion != *f.ExpectedCredentialVersion || *c.ExternalAccountID != identity || c.DeletedAt != nil {
		return core.NewError(core.ErrorCodeConflict, false, nil)
	}
	return nil
}

func (c *ServiceContainer) GetConnectorOnboardingService() *onboarding.Service {
	if c.connectorOnboardingService != nil {
		return c.connectorOnboardingService
	}
	setup := c.GetConnectorConnectionService()
	maintenance := c.GetConnectorMaintenanceService()
	authority := &connectorOnboardingAuthority{container: c}
	commit := func(ctx context.Context, tx *gorm.DB, f onboarding.Flow, values map[string]string) (uuid.UUID, error) {
		ctx = messaging.WithTransaction(ctx, tx)
		if err := authority.Check(ctx, f); err != nil {
			return uuid.Nil, err
		}
		repo := connections.NewGormRepository(tx.Session(&gorm.Session{DisableNestedTransaction: true}))
		key := "connector-onboarding:" + f.ID.String()
		if f.PendingSetupID != nil {
			key = "connector-pending:" + f.PendingSetupID.String()
		}
		var result *core.PersonalConnection
		if f.Intent == "update_app" || f.Intent == "reauthorize" {
			validIdentity := f.ProtocolVersion == 0 && f.ExpectedAppID != nil && values["region"] == f.Region && values["app_id"] == *f.ExpectedAppID
			if d, ok := catalog.MessagingDefinition(f.ConnectorID); ok && f.ProtocolVersion == 1 {
				validIdentity = f.ExpectedIdentity != nil && *f.ExpectedIdentity == catalog.CredentialIdentity(d, values)
			}
			if f.ConnectionID == nil || !validIdentity {
				return uuid.Nil, core.NewError(core.ErrorCodeConflict, false, nil)
			}
			locked := connections.NewGormRepository(tx.Clauses(clause.Locking{Strength: "UPDATE"}))
			current, err := locked.GetConnection(ctx, f.OrganizationID, f.OwnerAccountID, *f.ConnectionID)
			if err != nil {
				return uuid.Nil, err
			}
			if err = checkOnboardingConnection(f, current); err != nil {
				return uuid.Nil, err
			}
			service, err := maintenance.InTransactionRepository(repo)
			if err != nil {
				return uuid.Nil, err
			}
			ready, err := service.Reconnect(ctx, connections.ReconnectCommand{OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, ConnectionID: *f.ConnectionID, IdempotencyKey: key, Values: values})
			if err != nil {
				return uuid.Nil, err
			}
			if !ready.ReplacedExisting || ready.Connection == nil || ready.Connection.ID != *f.ConnectionID {
				return uuid.Nil, core.NewError(core.ErrorCodeConflict, false, nil)
			}
			result = ready.Connection
		} else {
			if f.PendingSetupID != nil && (values["region"] != f.Region || (f.ExpectedAppID != nil && values["app_id"] != *f.ExpectedAppID)) {
				return uuid.Nil, core.NewError(core.ErrorCodeConflict, false, nil)
			}
			service, err := setup.InTransactionRepository(repo)
			if err != nil {
				return uuid.Nil, err
			}
			ready, err := service.StartKeySetup(ctx, connections.KeySetupCommand{OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, ConnectorID: f.ConnectorID, IdempotencyKey: key, Values: values})
			if err != nil {
				return uuid.Nil, err
			}
			result = ready.Connection
		}
		if result == nil {
			return uuid.Nil, core.NewError(core.ErrorCodeOutcomeUnknown, false, nil)
		}
		// Recheck owner and accepted plugin after verification, before commit.
		// The update's original revision is already held by this transaction.
		check := f
		check.ConnectionID = nil
		if err := authority.Check(ctx, check); err != nil {
			return uuid.Nil, err
		}
		return result.ID, nil
	}
	registrar, err := onboarding.NewRegistrationClient(nil)
	if err != nil {
		panic("initialize Connector registration: invalid template")
	}
	service, err := onboarding.NewService(onboarding.NewGormRepository(c.db, commit), c.GetConnectorPluginManager(), c.GetConnectorConnectionService(), c.getConnectorCredentialCipher(), c.connectorCommandHasher, authority, registrar)
	if err != nil {
		panic("initialize Connector registration: unavailable dependencies")
	}
	if err = service.WithMessagingAdapters(c.messagingAdapters()...); err != nil {
		panic("initialize Connector registration adapters")
	}
	service.WithMessagingCleanup(messaging.NewStore(c.db, c.getConnectorCredentialCipher()).Cleanup)
	c.connectorOnboardingService = service
	return service
}
func (c *ServiceContainer) EnsureConnectorOnboardingSchema(ctx context.Context) error {
	var ready bool
	err := c.db.WithContext(ctx).Raw(`SELECT to_regclass('public.connector_onboarding_flows') IS NOT NULL
	 AND to_regclass('public.connector_pending_setups') IS NOT NULL
 AND to_regclass('public.connector_onboarding_secrets') IS NOT NULL
 AND to_regclass('public.connector_connection_provider_evidence') IS NOT NULL
 AND to_regclass('public.connector_messaging_states') IS NOT NULL
 AND to_regclass('public.connector_messaging_contexts') IS NOT NULL
 AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='connector_onboarding_flows' AND column_name='verification_deadline')`).Scan(&ready).Error
	if err != nil || !ready {
		return fmt.Errorf("Connector onboarding schema migration is required")
	}
	return nil
}
