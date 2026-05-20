package knowledgeretrieval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	goRedis "github.com/redis/go-redis/v9"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/llm"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	datasetmodel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
	redisUtil "github.com/zgiai/zgi/api/pkg/redis"
)

const legacyRateLimitPlan = "default"

func New(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...interface{},
) (shared.NodeInterface, error) {

	nd, nodeID, err := parseKnowledgeRetrievalNodeDataFromConfig(config)
	if err != nil {
		return nil, err
	}

	bns := base.NodeStruct{
		InstanceID: id,
		NodeID:     nodeID,
		NodeType:   shared.KnowledgeRetrieval,

		TenantID:          graphInitParams.TenantID,
		APPID:             graphInitParams.AppID,
		WorkflowType:      string(graphInitParams.WorkflowType),
		WorkflowID:        graphInitParams.WorkflowID,
		UserFrom:          string(graphInitParams.UserFrom),
		UserID:            graphInitParams.UserID,
		GraphConfig:       graphInitParams.GraphConfig,
		InvokeFrom:        string(graphInitParams.InvokeFrom),
		WorkflowCallDepth: graphInitParams.CallDepth,

		Graph:             graph,
		GraphRuntimeState: graphRuntimeState,
		PreviousNodeID:    previousNodeID,
	}

	n := &Node{
		NodeStruct:         bns,
		NodeData:           nd,
		organizationID:     graphInitParams.OrganizationID,
		billingSubjectType: graphInitParams.BillingSubjectType,
		fileOutputs:        []*file.File{},
		llmFileSaver:       llm.NewFileSaverImplGlobal(graphInitParams.UserID, graphInitParams.TenantID),
		db:                 database.GetDB(),
	}

	// Check optionalDeps for LLMClient and GraphFlowService
	for _, dep := range optionalDeps {
		if client, ok := dep.(llmclient.LLMClient); ok {
			n.llmClient = client
			llmInvoker, invErr := NewGatewayLLMInvoker(client, graphInitParams.OrganizationID, graphInitParams.WorkspaceID, graphInitParams.BillingSubjectType)
			if invErr != nil {
				return nil, invErr
			}
			embInvoker, embInvErr := NewGatewayEmbeddingInvoker(client, graphInitParams.OrganizationID, graphInitParams.WorkspaceID, graphInitParams.BillingSubjectType)
			if embInvErr != nil {
				return nil, embInvErr
			}
			n.llmInvoker = llmInvoker
			n.embInvoker = embInvoker
		} else if svc, ok := dep.(*graphflow.Service); ok {
			n.graphFlowService = svc
			logger.Info("GraphFlowService passed to KnowledgeRetrieval node: %s", id)
		}
	}

	return n, nil
}

// parseKnowledgeRetrievalNodeDataFromConfig parses node data and id from config
func parseKnowledgeRetrievalNodeDataFromConfig(config map[string]any) (NodeData, string, error) {
	nodeID, ok := config["id"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID is required")
	}

	nodeIDStr, ok := nodeID.(string)
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID Type is unsupported")
	}

	data, ok := config["data"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data is required")
	}

	// Convert to JSON and back to parse structure into NodeData
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return NodeData{}, "", err
	}

	var nodeData NodeData
	if err := json.Unmarshal(jsonBytes, &nodeData); err != nil {
		return NodeData{}, "", err
	}

	return nodeData, nodeIDStr, nil
}

// Run executes the knowledge retrieval node
func (n *Node) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	// Start event
	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunStarted,
		NodeID:    n.NodeID,
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Execute the LLM logic
	result, err := n.executeRun(ctx, eventChan)
	if err != nil {
		// Send failure event
		select {
		case eventChan <- &shared.NodeEventCh{
			Type:      shared.EventTypeRunFailed,
			NodeID:    n.NodeID,
			Error:     err,
			Timestamp: time.Now(),
		}:
		case <-ctx.Done():
			return ctx.Err()
		}
		return err
	}

	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunCompleted,
		NodeID:    n.NodeID,
		Data:      &shared.RunCompletedEvent{RunResult: result},
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

func (n *Node) executeRun(ctx context.Context, eventChan chan *shared.NodeEventCh) (*shared.NodeRunResult, error) {
	nodeInputs := make(map[string]any)
	resultText := ""
	usage := &shared.LLMUsage{}
	finishReason := ""

	variablePool := n.GraphRuntimeState.VariablePool
	vpSel := n.NodeData.QueryVariableSelector
	val := variablePool.GetWithPath(vpSel)

	if len(vpSel) == 0 {
		return &shared.NodeRunResult{
			Status: shared.FAILED,
			Inputs: nodeInputs,
			ErrMsg: "Query variable selector is empty.",
		}, nil
	}

	if val == nil {
		return &shared.NodeRunResult{
			Status: shared.FAILED,
			Inputs: nodeInputs,
			ErrMsg: "Query variable not found.",
		}, nil
	}

	if val.GetType() != shared.SegmentTypeString {
		return &shared.NodeRunResult{
			Status: shared.FAILED,
			Inputs: nodeInputs,
			ErrMsg: "Query variable is not string type.",
		}, nil
	}

	// Extract query text
	query, ok := val.ToObject().(string)
	if !ok {
		return &shared.NodeRunResult{
			Status: shared.FAILED,
			Inputs: nodeInputs,
			ErrMsg: "Query variable is not string type.",
		}, nil
	}

	queryKey := n.NodeData.QueryVariableSelector[1]
	nodeInputs[queryKey] = query

	if query == "" {
		return &shared.NodeRunResult{
			Status: shared.FAILED,
			Inputs: nodeInputs,
			ErrMsg: "Query is required.",
		}, nil
	}

	// check rate limit
	if isKnowledgeRateLimitEnabled() {
		if limited, err := n.checkAndUpdateKnowledgeRateLimit(ctx); err == nil && limited {
			// Record rate limit violation to database
			if err := n.recordRateLimitLog(ctx); err != nil {
				// Don't fail the request if rate limit logging fails
			}
			return &shared.NodeRunResult{
				Status:  shared.FAILED,
				Inputs:  nodeInputs,
				ErrMsg:  "Sorry, you have reached the knowledge base request rate limit of your subscription.",
				ErrType: "RateLimitExceeded",
			}, nil
		}
	}

	processData := map[string]any{
		"finish_reason": finishReason,
		"usage":         usage,
	}
	outputs := map[string]any{
		//"context":             resultText,
		"result":              resultText, // alias for prompt templates expecting `result`
		"retriever_resources": []*shared.RetrievalSourceMetadata{},
	}
	metadata := map[shared.WorkflowNodeExecutionMetadataKey]any{}

	// 1) Fetch available datasets
	datasets, err := n.fetchAvailableDatasets()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch available datasets: %w", err)
	}

	logger.Info("[KnowledgeRetrieval] Fetched datasets", map[string]interface{}{
		"dataset_count":  len(datasets),
		"embInvoker_nil": n.embInvoker == nil,
		"llmClient_nil":  n.llmClient == nil,
	})

	if len(datasets) == 0 {
		logger.Warn("[KnowledgeRetrieval] No datasets available, returning empty results", nil)
		return &shared.NodeRunResult{
			Status:      shared.SUCCEEDED,
			Inputs:      nodeInputs,
			ProcessData: processData,
			Outputs:     outputs,
			Metadata:    metadata,
			LLMUsage:    usage,
		}, nil
	}

	setsCnt := len(datasets)
	availableSetsIds := make([]string, 0, setsCnt)
	availableSets := make([]*datasetmodel.Dataset, 0, setsCnt)
	for _, d := range datasets {
		if d == nil {
			continue
		}
		availableSets = append(availableSets, d)
		availableSetsIds = append(availableSetsIds, d.ID)
	}

	// 2) Build metadata filter
	mf := NewMetadataFilter(*n, n.TenantID)
	docIdsGroupByDataSetId, cond, err := mf.Build(ctx, availableSetsIds, query, n.GraphRuntimeState.VariablePool)
	if err != nil {
		return nil, fmt.Errorf("failed to build metadata filter: %w", err)
	}

	// 3) Decide planning strategy & fetch model instance

	var docs []DocumentHit
	datasetRetrieval := NewRetrievalServiceWithEmbedding(n.db, n.llmClient, n.embInvoker, n.graphFlowService)
	logger.Info("[KnowledgeRetrieval] Created retrieval service", map[string]interface{}{
		"retrieval_mode": n.NodeData.RetrievalMode,
	})

	if n.NodeData.RetrievalMode == single {
		if n.NodeData.SingleRetrievalConfig == nil {
			return nil, errors.New("single retrieval config is required")
		}

		modelCfg := n.NodeData.SingleRetrievalConfig.ModelConfig
		if modelCfg.Mode == "" {
			modelCfg.Mode = llm.ModeChat
		}

		planning := PlanningStrategyREACTRouter
		if modelCfg.Mode == llm.ModeChat {
			planning = PlanningStrategyRouter
		}

		// Call retrieval service
		srp := SingleRetrieveParams{
			AvailableDatasets:  availableSets,
			TenantID:           n.TenantID,
			OrganizationID:     n.organizationID,
			BillingSubjectType: n.billingSubjectType,
			UserID:             n.UserID,
			AppID:              n.APPID,
			UserFrom:           n.UserFrom,
			Query:              query,
			ModelConfig:        modelCfg,
			Planning:           planning,
			MetadataDocIDs:     docIdsGroupByDataSetId,
			MetadataCond:       cond,
			DatasetIDs:         availableSetsIds,
		}

		docs, err = datasetRetrieval.SingleRetrieve(ctx, srp)
		if err != nil {
			return nil, fmt.Errorf("single retrieve failed: %w", err)
		}
	} else if n.NodeData.RetrievalMode == multiple {
		if n.NodeData.MultipleRetrievalConfig == nil {
			return nil, errors.New("multiple retrieval config is required")
		}

		// Configure reranking parameters
		var rerankingModel map[string]any
		var weights map[string]any

		if n.NodeData.MultipleRetrievalConfig.RerankingMode == RerankingModeModel {
			if n.NodeData.MultipleRetrievalConfig.RerankingModel != nil {
				rerankingModel = map[string]any{
					"reranking_provider_name": n.NodeData.MultipleRetrievalConfig.RerankingModel.Provider,
					"reranking_model_name":    n.NodeData.MultipleRetrievalConfig.RerankingModel.ModelSlug,
				}
			}
		} else if n.NodeData.MultipleRetrievalConfig.RerankingMode == RerankingModeWeightedScore {
			if n.NodeData.MultipleRetrievalConfig.Weights != nil {
				weights = map[string]any{
					"vector_setting": map[string]any{
						"vector_weight":           n.NodeData.MultipleRetrievalConfig.Weights.VectorSetting.VectorWeight,
						"embedding_provider_name": n.NodeData.MultipleRetrievalConfig.Weights.VectorSetting.EmbeddingProviderName,
						"embedding_model_name":    n.NodeData.MultipleRetrievalConfig.Weights.VectorSetting.EmbeddingModelName,
					},
					"keyword_setting": map[string]any{
						"keyword_weight": n.NodeData.MultipleRetrievalConfig.Weights.KeywordSetting.KeywordWeight,
					},
				}
			}
		}

		// Execute multiple retrieval
		mrp := MultipleRetrieveParams{
			TenantID:           n.TenantID,
			OrganizationID:     n.organizationID,
			BillingSubjectType: n.billingSubjectType,
			UserID:             n.UserID,
			AppID:              n.APPID,
			UserFrom:           n.UserFrom,
			Query:              query,
			AvailableDatasets:  availableSets,
			DatasetIDs:         availableSetsIds,
			TopK:               n.NodeData.MultipleRetrievalConfig.TopK,
			ScoreThreshold:     0.0,
			RerankingMode:      n.NodeData.MultipleRetrievalConfig.RerankingMode,
			RerankingModel:     rerankingModel,
			Weights:            weights,
			RerankingEnable:    n.NodeData.MultipleRetrievalConfig.RerankingEnable,
			MetadataDocIDs:     docIdsGroupByDataSetId,
			MetadataCond:       cond,
		}

		if n.NodeData.MultipleRetrievalConfig.ScoreThreshold != nil {
			mrp.ScoreThreshold = *n.NodeData.MultipleRetrievalConfig.ScoreThreshold
		}

		docs, err = datasetRetrieval.MultipleRetrieve(ctx, mrp)
		if err != nil {
			return nil, fmt.Errorf("multiple retrieve failed: %w", err)
		}
	}

	// 5) Format resources and sort by score desc; build context
	retrieverResources, ctxText := n.convertHitsToRetrieverResources(docs)

	// 6) Emit retriever resource event
	select {
	case eventChan <- &shared.NodeEventCh{
		Type:   shared.EventTypeRetrieverResource,
		NodeID: n.NodeID,
		Data: llm.RunRetrieverResourceEventData{
			RetrieverResources: retrieverResources,
			Context:            ctxText,
		},
		Timestamp: time.Now(),
	}:
	default:
	}

	// 7) Write outputs
	//outputs["context"] = ctxText
	outputs["result"] = ctxText
	outputs["retriever_resources"] = retrieverResources

	return &shared.NodeRunResult{
		Status:      shared.SUCCEEDED,
		Inputs:      nodeInputs,
		ProcessData: processData,
		Outputs:     outputs,
		Metadata:    metadata,
		LLMUsage:    usage,
	}, nil
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// isKnowledgeRateLimitEnabled reads the loaded .env configuration.
func isKnowledgeRateLimitEnabled() bool {
	return config.Current().Knowledge.RateLimitEnabled
}

// checkAndUpdateKnowledgeRateLimit enforces a sliding-window rate limit using Redis ZSET per tenant
func (n *Node) checkAndUpdateKnowledgeRateLimit(ctx context.Context) (bool, error) {
	client := redisUtil.GetClient()
	if client == nil {
		return false, nil
	}

	key := fmt.Sprintf("rate_limit_%s", n.TenantID)
	nowMs := time.Now().UnixMilli()

	knowledgeCfg := config.Current().Knowledge
	windowMs := knowledgeCfg.RateLimitWindowMS
	limit := knowledgeCfg.RateLimitMax

	// pipeline: add current, trim old, get count
	pipe := client.Pipeline()
	// Use UUID as member to avoid conflicts from multiple requests in the same millisecond
	uuidStr := uuid.NewString()
	member := fmt.Sprintf("%d_%s", nowMs, uuidStr)
	pipe.ZAdd(ctx, key, goRedis.Z{Score: float64(nowMs), Member: member})
	pipe.ZRemRangeByScore(ctx, key, "-inf", fmt.Sprintf("%d", nowMs-windowMs))
	countCmd := pipe.ZCard(ctx, key)
	pipe.Expire(ctx, key, time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, err
	}
	count := countCmd.Val()
	if count > limit {
		// optional: record a simple log list for visibility
		_ = client.LPush(ctx, fmt.Sprintf("rate_limit_log:%s", n.TenantID), fmt.Sprintf("knowledge|%d", nowMs)).Err()
		return true, nil
	}
	return false, nil
}

// fetchAvailableDatasets queries datasets that are available for retrieval
// Definition: datasets that (a) belong to current tenant and in provided IDs, and
// (b) have at least one available document (completed, enabled, not archived) OR provider = 'external'.
func (n *Node) fetchAvailableDatasets() ([]*datasetmodel.Dataset, error) {
	datasetIDs := n.NodeData.DatasetIds
	if len(datasetIDs) == 0 {
		return []*datasetmodel.Dataset{}, nil
	}

	db := n.db

	// Subquery: count available documents per dataset
	sub := db.Model(&datasetmodel.Document{}).
		Select("dataset_id, COUNT(id) AS available_document_count").
		Where("indexing_status = ? AND enabled = ? AND archived = ? AND dataset_id IN ?",
			datasetmodel.DocumentStatusCompleted, true, false, datasetIDs,
		).
		Group("dataset_id").
		Having("COUNT(id) > 0")

	// GraphInitParams.TenantID is the legacy carrier of the workflow workspace ID.
	// Dataset visibility in the console API is scoped by workspace_id, not organization_id.
	// Filtering by organization_id here incorrectly drops datasets when organization and
	// workspace IDs differ inside the same group.
	var datasets []*datasetmodel.Dataset
	if err := db.Model(&datasetmodel.Dataset{}).
		Joins("LEFT JOIN (?) AS s ON s.dataset_id = datasets.id", sub).
		Where("datasets.workspace_id = ? AND datasets.id IN ?", n.TenantID, datasetIDs).
		Where("(s.available_document_count > 0) OR (datasets.provider = ?)", "external").
		Find(&datasets).Error; err != nil {
		return nil, err
	}
	return datasets, nil
}

// convertHitsToRetrieverResources converts retrieval hits to workflow retriever resources and builds context text
func (n *Node) convertHitsToRetrieverResources(docs []DocumentHit) ([]*shared.RetrievalSourceMetadata, string) {
	// split by provider (external vs internal zgi)
	var internalDocs, externalDocs []DocumentHit
	for _, doc := range docs {
		if doc.Provider == "external" {
			externalDocs = append(externalDocs, doc)
		} else {
			internalDocs = append(internalDocs, doc)
		}
	}

	resources := make([]*shared.RetrievalSourceMetadata, 0, len(docs))
	ctxBuilder := make([]string, 0, len(docs))

	for _, doc := range externalDocs {
		var scorePtr *float64
		if doc.Score != 0 {
			s := doc.Score
			scorePtr = &s
		}
		var datasetID, datasetName, documentID, documentName, title string
		if doc.Metadata != nil {
			if v, ok := doc.Metadata["dataset_id"].(string); ok {
				datasetID = v
			}

			if v, ok := doc.Metadata["dataset_name"].(string); ok {
				datasetName = v
			}

			if v, ok := doc.Metadata["document_id"].(string); ok {
				documentID = v
			}

			if v, ok := doc.Metadata["title"].(string); ok {
				title = v
				if documentID == "" {
					documentID = title
				}
			}

			if v, ok := doc.Metadata["document_name"].(string); ok {
				documentName = v
			} else {
				documentName = title
			}
		}

		if documentID == "" && title != "" {
			documentID = title
		}
		res := &shared.RetrievalSourceMetadata{
			DatasetID:      strPtr(datasetID),
			DatasetName:    strPtr(datasetName),
			DocumentID:     strPtr(documentID),
			DocumentName:   strPtr(documentName),
			DataSourceType: strPtr("external"),
			RetrieverFrom:  strPtr("workflow"),
			Score:          scorePtr,
			Title:          strPtr(title),
			Content:        strPtr(doc.PageContent),
		}

		if doc.Metadata != nil {
			res.DocMetadata = make(map[string]any)
			for k, v := range doc.Metadata {
				res.DocMetadata[k] = v
			}
			res.DocMetadata["_source"] = "knowledge"
			res.DocMetadata["dataset_id"] = datasetID
			res.DocMetadata["dataset_name"] = datasetName
			res.DocMetadata["document_id"] = documentID
			res.DocMetadata["document_name"] = documentName
			res.DocMetadata["data_source_type"] = "external"
			res.DocMetadata["retriever_from"] = "workflow"
			res.DocMetadata["score"] = doc.Score
		}
		resources = append(resources, res)
		if doc.PageContent != "" {
			ctxBuilder = append(ctxBuilder, doc.PageContent)
		}
	}

	for _, doc := range internalDocs {
		var scorePtr *float64
		if doc.Score != 0 {
			s := doc.Score
			scorePtr = &s
		}
		var datasetID, datasetName, documentID, documentName, dataSourceType, segmentID, title string
		var segmentPosition, hitCount, wordCount int
		var indexNodeHash string
		var hasIndexHash bool
		var childChunks []map[string]any

		if doc.Metadata != nil {
			if v, ok := doc.Metadata["dataset_id"].(string); ok {
				datasetID = v
			}
			if v, ok := doc.Metadata["dataset_name"].(string); ok {
				datasetName = v
			}
			if v, ok := doc.Metadata["document_id"].(string); ok {
				documentID = v
			}
			if v, ok := doc.Metadata["document_name"].(string); ok {
				documentName = v
			}
			if v, ok := doc.Metadata["data_source_type"].(string); ok {
				dataSourceType = v
			}
			if v, ok := doc.Metadata["segment_id"].(string); ok {
				segmentID = v
			}
			if v, ok := doc.Metadata["segment_position"].(int); ok {
				segmentPosition = v
			}
			if v, ok := doc.Metadata["segment_hit_count"].(int); ok {
				hitCount = v
			}
			if v, ok := doc.Metadata["segment_word_count"].(int); ok {
				wordCount = v
			}
			if v, ok := doc.Metadata["segment_index_node_hash"].(string); ok {
				indexNodeHash = v
				hasIndexHash = true
			}
			if v, ok := doc.Metadata["title"].(string); ok {
				title = v
			}
			if v, ok := doc.Metadata["child_chunks"].([]map[string]any); ok {
				childChunks = v
			}
		}
		res := &shared.RetrievalSourceMetadata{
			DatasetID:      strPtr(datasetID),
			DatasetName:    strPtr(datasetName),
			DocumentID:     strPtr(documentID),
			DocumentName:   strPtr(documentName),
			DataSourceType: strPtr(dataSourceType),
			SegmentID:      strPtr(segmentID),
			RetrieverFrom:  strPtr("workflow"),
			Score:          scorePtr,
			Title:          strPtr(title),
			Content:        strPtr(doc.PageContent),
		}
		if segmentPosition != 0 {
			v := segmentPosition
			res.SegmentPosition = &v
		}
		if hitCount != 0 {
			v := hitCount
			res.HitCount = &v
		}
		if wordCount != 0 {
			v := wordCount
			res.WordCount = &v
		}
		if hasIndexHash {
			res.IndexNodeHash = &indexNodeHash
		}
		if doc.Metadata != nil {
			res.DocMetadata = make(map[string]any)

			// Add standard fields
			res.DocMetadata["_source"] = "knowledge"
			res.DocMetadata["dataset_id"] = datasetID
			res.DocMetadata["dataset_name"] = datasetName
			res.DocMetadata["document_id"] = documentID
			res.DocMetadata["document_name"] = documentName
			res.DocMetadata["data_source_type"] = dataSourceType
			res.DocMetadata["segment_id"] = segmentID
			res.DocMetadata["retriever_from"] = "workflow"
			res.DocMetadata["score"] = doc.Score

			// Add segment-related fields
			if childChunks != nil {
				res.DocMetadata["child_chunks"] = childChunks
			}
			res.DocMetadata["segment_hit_count"] = hitCount
			res.DocMetadata["segment_word_count"] = wordCount
			res.DocMetadata["segment_position"] = segmentPosition
			if hasIndexHash {
				res.DocMetadata["segment_index_node_hash"] = indexNodeHash
			}

			// Add document metadata
			if dm, ok := doc.Metadata["doc_metadata"].(map[string]any); ok {
				res.DocMetadata["doc_metadata"] = dm
			}
		}
		resources = append(resources, res)
		if doc.PageContent != "" {
			ctxBuilder = append(ctxBuilder, doc.PageContent)
		}
	}
	// Sort results
	if len(resources) > 0 {
		sort.Slice(resources, func(i, j int) bool {
			scoreI := 0.0
			scoreJ := 0.0

			if resources[i].Score != nil {
				scoreI = *resources[i].Score
			}
			if resources[j].Score != nil {
				scoreJ = *resources[j].Score
			}

			return scoreI > scoreJ // Descending order
		})

		// Add position information
		for position, resource := range resources {
			if resource.DocMetadata == nil {
				resource.DocMetadata = make(map[string]any)
			}
			resource.DocMetadata["position"] = position + 1 // Start counting from 1
		}
	}

	sep := "\n"
	maxChars := 0
	if n.NodeData.SingleRetrievalConfig != nil {
		if n.NodeData.SingleRetrievalConfig.ContextSeparator != "" {
			sep = n.NodeData.SingleRetrievalConfig.ContextSeparator
		}
		if n.NodeData.SingleRetrievalConfig.MaxContextChars > 0 {
			maxChars = n.NodeData.SingleRetrievalConfig.MaxContextChars
		}
	}
	contextText := ""
	for i, s := range ctxBuilder {
		if i == 0 {
			contextText = s
		} else {
			contextText += sep + s
		}
		if maxChars > 0 && len(contextText) >= maxChars {
			if len(contextText) > maxChars {
				contextText = contextText[:maxChars]
			}
			break
		}
	}
	return resources, contextText
}

// recordRateLimitLog records a rate limit violation to the database
func (n *Node) recordRateLimitLog(ctx context.Context) error {
	// subscription_plan is a legacy non-null column kept for existing schemas.
	rateLimitLog := &sharedmodel.RateLimitLog{
		TenantID:         n.TenantID,
		SubscriptionPlan: legacyRateLimitPlan,
		Operation:        "knowledge",
		CreatedAt:        time.Now(),
	}

	// Save to database
	if err := n.db.WithContext(ctx).Create(rateLimitLog).Error; err != nil {
		return fmt.Errorf("failed to create rate limit log: %w", err)
	}

	return nil
}
