package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	maxVideoPromptRunes = 4000
	videoRuntimeAppType = "video-runtime"
	defaultRatio        = "16:9"
	defaultResolution   = "720p"
	defaultDuration     = 5
	videoCreateTimeout  = 2 * time.Minute

	videoPredeductPointsPerSecond int64 = 143
	internalCreditsPerPoint       int64 = 1000
)

type LLMVideoClient interface {
	AppCreateVideo(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.VideoRequest) (*adapter.VideoResponse, error)
	AppGetVideoTask(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.VideoTaskRequest) (*adapter.VideoResponse, error)
}

type Service interface {
	ListModels(ctx context.Context, scope Scope) ([]VideoModel, error)
	Generate(ctx context.Context, scope Scope, req GenerateRequest) (*GenerateResult, error)
	ListTasks(ctx context.Context, scope Scope) ([]VideoTask, error)
	GetTask(ctx context.Context, scope Scope, taskID string) (*VideoTask, error)
}

type service struct {
	availableModels llmmodelsvc.AvailableModelsService
	llmClient       LLMVideoClient
	tasks           *taskRepository
}

func NewService(db *gorm.DB, availableModels llmmodelsvc.AvailableModelsService, llmClient interface{}) Service {
	videoClient, _ := llmClient.(LLMVideoClient)
	return &service{
		availableModels: availableModels,
		llmClient:       videoClient,
		tasks:           newTaskRepository(db),
	}
}

func (s *service) ListModels(ctx context.Context, scope Scope) ([]VideoModel, error) {
	models, err := s.availableVideoModels(ctx, scope.OrganizationID)
	if err != nil {
		return nil, err
	}
	result := make([]VideoModel, 0, len(models))
	for _, item := range models {
		if item == nil {
			continue
		}
		label := strings.TrimSpace(item.DisplayName)
		if label == "" {
			label = strings.TrimSpace(item.Name)
		}
		result = append(result, VideoModel{
			Provider:   strings.TrimSpace(item.Provider),
			Model:      strings.TrimSpace(item.Name),
			ModelLabel: label,
		})
	}
	return result, nil
}

func (s *service) Generate(ctx context.Context, scope Scope, req GenerateRequest) (*GenerateResult, error) {
	if s.llmClient == nil {
		return nil, ErrVideoRuntimeUnavailable
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		return nil, ErrPromptRequired
	}
	if len([]rune(prompt)) > maxVideoPromptRunes {
		return nil, ErrPromptTooLong
	}
	operationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), videoCreateTimeout)
	defer cancel()

	model, err := s.findAvailableModel(operationCtx, scope.OrganizationID, req.Provider, req.Model)
	if err != nil {
		return nil, err
	}
	if model == nil {
		return nil, ErrModelNotAvailable
	}

	modelLabel := strings.TrimSpace(model.DisplayName)
	if modelLabel == "" {
		modelLabel = strings.TrimSpace(model.Name)
	}
	options := normalizeGenerateOptions(req.Options)
	appID := uuid.New()
	appCtx, err := buildAppContext(scope, appID)
	if err != nil {
		return nil, err
	}
	referenceURLs := videoReferenceURLs(req)
	videoReq := buildVideoRequest(model.Provider, model.Name, prompt, req, options, scope.AccountID.String(), referenceURLs)
	now := time.Now()
	record := &videoTaskRecord{
		ID:               appID,
		OrganizationID:   scope.OrganizationID,
		AccountID:        scope.AccountID,
		WorkspaceID:      scope.WorkspaceID,
		TaskID:           localVideoTaskID(),
		Provider:         strings.TrimSpace(model.Provider),
		Model:            strings.TrimSpace(model.Name),
		ModelLabel:       modelLabel,
		Prompt:           prompt,
		Status:           "pending",
		DurationSeconds:  options.Duration,
		Resolution:       options.Resolution,
		Ratio:            options.Ratio,
		HasInputVideo:    hasVideoInputReference(req, videoReq, referenceURLs),
		GenerateAudio:    options.GenerateAudio,
		Voice:            strings.TrimSpace(options.Voice),
		RequestPayload:   jsonData(videoRequestPayload(videoReq)),
		ResponsePayload:  jsonData(map[string]any{}),
		CreatedAt:        now,
		UpdatedAt:        now,
		EstimatedCredits: estimateVideoTaskCredits(options),
		ActualCredits:    0,
	}
	if err := s.tasks.create(operationCtx, record); err != nil {
		return nil, err
	}

	s.startUpstreamVideoTask(*record, appCtx, videoReq)
	task := taskFromRecord(*record)
	return &GenerateResult{Task: task}, nil
}

func estimateVideoTaskCredits(options GenerateOptions) int64 {
	duration := options.Duration
	if duration <= 0 {
		duration = defaultDuration
	}
	count := options.Count
	if count <= 0 {
		count = 1
	}
	return int64(duration) * int64(count) * videoPredeductPointsPerSecond * internalCreditsPerPoint
}

func (s *service) startUpstreamVideoTask(record videoTaskRecord, appCtx *llmclient.AppContext, videoReq *adapter.VideoRequest) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), videoCreateTimeout)
		defer cancel()

		resp, err := s.llmClient.AppCreateVideo(ctx, appCtx, videoReq)
		now := time.Now()
		if err != nil {
			record.Status = "failed"
			record.ErrorMessage = fmt.Errorf("%w: %v", ErrUpstreamFailed, err).Error()
			record.UpdatedAt = now
			record.CompletedAt = &now
			_ = s.tasks.save(context.Background(), &record)
			return
		}
		if resp == nil {
			record.Status = "failed"
			record.ErrorMessage = ErrUpstreamFailed.Error()
			record.UpdatedAt = now
			record.CompletedAt = &now
			_ = s.tasks.save(context.Background(), &record)
			return
		}

		if resp.EstimatedCredits > 0 {
			record.EstimatedCredits = resp.EstimatedCredits
		}
		if resp.ActualCredits > 0 {
			record.ActualCredits = resp.ActualCredits
		}

		upstreamID := upstreamTaskID(resp)
		videoURL := firstVideoURL(resp)
		status := normalizeTaskStatus(resp.Status)
		if upstreamID == "" && videoURL == "" && status != "failed" {
			record.Status = "failed"
			record.ErrorMessage = fmt.Errorf("%w: upstream task id is empty", ErrUpstreamFailed).Error()
			record.ResponsePayload = jsonData(videoResponsePayload(resp))
			record.UpdatedAt = now
			record.CompletedAt = &now
			_ = s.tasks.save(context.Background(), &record)
			return
		}
		if status == "" {
			if videoURL != "" {
				status = "succeeded"
			} else {
				status = "pending"
			}
		}

		record.UpstreamTaskID = upstreamID
		record.Status = status
		record.VideoURL = videoURL
		if record.Status == "failed" {
			if message := videoResponseErrorMessage(resp); message != "" {
				record.ErrorMessage = message
			}
		}
		record.ResponsePayload = jsonData(videoResponsePayload(resp))
		record.UpdatedAt = now
		if isTerminalVideoStatus(record.Status) {
			record.CompletedAt = &now
		}
		_ = s.tasks.save(context.Background(), &record)
	}()
}
func (s *service) ListTasks(ctx context.Context, scope Scope) ([]VideoTask, error) {
	records, err := s.tasks.list(ctx, scope, 50)
	if err != nil {
		return nil, err
	}
	result := make([]VideoTask, 0, len(records))
	for _, record := range records {
		result = append(result, taskFromRecord(record))
	}
	return result, nil
}

func (s *service) GetTask(ctx context.Context, scope Scope, taskID string) (*VideoTask, error) {
	record, err := s.tasks.findByTaskID(ctx, scope, strings.TrimSpace(taskID))
	if err != nil {
		return nil, err
	}
	if shouldRefreshVideoTask(record) && s.llmClient != nil {
		if err := s.refreshTask(ctx, scope, record); err != nil {
			record.ErrorMessage = err.Error()
		}
	}
	task := taskFromRecord(*record)
	return &task, nil
}

func shouldRefreshVideoTask(record *videoTaskRecord) bool {
	if record == nil {
		return false
	}
	if !isTerminalVideoStatus(record.Status) {
		return strings.TrimSpace(record.UpstreamTaskID) != ""
	}
	return normalizeTaskStatus(record.Status) == "succeeded" && record.ActualCredits == 0 && strings.TrimSpace(record.UpstreamTaskID) != ""
}
func (s *service) refreshTask(ctx context.Context, scope Scope, record *videoTaskRecord) error {
	appCtx, err := buildAppContext(scope, record.ID)
	if err != nil {
		return err
	}
	upstreamID := strings.TrimSpace(record.UpstreamTaskID)
	if upstreamID == "" {
		return nil
	}
	additional := map[string]interface{}{
		"resolution":  strings.TrimSpace(record.Resolution),
		"input_video": record.HasInputVideo,
		"duration":    record.DurationSeconds,
	}
	copyVideoBillingMetadata(additional, mapFromJSON(record.ResponsePayload))
	resp, err := s.llmClient.AppGetVideoTask(ctx, appCtx, &adapter.VideoTaskRequest{
		Provider:             strings.TrimSpace(record.Provider),
		Model:                strings.TrimSpace(record.Model),
		TaskID:               upstreamID,
		AdditionalParameters: additional,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUpstreamFailed, err)
	}
	if resp == nil {
		return ErrUpstreamFailed
	}
	record.Status = normalizeTaskStatus(resp.Status)
	if record.Status == "" {
		record.Status = "running"
	}
	if record.Status == "failed" {
		if message := videoResponseErrorMessage(resp); message != "" {
			record.ErrorMessage = message
		}
	}
	if videoURL := firstVideoURL(resp); videoURL != "" {
		record.VideoURL = videoURL
	}
	if resp.EstimatedCredits > 0 {
		record.EstimatedCredits = resp.EstimatedCredits
	}
	if resp.ActualCredits > 0 {
		record.ActualCredits = resp.ActualCredits
	}
	record.ResponsePayload = jsonData(videoResponsePayload(resp))
	record.UpdatedAt = time.Now()
	if isTerminalVideoStatus(record.Status) && record.CompletedAt == nil {
		now := time.Now()
		record.CompletedAt = &now
	}
	return s.tasks.save(ctx, record)
}

func (s *service) availableVideoModels(ctx context.Context, organizationID uuid.UUID) ([]*llmmodelsvc.AvailableModel, error) {
	if s.availableModels == nil {
		return nil, ErrModelNotAvailable
	}
	return s.availableModels.ListAvailable(ctx, organizationID, "", string(llmmodelmodel.UseCaseVideoGen))
}

func (s *service) findAvailableModel(ctx context.Context, organizationID uuid.UUID, provider, modelName string) (*llmmodelsvc.AvailableModel, error) {
	available, err := s.availableVideoModels(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	for _, item := range available {
		if item != nil && strings.TrimSpace(item.Provider) == strings.TrimSpace(provider) && strings.TrimSpace(item.Name) == strings.TrimSpace(modelName) {
			return item, nil
		}
	}
	return nil, nil
}

func buildAppContext(scope Scope, appID uuid.UUID) (*llmclient.AppContext, error) {
	if scope.OrganizationID == uuid.Nil || scope.AccountID == uuid.Nil || appID == uuid.Nil {
		return nil, ErrBillingContextRequired
	}
	appCtx := &llmclient.AppContext{
		OrganizationID:     scope.OrganizationID.String(),
		BillingSubjectType: llmclient.BillingSubjectTypeOrganization,
		AppID:              appID.String(),
		AppType:            videoRuntimeAppType,
		ModelUseCase:       string(llmmodelmodel.UseCaseVideoGen),
		AccountID:          scope.AccountID.String(),
		SessionID:          appID.String(),
		ConversationID:     appID.String(),
	}
	if scope.WorkspaceID != nil && *scope.WorkspaceID != uuid.Nil {
		appCtx.WorkspaceID = scope.WorkspaceID.String()
	}
	return appCtx, nil
}

func normalizeGenerateOptions(options GenerateOptions) GenerateOptions {
	options.Ratio = strings.TrimSpace(options.Ratio)
	if options.Ratio == "" {
		options.Ratio = defaultRatio
	}
	options.Resolution = strings.ToLower(strings.TrimSpace(options.Resolution))
	if options.Resolution == "" {
		options.Resolution = defaultResolution
	}
	if options.Duration <= 0 {
		options.Duration = defaultDuration
	}
	if options.Count <= 0 {
		options.Count = 1
	}
	options.Voice = strings.TrimSpace(options.Voice)
	return options
}

func videoReferenceURLs(req GenerateRequest) []string {
	seen := map[string]struct{}{}
	urls := make([]string, 0, 1+len(req.ReferenceURLs))
	appendURL := func(raw string) {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return
		}
		if _, exists := seen[trimmed]; exists {
			return
		}
		seen[trimmed] = struct{}{}
		urls = append(urls, trimmed)
	}
	appendURL(req.ReferenceURL)
	for _, rawURL := range req.ReferenceURLs {
		appendURL(rawURL)
	}
	return urls
}

func buildVideoRequest(provider, modelName, prompt string, req GenerateRequest, options GenerateOptions, user string, referenceURLs []string) *adapter.VideoRequest {
	count := options.Count
	duration := options.Duration
	generateAudio := options.GenerateAudio
	promptExtend := options.PromptExtend
	watermark := options.Watermark
	videoReq := &adapter.VideoRequest{
		Provider:      strings.TrimSpace(provider),
		Model:         strings.TrimSpace(modelName),
		Prompt:        prompt,
		Ratio:         options.Ratio,
		Resolution:    options.Resolution,
		Duration:      &duration,
		N:             &count,
		GenerateAudio: &generateAudio,
		PromptExtend:  &promptExtend,
		Watermark:     &watermark,
		CallbackURL:   strings.TrimSpace(req.CallbackURL),
		User:          user,
	}
	videoReq.FirstFrameURL = strings.TrimSpace(req.FirstFrameURL)
	videoReq.LastFrameURL = strings.TrimSpace(req.LastFrameURL)
	if len(referenceURLs) > 0 {
		for index, referenceURL := range referenceURLs {
			switch referenceKindAt(req.ReferenceTypes, index, referenceURL) {
			case "video":
				if strings.TrimSpace(videoReq.VideoURL) == "" {
					videoReq.VideoURL = referenceURL
				}
			case "audio":
				if strings.TrimSpace(videoReq.AudioURL) == "" {
					videoReq.AudioURL = referenceURL
				}
			default:
				if strings.TrimSpace(videoReq.ImageURL) == "" {
					videoReq.ImageURL = referenceURL
				}
				videoReq.ImageURLs = append(videoReq.ImageURLs, referenceURL)
			}
		}
	}
	if options.Voice != "" {
		videoReq.AdditionalParameters = map[string]interface{}{"voice": options.Voice}
	}
	return videoReq
}

func hasVideoInputReference(req GenerateRequest, videoReq *adapter.VideoRequest, referenceURLs []string) bool {
	if videoReq != nil && strings.TrimSpace(videoReq.VideoURL) != "" {
		return true
	}
	for index, referenceURL := range referenceURLs {
		if referenceKindAt(req.ReferenceTypes, index, referenceURL) == "video" {
			return true
		}
	}
	return false
}

func referenceKindAt(referenceTypes []string, index int, referenceURL string) string {
	if index >= 0 && index < len(referenceTypes) {
		switch strings.ToLower(strings.TrimSpace(referenceTypes[index])) {
		case "image", "video", "audio":
			return strings.ToLower(strings.TrimSpace(referenceTypes[index]))
		}
	}
	return referenceKindFromURL(referenceURL)
}

func referenceKindFromURL(referenceURL string) string {
	value := strings.ToLower(strings.TrimSpace(referenceURL))
	if value == "" {
		return "image"
	}
	value = strings.Split(value, "?")[0]
	value = strings.Split(value, "#")[0]
	switch {
	case strings.HasSuffix(value, ".mp4"),
		strings.HasSuffix(value, ".mov"),
		strings.HasSuffix(value, ".webm"),
		strings.HasSuffix(value, ".m4v"),
		strings.HasSuffix(value, ".avi"),
		strings.HasSuffix(value, ".mkv"):
		return "video"
	case strings.HasSuffix(value, ".mp3"),
		strings.HasSuffix(value, ".wav"),
		strings.HasSuffix(value, ".m4a"),
		strings.HasSuffix(value, ".aac"),
		strings.HasSuffix(value, ".flac"),
		strings.HasSuffix(value, ".ogg"):
		return "audio"
	default:
		return "image"
	}
}

func localVideoTaskID() string {
	id := strings.ReplaceAll(uuid.NewString(), "-", "")
	if len(id) > 8 {
		id = id[:8]
	}
	return "video-" + time.Now().Format("20060102150405") + "-" + id
}
func normalizeTaskStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "pending", "created":
		return "pending"
	case "running", "processing", "in_progress":
		return "running"
	case "succeeded", "success", "completed", "done":
		return "succeeded"
	case "failed", "error":
		return "failed"
	case "cancelled", "canceled":
		return "cancelled"
	default:
		return strings.ToLower(strings.TrimSpace(status))
	}
}

func isTerminalVideoStatus(status string) bool {
	switch normalizeTaskStatus(status) {
	case "succeeded", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func firstVideoURL(resp *adapter.VideoResponse) string {
	if resp == nil {
		return ""
	}
	if strings.TrimSpace(resp.VideoURL) != "" {
		return strings.TrimSpace(resp.VideoURL)
	}
	for _, item := range resp.Data {
		if strings.TrimSpace(item.URL) != "" {
			return strings.TrimSpace(item.URL)
		}
	}
	return ""
}

func upstreamTaskID(resp *adapter.VideoResponse) string {
	if resp == nil {
		return ""
	}
	if strings.TrimSpace(resp.TaskID) != "" {
		return strings.TrimSpace(resp.TaskID)
	}
	return strings.TrimSpace(resp.ID)
}

func videoRequestPayload(req *adapter.VideoRequest) map[string]any {
	data := map[string]any{}
	raw, _ := json.Marshal(req)
	_ = json.Unmarshal(raw, &data)
	for k, v := range req.AdditionalParameters {
		data[k] = v
	}
	return data
}

func videoResponsePayload(resp *adapter.VideoResponse) map[string]any {
	if resp == nil {
		return map[string]any{}
	}
	data := map[string]any{}
	if len(resp.Raw) > 0 {
		for key, value := range resp.Raw {
			data[key] = value
		}
	} else {
		raw, _ := json.Marshal(resp)
		_ = json.Unmarshal(raw, &data)
	}
	if resp.EstimatedCredits > 0 {
		data["estimated_credits"] = resp.EstimatedCredits
		data["billing_estimated_credits"] = resp.EstimatedCredits
	}
	if resp.ActualCredits > 0 {
		data["actual_credits"] = resp.ActualCredits
	}
	if resp.BillingRequestID != "" {
		data["billing_request_id"] = resp.BillingRequestID
	}
	if resp.BillingAttemptID != "" {
		data["billing_attempt_id"] = resp.BillingAttemptID
	}
	return data
}

func videoResponseErrorMessage(resp *adapter.VideoResponse) string {
	if resp == nil {
		return ""
	}
	if message := errorMessageFromAny(resp.Error); message != "" {
		return message
	}
	if resp.Raw != nil {
		if message := errorMessageFromAny(resp.Raw["error"]); message != "" {
			return message
		}
		if message := errorMessageFromAny(resp.Raw["error_message"]); message != "" {
			return message
		}
	}
	return ""
}

func errorMessageFromAny(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"message", "error_message"} {
			if message := errorMessageFromAny(typed[key]); message != "" {
				return message
			}
		}
		if message := errorMessageFromAny(typed["error"]); message != "" {
			return message
		}
		if code := errorMessageFromAny(typed["code"]); code != "" {
			return code
		}
	case []any:
		for _, item := range typed {
			if message := errorMessageFromAny(item); message != "" {
				return message
			}
		}
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		var mapped map[string]any
		if err := json.Unmarshal(raw, &mapped); err != nil {
			return ""
		}
		return errorMessageFromAny(mapped)
	}
	return ""
}

func copyVideoBillingMetadata(dst map[string]interface{}, src map[string]any) {
	if dst == nil || src == nil {
		return
	}
	for _, key := range []string{"billing_request_id", "billing_attempt_id", "billing_estimated_credits", "estimated_credits"} {
		if value, ok := src[key]; ok && value != nil {
			dst[key] = value
		}
	}
	if _, ok := dst["billing_estimated_credits"]; !ok {
		if value, ok := src["estimated_credits"]; ok && value != nil {
			dst["billing_estimated_credits"] = value
		}
	}
}

func jsonData(value map[string]any) datatypes.JSON {
	raw, err := json.Marshal(value)
	if err != nil {
		return datatypes.JSON([]byte(`{}`))
	}
	return datatypes.JSON(raw)
}

func taskFromRecord(record videoTaskRecord) VideoTask {
	return VideoTask{
		ID:               record.ID.String(),
		TaskID:           record.TaskID,
		UpstreamTaskID:   record.UpstreamTaskID,
		Provider:         record.Provider,
		Model:            record.Model,
		ModelLabel:       record.ModelLabel,
		Prompt:           record.Prompt,
		Status:           record.Status,
		VideoURL:         record.VideoURL,
		ErrorMessage:     record.ErrorMessage,
		DurationSeconds:  record.DurationSeconds,
		Resolution:       record.Resolution,
		Ratio:            record.Ratio,
		HasInputVideo:    record.HasInputVideo,
		GenerateAudio:    record.GenerateAudio,
		Voice:            record.Voice,
		EstimatedCredits: record.EstimatedCredits,
		ActualCredits:    record.ActualCredits,
		RequestPayload:   mapFromJSON(record.RequestPayload),
		ResponsePayload:  mapFromJSON(record.ResponsePayload),
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
		CompletedAt:      record.CompletedAt,
	}
}

func mapFromJSON(data datatypes.JSON) map[string]any {
	if len(data) == 0 {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}
