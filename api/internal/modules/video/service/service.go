package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	llmnode "github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/llm"
	workflowtoolfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelsvc "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	maxVideoPromptRunes          = 4000
	maxVideoSearchRunes          = 200
	maxVideoClientRequestIDRunes = 120
	videoRuntimeAppType          = "video-runtime"
	defaultRatio                 = "16:9"
	defaultResolution            = "720p"
	defaultDuration              = 5
	videoCreateTimeout           = 2 * time.Minute

	videoPredeductPointsPerSecond int64 = 143
	internalCreditsPerPoint       int64 = 1000
)

type LLMVideoClient interface {
	AppCreateVideo(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.VideoRequest) (*adapter.VideoResponse, error)
	AppGetVideoTask(ctx context.Context, appCtx *llmclient.AppContext, req *adapter.VideoTaskRequest) (*adapter.VideoResponse, error)
}

type VideoArtifactSaver interface {
	SaveRemoteVideo(ctx context.Context, scope Scope, videoURL string) (string, error)
}

type Service interface {
	ListModels(ctx context.Context, scope Scope) ([]VideoModel, error)
	Generate(ctx context.Context, scope Scope, req GenerateRequest) (*GenerateResult, error)
	ListTasks(ctx context.Context, scope Scope, query ListTasksQuery) (*ListTasksResult, error)
	GetTask(ctx context.Context, scope Scope, taskID string) (*VideoTask, error)
	DeleteTask(ctx context.Context, scope Scope, taskID string) error
}

type service struct {
	availableModels llmmodelsvc.AvailableModelsService
	llmClient       LLMVideoClient
	tasks           *taskRepository
	artifactSaver   VideoArtifactSaver
}

func NewService(db *gorm.DB, availableModels llmmodelsvc.AvailableModelsService, llmClient interface{}) Service {
	videoClient, _ := llmClient.(LLMVideoClient)
	return &service{
		availableModels: availableModels,
		llmClient:       videoClient,
		tasks:           newTaskRepository(db),
		artifactSaver:   defaultVideoArtifactSaver{},
	}
}

type defaultVideoArtifactSaver struct{}

func (defaultVideoArtifactSaver) SaveRemoteVideo(_ context.Context, scope Scope, videoURL string) (string, error) {
	videoURL = strings.TrimSpace(videoURL)
	if videoURL == "" {
		return "", errors.New("video url is empty")
	}
	saver := llmnode.NewFileSaverImplGlobalWithLifecycleAndURLMode(
		scope.AccountID.String(),
		scope.OrganizationID.String(),
		workflowtoolfile.ToolFileLifecyclePersistent,
		nil,
		workflowtoolfile.ToolFileURLModePermanent,
	)
	stored, err := saver.SaveRemoteURL(videoURL, workflowfile.FileTypeVideo)
	if err != nil {
		return "", err
	}
	if stored == nil || stored.URL == nil || strings.TrimSpace(*stored.URL) == "" {
		return "", errors.New("stored video url is empty")
	}
	return strings.TrimSpace(*stored.URL), nil
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
	clientRequestID := normalizeClientRequestID(req.ClientRequestID)
	if clientRequestID != "" {
		existing, err := s.tasks.findByClientRequestID(operationCtx, scope, clientRequestID)
		if err == nil {
			return &GenerateResult{Task: taskFromRecord(*existing)}, nil
		}
		if !errors.Is(err, ErrTaskNotFound) {
			return nil, err
		}
	}

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
	now := time.Now().UTC()
	record := &videoTaskRecord{
		ID:               appID,
		OrganizationID:   scope.OrganizationID,
		AccountID:        scope.AccountID,
		WorkspaceID:      scope.WorkspaceID,
		TaskID:           localVideoTaskID(),
		ClientRequestID:  clientRequestID,
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
		if clientRequestID != "" {
			if existing, findErr := s.tasks.findByClientRequestID(operationCtx, scope, clientRequestID); findErr == nil {
				return &GenerateResult{Task: taskFromRecord(*existing)}, nil
			}
		}
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

func normalizeClientRequestID(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= maxVideoClientRequestIDRunes {
		return value
	}
	return string(runes[:maxVideoClientRequestIDRunes])
}

func (s *service) startUpstreamVideoTask(record videoTaskRecord, appCtx *llmclient.AppContext, videoReq *adapter.VideoRequest) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), videoCreateTimeout)
		defer cancel()

		resp, err := s.llmClient.AppCreateVideo(ctx, appCtx, videoReq)
		now := time.Now().UTC()
		if err != nil {
			record.Status = "failed"
			record.ErrorMessage = videoErrorMessage(err)
			record.EstimatedCredits = 0
			record.ActualCredits = 0
			record.UpdatedAt = now
			record.CompletedAt = &now
			_ = s.tasks.save(context.Background(), &record)
			return
		}
		if resp == nil {
			record.Status = "failed"
			record.ErrorMessage = ErrUpstreamFailed.Error()
			record.EstimatedCredits = 0
			record.ActualCredits = 0
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
		if upstreamID == "" && status != "failed" {
			record.Status = "failed"
			record.ErrorMessage = ErrUpstreamFailed.Error() + ": upstream task id is empty"
			if message := videoResponseErrorMessage(resp); message != "" {
				record.ErrorMessage = message
			}
			record.EstimatedCredits = 0
			record.ActualCredits = 0
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

		payload := videoResponsePayload(resp)
		if status == "succeeded" && videoURL != "" {
			scope := Scope{
				OrganizationID: record.OrganizationID,
				AccountID:      record.AccountID,
				WorkspaceID:    record.WorkspaceID,
			}
			videoURL = s.storeVideoArtifact(ctx, scope, videoURL, payload)
		}
		record.UpstreamTaskID = upstreamID
		record.Status = status
		record.VideoURL = videoURL
		if record.Status == "failed" {
			if message := videoResponseErrorMessage(resp); message != "" {
				record.ErrorMessage = message
			}
			record.EstimatedCredits = 0
			record.ActualCredits = 0
		}
		record.ResponsePayload = jsonData(payload)
		record.UpdatedAt = now
		if isTerminalVideoStatus(record.Status) {
			record.CompletedAt = &now
		}
		_ = s.tasks.save(context.Background(), &record)
	}()
}

type videoTaskCursor struct {
	CreatedAt time.Time `json:"created_at"`
	ID        uuid.UUID `json:"id"`
}

func (s *service) ListTasks(ctx context.Context, scope Scope, query ListTasksQuery) (*ListTasksResult, error) {
	search := strings.TrimSpace(query.Search)
	if len([]rune(search)) > maxVideoSearchRunes {
		return nil, ErrSearchTooLong
	}
	limit := query.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	params := taskListParams{Limit: limit, Search: search}
	if cursor := strings.TrimSpace(query.Cursor); cursor != "" {
		decoded, err := decodeVideoTaskCursor(cursor)
		if err != nil {
			return nil, ErrInvalidCursor
		}
		params.BeforeCreatedAt = &decoded.CreatedAt
		params.BeforeID = &decoded.ID
	}
	page, err := s.tasks.list(ctx, scope, params)
	if err != nil {
		return nil, err
	}
	result := make([]VideoTask, 0, len(page.Records))
	for _, record := range page.Records {
		result = append(result, taskFromRecord(record))
	}
	nextCursor := ""
	if page.HasMore && len(page.Records) > 0 {
		last := page.Records[len(page.Records)-1]
		nextCursor = encodeVideoTaskCursor(videoTaskCursor{CreatedAt: last.CreatedAt, ID: last.ID})
	}
	return &ListTasksResult{
		Data:       result,
		Total:      page.Total,
		HasMore:    page.HasMore,
		NextCursor: nextCursor,
	}, nil
}

func encodeVideoTaskCursor(cursor videoTaskCursor) string {
	payload, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeVideoTaskCursor(value string) (videoTaskCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return videoTaskCursor{}, err
	}
	var cursor videoTaskCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return videoTaskCursor{}, err
	}
	if cursor.CreatedAt.IsZero() || cursor.ID == uuid.Nil {
		return videoTaskCursor{}, errors.New("cursor is incomplete")
	}
	return cursor, nil
}

func (s *service) GetTask(ctx context.Context, scope Scope, taskID string) (*VideoTask, error) {
	record, err := s.tasks.findByTaskID(ctx, scope, strings.TrimSpace(taskID))
	if err != nil {
		return nil, err
	}
	if shouldRefreshVideoTask(record) && s.llmClient != nil {
		if err := s.refreshTask(ctx, scope, record); err != nil {
			_ = s.markVideoRuntimeTaskFailedFromError(ctx, record, err)
		}
	}
	task := taskFromRecord(*record)
	return &task, nil
}

func (s *service) DeleteTask(ctx context.Context, scope Scope, taskID string) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ErrTaskNotFound
	}
	return s.tasks.deleteByTaskID(ctx, scope, taskID)
}

func (s *service) markVideoRuntimeTaskFailedFromError(ctx context.Context, record *videoTaskRecord, err error) error {
	if s == nil || s.tasks == nil || record == nil || err == nil {
		return nil
	}
	message := err.Error()
	if !errors.Is(err, ErrUpstreamFailed) {
		message = fmt.Errorf("%w: %v", ErrUpstreamFailed, err).Error()
	}
	now := time.Now().UTC()
	record.Status = "failed"
	record.ErrorMessage = message
	if extracted := videoErrorMessage(err); extracted != "" {
		record.ErrorMessage = extracted
	}
	record.EstimatedCredits = 0
	record.ActualCredits = 0
	record.UpdatedAt = now
	record.CompletedAt = &now
	return s.tasks.save(ctx, record)
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
		record.EstimatedCredits = 0
		record.ActualCredits = 0
	}
	payload := videoResponsePayload(resp)
	if videoURL := firstVideoURL(resp); videoURL != "" {
		if record.Status == "succeeded" {
			videoURL = s.storeVideoArtifact(ctx, scope, videoURL, payload)
		}
		record.VideoURL = videoURL
	}
	if record.Status != "failed" && resp.EstimatedCredits > 0 {
		record.EstimatedCredits = resp.EstimatedCredits
	}
	if record.Status != "failed" && resp.ActualCredits > 0 {
		record.ActualCredits = resp.ActualCredits
	}
	record.ResponsePayload = jsonData(payload)
	record.UpdatedAt = time.Now().UTC()
	if isTerminalVideoStatus(record.Status) && record.CompletedAt == nil {
		now := time.Now().UTC()
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
	return "video-" + time.Now().UTC().Format("20060102150405") + "-" + id
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

func (s *service) storeVideoArtifact(ctx context.Context, scope Scope, videoURL string, payload map[string]any) string {
	videoURL = strings.TrimSpace(videoURL)
	if videoURL == "" || s == nil || s.artifactSaver == nil || isStoredVideoArtifactURL(videoURL) {
		return videoURL
	}
	storedURL, err := s.artifactSaver.SaveRemoteVideo(ctx, scope, videoURL)
	if err != nil {
		if payload != nil {
			payload["video_transfer_status"] = "failed"
			payload["video_transfer_error"] = err.Error()
			payload["video_source_url"] = videoURL
		}
		return videoURL
	}
	if payload != nil {
		payload["video_transfer_status"] = "succeeded"
		payload["video_source_url"] = videoURL
		payload["stored_video_url"] = storedURL
	}
	return storedURL
}

func isStoredVideoArtifactURL(videoURL string) bool {
	value := strings.ToLower(strings.TrimSpace(videoURL))
	return strings.Contains(value, "/console/api/files/tools/")
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
		if message := nestedErrorMessage(resp.Raw); message != "" {
			return message
		}
		if message := errorMessageFromAny(resp.Raw["error_message"]); message != "" {
			return message
		}
	}
	return ""
}

func videoErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	var adapterErr *adapter.AdapterError
	if errors.As(err, &adapterErr) {
		if message := embeddedJSONErrorMessage(adapterErr.Message); message != "" {
			return message
		}
		if message := strings.TrimSpace(adapterErr.Message); message != "" {
			return message
		}
	}
	if message := embeddedJSONErrorMessage(err.Error()); message != "" {
		return message
	}
	if message := upstreamErrorMessage(err.Error()); message != "" {
		return message
	}
	if errors.Is(err, ErrUpstreamFailed) {
		return err.Error()
	}
	return fmt.Errorf("%w: %v", ErrUpstreamFailed, err).Error()
}

func upstreamErrorMessage(value string) string {
	const marker = "upstream error:"
	index := strings.LastIndex(strings.ToLower(value), marker)
	if index < 0 {
		return ""
	}
	message := strings.TrimSpace(value[index+len(marker):])
	if message == "" {
		return ""
	}
	return message
}

func embeddedJSONErrorMessage(value string) string {
	value = strings.TrimSpace(value)
	for index, char := range value {
		if char != '{' {
			continue
		}
		var data any
		decoder := json.NewDecoder(strings.NewReader(value[index:]))
		if err := decoder.Decode(&data); err != nil {
			continue
		}
		if message := errorMessageFromAny(data); message != "" {
			return message
		}
	}
	return ""
}

func nestedErrorMessage(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case map[string]any:
		for _, key := range []string{"error", "errors", "error_message", "errorMessage"} {
			if message := errorMessageFromAny(typed[key]); message != "" {
				return message
			}
		}
		for _, item := range typed {
			if message := nestedErrorMessage(item); message != "" {
				return message
			}
		}
	case []any:
		for _, item := range typed {
			if message := nestedErrorMessage(item); message != "" {
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
		return nestedErrorMessage(mapped)
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
