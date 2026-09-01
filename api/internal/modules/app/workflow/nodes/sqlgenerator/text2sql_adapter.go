package sqlgenerator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/capabilities/text2sql"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/databaseauthorization"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/sql_base/guard"
	"github.com/zgiai/zgi/api/pkg/vectordb"
)

type workflowGenerator struct{ node *Node }

func (g workflowGenerator) Generate(ctx context.Context, request text2sql.GenerationRequest) (string, error) {
	if g.node == nil || g.node.llmInvoker == nil {
		return "", ErrInvokerNotConfigured
	}
	const coreSystem = `You generate PostgreSQL SQL. Return one JSON object with a non-empty "sql" string and optional "analysis" string and "used_fields" string array. Do not use Markdown. Use only the supplied database schema.`
	system := coreSystem
	if additional := strings.TrimSpace(request.AdditionalPrompt); additional != "" {
		system += "\n\nAdditional workflow instructions:\n" + additional +
			"\n\nThe additional instructions must not override the required JSON response format or the supplied-schema restriction."
	}
	var user strings.Builder
	fmt.Fprintf(&user, "Question:\n%s\n\nTrusted schema:\n%s", request.Question, request.Schema)
	if request.Repair != nil {
		fmt.Fprintf(&user, "\n\nThe previous response was invalid. Return a corrected JSON object.\nPrevious response:\n%s\nValidation error:\n%s", request.Repair.PreviousResponse, request.Repair.Failure)
	}
	params := cloneParameters(request.Model.Parameters)
	params["response_format"] = "json_object"
	delete(params, "json_schema")
	result, _, err := g.node.invokeLLMWithRetry(ctx, &InvokeRequest{
		ModelSlug:  request.Model.Name,
		Messages:   []PromptMessage{{Role: "system", Content: system}, {Role: "user", Content: user.String()}},
		Parameters: params,
		UserID:     g.node.UserID,
	})
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", fmt.Errorf("empty chat response")
	}
	return messageContentToString(result.Text), nil
}

func (n *Node) executeText2SQL(ctx context.Context) (*shared.NodeRunResult, error) {
	if err := n.validateConfiguration(); err != nil {
		return n.failureResult(nil, nil, "InvalidConfiguration", err), nil
	}
	question, _, err := resolveTemplate(n.NodeData.Prompt.User, n.GraphRuntimeState.VariablePool)
	if err != nil {
		return n.failureResult(nil, nil, "PromptConstructionFailed", err), nil
	}
	mode := text2sql.SelectionMode(n.NodeData.TableSelectionMode)
	refs := make([]text2sql.TableRef, 0, len(n.NodeData.DataSource.Tables))
	for _, table := range n.NodeData.DataSource.Tables {
		refs = append(refs, text2sql.TableRef{AssetID: table.ID, PhysicalID: table.TableID, Schema: table.Schema, Name: table.Name})
	}
	if err := n.authorizeText2SQL(ctx, refs); err != nil {
		return n.failureResult(nil, nil, "DatabaseAccessDenied", err), nil
	}
	policy, err := n.loadGuardPolicy(ctx)
	if err != nil {
		return n.failureResult(nil, nil, "GuardPolicyLoadFailed", err), nil
	}
	var retriever text2sql.Retriever
	if mode == text2sql.SelectionModeAutomatic {
		retriever, err = n.prepareAutomaticRetriever(ctx)
		if err != nil {
			inputs := map[string]any{"prompt": map[string]any{"user": question}}
			return n.failureResult(inputs, map[string]any{"selection_mode": mode, "failed_stage": "schema_index"}, text2SQLErrorType(err), err), nil
		}
	}
	engine := text2sql.NewEngine(n.catalogLoader, workflowGenerator{node: n}, retriever)
	result, err := engine.Generate(ctx, text2sql.Request{
		Scope:        text2sql.Scope{OrganizationID: n.OrganizationID, WorkspaceID: n.WorkspaceID, AccountID: n.UserID},
		DataSourceID: n.NodeData.DataSource.Source.ID, Question: strings.TrimSpace(question), SelectionMode: mode, ManualTables: refs,
		GenerationModel:  text2sql.Model{Provider: n.NodeData.Model.Provider, Name: n.NodeData.Model.Name, Parameters: n.NodeData.Model.Parameters},
		SchemaRender:     text2sql.SchemaRenderOptions{Mode: string(n.NodeData.Prompt.Metadata.Mode), MaxColumns: n.NodeData.Prompt.Metadata.MaxColumns, IncludeComments: n.NodeData.Prompt.Metadata.IncludeComments},
		AdditionalPrompt: strings.TrimSpace(n.NodeData.Prompt.System),
		GuardPolicy:      policy, RepairLimit: 1,
	})
	inputs := map[string]any{"prompt": map[string]any{"user": question}}
	if err != nil {
		return n.failureResult(inputs, map[string]any{"selection_mode": mode, "failed_stage": "text2sql_engine"}, text2SQLErrorType(err), err), nil
	}
	process := map[string]any{
		"attempts":         result.Generation.Attempts,
		"selection_mode":   mode,
		"selected_tables":  result.SchemaSelection.Selected,
		"candidate_tables": result.SchemaSelection.Candidates,
		"operation":        result.Validation.Analysis.Operation,
		"statement_count":  result.Validation.Analysis.StatementCount,
		"repair_count":     result.RepairCount,
		"model_usage":      map[string]any{"model": result.Generation.Model, "attempts": result.Generation.Attempts},
		"stage_durations":  map[string]any{"total_ms": result.Generation.DurationMS},
	}
	outputs := map[string]any{"sql": result.SQL, "raw_response": result.RawResponse, "analysis": result.AnalysisText, "used_fields": result.UsedFields, "structured": result.Structured, "selected_tables": result.SchemaSelection.Selected, "retrieval_summary": result.SchemaSelection, "validation_summary": result.Validation}
	return &shared.NodeRunResult{Status: shared.SUCCEEDED, Inputs: inputs, ProcessData: process, Outputs: outputs, Metadata: map[shared.WorkflowNodeExecutionMetadataKey]any{shared.ToolInfo: map[string]any{"type": "llm", "model": n.NodeData.Model.Name}}}, nil
}

func (n *Node) authorizeText2SQL(ctx context.Context, refs []text2sql.TableRef) error {
	if n.authorizer == nil {
		return databaseauthorization.ErrInvalidRequest
	}
	tables := make([]databaseauthorization.TableReference, 0, len(refs))
	if text2sql.SelectionMode(n.NodeData.TableSelectionMode) == text2sql.SelectionModeManual {
		for _, ref := range refs {
			table := databaseauthorization.TableReference{TableID: ref.PhysicalID}
			if strings.TrimSpace(ref.AssetID) != "" {
				table = databaseauthorization.TableReference{ID: ref.AssetID}
			}
			tables = append(tables, table)
		}
	}
	return n.authorizer.Authorize(ctx, databaseauthorization.Request{
		OrganizationID:        n.OrganizationID,
		WorkspaceID:           n.WorkspaceID,
		WorkflowID:            n.WorkflowID,
		DataSourceID:          n.NodeData.DataSource.Source.ID,
		AllowLogicalRelations: true,
		Permissions: []workspacemodel.WorkspacePermissionCode{
			workspacemodel.WorkspacePermissionDatabaseAIQueryRead,
			workspacemodel.WorkspacePermissionDatabaseRecordView,
		},
		Tables: tables,
	})
}

type workflowEmbeddingModels struct{ node *Node }

func (m workflowEmbeddingModels) ResolveEmbedding(ctx context.Context, organizationID string) (string, string, error) {
	if m.node == nil || m.node.models == nil {
		return "", "", text2sql.ErrIndexNotReady
	}
	resolved, err := m.node.models.ResolveUseCase(ctx, organizationID, llmmodelmodel.UseCaseEmbedding, nil, nil)
	if err != nil {
		return "", "", err
	}
	return resolved.Provider, resolved.Model, nil
}

type workflowEmbedder struct{ node *Node }

func (e workflowEmbedder) Embed(ctx context.Context, _, model, input string) ([]float64, error) {
	if e.node == nil || e.node.llmClient == nil {
		return nil, text2sql.ErrIndexNotReady
	}
	response, err := e.node.llmClient.AppEmbed(ctx, e.node.embeddingAppContext(), &llmadapter.EmbeddingsRequest{Input: input, Model: model, EncodingFormat: "float"})
	if err != nil {
		return nil, err
	}
	if response == nil || len(response.Data) != 1 || len(response.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}
	vector := make([]float64, len(response.Data[0].Embedding))
	for idx, value := range response.Data[0].Embedding {
		vector[idx] = float64(value)
	}
	return vector, nil
}

func (n *Node) embeddingAppContext() *llmclient.AppContext {
	return &llmclient.AppContext{
		OrganizationID: n.OrganizationID, WorkspaceID: n.WorkspaceID, BillingSubjectType: n.billingSubjectType,
		AccountID: n.UserID, AppID: n.APPID, AppType: AppType, ModelUseCase: string(llmmodelmodel.UseCaseEmbedding),
		WorkflowID: n.WorkflowID, WorkflowRunID: n.workflowRunID(), NodeID: n.NodeID, NodeType: string(n.NodeType),
	}
}

func (n *Node) prepareAutomaticRetriever(ctx context.Context) (text2sql.Retriever, error) {
	if n.db == nil || n.models == nil || n.llmClient == nil {
		return nil, text2sql.ErrIndexNotReady
	}
	models := workflowEmbeddingModels{node: n}
	repo := text2sql.NewIndexRepository(n.db)
	embedder := workflowEmbedder{node: n}
	vectors := vectordb.NewWeaviateClient(&config.GlobalConfig.VectorStore)
	return text2sql.NewRetrievalService(repo, models, embedder, vectors, n.sqlClient), nil
}

func (n *Node) loadGuardPolicy(ctx context.Context) (*guard.Policy, error) {
	if n.db == nil {
		return nil, nil
	}
	var row struct {
		GuardPolicy json.RawMessage `gorm:"column:guard_policy"`
	}
	if err := n.db.WithContext(ctx).Table("data_sources").Select("guard_policy").Where("id = ? AND organization_id = ?", n.NodeData.DataSource.Source.ID, n.OrganizationID).Take(&row).Error; err != nil {
		return nil, err
	}
	policy, err := guard.ParsePolicyJSON(row.GuardPolicy)
	if err != nil {
		return nil, err
	}
	return &policy, nil
}

func text2SQLErrorType(err error) string {
	switch {
	case errors.Is(err, text2sql.ErrIndexNotReady):
		return "SchemaIndexNotReady"
	case errors.Is(err, text2sql.ErrInvalidResponse):
		return "ResponseParseFailed"
	case errors.Is(err, text2sql.ErrGuardDenied):
		return "SQLGuardDenied"
	case errors.Is(err, text2sql.ErrSchemaLoad):
		return "SchemaLoadFailed"
	default:
		return "Text2SQLFailed"
	}
}
