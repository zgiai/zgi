package container

import (
	"context"
	"fmt"

	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorcredentials "github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorruntimeconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	digitalhuman "github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

// ConnectorRuntimeConfirmationService is the runtime boundary consumed by the
// HTTP transport. Its implementation requires configured persistence before a
// previously registered route may resume an operation.
type ConnectorRuntimeConfirmationService interface {
	ReadStatus(context.Context, connectorruntimeconfirmation.StatusQuery) (connectorruntimeconfirmation.StatusResult, error)
	Continue(
		context.Context,
		connectorruntimeconfirmation.Command,
	) (connectorruntimeconfirmation.Result, error)
}

// GetConnectorConfirmationPayloadProtector returns the purpose-separated
// authenticated payload protector when configuration is available. It uses
// the Connector keyring shared with the offline native verifier.
func (c *ServiceContainer) GetConnectorConfirmationPayloadProtector() connectorexecution.ContinuationPayloadProtector {
	if c == nil || c.config == nil {
		return nil
	}
	if c.connectorConfirmationProtector == nil {
		protector, err := connectorcredentials.NewContinuationPayloadProtector(c.getConnectorCredentialCipher())
		if err != nil {
			panic(fmt.Sprintf("initialize Connector confirmation payload protector: %v", err))
		}
		c.connectorConfirmationProtector = protector
	}
	return c.connectorConfirmationProtector
}

// GetConnectorConfirmationContinuationStore returns the one owner-scoped
// GORM continuation store shared by Chat, Agent, Workflow, and HTTP resume.
func (c *ServiceContainer) GetConnectorConfirmationContinuationStore() *connectorexecution.GormConfirmationContinuationStore {
	if c == nil || c.config == nil || c.db == nil {
		return nil
	}
	if c.connectorConfirmationStore == nil {
		store, err := connectorexecution.NewGormConfirmationContinuationStore(
			c.db,
			c.GetConnectorConfirmationPayloadProtector(),
		)
		if err != nil {
			panic(fmt.Sprintf("initialize Connector confirmation continuation store: %v", err))
		}
		c.connectorConfirmationStore = store
	}
	return c.connectorConfirmationStore
}

// GetConnectorRuntimeConfirmationService is safe to register during runtime
// assembly. The boundary requires configuration and persistence before
// constructing or invoking the Connector-native service graph.
func (c *ServiceContainer) GetConnectorRuntimeConfirmationService() ConnectorRuntimeConfirmationService {
	if c == nil {
		return nil
	}
	if c.connectorRuntimeConfirmationBoundary == nil {
		c.connectorRuntimeConfirmationBoundary = &connectorRuntimeConfirmationBoundary{container: c}
	}
	return c.connectorRuntimeConfirmationBoundary
}

func (c *ServiceContainer) getConnectorRuntimeConfirmationCore() *connectorruntimeconfirmation.Service {
	if c == nil || c.config == nil || c.db == nil {
		return nil
	}
	if c.connectorRuntimeConfirmation == nil {
		recoverer, err := connectorruntimeconfirmation.NewAuditTerminalRecoverer(
			c.GetConnectorAuditRepository(),
		)
		if err != nil {
			panic(fmt.Sprintf("initialize Connector confirmation terminal recovery: %v", err))
		}
		service, err := connectorruntimeconfirmation.NewService(
			c.GetConnectorConfirmationContinuationStore(),
			c.GetConnectorExecutionRouter(),
			recoverer,
		)
		if err != nil {
			panic(fmt.Sprintf("initialize Connector runtime confirmation service: %v", err))
		}
		service = service.WithDigitalHumanContinuationGuard(
			c.getConnectorDigitalHumanContinuationGuard(),
		)
		c.connectorRuntimeConfirmation = service
	}
	return c.connectorRuntimeConfirmation
}

// getConnectorDigitalHumanContinuationGuard returns the fail-closed durable
// gate that re-reads the waiting DigitalHuman run before a confirmed
// continuation may route. It reads only durable host state and carries no
// credential or request input, so it stays unexported as internal wiring.
func (c *ServiceContainer) getConnectorDigitalHumanContinuationGuard() connectorruntimeconfirmation.DigitalHumanContinuationGuard {
	if c == nil || c.config == nil || c.db == nil {
		return nil
	}
	guard, err := digitalhuman.NewConnectorConfirmationGuard(c.db)
	if err != nil {
		panic(fmt.Sprintf("initialize Connector confirmation DigitalHuman guard: %v", err))
	}
	return guard
}

type connectorRuntimeConfirmationBoundary struct {
	container *ServiceContainer
}

func (gate *connectorRuntimeConfirmationBoundary) ReadStatus(ctx context.Context, query connectorruntimeconfirmation.StatusQuery) (connectorruntimeconfirmation.StatusResult, error) {
	if gate == nil || gate.container == nil {
		return connectorruntimeconfirmation.StatusResult{}, connectorcore.NewError(connectorcore.ErrorCodeConnectorUnavailable, false, nil)
	}
	service := gate.container.getConnectorRuntimeConfirmationCore()
	if service == nil {
		return connectorruntimeconfirmation.StatusResult{}, connectorcore.NewError(connectorcore.ErrorCodeConnectorUnavailable, false, nil)
	}
	return service.ReadStatus(ctx, query)
}

func (gate *connectorRuntimeConfirmationBoundary) Continue(
	ctx context.Context,
	command connectorruntimeconfirmation.Command,
) (connectorruntimeconfirmation.Result, error) {
	if gate == nil || gate.container == nil || gate.container.config == nil || gate.container.db == nil {
		return connectorruntimeconfirmation.Result{}, connectorcore.NewError(
			connectorcore.ErrorCodeConnectorUnavailable,
			false,
			nil,
		)
	}
	service := gate.container.getConnectorRuntimeConfirmationCore()
	if service == nil {
		return connectorruntimeconfirmation.Result{}, connectorcore.NewError(
			connectorcore.ErrorCodeConnectorUnavailable,
			false,
			nil,
		)
	}
	return service.Continue(ctx, command)
}
