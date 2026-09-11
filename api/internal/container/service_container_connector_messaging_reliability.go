package container

import (
	"context"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/onboarding"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func logMessagingDiagnostic(ctx context.Context, provider string, code core.ErrorCode, d core.ProviderDiagnostic) {
	if (provider != "weixin" && provider != "wecom") || d.SafeCopy() == nil {
		return
	}
	logger.WarnContext(ctx, "Connector messaging verification failed",
		"connector_id", provider, "code", string(code), "trace_id", d.TraceID, "phase", d.Phase,
		"category", d.Category, "http_status", d.HTTPStatus, "provider_code", d.ProviderCode,
		"elapsed_ms", d.ElapsedMS, "request_written", d.RequestWritten, "response_received", d.ResponseReceived)
}

func (c *ServiceContainer) messagingCommit(authority *connectorOnboardingAuthority) onboarding.MessagingCommit {
	return func(ctx context.Context, tx *gorm.DB, f onboarding.Flow, values map[string]string, verified connections.CandidateVerificationResult) (uuid.UUID, error) {
		if f.CandidateConnectionID == nil {
			return uuid.Nil, core.NewError(core.ErrorCodeConflict, false, nil)
		}
		if err := authority.Check(ctx, f); err != nil {
			return uuid.Nil, err
		}
		repo := connections.NewGormRepository(tx.Session(&gorm.Session{DisableNestedTransaction: true}))
		key := "connector-onboarding:" + f.ID.String()
		var result *core.PersonalConnection
		if f.Intent == "reauthorize" {
			if f.ConnectionID == nil || *f.ConnectionID != *f.CandidateConnectionID || f.ExpectedConnectionRevision == nil || f.ExpectedCredentialVersion == nil {
				return uuid.Nil, core.NewError(core.ErrorCodeConflict, false, nil)
			}
			locked := connections.NewGormRepository(tx.Clauses(clause.Locking{Strength: "UPDATE"}))
			existing, err := locked.GetConnection(ctx, f.OrganizationID, f.OwnerAccountID, *f.ConnectionID)
			if err != nil {
				return uuid.Nil, err
			}
			if err = checkOnboardingConnection(f, existing); err != nil {
				return uuid.Nil, err
			}
			service, err := c.GetConnectorMaintenanceService().InTransactionRepository(repo)
			if err != nil {
				return uuid.Nil, err
			}
			ready, err := service.CommitVerifiedMessagingReconnect(ctx, connections.ReconnectCommand{OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, ConnectionID: *f.ConnectionID, IdempotencyKey: key, Values: values}, *f.ExpectedConnectionRevision, *f.ExpectedCredentialVersion, verified)
			if err != nil {
				return uuid.Nil, err
			}
			if !ready.ReplacedExisting {
				return uuid.Nil, core.NewError(core.ErrorCodeConflict, false, nil)
			}
			result = ready.Connection
		} else {
			service, err := c.GetConnectorConnectionService().InTransactionRepository(repo)
			if err != nil {
				return uuid.Nil, err
			}
			ready, err := service.CommitVerifiedMessaging(ctx, connections.KeySetupCommand{OrganizationID: f.OrganizationID, OwnerAccountID: f.OwnerAccountID, ConnectorID: f.ConnectorID, IdempotencyKey: key, Values: values}, *f.CandidateConnectionID, verified)
			if err != nil {
				return uuid.Nil, err
			}
			result = ready.Connection
		}
		if result == nil || result.ID != *f.CandidateConnectionID {
			return uuid.Nil, core.NewError(core.ErrorCodeConflict, false, nil)
		}
		check := f
		check.ConnectionID = nil
		if err := authority.Check(ctx, check); err != nil {
			return uuid.Nil, err
		}
		return result.ID, nil
	}
}
