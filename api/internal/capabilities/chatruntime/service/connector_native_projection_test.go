package service

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	connectorruntimeauth "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestConnectorNativeProjectionSeparatesInstructionsFromDirectSchemas(t *testing.T) {
	ctx := context.Background()
	definition := connectorNativeLoopWebSearchDefinition(t)
	snapshot := connectorChatTestSnapshot(uuid.New(), uuid.New(), uuid.New())
	snapshot.Capabilities[0].ConnectorID = "web-search"
	snapshot.Capabilities[0].Definition = definition
	snapshot.Capabilities[0].Operations = definition.Operations
	ordinary := skills.SkillDocument{
		Metadata:     skills.SkillMetadata{ID: "ordinary", Name: "Ordinary candidate"},
		Instructions: "Ordinary instructions stay inactive until requested.",
	}
	resolved, err := projectConnectorOperations(&skills.ResolvedSkills{Skills: []skills.SkillDocument{ordinary}}, snapshot, catalog.SurfaceChat)
	if err != nil {
		t.Fatal(err)
	}
	projectionResolved, projections := splitConnectorNativeToolProjections(resolved)
	if len(projections) != 2 {
		t.Fatalf("native candidates = %d, want two Web Search operations", len(projections))
	}
	projectedDoc, _ := projectionResolved.Get(skills.SkillConnectorRuntime)
	sourceDoc, _ := resolved.Get(skills.SkillConnectorRuntime)
	if len(projectedDoc.Tools) != 0 || len(projectedDoc.Metadata.Tools) != 0 || len(sourceDoc.Tools) != 2 {
		t.Fatal("schema separation either kept native schemas in activation or mutated the execution source")
	}
	if projectedDoc.Instructions != sourceDoc.Instructions || projectedDoc.Instructions == "" {
		t.Fatal("native instruction policy was lost during schema separation")
	}
	runtime := skills.NewRuntime(nil, nil)
	candidates := skills.BuildNativeSkillCatalog(resolved, nil, 8000, 0, nil)
	session := skills.NewNativeSkillSession(runtime, projectionResolved, candidates, skills.NativeToolSetOptions{BudgetChars: 50000})
	session.Activate(ctx, []string{skills.SkillConnectorRuntime}, "runtime_preload")
	before := session.ToolSet()
	if len(before.ProviderTools) != 0 || before.SchemaChars != 0 || before.InstructionChars == 0 {
		t.Fatalf("instruction activation still preloaded native schemas: %#v", before)
	}
	if added := projectNativeDirectToolsToSession(session, projections, nil, nil); added != 2 {
		t.Fatalf("directly exposed operations = %d, want 2", added)
	}
	for _, binding := range session.ToolSet().ToolBindings {
		if binding.SkillID != skills.SkillConnectorRuntime || binding.ConnectorOperation == nil || binding.ConnectorOperation.ConnectorID == "" || binding.ConnectorOperation.OperationID == "" {
			t.Fatalf("native operation lost its Connector identity: %#v", binding)
		}
	}
	// Later activations retain aliases and cannot repopulate the whole native
	// group, even when its original execution document contains more tools.
	session.Activate(ctx, []string{skills.SkillConnectorRuntime, ordinary.Metadata.ID}, "model_request")
	if got := session.ToolSet(); len(got.ProviderTools) != 2 {
		t.Fatalf("later activation duplicated native direct schemas: %#v", got.ProviderTools)
	}
	if got, _ := projectionResolved.Get(ordinary.Metadata.ID); !reflect.DeepEqual(*got, ordinary) {
		t.Fatal("native projection modified an unrelated candidate")
	}
}

func TestConnectorNativeProjectionObeysAuthorizationAndInstructionBudget(t *testing.T) {
	ctx := context.Background()
	snapshot := connectorChatTestSnapshot(uuid.New(), uuid.New(), uuid.New())
	resolved, err := projectConnectorOperations(nil, snapshot, catalog.SurfaceChat)
	if err != nil {
		t.Fatal(err)
	}
	projectionResolved, projections := splitConnectorNativeToolProjections(resolved)
	for _, options := range []skills.NativeToolSetOptions{
		{BudgetChars: 1},
		{BudgetChars: 10000, AuthorizeSkill: func(context.Context, string) (bool, error) { return false, nil }},
	} {
		session := skills.NewNativeSkillSession(skills.NewRuntime(nil, nil), projectionResolved,
			skills.BuildNativeSkillCatalog(resolved, nil, 8000, 0, nil), options)
		session.Activate(ctx, []string{skills.SkillConnectorRuntime}, "runtime_preload")
		if added := projectNativeDirectToolsToSession(session, projections, nil, nil); added != 0 {
			t.Fatal("direct schemas bypassed Skill authorization or instruction budget")
		}
		if got := restrictConnectorNativeExecutionTools(resolved, session.ToolSet()); hasConnectorNativeTools(&got.Skills[0]) {
			t.Fatal("an unauthorized/unexposed native definition remained executable by guessed name")
		}
	}
}

func TestConnectorNativeProjectionCutsIndividualSchemasInsteadOfWholeGroup(t *testing.T) {
	ctx := context.Background()
	snapshot := connectorChatTestSnapshot(uuid.New(), uuid.New(), uuid.New())
	large := snapshot.Capabilities[0].Operations[0]
	large.ID, large.ToolName = "github.large", "large_operation"
	large.Description = strings.Repeat("large schema description ", 1000)
	invalid := large
	invalid.ID, invalid.ToolName = "github.invalid", "invalid_operation"
	invalid.InputSchema = map[string]interface{}{"type": "string"}
	snapshot.Capabilities[0].Operations = append([]catalog.Operation{large, invalid}, snapshot.Capabilities[0].Operations...)
	resolved, err := projectConnectorOperations(nil, snapshot, catalog.SurfaceChat)
	if err != nil {
		t.Fatal(err)
	}
	projectionResolved, projections := splitConnectorNativeToolProjections(resolved)
	runtime := skills.NewRuntime(nil, nil)
	set := runtime.BuildNativeToolSet(ctx, projectionResolved, skills.NativeToolSetOptions{BudgetChars: 2000})
	if added := projectNativeDirectTools(&set, projections, nil, nil); added != 1 || set.ProviderTools[0].Function.Name != "get_github_user" {
		t.Fatalf("one bad/oversized schema hid the usable operation: added=%d tools=%#v skipped=%#v", added, set.ProviderTools, set.SkippedTools)
	}
	if len(set.SkippedTools) != 2 {
		t.Fatalf("missing per-operation skip diagnostics: %#v", set.SkippedTools)
	}
	executable := restrictConnectorNativeExecutionTools(resolved, set)
	doc, _ := executable.Get(skills.SkillConnectorRuntime)
	if len(doc.Tools) != 1 || doc.Tools[0].ConnectorOperationID != "github.user.get" || doc.Tools[0].ProviderID != "github" {
		t.Fatalf("execution did not retain exactly the selected native identity: %#v", doc.Tools)
	}
	if original, _ := resolved.Get(skills.SkillConnectorRuntime); len(original.Tools) != 3 {
		t.Fatal("execution filtering mutated the input projection")
	}
}

func TestConnectorNativeProjectionKeepsExistingCapacityAndQueryPriority(t *testing.T) {
	snapshot := connectorChatTestSnapshot(uuid.New(), uuid.New(), uuid.New())
	prototype := snapshot.Capabilities[0].Operations[0]
	operations := make([]catalog.Operation, 140)
	for index := range operations {
		operations[index] = prototype
		operations[index].ID = fmt.Sprintf("github.read-%03d", index)
		operations[index].ToolName = fmt.Sprintf("read_record_%03d", index)
	}
	operations[len(operations)-1].Description = "Find the special weather forecast."
	snapshot.Capabilities[0].Operations = operations
	resolved, err := projectConnectorOperations(nil, snapshot, catalog.SurfaceChat)
	if err != nil {
		t.Fatal(err)
	}
	projectionResolved, projections := splitConnectorNativeToolProjections(resolved)
	projections = skills.RankNativeToolProjections(projections, "special weather forecast")
	set := skills.NewRuntime(nil, nil).BuildNativeToolSet(context.Background(), projectionResolved, skills.NativeToolSetOptions{BudgetChars: 200000})
	if added := projectNativeDirectTools(&set, projections, nil, nil); added != skills.DefaultMaxNativeToolProjections {
		t.Fatalf("direct capacity changed: got %d, want %d", added, skills.DefaultMaxNativeToolProjections)
	}
	// Relevance decides what survives the cut; the order is fixed (ADR 0032).
	kept := false
	for _, tool := range set.ProviderTools {
		kept = kept || tool.Function.Name == operations[len(operations)-1].ToolName
	}
	if !kept {
		t.Fatal("relevance did not keep the requested operation before the capacity cut")
	}
	for _, skip := range set.SkippedTools {
		if skip.Reason != "tool_limit_exceeded" {
			t.Fatalf("unexpected capacity skip: %#v", skip)
		}
	}
	executable := restrictConnectorNativeExecutionTools(resolved, set)
	doc, _ := executable.Get(skills.SkillConnectorRuntime)
	if len(doc.Tools) != skills.DefaultMaxNativeToolProjections {
		t.Fatalf("capacity-excluded operation remained in execution lookup: %d", len(doc.Tools))
	}
}

func TestConnectorNativeProjectionRebuildsOnlyFreshBoundOperations(t *testing.T) {
	snapshot := connectorChatTestSnapshot(uuid.New(), uuid.New(), uuid.New())
	base, err := projectConnectorOperations(nil, snapshot, catalog.SurfaceChat)
	if err != nil {
		t.Fatal(err)
	}
	baseDoc, _ := base.Get(skills.SkillConnectorRuntime)
	baseDoc.Tools[0].InputSchema = map[string]interface{}{"type": "object", "properties": map[string]interface{}{"untrusted_override": map[string]interface{}{"type": "string"}}}
	base.Skills = append(base.Skills, skills.SkillDocument{
		Metadata: skills.SkillMetadata{ID: "another-group", Tools: []string{"stale_operation"}},
		Tools:    append([]skills.SkillToolDefinition(nil), baseDoc.Tools...),
	})
	fresh, err := projectConnectorOperations(base, snapshot, catalog.SurfaceChat)
	if err != nil {
		t.Fatal(err)
	}
	freshDoc, _ := fresh.Get(skills.SkillConnectorRuntime)
	if !reflect.DeepEqual(freshDoc.Tools[0].InputSchema, snapshot.Capabilities[0].Operations[0].InputSchema) {
		t.Fatal("base provider metadata overrode the current authorized schema")
	}
	staleDoc, _ := fresh.Get("another-group")
	if len(staleDoc.Tools) != 0 || len(staleDoc.Metadata.Tools) != 0 {
		t.Fatal("a native operation outside the current binding snapshot survived")
	}
	detached, err := projectConnectorOperations(base, connectorruntimeauth.ChatCapabilitySnapshot{}, catalog.SurfaceChat)
	if err != nil {
		t.Fatal(err)
	}
	_, projections := splitConnectorNativeToolProjections(detached)
	if len(projections) != 0 {
		t.Fatal("detached base-native operations still became direct candidates")
	}
}

func TestConnectorNativeProjectionSameProviderConnectionSwitchKeepsOneStableToolSet(t *testing.T) {
	snapshot := connectorChatTestSnapshot(uuid.New(), uuid.New(), uuid.New())
	firstConnectionID := snapshot.Capabilities[0].ConnectionID
	base, err := projectConnectorOperations(nil, snapshot, catalog.SurfaceChat)
	if err != nil {
		t.Fatal(err)
	}
	baseDoc, _ := base.Get(skills.SkillConnectorRuntime)

	// The target selects a different saved Key for the same provider. Connection
	// identity remains in the authorized snapshot, not in model-visible schemas.
	snapshot.Capabilities[0].ConnectionID = uuid.New()
	snapshot.Capabilities[0].Definition.NameI18n = catalog.LocalizedText{"zh-Hans": "GitHub"}
	fresh, err := projectConnectorOperations(base, snapshot, catalog.SurfaceChat)
	if err != nil {
		t.Fatal(err)
	}
	freshDoc, _ := fresh.Get(skills.SkillConnectorRuntime)
	// Localized catalog names may improve runtime presentation; switching a
	// connection must not change identities, schemas or operation permissions.
	expected := append([]skills.SkillToolDefinition(nil), baseDoc.Tools...)
	if len(expected) == 1 && len(freshDoc.Tools) == 1 {
		expected[0].RuntimeDescription = freshDoc.Tools[0].RuntimeDescription
	}
	if !reflect.DeepEqual(freshDoc.Tools, expected) || len(freshDoc.Tools) != 1 {
		t.Fatalf("switching the selected Key duplicated or renamed the native tool set: %#v", freshDoc.Tools)
	}
	_, projections := splitConnectorNativeToolProjections(fresh)
	if len(projections) != 1 || snapshot.Capabilities[0].ConnectionID == firstConnectionID {
		t.Fatal("the independently selected connection must keep exactly one operation projection")
	}
}

func TestConnectorNativeProjectionPreservesOrdinaryToolsAndRejectsIdentityCollisions(t *testing.T) {
	snapshot := connectorChatTestSnapshot(uuid.New(), uuid.New(), uuid.New())
	legacy := skills.SkillToolDefinition{
		Name: "ordinary_tool", ProviderType: tools.ToolProviderTypeBuiltin,
		ProviderID:  skills.SkillConnectorRuntime,
		InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}},
	}
	base := &skills.ResolvedSkills{Skills: []skills.SkillDocument{{
		Metadata: skills.SkillMetadata{ID: skills.SkillConnectorRuntime, Tools: []string{legacy.Name}},
		Tools:    []skills.SkillToolDefinition{legacy},
	}}}
	resolved, err := projectConnectorOperations(base, snapshot, catalog.SurfaceChat)
	if err != nil {
		t.Fatal(err)
	}
	projectionResolved, projections := splitConnectorNativeToolProjections(resolved)
	doc, _ := projectionResolved.Get(skills.SkillConnectorRuntime)
	if len(projections) != 1 || !reflect.DeepEqual(doc.Tools, []skills.SkillToolDefinition{legacy}) {
		t.Fatal("direct Connector selection modified the legacy tool contract")
	}
	pruned := restrictConnectorNativeExecutionTools(resolved, skills.NativeToolSet{})
	prunedDoc, _ := pruned.Get(skills.SkillConnectorRuntime)
	if !reflect.DeepEqual(prunedDoc.Tools, []skills.SkillToolDefinition{legacy}) {
		t.Fatal("native execution pruning removed an ordinary/legacy tool")
	}
	base.Skills[0].Tools[0].Name = snapshot.Capabilities[0].Operations[0].ToolName
	if _, err := projectConnectorOperations(base, snapshot, catalog.SurfaceChat); err == nil {
		t.Fatal("native and legacy definitions with the same Skill/tool name did not fail closed")
	}
}
