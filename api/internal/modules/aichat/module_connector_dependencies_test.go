package aichat

import (
	"context"
	"testing"

	"github.com/google/uuid"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	connectorcatalog "github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorruntimeauth "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

func TestResolveOptionalModuleDependenciesRecognizesAllConnectorRuntimeDependencies(t *testing.T) {
	bindings := moduleConnectorBindingLister{}
	connections := moduleConnectorConnectionReader{}
	catalog := moduleConnectorCatalog{}
	chatResolver, err := connectorruntimeauth.NewChatResolver(bindings, connections, catalog)
	if err != nil {
		t.Fatal(err)
	}
	agentResolver, err := connectorruntimeauth.NewAgentResolver(bindings, connections, catalog)
	if err != nil {
		t.Fatal(err)
	}
	store := &moduleConnectorConfirmationStore{}
	productCatalog, err := appcatalog.NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(productCatalog)
	if err != nil {
		t.Fatal(err)
	}

	dependencies := resolveOptionalModuleDependencies([]interface{}{
		chatResolver,
		agentResolver,
		store,
		projector,
	})
	if dependencies.connectorChatRuntime != chatResolver {
		t.Fatalf("Chat resolver = %#v", dependencies.connectorChatRuntime)
	}
	if dependencies.connectorAgentRuntime != agentResolver {
		t.Fatalf("Agent resolver = %#v", dependencies.connectorAgentRuntime)
	}
	if dependencies.connectorConfirmationStore != store {
		t.Fatalf("confirmation store = %#v", dependencies.connectorConfirmationStore)
	}
	if dependencies.errorProjector != projector {
		t.Fatal("application error projector was not injected into Chat")
	}
}

type moduleConnectorBindingLister struct{}

func (moduleConnectorBindingLister) ListTargetBindings(
	context.Context,
	uuid.UUID,
	uuid.UUID,
	connectorbindings.TargetIdentity,
) ([]connectorcore.TargetBinding, error) {
	return nil, nil
}

type moduleConnectorConnectionReader struct{}

func (moduleConnectorConnectionReader) GetConnection(
	context.Context,
	uuid.UUID,
	uuid.UUID,
	uuid.UUID,
) (connectorcore.PersonalConnection, error) {
	return connectorcore.PersonalConnection{}, nil
}

type moduleConnectorCatalog struct{}

func (moduleConnectorCatalog) Get(string) (connectorcatalog.Definition, bool) {
	return connectorcatalog.Definition{}, false
}

type moduleConnectorConfirmationStore struct{}

func (*moduleConnectorConfirmationStore) Freeze(
	context.Context,
	connectorexecution.FrozenConfirmationRequest,
) (connectorexecution.ConfirmationContinuation, error) {
	return connectorexecution.ConfirmationContinuation{}, nil
}

func (*moduleConnectorConfirmationStore) Load(
	context.Context,
	uuid.UUID,
	uuid.UUID,
	connectorexecution.RouteClaimReceipt,
) (connectorexecution.ConfirmationContinuation, error) {
	return connectorexecution.ConfirmationContinuation{}, nil
}

func (*moduleConnectorConfirmationStore) RecordTerminal(
	context.Context,
	uuid.UUID,
	uuid.UUID,
	connectorexecution.RouteClaimReceipt,
	connectorexecution.ConfirmationTerminal,
) (connectorexecution.ConfirmationContinuation, error) {
	return connectorexecution.ConfirmationContinuation{}, nil
}
