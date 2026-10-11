package v1

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	"github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

type digitalHumanConnectorResolver struct {
	runtimeauth.DigitalHumanRuntimeResolver
}

func (digitalHumanConnectorResolver) ResolveConnectorIdentity(_ context.Context, r dispatch.IdentityResolutionRequest) (dispatch.Identity, error) {
	return dispatch.Identity{OrganizationID: r.OrganizationID, OwnerAccountID: r.OwnerAccountID, ActingPrincipalID: r.ActingPrincipalID, BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: r.ConnectorID, OperationID: r.OperationID, Target: r.Target, LogicalOperationID: r.LogicalOperationID, CorrelationID: r.CorrelationID, Interactive: r.Interactive}, nil
}

type digitalHumanConnectorCatalog struct{ digitalhuman.ConnectorCatalog }
type digitalHumanConnectorStore struct {
	execution.ConfirmationContinuationStore
}
type digitalHumanConnectorDispatcher struct {
	calls    int
	identity dispatch.Identity
}

func (d *digitalHumanConnectorDispatcher) Dispatch(_ context.Context, i dispatch.Invocation) (execution.ExecutionResult, error) {
	d.calls++
	d.identity = i.Identity
	return execution.ExecutionResult{State: execution.ExecutionStateCompleted}, nil
}

func digitalHumanConnectorDeps() DigitalHumanRuntimeDeps {
	manager := tools.NewToolManager(nil)
	return DigitalHumanRuntimeDeps{ToolEngine: tools.NewToolEngine(manager), ToolManager: manager, ConnectorDispatcher: &digitalHumanConnectorDispatcher{}, ConnectorRuntime: digitalHumanConnectorResolver{}, ConnectorCatalog: digitalHumanConnectorCatalog{}, ConnectorConfirmationStore: digitalHumanConnectorStore{}}
}

func TestDigitalHumanRuntimeConnectorDependenciesFailBeforeSideEffects(t *testing.T) {
	for _, missing := range []string{"dispatcher", "resolver", "catalog", "confirmation"} {
		t.Run(missing, func(t *testing.T) {
			deps := digitalHumanConnectorDeps()
			switch missing {
			case "dispatcher":
				deps.ConnectorDispatcher = nil
			case "resolver":
				deps.ConnectorRuntime = nil
			case "catalog":
				deps.ConnectorCatalog = nil
			case "confirmation":
				deps.ConnectorConfirmationStore = nil
			}
			defer func() {
				if got := recover(); got == nil || !strings.Contains(fmt.Sprint(got), "Connector dependencies") {
					t.Errorf("expected Connector preflight panic, got %v", got)
				}
			}()
			// All other dependencies are intentionally absent: Connector validation
			// must run before touching any DB, queue, or runtime component.
			InitializeDigitalHumanRuntime(deps)
		})
	}
}

func TestDigitalHumanSkillRuntimeDispatchesWithPublishedIdentity(t *testing.T) {
	deps := digitalHumanConnectorDeps()
	runtime := newDigitalHumanSkillRuntime(deps)
	instance, version, subject := uuid.NewString(), uuid.NewString(), uuid.NewString()
	owner, organization := uuid.NewString(), uuid.NewString()
	resolved := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillConnectorRuntime, TimeoutSeconds: 5},
		Tools:    []skills.SkillToolDefinition{{Name: "read_item", ProviderID: "test-connector", ProviderType: tools.ToolProviderTypeConnector, ConnectorNative: true, ConnectorOperationID: "test.read", ConnectorBindingInstanceID: instance, InputSchema: map[string]any{"type": "object"}}},
	}}}
	ledger := dispatch.ActionLedger{}
	result, err := runtime.CallSkillTool(t.Context(), resolved, skills.SkillConnectorRuntime, "read_item", map[string]any{}, skills.ExecutionContext{
		OrganizationID: organization, UserID: owner, AppID: subject, ConversationID: uuid.NewString(), MessageID: uuid.NewString(), InvokeFrom: tools.ToolInvokeFromAIChat,
		ConnectorTargetType: core.TargetTypeDigitalHuman, ConnectorTargetScope: core.ScopePublished, ConnectorTargetVersionID: version, ConnectorInteractive: true, ConnectorIdentityResolver: deps.ConnectorRuntime,
		ConnectorActionResolver: func(_ context.Context, identity dispatch.Identity, input any) (dispatch.Identity, error) {
			persisted, _, err := dispatch.ResolveActionInstance(&ledger, "turn", identity.CorrelationID, identity, input)
			return persisted, err
		},
	}, "call-digital-human")
	dispatcher := deps.ConnectorDispatcher.(*digitalHumanConnectorDispatcher)
	if err != nil || result == nil || dispatcher.calls != 1 {
		t.Fatalf("runtime dispatch failed: %v calls=%d", err, dispatcher.calls)
	}
	identity := dispatcher.identity
	if identity.Target != (core.TargetRef{Type: core.TargetTypeDigitalHuman, ID: subject, Scope: core.ScopePublished, VersionID: version, BindingInstanceID: instance}) || identity.OwnerAccountID.String() != owner || identity.ActingPrincipalID.String() != owner || identity.OrganizationID.String() != organization || !identity.Interactive || len(ledger.Instances) == 0 {
		t.Fatalf("identity was not preserved: %#v", identity)
	}
}
