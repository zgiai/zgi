package container

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

// connectorHostDelegationAuthorizer is the composition-root adapter for the
// runtimeauth delegated-host port. The connector module deliberately does not
// know how a workspace member, API key, or durable DigitalHuman run proves
// access to a published target.
type connectorHostDelegationAuthorizer struct {
	container *ServiceContainer
}

var _ runtimeauth.DelegatedRunAuthorizer = (*connectorHostDelegationAuthorizer)(nil)

func (c *ServiceContainer) GetConnectorHostDelegationAuthorizer() runtimeauth.DelegatedRunAuthorizer {
	if c == nil || c.config == nil || c.db == nil {
		return nil
	}
	return &connectorHostDelegationAuthorizer{container: c}
}

func (authorizer *connectorHostDelegationAuthorizer) AuthorizeHostUse(ctx context.Context, request runtimeauth.DelegatedHostRequest) error {
	if authorizer == nil || authorizer.container == nil || authorizer.container.db == nil ||
		ctx == nil || request.OrganizationID == uuid.Nil || request.RequesterID == uuid.Nil ||
		request.OwnerAccountID == uuid.Nil || request.BindingID == uuid.Nil || request.Target.Validate() != nil {
		return targetDenied()
	}

	switch request.Target.Type {
	case core.TargetTypeAgent:
		return authorizer.container.GetConnectorTargetAuthority().RequireTargetRead(
			ctx,
			request.OrganizationID,
			request.RequesterID,
			bindings.TargetIdentity{Type: request.Target.Type, ID: request.Target.ID, Scope: request.Target.Scope, VersionID: request.Target.VersionID},
		)
	case core.TargetTypeDigitalHuman:
		return authorizer.authorizeDigitalHumanRun(ctx, request)
	default:
		return targetDenied()
	}
}

func (authorizer *connectorHostDelegationAuthorizer) authorizeDigitalHumanRun(ctx context.Context, request runtimeauth.DelegatedHostRequest) error {
	if request.RunID == uuid.Nil {
		return targetDenied()
	}

	var run digitalhuman.Run
	err := authorizer.container.db.WithContext(ctx).
		Where(
			"id = ? AND organization_id = ? AND digital_human_id = ? AND digital_human_version_id = ? AND cancel_requested_at IS NULL",
			request.RunID,
			request.OrganizationID,
			request.Target.ID,
			request.Target.VersionID,
		).
		Where("status IN ?", []digitalhuman.RunStatus{
			digitalhuman.RunStatusQueued,
			digitalhuman.RunStatusRunning,
			digitalhuman.RunStatusWaiting,
		}).
		Where("actor_id = ? OR principal_id = ? OR user_subject_id = ?", request.RequesterID, request.RequesterID, request.RequesterID).
		Take(&run).Error
	if err != nil || run.ID != request.RunID || run.OrganizationID != request.OrganizationID ||
		run.DigitalHumanID.String() != request.Target.ID || run.DigitalHumanVersionID.String() != request.Target.VersionID {
		return targetDenied()
	}
	return nil
}
