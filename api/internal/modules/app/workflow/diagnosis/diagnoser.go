package diagnosis

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/ginext/internal/modules/llm/client"
	adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/ginext/internal/prompt"
	"github.com/zgiai/ginext/pkg/logger"
)

// DiagnoseContext carries all possible context sources for diagnosis
type DiagnoseContext struct {
	ErrCtx        *ErrorContext
	Engine        *graph_engine.WorkflowEngine
	PredecessorID string
	Lang          string
	// Fallback maps from handler (streaming path)
	NodeMap          map[string]map[string]interface{}
	ExecutionOutputs map[string]map[string]interface{}
}

// Diagnoser orchestrates the diagnostic logic
type Diagnoser struct {
	classifier *Classifier
	extractor  *Extractor
	cache      *Cache
	llmClient  client.LLMClient
}

// NewDiagnoser initializes a new Diagnoser module
func NewDiagnoser(ctx context.Context, llmClient client.LLMClient) *Diagnoser {
	return &Diagnoser{
		classifier: NewClassifier(),
		extractor:  NewExtractor(),
		cache:      NewCache(ctx, 5*time.Minute),
		llmClient:  llmClient,
	}
}

// extractErrorContext populates the rest of ErrorContext from available sources
func (d *Diagnoser) extractErrorContext(ctx context.Context, dc *DiagnoseContext) {
	// If already populated (e.g., from DB during manual diagnosis), do not overwrite
	if dc.ErrCtx.NodeConfig != nil {
		return
	}

	var nodeConfig map[string]any
	var upstreamContexts map[string]UpstreamNodeContext

	// Try engine first
	if dc.Engine != nil {
		nodeConfig, upstreamContexts = d.extractor.ExtractFromEngine(dc.Engine, dc.ErrCtx.NodeID, dc.PredecessorID)
	}

	// Fall back to handler maps if engine extraction failed or was missing
	if nodeConfig == nil && dc.NodeMap != nil {
		nodeConfig, upstreamContexts = d.extractor.ExtractFromMaps(dc.ErrCtx.NodeID, dc.PredecessorID, dc.NodeMap, dc.ExecutionOutputs)
	}

	if nodeConfig != nil {
		dc.ErrCtx.NodeConfig = nodeConfig
	}
	if len(upstreamContexts) > 0 {
		dc.ErrCtx.UpstreamContexts = upstreamContexts
	}

	// Sanitize snapshot
	dc.ErrCtx.InputSnapshot = SanitizeInputSnapshot(dc.ErrCtx.InputSnapshot)
}

// ExtractResult only captures the diagnostic snapshots and classifies the error without calling LLM
func (d *Diagnoser) ExtractResult(ctx context.Context, dc *DiagnoseContext) DiagnosisResult {
	start := time.Now()

	// 1. Setup default results
	res := DiagnosisResult{
		ResultText:      "",
		IsDiagnosed:     false,
		ModelUsed:       "none",
		NodeYAML:        "{}",
		UpstreamYAML:    "{}",
		InputSnapshot:   "{}",
		UpstreamOutputs: "{}",
	}

	// 2. Classify rule-based
	errType, _, _ := d.classifier.Classify(dc.ErrCtx.ErrorMessage)
	dc.ErrCtx.ErrorType = errType

	// 3. Extract rich context
	d.extractErrorContext(ctx, dc)

	// 4. Populate snapshots into result
	d.populateSnapshots(&res, dc.ErrCtx)

	res.LatencyMs = int(time.Since(start).Milliseconds())
	return res
}

func (d *Diagnoser) populateSnapshots(res *DiagnosisResult, errCtx *ErrorContext) {
	// Set raw context snapshots for persistence as JSON
	if b, err := json.Marshal(errCtx.NodeConfig); err == nil {
		res.NodeYAML = string(b)
	}

	logger.Info("populateSnapshots InputSnapshot serialization", "inputMapLen", len(errCtx.InputSnapshot))
	if b, err := json.Marshal(errCtx.InputSnapshot); err == nil {
		res.InputSnapshot = string(b)
		logger.Info("populateSnapshots InputSnapshot success", "jsonLen", len(b), "jsonStr", string(b))
	} else {
		logger.Error("populateSnapshots InputSnapshot failed to serialize", err)
	}

	// Build upstream config and output maps for JSON storage
	upstreamConfigs := make(map[string]any)
	upstreamOutputs := make(map[string]any)
	for id, ctx := range errCtx.UpstreamContexts {
		if ctx.Config != nil {
			upstreamConfigs[id] = ctx.Config
		}
		if ctx.Output != nil {
			upstreamOutputs[id] = ctx.Output
		}
	}
	if b, err := json.Marshal(upstreamConfigs); err == nil {
		res.UpstreamYAML = string(b)
	}
	if b, err := json.Marshal(upstreamOutputs); err == nil {
		res.UpstreamOutputs = string(b)
	}
}

// Diagnose executes the end-to-end diagnostic process
func (d *Diagnoser) Diagnose(ctx context.Context, dc *DiagnoseContext, modelOverride ...string) DiagnosisResult {
	start := time.Now()

	// 1. Extract context first if not already done
	res := d.ExtractResult(ctx, dc)

	// 2. Check if rule-based classifier already handled it in ExtractResult
	// We can use the error type determined in step 1
	errType, staticMsg, handled := d.classifier.Classify(dc.ErrCtx.ErrorMessage)
	if handled {
		logger.Info("Diagnosis rule-based match", "nodeType", dc.ErrCtx.NodeType, "errorType", errType, "result", staticMsg)
		res.ResultText = staticMsg
		res.IsDiagnosed = true
		res.LatencyMs = int(time.Since(start).Milliseconds())
		return res
	}

	// 3. Rate limiting and Caching (Skip if model is overridden)
	if len(modelOverride) == 0 {
		if cachedRes, hit := d.cache.Record(dc.ErrCtx.NodeType, errType); hit && cachedRes != nil {
			logger.Info("Diagnosis cache hit for NodeType " + dc.ErrCtx.NodeType + " ErrorType " + string(errType))
			cachedRes.LatencyMs = int(time.Since(start).Milliseconds())
			return *cachedRes
		}
	}

	// 4. LLM Call
	sysPrompt, err := d.buildSystemPrompt()
	if err != nil {
		logger.Error("Failed to build diagnosis system prompt", err)
		return res
	}

	userPrompt, err := d.buildUserPrompt(dc.ErrCtx, dc.Lang)
	if err != nil {
		logger.Error("Failed to build diagnosis user prompt", err)
		return res
	}

	model := ""
	if len(modelOverride) > 0 {
		model = modelOverride[0]
	}

	d.callLLM(ctx, dc.ErrCtx, sysPrompt, userPrompt, &res, start, model)

	// Cache successful result
	if res.IsDiagnosed && len(modelOverride) == 0 {
		d.cache.SaveResult(dc.ErrCtx.NodeType, errType, res)
	}

	return res
}

func (d *Diagnoser) buildSystemPrompt() (string, error) {
	tmpl, err := prompt.GetTemplate(prompt.WorkflowDiagnosisSystem)
	if err != nil {
		return "", err
	}
	return tmpl.Render(nil)
}

func (d *Diagnoser) buildUserPrompt(errCtx *ErrorContext, lang string) (string, error) {
	tmplID := prompt.WorkflowDiagnosisUserEn
	if strings.HasPrefix(strings.ToLower(lang), "zh") {
		tmplID = prompt.WorkflowDiagnosisUserZh
	}

	tmpl, err := prompt.GetTemplate(tmplID)
	if err != nil {
		return "", err
	}

	// Build a template-friendly data struct with serialized maps
	data := struct {
		NodeName         string
		ErrorMessage     string
		ErrorType        string
		NodeType         string
		NodeYAML         string
		InputSnapshot    string
		UpstreamContexts map[string]struct {
			YAML   string
			Output string
		}
	}{
		NodeName:     errCtx.NodeName,
		ErrorMessage: errCtx.ErrorMessage,
		ErrorType:    string(errCtx.ErrorType),
		NodeType:     errCtx.NodeType,
		NodeYAML:     MapToYAML(errCtx.NodeConfig),
	}

	// Serialize InputSnapshot to readable YAML for prompt
	if errCtx.InputSnapshot != nil && len(errCtx.InputSnapshot) > 0 {
		data.InputSnapshot = MapToYAML(errCtx.InputSnapshot)
	}

	// Convert UpstreamContexts to prompt-friendly format
	data.UpstreamContexts = make(map[string]struct {
		YAML   string
		Output string
	})
	for id, ctx := range errCtx.UpstreamContexts {
		data.UpstreamContexts[id] = struct {
			YAML   string
			Output string
		}{
			YAML:   MapToYAML(ctx.Config),
			Output: MapToYAML(ctx.Output),
		}
	}

	return tmpl.Render(data)
}

func (d *Diagnoser) callLLM(ctx context.Context, errCtx *ErrorContext, sysPrompt, userPrompt string, res *DiagnosisResult, start time.Time, modelOverride string) {
	if d.llmClient == nil {
		res.ResultText = "LLM settings are not available to diagnose this issue."
		return
	}

	// Use centralized config for model selection if not overridden
	modelName := "gpt-4o-mini"
	if modelOverride != "" {
		modelName = modelOverride
	}
	res.ModelUsed = modelName

	timeout := 100 * time.Second
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	temperature := 0.1
	req := &adapter.ChatRequest{
		Model: modelName,
		Messages: []adapter.Message{
			{Role: "system", Content: sysPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: &temperature, // Low temp for analytical task
	}

	appCtx := &client.AppContext{
		OrganizationID:     errCtx.OrgID,
		WorkspaceID:        errCtx.WorkspaceID,
		BillingSubjectType: client.BillingSubjectTypeWorkspace,
		AppID:              errCtx.WorkflowID,
		AppType:            "workflow",
		AccountID:          errCtx.UserID,
	}

	logger.Info("Calling LLM for diagnosis",
		"model", modelName,
		"workflowID", errCtx.WorkflowID,
		"orgID", errCtx.OrgID,
		"workspaceID", errCtx.WorkspaceID)
	resp, err := d.llmClient.AppChat(timeoutCtx, appCtx, req)
	res.LatencyMs = int(time.Since(start).Milliseconds())

	if err != nil {
		logger.Error("LLM diagnosis failed", err)
		if timeoutCtx.Err() == context.DeadlineExceeded {
			res.ResultText = "诊断服务响应超时，请稍后重试。详情: " + err.Error()
		} else {
			res.ResultText = "诊断服务调用失败，请检查配置。详情: " + err.Error()
		}
		res.IsDiagnosed = false
		logger.Info("LLM diagnosis call finished", "model", modelName, "latencyMs", res.LatencyMs, "success", false, "result", res.ResultText)
		return
	}
	if len(resp.Choices) > 0 {
		if contentStr, ok := resp.Choices[0].Message.Content.(string); ok {
			res.ResultText = contentStr
		} else {
			res.ResultText = "The diagnosis model returned an incompatible response format."
		}
	} else {
		res.ResultText = "The diagnosis model returned an empty response."
	}
	res.IsDiagnosed = true
	if resp.Usage != nil {
		res.Tokens = resp.Usage.TotalTokens
	}
	logger.Info("LLM diagnosis call finished", "model", modelName, "latencyMs", res.LatencyMs, "success", true, "result", res.ResultText)
}
