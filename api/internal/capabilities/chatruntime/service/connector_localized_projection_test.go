package service

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorruntimeauth "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestConnectorLocalizedRealCatalogRanksBusinessNames(t *testing.T) {
	for _, surface := range []catalog.Surface{catalog.SurfaceChat, catalog.SurfaceAgent} {
		resolved, snapshot := connectorLocalizedCatalogSnapshot(t, surface)
		_, projections := splitConnectorNativeToolProjections(resolved)
		for _, test := range []struct{ query, operation string }{
			{"按姓名查找用户", "feishu.contact.search"},
			{"飞书按姓名查找用户", "feishu.contact.search"},
			{"Feishu 按姓名查找用户", "feishu.contact.search"},
			{"Search employee names", "feishu.contact.search"},
			{"发送文本消息", "feishu.message.send"},
			{"获取企业信息", "feishu.tenant.get"},
			{"Get tenant information", "feishu.tenant.get"},
			{"飞书查询群成员", "feishu.chat.members.list"},
			{"Feishu 查询群成员", "feishu.chat.members.list"},
			{"List chat members", "feishu.chat.members.list"},
			{"搜索网页", "web.search"},
			{"搜索 GitHub 仓库", "github.repository.search"},
		} {
			t.Run(string(surface)+"/"+test.query, func(t *testing.T) {
				ranked := skills.RankNativeToolProjections(projections, test.query)
				if got := ranked[0].Binding.ConnectorOperation.OperationID; got != test.operation {
					t.Fatalf("query %q ranked %s first, want %s", test.query, got, test.operation)
				}
			})
		}
		ranked := skills.RankNativeToolProjections(projections, "发送消息")
		positions := map[string]int{}
		for i, projection := range ranked {
			positions[projection.Binding.ConnectorOperation.OperationID] = i
		}
		if positions["feishu.message.send"] >= positions["feishu.contact.search"] {
			t.Fatal("the contact search's negative 'does not send' description outranked the sending operation")
		}
		if got := skills.RankNativeToolProjections(projections, " \t\n"); !reflect.DeepEqual(got, projections) {
			t.Fatal("empty query changed existing catalog order or native bindings")
		}
		for _, capability := range snapshot.Capabilities {
			for _, operation := range capability.Operations {
				entity, err := connectortools.OperationEntity(capability.Definition, operation.ID)
				if err != nil {
					t.Fatal(err)
				}
				for _, projection := range projections {
					if projection.Binding.ConnectorOperation.OperationID == operation.ID && projection.Description != entity.Description.LLM {
						t.Fatalf("%s: Chat/Agent and ToolEntity model descriptions differ", operation.ID)
					}
				}
			}
		}
	}
}

func TestConnectorLocalizedProjectionBothPathsRespectBudgetAndAliases(t *testing.T) {
	for _, surface := range []catalog.Surface{catalog.SurfaceChat, catalog.SurfaceAgent} {
		resolved, _ := connectorLocalizedCatalogSnapshot(t, surface)
		projectionResolved, candidates := splitConnectorNativeToolProjections(resolved)
		for _, requested := range []struct{ query, operation, toolName string }{
			{"按姓名查找用户", "feishu.contact.search", "feishu_contact_search"},
			{"获取企业信息", "feishu.tenant.get", "feishu_tenant_get"},
			{"查询群成员", "feishu.chat.members.list", "feishu_chat_members_list"},
		} {
			projections := skills.RankNativeToolProjections(candidates, requested.query)
			for _, options := range []skills.NativeToolSetOptions{
				{BudgetChars: len([]rune(connectorNativeSkillInstructions)) + 2500},
				{BudgetChars: 100000, BudgetTokens: 2, EstimateTokens: func(messages []adapter.Message, tools []adapter.Tool) int {
					// Deterministic mock: one instruction/tool consumes one unit. The
					// production tokenizer and existing token-budget algorithm stay intact.
					return len(messages) + len(tools)
				}},
			} {
				for _, progressive := range []bool{false, true} {
					runtime := skills.NewRuntime(nil, nil)
					reserved := []string{requested.toolName}
					var set skills.NativeToolSet
					if progressive {
						session := skills.NewNativeSkillSession(runtime, projectionResolved,
							skills.BuildNativeSkillCatalog(resolved, nil, 8000, 0, nil), options)
						session.Activate(context.Background(), []string{skills.SkillConnectorRuntime}, "runtime_preload")
						projectNativeDirectToolsToSession(session, projections, reserved, options.EstimateTokens)
						set = session.ToolSet()
					} else {
						set = runtime.BuildNativeToolSet(context.Background(), projectionResolved, options)
						projectNativeDirectTools(&set, projections, reserved, options.EstimateTokens)
					}
					if len(set.ProviderTools) == 0 || len(set.ProviderTools) >= len(projections) || len(set.SkippedTools) == 0 {
						t.Fatalf("surface=%s progressive=%t: budget did not select a bounded subset: tools=%d skipped=%#v", surface, progressive, len(set.ProviderTools), set.SkippedTools)
					}
					first := set.ProviderTools[0].Function
					binding := set.ToolBindings[first.Name]
					if binding.ConnectorOperation == nil || binding.ConnectorOperation.OperationID != requested.operation {
						t.Fatalf("surface=%s progressive=%t: tight budget lost relevant operation: %#v", surface, progressive, binding)
					}
					if first.Name == requested.toolName || !strings.Contains(first.Description, requested.query) {
						t.Fatalf("reserved-name alias or Chinese model description was lost: %#v", first)
					}
					if set.InstructionChars+set.SchemaChars > options.BudgetChars {
						t.Fatal("localized discovery bypassed the shared character budget")
					}
					if options.BudgetTokens > 0 && (set.InstructionTokens+set.SchemaTokens > options.BudgetTokens || len(set.ProviderTools) != 1) {
						t.Fatal("localized discovery bypassed the shared token budget")
					}
					pruned := restrictConnectorNativeExecutionTools(resolved, set)
					_, available := splitConnectorNativeToolProjections(pruned)
					if len(available) != len(set.ProviderTools) {
						t.Fatal("budget-pruned operations remained executable by their original names")
					}
				}
			}
		}
	}
}

func TestConnectorLocalizedProjectionCannotAddUnboundActionsOrChangeOrdinarySkills(t *testing.T) {
	ordinary := skills.SkillDocument{
		Metadata:     skills.SkillMetadata{ID: "ordinary", Name: "普通技能", Description: "Do not change this description."},
		Instructions: "Keep ordinary instructions unchanged.",
		Tools: []skills.SkillToolDefinition{{Name: "ordinary_tool", RuntimeDescription: "Ordinary tool description.",
			InputSchema: map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}}},
	}
	base := &skills.ResolvedSkills{Skills: []skills.SkillDocument{ordinary}}
	withoutConnectors, err := projectConnectorOperations(base, connectorruntimeauth.ChatCapabilitySnapshot{}, catalog.SurfaceChat)
	if err != nil || !reflect.DeepEqual(withoutConnectors, base) {
		t.Fatalf("absence of Connector bindings changed an ordinary Skill: %v", err)
	}
	_, snapshot := connectorLocalizedCatalogSnapshot(t, catalog.SurfaceChat)
	for index := range snapshot.Capabilities {
		capability := &snapshot.Capabilities[index]
		allowed := []catalog.Operation{}
		for _, operation := range capability.Operations {
			if operation.ID == "feishu.contact.search" {
				allowed = append(allowed, operation)
			}
		}
		// The public catalog still contains sending and all other actions, but
		// this target's live intersection grants only the single search action.
		capability.Operations = allowed
	}
	withConnector, err := projectConnectorOperations(base, snapshot, catalog.SurfaceChat)
	if err != nil {
		t.Fatal(err)
	}
	gotOrdinary, _ := withConnector.Get("ordinary")
	if !reflect.DeepEqual(*gotOrdinary, ordinary) || !reflect.DeepEqual(base.Skills[0], ordinary) {
		t.Fatal("localized Connector names affected ordinary Skill metadata or instructions")
	}
	_, projections := splitConnectorNativeToolProjections(withConnector)
	for _, query := range []string{"发送文本消息", "获取企业信息", "查询群成员"} {
		ranked := skills.RankNativeToolProjections(projections, query)
		if len(ranked) != 1 || ranked[0].Binding.ConnectorOperation.OperationID != "feishu.contact.search" {
			t.Fatalf("Chinese query %q manufactured an action outside the authorized snapshot", query)
		}
	}
}

func TestConnectorLocalizedRankingPreservesPinnedContinuationPriority(t *testing.T) {
	resolved, _ := connectorLocalizedCatalogSnapshot(t, catalog.SurfaceChat)
	_, projections := splitConnectorNativeToolProjections(resolved)
	projections[0].Binding.Pinned = true
	operationID := projections[0].Binding.ConnectorOperation.OperationID
	ranked := skills.RankNativeToolProjections(projections, "按姓名查找用户")
	if ranked[0].Binding.ConnectorOperation.OperationID != operationID || !ranked[0].Binding.Pinned {
		t.Fatal("Chinese relevance displaced a continuation-pinned operation")
	}
}

func connectorLocalizedCatalogSnapshot(t *testing.T, surface catalog.Surface) (*skills.ResolvedSkills, connectorruntimeauth.ChatCapabilitySnapshot) {
	t.Helper()
	definitions, err := catalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	snapshot := connectorruntimeauth.ChatCapabilitySnapshot{}
	for _, definition := range definitions {
		// Only built-ins that support the requested surface belong in a
		// surface-scoped capability snapshot. The DigitalHuman-only SSH
		// contract is projected by its own DigitalHuman host instead.
		if !definition.Supports(surface) {
			continue
		}
		snapshot.Capabilities = append(snapshot.Capabilities, connectorruntimeauth.ChatCapability{
			BindingID: uuid.New(), ConnectionID: uuid.New(), ConnectorID: core.Identifier(definition.ID),
			Definition: definition, Operations: definition.Operations,
		})
	}
	resolved, err := projectConnectorOperations(nil, snapshot, surface)
	if err != nil {
		t.Fatal(err)
	}
	return resolved, snapshot
}
