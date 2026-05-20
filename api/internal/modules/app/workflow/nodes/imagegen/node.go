package imagegen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	llmnode "github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/llm"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/template"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

var hashPlaceholderPattern = regexp.MustCompile(`\{\{(#[a-zA-Z0-9_]{1,50}(?:\.[a-zA-Z_][a-zA-Z0-9_]{0,29}){1,10}#)\}\}`)

var supportedAspectRatios = map[string]string{
	"1:1":  "1024x1024",
	"4:3":  "1024x768",
	"3:4":  "768x1024",
	"16:9": "1920x1080",
	"9:16": "1080x1920",
}

var doubaoHighResolutionAspectRatios = map[string]string{
	"1:1":  "2048x2048",
	"4:3":  "2304x1728",
	"3:4":  "1728x2304",
	"16:9": "2560x1440",
	"9:16": "1440x2560",
}

var doubaoSeedream40AspectRatios = map[string]string{
	"1:1":  "1024x1024",
	"3:4":  "864x1152",
	"4:3":  "1152x864",
	"16:9": "1312x736",
	"9:16": "736x1312",
	"2:3":  "832x1248",
	"3:2":  "1248x832",
	"21:9": "1568x672",
}

var doubaoSeedream40SupportedSizes = map[string]struct{}{
	"1024x1024": {},
	"864x1152":  {},
	"1152x864":  {},
	"1312x736":  {},
	"736x1312":  {},
	"832x1248":  {},
	"1248x832":  {},
	"1568x672":  {},
	"2048x2048": {},
	"1728x2304": {},
	"2304x1728": {},
	"2848x1600": {},
	"1600x2848": {},
	"2496x1664": {},
	"1664x2496": {},
	"3136x1344": {},
	"4096x4096": {},
	"3520x4704": {},
	"4704x3520": {},
	"5504x3040": {},
	"3040x5504": {},
	"3328x4992": {},
	"4992x3328": {},
	"6240x2656": {},
}

const (
	defaultImageSize                 = "1024x1024"
	doubaoHighResolutionDefaultSize  = "2048x2048"
	doubaoHighResolutionMinimumPixel = 3686400

	doubaoProviderName          = "doubao"
	doubaoSeedream50ModelPrefix = "doubao-seedream-5-0"
	doubaoSeedream45ModelPrefix = "doubao-seedream-4-5"
	doubaoSeedream40ModelPrefix = "doubao-seedream-4-0"

	imageGenerationNoImagesError = "image generation returned no images"
)

type generationSizeSource string

const (
	generationSizeSourceDefault           generationSizeSource = "default"
	generationSizeSourceAspectRatioPreset generationSizeSource = "aspect_ratio_preset"
	generationSizeSourceExplicitSize      generationSizeSource = "explicit_size"
)

type Node struct {
	base.NodeStruct
	nodeData     NodeData
	imageInvoker ImageInvoker
	fileSaver    llmnode.FileSaver
}

func New(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...any,
) (shared.NodeInterface, error) {
	nodeData, nodeID, err := parseNodeDataFromConfig(config)
	if err != nil {
		return nil, err
	}

	n := &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.ImageGen,

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
		},
		nodeData: nodeData,
	}

	for _, dep := range optionalDeps {
		client, ok := dep.(llmclient.LLMClient)
		if !ok {
			continue
		}

		invoker, invErr := NewGatewayImageInvoker(client, graphInitParams.OrganizationID, graphInitParams.WorkspaceID, graphInitParams.BillingSubjectType)
		if invErr != nil {
			return nil, invErr
		}
		n.imageInvoker = invoker
		break
	}

	n.fileSaver = buildFileSaver(graphInitParams.UserID, graphInitParams.TenantID, nodeData.Output.Lifecycle)

	return n, nil
}

func (n *Node) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunStarted,
		NodeID:    n.NodeID,
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	result, err := n.executeRun(ctx)
	if err != nil {
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

func (n *Node) logContext(ctx context.Context) context.Context {
	return logger.WithFields(ctx,
		zap.String("node_id", n.NodeID),
		zap.String("node_type", string(n.NodeType)),
		zap.String("workflow_id", n.WorkflowID),
		zap.String("workflow_run_id", n.workflowRunID()),
		zap.String("tenant_id", n.TenantID),
	)
}

func (n *Node) workflowRunID() string {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return ""
	}
	return n.GraphRuntimeState.VariablePool.SystemVariables.WorkflowRunID
}

func (n *Node) executeRun(ctx context.Context) (*shared.NodeRunResult, error) {
	if n.imageInvoker == nil {
		return nil, ErrInvokerNotConfigured
	}
	if n.fileSaver == nil {
		return nil, fmt.Errorf("image file saver not configured")
	}

	modelConfig, modelSource, err := n.resolveModelConfig()
	if err != nil {
		return nil, err
	}
	generationConfig, err := n.resolveGenerationConfig(modelConfig)
	if err != nil {
		return nil, err
	}

	renderedPrompt, err := n.renderPrompt()
	if err != nil {
		return nil, err
	}

	appID := strings.TrimSpace(n.WorkflowID)
	if appID == "" {
		appID = strings.TrimSpace(n.APPID)
	}
	if appID == "" {
		return nil, fmt.Errorf("workflow app ID is required for image generation")
	}

	invokeReq := &InvokeRequest{
		ModelSlug: strings.TrimSpace(modelConfig.Name),
		Prompt:    renderedPrompt,
		N:         generationConfig.N,
		Size:      generationConfig.Size,
		Quality:   generationConfig.Quality,
		Style:     generationConfig.Style,
		UserID:    n.UserID,
	}

	result, attempts, err := n.invokeWithRetry(ctx, appID, modelConfig.Provider, invokeReq)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New(imageGenerationNoImagesError)
	}
	if len(result.Images) == 0 {
		return nil, errors.New(imageGenerationNoImagesError)
	}

	files := make([]*file.File, 0, len(result.Images))
	urls := make([]string, 0, len(result.Images))
	revisedPrompts := make([]string, 0, len(result.Images))

	for idx, item := range result.Images {
		savedFile, saveErr := n.saveImage(item)
		if saveErr != nil {
			return nil, fmt.Errorf("failed to persist generated image %d: %w", idx, saveErr)
		}

		files = append(files, savedFile)
		if savedFile != nil && savedFile.URL != nil {
			urls = append(urls, *savedFile.URL)
		} else {
			urls = append(urls, "")
		}
		revisedPrompts = append(revisedPrompts, item.RevisedPrompt)
	}

	inputs := map[string]any{
		"prompt_variables": n.resolvePromptVariables(),
		"prompt":           renderedPrompt,
	}

	return &shared.NodeRunResult{
		Status: shared.SUCCEEDED,
		Inputs: inputs,
		ProcessData: map[string]any{
			"rendered_prompt": renderedPrompt,
			"attempts":        attempts,
		},
		Outputs: map[string]any{
			"files":           files,
			"urls":            urls,
			"revised_prompts": revisedPrompts,
		},
		Metadata: map[shared.WorkflowNodeExecutionMetadataKey]any{
			shared.ResolvedModelProvider: modelConfig.Provider,
			shared.ResolvedModelName:     modelConfig.Name,
			shared.ResolvedModelSource:   modelSource,
		},
	}, nil
}

func (n *Node) resolvePromptVariables() map[string]any {
	result := map[string]any{}
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return result
	}

	for _, holder := range extractPlaceholders(n.nodeData.Prompt) {
		addPromptVariable(result, holder.valueSelector, resolveSelectorVariable(n.GraphRuntimeState.VariablePool, holder.valueSelector))
	}

	for _, templateVariable := range n.nodeData.PromptConfig.TemplateVariables {
		variableName := strings.TrimSpace(templateVariable.Variable)
		if variableName == "" {
			variableName = strings.Join(templateVariable.ValueSelector, "_")
		}
		if variableName == "" {
			continue
		}
		variable := resolveSelectorVariable(n.GraphRuntimeState.VariablePool, templateVariable.ValueSelector)
		if variable == nil {
			result[variableName] = ""
			continue
		}
		result[variableName] = normalizeTemplateInputValue(variable.ToObject())
	}

	return result
}

func addPromptVariable(result map[string]any, selector []string, variable entities.Variable) {
	if len(selector) == 0 || variable == nil {
		return
	}
	key := strings.Join(selector, ".")
	result[key] = normalizeTemplateInputValue(variable.ToObject())
}

func (n *Node) invokeWithRetry(ctx context.Context, appID string, provider string, req *InvokeRequest) (*InvokeResult, int, error) {
	maxRetries := 0
	if n.nodeData.RetryConfig.Enable && n.nodeData.RetryConfig.MaxTimes > 0 {
		maxRetries = n.nodeData.RetryConfig.MaxTimes
	}

	totalAttempts := maxRetries + 1
	var lastErr error
	for attempt := 1; attempt <= totalAttempts; attempt++ {
		resp, err := n.imageInvoker.Invoke(ctx, n.UserID, appID, workflowAppType, req)
		if err == nil {
			return resp, attempt, nil
		}

		if ctx.Err() != nil {
			return nil, attempt, ctx.Err()
		}

		lastErr = err
		if attempt >= totalAttempts {
			break
		}

		backoffDuration := time.Duration(attempt) * 150 * time.Millisecond
		logger.WarnContext(n.logContext(ctx), "image generation invocation retrying",
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", totalAttempts),
			zap.Int64("retry_delay_ms", backoffDuration.Milliseconds()),
			zap.String("provider", provider),
			zap.String("model", req.ModelSlug),
			zap.Error(err),
		)

		select {
		case <-time.After(backoffDuration):
		case <-ctx.Done():
			return nil, attempt, ctx.Err()
		}
	}

	return nil, totalAttempts, lastErr
}

func (n *Node) resolveModelConfig() (*ImageModelConfig, string, error) {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil || n.GraphRuntimeState.VariablePool.UserInputs == nil {
		model := n.nodeData.Model
		return &model, shared.ResolvedModelSourceNodeDefault, nil
	}

	userInputs := n.GraphRuntimeState.VariablePool.UserInputs
	modelConfigRaw, exists := userInputs["model_config"]
	if !exists {
		model := n.nodeData.Model
		return &model, shared.ResolvedModelSourceNodeDefault, nil
	}

	modelMap, ok := modelConfigRaw.(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("runtime model_config must be an object")
	}

	model := &ImageModelConfig{}
	if provider, ok := modelMap["provider"].(string); ok {
		model.Provider = strings.TrimSpace(provider)
	}
	if name, ok := modelMap["name"].(string); ok && strings.TrimSpace(name) != "" {
		model.Name = strings.TrimSpace(name)
	}
	if model.Name == "" {
		if name, ok := modelMap["model"].(string); ok {
			model.Name = strings.TrimSpace(name)
		}
	}

	if model.Provider == "" || model.Name == "" {
		return nil, "", fmt.Errorf("runtime model_config must include provider and model/name")
	}

	return model, shared.ResolvedModelSourceInputOverride, nil
}

func (n *Node) resolveGenerationConfig(modelConfig *ImageModelConfig) (*GenerationConfig, error) {
	generation := n.nodeData.Generation
	sizeSource := generationSizeSourceDefault
	aspectRatioPreset := ""

	if strings.TrimSpace(generation.Size) != "" {
		sizeSource = generationSizeSourceExplicitSize
	}

	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil || n.GraphRuntimeState.VariablePool.UserInputs == nil {
		normalizedSize, err := normalizeGenerationSize(modelConfig, sizeSource, aspectRatioPreset, generation.Size)
		if err != nil {
			return nil, err
		}
		generation.Size = normalizedSize
		return &generation, nil
	}

	userInputs := n.GraphRuntimeState.VariablePool.UserInputs
	rawConfig, exists := userInputs["image_gen_config"]
	if !exists {
		normalizedSize, err := normalizeGenerationSize(modelConfig, sizeSource, aspectRatioPreset, generation.Size)
		if err != nil {
			return nil, err
		}
		generation.Size = normalizedSize
		return &generation, nil
	}

	configMap, ok := rawConfig.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("runtime image_gen_config must be an object")
	}

	if aspectRatioRaw, exists := configMap["aspect_ratio"]; exists {
		aspectRatio, ok := aspectRatioRaw.(string)
		if !ok {
			return nil, fmt.Errorf("runtime image_gen_config.aspect_ratio must be a string")
		}
		aspectRatio = strings.TrimSpace(aspectRatio)
		if aspectRatio != "" {
			aspectRatioPreset = aspectRatio
			sizeSource = generationSizeSourceAspectRatioPreset
		}
	}

	if qualityRaw, exists := configMap["quality"]; exists {
		quality, ok := qualityRaw.(string)
		if !ok {
			return nil, fmt.Errorf("runtime image_gen_config.quality must be a string")
		}
		quality = strings.TrimSpace(quality)
		if quality != "" {
			generation.Quality = quality
		}
	}

	if nRaw, exists := configMap["n"]; exists {
		nValue, err := parseRuntimeImageCount(nRaw)
		if err != nil {
			return nil, err
		}
		if nValue < 1 || nValue > 4 {
			return nil, fmt.Errorf("runtime image_gen_config.n must be between 1 and 4")
		}
		generation.N = nValue
	}

	normalizedSize, err := normalizeGenerationSize(modelConfig, sizeSource, aspectRatioPreset, generation.Size)
	if err != nil {
		return nil, err
	}
	generation.Size = normalizedSize

	return &generation, nil
}

func normalizeGenerationSize(modelConfig *ImageModelConfig, source generationSizeSource, aspectRatioPreset, explicitSize string) (string, error) {
	switch source {
	case generationSizeSourceAspectRatioPreset:
		return resolveAspectRatioSize(modelConfig, aspectRatioPreset)
	case generationSizeSourceExplicitSize:
		size := strings.TrimSpace(explicitSize)
		if err := validateExplicitGenerationSize(modelConfig, size); err != nil {
			return "", err
		}
		return size, nil
	default:
		return defaultGenerationSize(modelConfig), nil
	}
}

func resolveAspectRatioSize(modelConfig *ImageModelConfig, aspectRatio string) (string, error) {
	aspectRatio = strings.TrimSpace(aspectRatio)
	if aspectRatio == "" {
		return "", fmt.Errorf("aspect ratio preset is required")
	}

	if isDoubaoHighResolutionModel(modelConfig) {
		size, exists := doubaoHighResolutionAspectRatios[aspectRatio]
		if !exists {
			return "", fmt.Errorf("unsupported runtime image_gen_config.aspect_ratio: %s", aspectRatio)
		}
		return size, nil
	}

	if isDoubaoSeedream40Model(modelConfig) {
		size, exists := doubaoSeedream40AspectRatios[aspectRatio]
		if !exists {
			return "", fmt.Errorf("unsupported runtime image_gen_config.aspect_ratio: %s", aspectRatio)
		}
		return size, nil
	}

	size, exists := supportedAspectRatios[aspectRatio]
	if !exists {
		return "", fmt.Errorf("unsupported runtime image_gen_config.aspect_ratio: %s", aspectRatio)
	}
	return size, nil
}

func defaultGenerationSize(modelConfig *ImageModelConfig) string {
	if isDoubaoHighResolutionModel(modelConfig) {
		return doubaoHighResolutionDefaultSize
	}
	return defaultImageSize
}

func validateExplicitGenerationSize(modelConfig *ImageModelConfig, size string) error {
	if !isDoubaoHighResolutionModel(modelConfig) {
		if !isDoubaoSeedream40Model(modelConfig) {
			return nil
		}

		if _, ok := doubaoSeedream40SupportedSizes[size]; !ok {
			return fmt.Errorf(
				"model %q received invalid size %q: use a supported aspect_ratio preset or choose one of the official 1K/2K/4K sizes",
				strings.TrimSpace(modelConfig.Name),
				size,
			)
		}

		return nil
	}

	width, height, err := parseGenerationSize(size)
	if err != nil {
		return fmt.Errorf(
			"model %q received invalid size %q: %w; use a supported aspect_ratio preset or choose a larger size with at least %d pixels",
			strings.TrimSpace(modelConfig.Name),
			size,
			err,
			doubaoHighResolutionMinimumPixel,
		)
	}

	if width*height < doubaoHighResolutionMinimumPixel {
		return fmt.Errorf(
			"model %q received invalid size %q: image size must be at least %d pixels; use a supported aspect_ratio preset or choose a larger size",
			strings.TrimSpace(modelConfig.Name),
			size,
			doubaoHighResolutionMinimumPixel,
		)
	}

	return nil
}

func parseGenerationSize(size string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(size), "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("size must use WIDTHxHEIGHT format")
	}

	width := strings.TrimSpace(parts[0])
	height := strings.TrimSpace(parts[1])
	if width == "" || height == "" {
		return 0, 0, fmt.Errorf("size must use WIDTHxHEIGHT format")
	}

	widthValue, err := strconv.Atoi(width)
	if err != nil {
		return 0, 0, fmt.Errorf("size must use WIDTHxHEIGHT format")
	}

	heightValue, err := strconv.Atoi(height)
	if err != nil {
		return 0, 0, fmt.Errorf("size must use WIDTHxHEIGHT format")
	}

	if widthValue <= 0 || heightValue <= 0 {
		return 0, 0, fmt.Errorf("size width and height must be positive integers")
	}

	return widthValue, heightValue, nil
}

func isDoubaoHighResolutionModel(modelConfig *ImageModelConfig) bool {
	if modelConfig == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(modelConfig.Provider), doubaoProviderName) {
		return false
	}

	modelName := strings.ToLower(strings.TrimSpace(modelConfig.Name))
	return strings.HasPrefix(modelName, doubaoSeedream50ModelPrefix) || strings.HasPrefix(modelName, doubaoSeedream45ModelPrefix)
}

func isDoubaoSeedream40Model(modelConfig *ImageModelConfig) bool {
	if modelConfig == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(modelConfig.Provider), doubaoProviderName) {
		return false
	}

	modelName := strings.ToLower(strings.TrimSpace(modelConfig.Name))
	return strings.HasPrefix(modelName, doubaoSeedream40ModelPrefix)
}

func parseRuntimeImageCount(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case float64:
		if v != math.Trunc(v) {
			return 0, fmt.Errorf("runtime image_gen_config.n must be an integer")
		}
		return int(v), nil
	default:
		return 0, fmt.Errorf("runtime image_gen_config.n must be an integer")
	}
}

func (n *Node) renderPrompt() (string, error) {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return n.nodeData.Prompt, nil
	}

	renderedByPath, err := resolveHashPlaceholders(n.nodeData.Prompt, n.GraphRuntimeState.VariablePool)
	if err != nil {
		return "", err
	}

	if len(n.nodeData.PromptConfig.TemplateVariables) == 0 {
		return renderedByPath, nil
	}

	templateInputs := make(map[string]any, len(n.nodeData.PromptConfig.TemplateVariables))
	for _, templateVariable := range n.nodeData.PromptConfig.TemplateVariables {
		variableName := strings.TrimSpace(templateVariable.Variable)
		if variableName == "" {
			variableName = strings.Join(templateVariable.ValueSelector, "_")
		}

		variable := resolveSelectorVariable(n.GraphRuntimeState.VariablePool, templateVariable.ValueSelector)
		if variable == nil {
			templateInputs[variableName] = ""
			continue
		}

		templateInputs[variableName] = normalizeTemplateInputValue(variable.ToObject())
	}

	result, err := template.ExecutePongo2Template(renderedByPath, templateInputs)
	if err != nil {
		return "", fmt.Errorf("failed to render prompt template: %w", err)
	}

	return result, nil
}

func (n *Node) saveImage(item llmadapter.ImageItem) (*file.File, error) {
	switch {
	case strings.TrimSpace(item.URL) != "":
		return n.fileSaver.SaveRemoteURL(item.URL, file.FileTypeImage)
	case strings.TrimSpace(item.B64JSON) != "":
		data, err := decodeBase64Image(item.B64JSON)
		if err != nil {
			return nil, err
		}
		return n.fileSaver.SaveBinaryString(data, "image/png", file.FileTypeImage, nil)
	default:
		return nil, fmt.Errorf("image item does not contain url or b64_json")
	}
}

func resolveSelectorVariable(variablePool *entities.VariablePool, selector []string) entities.Variable {
	if variablePool == nil || len(selector) == 0 {
		return nil
	}
	return variablePool.GetWithPath(selector)
}

func normalizeTemplateInputValue(value any) any {
	switch v := value.(type) {
	case float64:
		if v == math.Trunc(v) {
			return int64(v)
		}
		return v
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = normalizeTemplateInputValue(item)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, item := range v {
			result[key] = normalizeTemplateInputValue(item)
		}
		return result
	default:
		return value
	}
}

type placeholder struct {
	token         string
	valueSelector []string
}

func extractPlaceholders(tpl string) []placeholder {
	matches := hashPlaceholderPattern.FindAllStringSubmatch(tpl, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(matches))
	result := make([]placeholder, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}

		token := match[1]
		if seen[token] {
			continue
		}
		seen[token] = true

		trimmed := strings.Trim(token, "#")
		parts := strings.Split(trimmed, ".")
		if len(parts) < 2 {
			continue
		}

		result = append(result, placeholder{
			token:         token,
			valueSelector: parts,
		})
	}

	return result
}

func resolveHashPlaceholders(tpl string, vp *entities.VariablePool) (string, error) {
	holders := extractPlaceholders(tpl)
	if len(holders) == 0 {
		return tpl, nil
	}

	replacements := make(map[string]string, len(holders))
	for _, holder := range holders {
		variable := resolveSelectorVariable(vp, holder.valueSelector)
		if variable == nil {
			return "", fmt.Errorf("variable %s not found", strings.Trim(holder.token, "#"))
		}
		replacements[holder.token] = formatAny(variable.ToObject())
	}

	return hashPlaceholderPattern.ReplaceAllStringFunc(tpl, func(match string) string {
		submatches := hashPlaceholderPattern.FindStringSubmatch(match)
		if len(submatches) < 2 {
			return match
		}
		if replacement, ok := replacements[submatches[1]]; ok {
			return replacement
		}
		return ""
	}), nil
}

func formatAny(v any) string {
	if v == nil {
		return ""
	}

	switch val := v.(type) {
	case string:
		return val
	case fmt.Stringer:
		return val.String()
	case []byte:
		return string(val)
	case map[string]any, []any:
		data, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(data)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func decodeBase64Image(raw string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(raw)
	if err == nil {
		return data, nil
	}

	data, rawErr := base64.RawStdEncoding.DecodeString(raw)
	if rawErr == nil {
		return data, nil
	}

	return nil, fmt.Errorf("failed to decode image base64: %w", err)
}

func buildFileSaver(userID, tenantID, lifecycle string) llmnode.FileSaver {
	switch strings.TrimSpace(lifecycle) {
	case string(tool_file.ToolFileLifecycleTemporary):
		return llmnode.NewFileSaverImplGlobalWithLifecycleAndURLMode(userID, tenantID, tool_file.ToolFileLifecycleTemporary, nil, tool_file.ToolFileURLModePermanent)
	default:
		return llmnode.NewFileSaverImplGlobalWithLifecycleAndURLMode(userID, tenantID, tool_file.ToolFileLifecyclePersistent, nil, tool_file.ToolFileURLModePermanent)
	}
}
