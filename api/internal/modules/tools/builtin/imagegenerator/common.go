package imagegenerator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	defaultmodelservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

const (
	defaultAspectRatio = "1:1"
	defaultImageCount  = 1
	defaultImageMIME   = "image/png"
	defaultImageExt    = ".png"
	maxPromptRunes     = 4000
	maxImageBytes      = 20 * 1024 * 1024
)

var unsafeFilenamePattern = regexp.MustCompile(`[^a-zA-Z0-9._\-\p{Han}]`)

// ReferenceImageFileService is the minimal file contract needed for reference-image requests.
type ReferenceImageFileService interface {
	GetUploadConfig() *interfaces.FileUploadConfigResponse
	GetFileByID(ctx context.Context, fileID string) (*dto.UploadFile, error)
	GetFileURL(ctx context.Context, fileID string) (string, error)
}

type imageToolBase struct {
	runtime       *tools.ToolRuntime
	fileService   ReferenceImageFileService
	llmClient     llmclient.LLMClient
	defaultModels defaultmodelservice.DefaultModelResolver
}

type generationRequest struct {
	Prompt        string
	Style         string
	AspectRatio   string
	Count         int
	Negative      string
	Filename      string
	Lifecycle     tool_file.ToolFileLifecycle
	Provider      string
	Model         string
	ReferenceURL  string
	ReferenceName string
	EditType      string
}

type resolvedImageModel struct {
	Provider string
	Model    string
	Source   string
}

func (b *imageToolBase) invokeGeneration(
	ctx context.Context,
	tenantID string,
	userID string,
	conversationID *string,
	appID *string,
	params map[string]interface{},
	req generationRequest,
) ([]tools.ToolInvokeMessage, error) {
	if b.llmClient == nil {
		return nil, fmt.Errorf("llm client is required")
	}
	if strings.TrimSpace(tenantID) == "" {
		return nil, fmt.Errorf("tenant id is required")
	}
	if strings.TrimSpace(userID) == "" {
		return nil, fmt.Errorf("user id is required")
	}

	resolved, err := b.resolveModel(ctx, tenantID, params)
	if err != nil {
		return nil, err
	}
	imageReq := &adapter.ImageRequest{
		Model:          resolved.Model,
		Prompt:         buildImagePrompt(req),
		Size:           aspectRatioSize(req.AspectRatio),
		ResponseFormat: "url",
		User:           userID,
	}
	if req.Count > 0 {
		n := req.Count
		imageReq.N = &n
	}

	resp, err := b.llmClient.AppCreateImage(ctx, buildAppContext(tenantID, userID, conversationID, appID), imageReq)
	if err != nil {
		return nil, fmt.Errorf("failed to generate image: %w", err)
	}
	if resp == nil || len(resp.Data) == 0 {
		return nil, fmt.Errorf("image generation returned no images")
	}

	files := make([]map[string]interface{}, 0, len(resp.Data))
	messages := make([]tools.ToolInvokeMessage, 0, len(resp.Data)+1)
	revisedPrompts := make([]string, 0, len(resp.Data))
	for idx, item := range resp.Data {
		fileMeta, err := saveGeneratedImage(ctx, tenantID, userID, conversationID, item, req.Filename, idx, req.Lifecycle)
		if err != nil {
			return nil, fmt.Errorf("failed to persist generated image %d: %w", idx+1, err)
		}
		files = append(files, fileMeta)
		revisedPrompts = append(revisedPrompts, strings.TrimSpace(item.RevisedPrompt))
		if downloadURL, _ := fileMeta["download_url"].(string); downloadURL != "" {
			messages = append(messages, tools.ToolInvokeMessage{
				Type: tools.ToolInvokeMessageTypeFile,
				Text: downloadURL,
				Meta: map[string]interface{}{"file": fileMeta},
			})
		}
	}

	messages = append(messages, builtin.CreateJSONMessage(map[string]interface{}{
		"files":           files,
		"count":           len(files),
		"aspect_ratio":    req.AspectRatio,
		"style":           req.Style,
		"format":          "png",
		"mime_type":       defaultImageMIME,
		"model":           resolved.Model,
		"model_provider":  resolved.Provider,
		"model_source":    resolved.Source,
		"revised_prompts": revisedPrompts,
		"safety_notes":    safetyNotes(req),
	}))
	return messages, nil
}

func (b *imageToolBase) resolveModel(ctx context.Context, tenantID string, params map[string]interface{}) (resolvedImageModel, error) {
	explicitProvider := optionalStringPointer(rawStringParam(params, "provider"))
	explicitModel := optionalStringPointer(rawStringParam(params, "model"))
	if b.defaultModels != nil {
		resolved, err := b.defaultModels.ResolveUseCase(ctx, tenantID, llmmodelmodel.UseCaseImageGen, explicitProvider, explicitModel)
		if err != nil {
			return resolvedImageModel{}, fmt.Errorf("failed to resolve image generation model: %w", err)
		}
		if resolved == nil || strings.TrimSpace(resolved.Model) == "" {
			return resolvedImageModel{}, fmt.Errorf("default image generation model not found")
		}
		return resolvedImageModel{Provider: strings.TrimSpace(resolved.Provider), Model: strings.TrimSpace(resolved.Model), Source: strings.TrimSpace(resolved.Source)}, nil
	}
	if explicitModel == nil || strings.TrimSpace(*explicitModel) == "" {
		return resolvedImageModel{}, fmt.Errorf("image generation model is required when default model service is unavailable")
	}
	provider := ""
	if explicitProvider != nil {
		provider = strings.TrimSpace(*explicitProvider)
	}
	return resolvedImageModel{Provider: provider, Model: strings.TrimSpace(*explicitModel), Source: "explicit"}, nil
}

func buildAppContext(tenantID string, userID string, conversationID *string, appID *string) *llmclient.AppContext {
	sessionID := optionalString(conversationID)
	return &llmclient.AppContext{
		OrganizationID:     tenantID,
		BillingSubjectType: llmclient.BillingSubjectTypeOrganization,
		AppID:              firstNonEmpty(optionalString(appID), sessionID, ProviderID),
		AppType:            "agent",
		AccountID:          userID,
		SessionID:          sessionID,
		ConversationID:     sessionID,
	}
}

func buildImagePrompt(req generationRequest) string {
	parts := []string{strings.TrimSpace(req.Prompt)}
	if req.EditType != "" {
		parts = append(parts, "Edit type: "+req.EditType+".")
	}
	if req.ReferenceName != "" {
		parts = append(parts, "Use the provided reference image as loose visual guidance when the model supports reference-image generation. Reference file: "+req.ReferenceName+".")
	}
	if req.ReferenceURL != "" {
		parts = append(parts, "Reference image URL for providers that can use it: "+req.ReferenceURL+".")
	}
	if stylePrompt := styleInstruction(req.Style); stylePrompt != "" {
		parts = append(parts, stylePrompt)
	}
	if req.Negative != "" {
		parts = append(parts, "Avoid: "+req.Negative+".")
	}
	return strings.Join(parts, "\n")
}

func styleInstruction(style string) string {
	switch normalizeStyleNoError(style) {
	case "realistic":
		return "Style: realistic, photo-like, natural lighting, believable materials."
	case "illustration":
		return "Style: editorial illustration, polished composition."
	case "flat":
		return "Style: flat vector-like design, simple shapes, clean colors."
	case "3d":
		return "Style: polished 3D rendered scene or object."
	case "guofeng":
		return "Style: Chinese-inspired guofeng aesthetic, elegant traditional visual language."
	case "tech":
		return "Style: futuristic high-tech visual, clean digital atmosphere."
	case "poster":
		return "Style: poster key visual with strong composition and negative space."
	case "product":
		return "Style: product-focused commercial concept image."
	case "icon":
		return "Style: simple icon draft, clear silhouette, minimal background."
	case "cover":
		return "Style: cover image composition with room for title text."
	default:
		return ""
	}
}

func saveGeneratedImage(ctx context.Context, tenantID string, userID string, conversationID *string, item adapter.ImageItem, baseFilename string, index int, lifecycle tool_file.ToolFileLifecycle) (map[string]interface{}, error) {
	var toolFile *tool_file.ToolFile
	var err error
	switch {
	case strings.TrimSpace(item.B64JSON) != "":
		data, decodeErr := decodeBase64Image(item.B64JSON)
		if decodeErr != nil {
			return nil, decodeErr
		}
		_, mimeType, extension, validateErr := validateGeneratedImageData(data, "")
		if validateErr != nil {
			return nil, validateErr
		}
		filename := buildImageFilename(baseFilename, index, extension)
		toolFile, err = tool_file.CreateFileByRawGlobal(ctx, tool_file.CreateFileByRawParams{
			UserID:         userID,
			TenantID:       tenantID,
			ConversationID: conversationID,
			FileData:       data,
			MimeType:       mimeType,
			Filename:       &filename,
			Lifecycle:      lifecycle,
		})
	case strings.TrimSpace(item.URL) != "":
		data, mimeType, extension, downloadErr := downloadGeneratedImage(ctx, strings.TrimSpace(item.URL))
		if downloadErr != nil {
			return nil, downloadErr
		}
		filename := buildImageFilename(baseFilename, index, extension)
		toolFile, err = tool_file.CreateFileByRawGlobal(ctx, tool_file.CreateFileByRawParams{
			UserID:         userID,
			TenantID:       tenantID,
			ConversationID: conversationID,
			FileData:       data,
			MimeType:       mimeType,
			Filename:       &filename,
			Lifecycle:      lifecycle,
		})
	default:
		return nil, fmt.Errorf("image item does not contain url or b64_json")
	}
	if err != nil {
		return nil, err
	}
	extension := toolFile.GetFileExtension()
	if extension == "" {
		extension = extensionFromMIME(toolFile.MimeType)
	}
	if extension == "" {
		extension = defaultImageExt
	}
	url, err := tool_file.SignToolFileGlobal(toolFile.ID, extension)
	if err != nil {
		return nil, fmt.Errorf("failed to sign generated image: %w", err)
	}
	downloadURL := appendDownloadQuery(url)
	mimeType := strings.TrimSpace(toolFile.MimeType)
	if mimeType == "" {
		mimeType = defaultImageMIME
	}
	fileObj := workflowfile.NewFile(
		tenantID,
		workflowfile.FileTypeImage,
		workflowfile.FileTransferMethodToolFile,
		workflowfile.WithID(toolFile.ID),
		workflowfile.WithRelatedID(toolFile.ID),
		workflowfile.WithFilename(toolFile.Name),
		workflowfile.WithExtension(extension),
		workflowfile.WithMimeType(mimeType),
		workflowfile.WithSize(int(toolFile.Size)),
		workflowfile.WithURL(url),
	)
	fileMeta := fileObj.ToDict()
	fileMeta["file_id"] = toolFile.ID
	fileMeta["filename"] = toolFile.Name
	fileMeta["format"] = strings.TrimPrefix(extension, ".")
	fileMeta["mime_type"] = mimeType
	fileMeta["url"] = url
	fileMeta["download_url"] = downloadURL
	fileMeta["lifecycle"] = toolFile.Lifecycle
	if toolFile.ExpiresAt != nil {
		fileMeta["expires_at"] = toolFile.ExpiresAt.Unix()
	}
	return fileMeta, nil
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

func downloadGeneratedImage(ctx context.Context, rawURL string) ([]byte, string, string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to create generated image download request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to download generated image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", "", fmt.Errorf("failed to download generated image: status %d", resp.StatusCode)
	}
	if resp.ContentLength > maxImageBytes {
		return nil, "", "", fmt.Errorf("generated image exceeds %d bytes", maxImageBytes)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to read generated image: %w", err)
	}
	return validateGeneratedImageData(data, resp.Header.Get("Content-Type"))
}

func validateGeneratedImageData(data []byte, rawContentType string) ([]byte, string, string, error) {
	if len(data) == 0 {
		return nil, "", "", fmt.Errorf("generated image is empty")
	}
	if len(data) > maxImageBytes {
		return nil, "", "", fmt.Errorf("generated image exceeds %d bytes", maxImageBytes)
	}

	headerMIME := ""
	if rawContentType != "" {
		if parsed, _, err := mime.ParseMediaType(rawContentType); err == nil {
			headerMIME = strings.ToLower(strings.TrimSpace(parsed))
		}
	}
	detected := strings.ToLower(strings.TrimSpace(http.DetectContentType(data)))
	if isSupportedImageMIME(detected) {
		return data, detected, extensionFromMIME(detected), nil
	}
	if isSupportedImageMIME(headerMIME) && detected == "application/octet-stream" {
		return data, headerMIME, extensionFromMIME(headerMIME), nil
	}
	return nil, "", "", fmt.Errorf("generated result is not a supported image: detected=%s content_type=%s", detected, headerMIME)
}

func parseGenerationRequest(params map[string]interface{}, promptKey string) (generationRequest, error) {
	prompt := rawStringParam(params, promptKey)
	if prompt == "" {
		return generationRequest{}, fmt.Errorf("%s is required", promptKey)
	}
	if len([]rune(prompt)) > maxPromptRunes {
		return generationRequest{}, fmt.Errorf("%s exceeds %d characters", promptKey, maxPromptRunes)
	}
	style, err := normalizeStyle(rawStringParam(params, "style"))
	if err != nil {
		return generationRequest{}, err
	}
	aspectRatio, err := normalizeAspectRatio(rawStringParam(params, "aspect_ratio"))
	if err != nil {
		return generationRequest{}, err
	}
	count, err := parseCount(rawAnyParam(params, "count"))
	if err != nil {
		return generationRequest{}, err
	}
	lifecycle, err := resolveLifecycle(rawStringParam(params, "lifecycle"))
	if err != nil {
		return generationRequest{}, err
	}
	return generationRequest{
		Prompt:       prompt,
		Style:        style,
		AspectRatio:  aspectRatio,
		Count:        count,
		Negative:     rawStringParam(params, "negative_prompt"),
		Filename:     rawStringParam(params, "filename"),
		Lifecycle:    lifecycle,
		Provider:     rawStringParam(params, "provider"),
		Model:        rawStringParam(params, "model"),
		ReferenceURL: "",
	}, nil
}

func resolveLifecycle(raw string) (tool_file.ToolFileLifecycle, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "persistent":
		return tool_file.ToolFileLifecyclePersistent, nil
	case "temporary":
		return tool_file.ToolFileLifecycleTemporary, nil
	default:
		return "", fmt.Errorf("unsupported lifecycle: %s", raw)
	}
}

func normalizeStyle(raw string) (string, error) {
	value := normalizeStyleNoError(raw)
	switch value {
	case "auto", "realistic", "illustration", "flat", "3d", "guofeng", "tech", "poster", "product", "icon", "cover":
		return value, nil
	default:
		return "", fmt.Errorf("unsupported style: %s", raw)
	}
}

func normalizeStyleNoError(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "auto":
		return "auto"
	case "realistic", "photo", "photorealistic":
		return "realistic"
	case "illustration", "illustrated":
		return "illustration"
	case "flat", "vector":
		return "flat"
	case "3d", "three_d", "three-dimensional":
		return "3d"
	case "guofeng", "chinese":
		return "guofeng"
	case "tech", "technology", "sci-fi":
		return "tech"
	case "poster":
		return "poster"
	case "product":
		return "product"
	case "icon":
		return "icon"
	case "cover":
		return "cover"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func normalizeAspectRatio(raw string) (string, error) {
	switch strings.TrimSpace(raw) {
	case "", "1:1":
		return "1:1", nil
	case "16:9", "9:16", "4:3":
		return strings.TrimSpace(raw), nil
	default:
		return "", fmt.Errorf("unsupported aspect_ratio: %s", raw)
	}
}

func aspectRatioSize(ratio string) string {
	switch ratio {
	case "16:9":
		return "1792x1024"
	case "9:16":
		return "1024x1792"
	case "4:3":
		return "1024x768"
	default:
		return "1024x1024"
	}
}

func parseCount(raw interface{}) (int, error) {
	if raw == nil {
		return defaultImageCount, nil
	}
	var value int
	switch v := raw.(type) {
	case int:
		value = v
	case int64:
		value = int(v)
	case float64:
		if v != float64(int(v)) {
			return 0, fmt.Errorf("count must be an integer")
		}
		value = int(v)
	case json.Number:
		parsed, err := strconv.Atoi(v.String())
		if err != nil {
			return 0, fmt.Errorf("count must be an integer")
		}
		value = parsed
	case string:
		if strings.TrimSpace(v) == "" {
			return defaultImageCount, nil
		}
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, fmt.Errorf("count must be an integer")
		}
		value = parsed
	default:
		return 0, fmt.Errorf("count must be an integer")
	}
	if value < 1 || value > 4 {
		return 0, fmt.Errorf("count must be between 1 and 4")
	}
	return value, nil
}

func resolveReferenceImage(ctx context.Context, fileService ReferenceImageFileService, params map[string]interface{}, tenantID string, userID string, keys ...string) (string, string, error) {
	if fileService == nil {
		return "", "", fmt.Errorf("file service is required")
	}
	fileID := ""
	for _, key := range keys {
		if id := fileIDFromAny(rawAnyParam(params, key)); id != "" {
			fileID = id
			break
		}
	}
	if fileID == "" {
		return "", "", fmt.Errorf("%s file object is required", keys[0])
	}
	file, err := fileService.GetFileByID(ctx, fileID)
	if err != nil {
		return "", "", fmt.Errorf("reference image file not found: %w", err)
	}
	if err := validateReferenceImage(file, tenantID, userID); err != nil {
		return "", "", err
	}
	url, err := fileService.GetFileURL(ctx, fileID)
	if err != nil {
		return "", "", fmt.Errorf("failed to prepare reference image URL: %w", err)
	}
	return strings.TrimSpace(url), strings.TrimSpace(file.Name), nil
}

func validateReferenceImage(file *dto.UploadFile, tenantID string, userID string) error {
	if file == nil {
		return fmt.Errorf("reference image file is required")
	}
	fileTenantID := firstNonEmpty(file.OrganizationID, file.TenantID)
	if fileTenantID == "" || fileTenantID != strings.TrimSpace(tenantID) {
		return fmt.Errorf("reference image file is not accessible")
	}
	if strings.TrimSpace(file.CreatedBy) == "" || strings.TrimSpace(file.CreatedBy) != strings.TrimSpace(userID) {
		return fmt.Errorf("reference image file is not accessible")
	}
	extension := normalizeExtension(firstNonEmpty(file.Extension, filepath.Ext(file.Name)))
	mimeType := strings.ToLower(strings.TrimSpace(file.MimeType))
	if !isSupportedImage(extension, mimeType) {
		return fmt.Errorf("unsupported reference image format: extension=%s mime_type=%s", extension, mimeType)
	}
	return nil
}

func isSupportedImage(extension string, mimeType string) bool {
	extension = normalizeExtension(extension)
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	switch extension {
	case "png":
		return mimeType == "" || mimeType == "image/png"
	case "jpg", "jpeg":
		return mimeType == "" || mimeType == "image/jpeg" || mimeType == "image/jpg"
	case "webp":
		return mimeType == "" || mimeType == "image/webp"
	default:
		return isSupportedImageMIME(mimeType)
	}
}

func isSupportedImageMIME(mimeType string) bool {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png", "image/jpeg", "image/jpg", "image/webp":
		return true
	default:
		return false
	}
}

func fileIDFromAny(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return ""
		}
		if strings.HasPrefix(text, "{") {
			var obj map[string]interface{}
			if err := json.Unmarshal([]byte(text), &obj); err == nil {
				return fileIDFromAny(obj)
			}
		}
		return text
	case map[string]interface{}:
		return firstNonEmpty(stringFromMap(v, "upload_file_id"), stringFromMap(v, "file_id"), stringFromMap(v, "id"), stringFromMap(v, "related_id"))
	default:
		return ""
	}
}

func rawAnyParam(params map[string]interface{}, key string) interface{} {
	if params == nil {
		return nil
	}
	return params[key]
}

func rawStringParam(params map[string]interface{}, key string) string {
	if params == nil {
		return ""
	}
	if text, ok := params[key].(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func stringFromMap(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}
	if text, ok := values[key].(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func runtimeTenantID(runtime *tools.ToolRuntime) string {
	if runtime == nil {
		return ""
	}
	return strings.TrimSpace(runtime.TenantID)
}

func optionalStringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func normalizeExtension(raw string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(raw)), ".")
}

func buildImageFilename(raw string, index int, extension string) string {
	name := sanitizeFilename(raw)
	if name == "" {
		name = "generated-image"
	}
	if index > 0 {
		name = fmt.Sprintf("%s-%d", name, index+1)
	}
	currentExt := filepath.Ext(name)
	if currentExt != "" {
		name = strings.TrimSuffix(name, currentExt)
	}
	return name + extension
}

func sanitizeFilename(raw string) string {
	name := strings.TrimSpace(filepath.Base(raw))
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	name = unsafeFilenamePattern.ReplaceAllString(name, "_")
	name = strings.Trim(name, "._- ")
	if len(name) > 120 {
		name = name[:120]
	}
	return name
}

func extensionFromMIME(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}

func appendDownloadQuery(rawURL string) string {
	if strings.Contains(rawURL, "?") {
		return rawURL + "&download=1"
	}
	return rawURL + "?download=1"
}

func safetyNotes(req generationRequest) []string {
	notes := []string{"Commercial use requires copyright, trademark, portrait-rights, and brand-compliance review."}
	if req.ReferenceName != "" || req.EditType != "" {
		notes = append(notes, "Reference-image variants and edit-style regeneration depend on the configured image model; exact local pixel editing is not guaranteed.")
	}
	return notes
}
