package container

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	"github.com/zgiai/zgi/api/internal/modules/automation/serverwatch"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/serveropsssh"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

type serverWatchConnectionReader interface {
	GetConnection(ctx context.Context, organizationID, ownerAccountID, connectionID uuid.UUID) (connectorcore.PersonalConnection, error)
}

// ServerWatchProbe reads watched service states through a DigitalHuman's SSH
// binding without a model (ADR 0031). It is the platform's own fixed,
// read-only check: it never runs anything but the service status action.
type ServerWatchProbe struct {
	bindings    connectorbindings.Repository
	employees   backgroundEmployeeReader
	connections serverWatchConnectionReader
	credentials interface {
		ResolveConnectionCredentials(context.Context, connectorcore.PersonalConnection) (map[string]string, error)
	}
	service *serverops.Service
}

var (
	_ serverwatch.Connections = (*ServerWatchProbe)(nil)
	_ serverwatch.Prober      = (*ServerWatchProbe)(nil)
)

// Usable confirms the owner's ready SSH connection backs an active binding of
// the DigitalHuman's published version.
func (p *ServerWatchProbe) Usable(ctx context.Context, target serverwatch.Target) error {
	_, err := p.connection(ctx, target)
	return err
}

func (p *ServerWatchProbe) connection(ctx context.Context, target serverwatch.Target) (connectorcore.PersonalConnection, error) {
	if p == nil || p.bindings == nil || p.employees == nil || p.connections == nil {
		return connectorcore.PersonalConnection{}, serverwatch.ErrNotUsable
	}
	detail, err := p.employees.Get(ctx, digitalhuman.Scope{OrganizationID: target.OrganizationID, WorkspaceID: target.WorkspaceID, ActorID: target.OwnerAccountID}, target.DigitalHumanID)
	if err != nil || detail == nil || detail.Definition == nil || detail.Definition.Status != digitalhuman.StatusActive || detail.Definition.PublishedVersionID == nil {
		return connectorcore.PersonalConnection{}, serverwatch.ErrNotUsable
	}
	bindings, err := p.bindings.ListBindingsByTarget(ctx, target.OrganizationID, target.OwnerAccountID, connectorcore.TargetTypeDigitalHuman,
		target.DigitalHumanID.String(), connectorbindings.ListOptions{
			Scope: connectorcore.ScopePublished, VersionID: detail.Definition.PublishedVersionID.String(),
			Statuses: []connectorcore.BindingStatus{connectorcore.BindingStatusActive},
		})
	if err != nil {
		return connectorcore.PersonalConnection{}, err
	}
	bound := false
	for _, binding := range bindings {
		if binding.ConnectionID == target.ConnectionID && binding.OwnerAccountID == target.OwnerAccountID &&
			binding.Status == connectorcore.BindingStatusActive && string(binding.ConnectorID) == catalog.ServerOpsSSHConnectorID {
			bound = true
			break
		}
	}
	if !bound {
		return connectorcore.PersonalConnection{}, serverwatch.ErrNotUsable
	}
	connection, err := p.connections.GetConnection(ctx, target.OrganizationID, target.OwnerAccountID, target.ConnectionID)
	if err != nil {
		if connectorcore.ErrorCodeOf(err) == connectorcore.ErrorCodeNotFound {
			return connectorcore.PersonalConnection{}, serverwatch.ErrNotUsable
		}
		return connectorcore.PersonalConnection{}, err
	}
	if connection.ID != target.ConnectionID || connection.OwnerAccountID != target.OwnerAccountID ||
		string(connection.ConnectorID) != catalog.ServerOpsSSHConnectorID || connection.State != connectorcore.ReadinessReady {
		return connectorcore.PersonalConnection{}, serverwatch.ErrNotUsable
	}
	return connection, nil
}

// Probe reads each service's active state. A failure to reach or log in to
// the host stops the check and is reported as the host's error class.
func (p *ServerWatchProbe) Probe(ctx context.Context, target serverwatch.Target, services []string) (serverwatch.Observation, error) {
	connection, err := p.connection(ctx, target)
	if err != nil {
		return serverwatch.Observation{}, err
	}
	if p.credentials == nil || p.service == nil {
		return serverwatch.Observation{}, serverwatch.ErrNotUsable
	}
	values, err := p.credentials.ResolveConnectionCredentials(ctx, connection)
	if err != nil {
		return serverwatch.Observation{}, err
	}
	defer connectorcore.DestroySecretValues(values)
	observation := serverwatch.Observation{Services: make(map[string]string, len(services))}
	for _, name := range services {
		ssh, release, buildErr := serveropsssh.WatchConnection(values)
		if buildErr != nil {
			release()
			return serverwatch.Observation{}, serverwatch.ErrNotUsable
		}
		status, statusErr := p.service.ServiceStatus(ctx, ssh, name)
		release()
		switch {
		case statusErr == nil && status.LoadState == "not-found":
			observation.Services[name] = "not-found"
		case statusErr == nil:
			observation.Services[name] = status.ActiveState
		case errors.Is(statusErr, serverops.ErrInvalidInput), errors.Is(statusErr, serverops.ErrProviderRejected), errors.Is(statusErr, serverops.ErrResponseInvalid):
			observation.Services[name] = "check_failed"
		default:
			return serverwatch.Observation{HostError: hostErrorClass(statusErr)}, nil
		}
	}
	return observation, nil
}

func hostErrorClass(err error) string {
	switch {
	case errors.Is(err, serverops.ErrTimeout):
		return "timeout"
	case errors.Is(err, serverops.ErrAuthInvalid):
		return "host_key_mismatch"
	case errors.Is(err, serverops.ErrConnectionInvalid):
		return "login_failed"
	default:
		return "unreachable"
	}
}

// GetServerWatchProbe returns the probe shared by the watch API and sweep.
func (c *ServiceContainer) GetServerWatchProbe() *ServerWatchProbe {
	service, _ := c.getServerOpsSSHClient()
	return &ServerWatchProbe{
		bindings: c.GetConnectorBindingRepository(), employees: c.GetDigitalHumanService(), connections: c.GetConnectorConnectionRepository(),
		credentials: c.GetConnectorCredentialResolver(), service: service,
	}
}
