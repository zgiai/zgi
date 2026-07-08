package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	promptdto "github.com/zgiai/zgi/api/internal/modules/prompts/dto"
	promptmodel "github.com/zgiai/zgi/api/internal/modules/prompts/model"
	promptrepo "github.com/zgiai/zgi/api/internal/modules/prompts/repository"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	shared_visibility "github.com/zgiai/zgi/api/internal/modules/shared/visibility"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	latestLabel     = "latest"
	productionLabel = "production"
)

var slugSanitizer = regexp.MustCompile(`[^a-z0-9/_-]+`)

type promptChatMessageInput struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type PromptService interface {
	List(ctx context.Context, organizationID, accountID string, req promptdto.PromptListRequest) (*promptdto.PromptListResponse, error)
	GetDetail(ctx context.Context, organizationID, accountID, id string) (*promptdto.PromptDetailResponse, error)
	Create(ctx context.Context, organizationID, accountID string, req promptdto.CreatePromptRequest) (*promptdto.PromptDetailResponse, error)
	Update(ctx context.Context, organizationID, accountID, id string, req promptdto.UpdatePromptRequest) (*promptdto.PromptDetailResponse, error)
	CreateVersion(ctx context.Context, organizationID, accountID, id string, req promptdto.PromptVersionInput) (*promptdto.PromptDetailResponse, error)
	SetLabels(ctx context.Context, organizationID, accountID, id string, req promptdto.SetPromptLabelsRequest) (*promptdto.PromptDetailResponse, error)
	Optimize(ctx context.Context, organizationID, accountID, workspaceID string, req promptdto.PromptOptimizeRequest) (*promptdto.PromptOptimizeResponse, error)
	OptimizeStream(ctx context.Context, organizationID, accountID, workspaceID string, req promptdto.PromptOptimizeRequest, onEvent func(PromptOptimizeStreamEvent) error) (*promptdto.PromptOptimizeResponse, error)
	PlaygroundStream(ctx context.Context, organizationID, accountID, workspaceID string, req promptdto.PromptPlaygroundRequest, onEvent func(PromptOptimizeStreamEvent) error) error
	GetUsageSummary(ctx context.Context, organizationID, accountID, id string) (*promptdto.PromptUsageSummaryResponse, error)
	ListOptimizationRuns(ctx context.Context, organizationID, accountID, promptID string, req promptdto.PromptOptimizationRunListRequest) (*promptdto.PromptOptimizationRunListResponse, error)
	AdoptOptimizationRun(ctx context.Context, organizationID, accountID, promptID, runID string, req promptdto.PromptOptimizationAdoptRequest) (*promptdto.PromptDetailResponse, error)
	ResolveRuntimeReference(ctx context.Context, organizationID, workspaceID string, ref RuntimePromptReference) (*ResolvedRuntimePrompt, error)
}

type PromptOptimizeStreamEvent struct {
	Event string
	Data  map[string]interface{}
}

type RuntimePromptReference struct {
	PromptID string
	Version  *int
	Label    *string
}

type ResolvedRuntimePrompt struct {
	Prompt   *promptmodel.Prompt
	Version  *promptmodel.PromptVersion
	Resolved string
}

type promptService struct {
	repo                promptrepo.PromptRepository
	organizationService interfaces.OrganizationService
	llmClient           llmclient.LLMClient
	defaultModelSvc     llmdefaultservice.DefaultModelService
}

func NewPromptService(
	repo promptrepo.PromptRepository,
	organizationService interfaces.OrganizationService,
	llmClient llmclient.LLMClient,
	defaultModelSvc llmdefaultservice.DefaultModelService,
) PromptService {
	return &promptService{
		repo:                repo,
		organizationService: organizationService,
		llmClient:           llmClient,
		defaultModelSvc:     defaultModelSvc,
	}
}

func (s *promptService) List(ctx context.Context, organizationID, accountID string, req promptdto.PromptListRequest) (*promptdto.PromptListResponse, error) {
	page, limit := normalizePageLimit(req.Page, req.Limit)
	scope, err := shared_visibility.ResolveVisibleWorkspaceScope(
		ctx,
		s.organizationService,
		organizationID,
		accountID,
		req.WorkspaceID,
		promptVisiblePermissionCodes()...,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve prompt visibility scope: %w", err)
	}
	if len(scope.WorkspaceIDs) == 0 && !scope.AllowOrganizationScoped {
		return &promptdto.PromptListResponse{
			Data:    []promptdto.PromptSummaryResponse{},
			HasMore: false,
			Limit:   limit,
			Page:    page,
			Total:   0,
		}, nil
	}

	query := applyAccessibleQuery(s.repo.DB().Model(&promptmodel.Prompt{}), scope, accountID)

	if keyword := strings.TrimSpace(req.Keyword); keyword != "" {
		pattern := "%" + keyword + "%"
		query = query.Where("name ILIKE ? OR slug ILIKE ? OR description ILIKE ?", pattern, pattern, pattern)
	}
	if locale := strings.TrimSpace(req.Locale); locale != "" {
		query = query.Where("locale = ?", locale)
	}
	if source := strings.TrimSpace(req.Source); source != "" {
		query = query.Where("source = ?", source)
	}
	if category := strings.TrimSpace(req.Category); category != "" {
		query = query.Where("category = ?", category)
	}
	if req.WorkspaceID != "" {
		query = query.Where("(workspace_id = ? OR source = ?)", req.WorkspaceID, promptmodel.PromptSourceOfficial)
	}

	prompts, total, err := s.repo.List(ctx, query, page, limit)
	if err != nil {
		return nil, fmt.Errorf("list prompts: %w", err)
	}

	promptIDs := make([]string, 0, len(prompts))
	for _, prompt := range prompts {
		promptIDs = append(promptIDs, prompt.ID)
	}
	latestVersions, err := s.repo.FindLatestVersions(ctx, promptIDs)
	if err != nil {
		return nil, fmt.Errorf("load latest prompt versions: %w", err)
	}
	allVersions, err := s.repo.FindVersionsByPromptIDs(ctx, promptIDs)
	if err != nil {
		return nil, fmt.Errorf("load prompt release versions: %w", err)
	}
	productionVersions := versionsByLabel(allVersions, productionLabel)

	items := make([]promptdto.PromptSummaryResponse, 0, len(prompts))
	for _, prompt := range prompts {
		items = append(items, promptdto.BuildPromptSummary(prompt, latestVersions[prompt.ID], productionVersions[prompt.ID], accountID))
	}

	return &promptdto.PromptListResponse{
		Data:    items,
		HasMore: int64(page*limit) < total,
		Limit:   limit,
		Page:    page,
		Total:   total,
	}, nil
}

func (s *promptService) GetDetail(ctx context.Context, organizationID, accountID, id string) (*promptdto.PromptDetailResponse, error) {
	prompt, err := s.getAccessiblePrompt(ctx, organizationID, accountID, id, promptVisiblePermissionCodes()...)
	if err != nil {
		return nil, err
	}
	return s.buildDetail(ctx, prompt, accountID)
}

func (s *promptService) Create(ctx context.Context, organizationID, accountID string, req promptdto.CreatePromptRequest) (*promptdto.PromptDetailResponse, error) {
	source := promptmodel.PromptSource(req.Source)
	if source != promptmodel.PromptSourcePersonal && source != promptmodel.PromptSourceWorkspace {
		return nil, fmt.Errorf("unsupported prompt source %q", req.Source)
	}

	scope, err := shared_visibility.ResolveVisibleWorkspaceScope(
		ctx,
		s.organizationService,
		organizationID,
		accountID,
		req.WorkspaceID,
		promptCreatePermissionCodes()...,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace scope: %w", err)
	}
	if !slices.Contains(scope.WorkspaceIDs, req.WorkspaceID) {
		return nil, fmt.Errorf("workspace not accessible")
	}

	content, config, promptType, labels, err := normalizeVersionInput(req.InitialVersion, true)
	if err != nil {
		return nil, err
	}

	workspaceID := req.WorkspaceID
	ownerID := accountID
	prompt := &promptmodel.Prompt{
		OrganizationID: stringPtr(organizationID),
		WorkspaceID:    &workspaceID,
		OwnerAccountID: &ownerID,
		Source:         source,
		Name:           strings.TrimSpace(req.Name),
		Slug:           ensurePromptSlug(req.Slug, req.Name),
		Description:    trimOptional(req.Description),
		Locale:         normalizeLocale(req.Locale),
		Category:       trimOptional(req.Category),
		Tags:           uniqueNonEmpty(req.Tags),
		LatestVersion:  1,
	}
	version := &promptmodel.PromptVersion{
		PromptType:    promptType,
		Content:       content,
		Config:        config,
		Labels:        labels,
		CommitMessage: trimOptional(req.InitialVersion.CommitMessage),
		CreatedBy:     &accountID,
		Version:       1,
	}

	err = s.repo.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(prompt).Error; err != nil {
			return err
		}
		version.PromptID = prompt.ID
		if err := tx.Create(version).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("create prompt: %w", err)
	}
	return s.buildDetail(ctx, prompt, accountID)
}

func (s *promptService) Update(ctx context.Context, organizationID, accountID, id string, req promptdto.UpdatePromptRequest) (*promptdto.PromptDetailResponse, error) {
	prompt, err := s.getAccessiblePrompt(ctx, organizationID, accountID, id, promptUpdatePermissionCodes()...)
	if err != nil {
		return nil, err
	}
	if prompt.Source == promptmodel.PromptSourceOfficial {
		return nil, fmt.Errorf("official prompts are read only")
	}
	if req.Name != nil {
		prompt.Name = strings.TrimSpace(*req.Name)
		if prompt.Name == "" {
			return nil, fmt.Errorf("name cannot be empty")
		}
	}
	if req.Description != nil {
		prompt.Description = trimOptional(req.Description)
	}
	if req.Locale != nil {
		prompt.Locale = normalizeLocale(*req.Locale)
	}
	if req.Category != nil {
		prompt.Category = trimOptional(req.Category)
	}
	if req.Source != nil {
		source := promptmodel.PromptSource(*req.Source)
		if source != promptmodel.PromptSourcePersonal && source != promptmodel.PromptSourceWorkspace {
			return nil, fmt.Errorf("unsupported prompt source")
		}
		prompt.Source = source
	}
	if req.Tags != nil {
		prompt.Tags = uniqueNonEmpty(req.Tags)
	}
	if err := s.repo.Update(ctx, prompt); err != nil {
		return nil, fmt.Errorf("update prompt: %w", err)
	}
	return s.buildDetail(ctx, prompt, accountID)
}

func (s *promptService) CreateVersion(ctx context.Context, organizationID, accountID, id string, req promptdto.PromptVersionInput) (*promptdto.PromptDetailResponse, error) {
	prompt, err := s.getAccessiblePrompt(ctx, organizationID, accountID, id, promptVersionManagePermissionCodes()...)
	if err != nil {
		return nil, err
	}
	if prompt.Source == promptmodel.PromptSourceOfficial {
		return nil, fmt.Errorf("official prompts are read only")
	}
	content, config, promptType, labels, err := normalizeVersionInput(req, true)
	if err != nil {
		return nil, err
	}

	version := &promptmodel.PromptVersion{
		PromptID:      prompt.ID,
		Version:       prompt.LatestVersion + 1,
		PromptType:    promptType,
		Content:       content,
		Config:        config,
		Labels:        labels,
		CommitMessage: trimOptional(req.CommitMessage),
		CreatedBy:     &accountID,
	}

	err = s.repo.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var versions []*promptmodel.PromptVersion
		if err := tx.Where("prompt_id = ?", prompt.ID).Find(&versions).Error; err != nil {
			return err
		}
		if err := reassignLabels(tx, versions, version.Version, labels, false); err != nil {
			return err
		}
		prompt.LatestVersion = version.Version
		if err := tx.Save(prompt).Error; err != nil {
			return err
		}
		return tx.Create(version).Error
	})
	if err != nil {
		return nil, fmt.Errorf("create prompt version: %w", err)
	}
	return s.buildDetail(ctx, prompt, accountID)
}

func (s *promptService) SetLabels(ctx context.Context, organizationID, accountID, id string, req promptdto.SetPromptLabelsRequest) (*promptdto.PromptDetailResponse, error) {
	prompt, err := s.getAccessiblePrompt(ctx, organizationID, accountID, id, promptLabelManagePermissionCodes()...)
	if err != nil {
		return nil, err
	}
	if prompt.Source == promptmodel.PromptSourceOfficial {
		return nil, fmt.Errorf("official prompts are read only")
	}
	labels := uniqueLabels(req.Labels, req.Version == prompt.LatestVersion)
	err = s.repo.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var versions []*promptmodel.PromptVersion
		if err := tx.Where("prompt_id = ?", prompt.ID).Find(&versions).Error; err != nil {
			return err
		}
		if err := reassignLabels(tx, versions, req.Version, labels, true); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("set prompt labels: %w", err)
	}
	return s.buildDetail(ctx, prompt, accountID)
}

func (s *promptService) ResolveRuntimeReference(ctx context.Context, organizationID, workspaceID string, ref RuntimePromptReference) (*ResolvedRuntimePrompt, error) {
	if strings.TrimSpace(ref.PromptID) == "" {
		return nil, fmt.Errorf("prompt_id is required")
	}

	prompt, err := s.repo.FindByID(ctx, ref.PromptID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("prompt not found")
		}
		return nil, fmt.Errorf("load prompt: %w", err)
	}

	if !promptVisibleAtRuntime(prompt, organizationID, workspaceID) {
		return nil, fmt.Errorf("prompt not accessible in this workflow context")
	}

	label := runtimeReferenceLabel(ref)

	var version promptmodel.PromptVersion
	query := s.repo.DB().WithContext(ctx).Where("prompt_id = ?", prompt.ID)
	if ref.Version != nil && *ref.Version > 0 {
		query = query.Where("version = ?", *ref.Version)
	} else {
		query = query.Where("labels::jsonb ? ?", label)
	}

	if err := query.Order("version DESC").First(&version).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("prompt version not found")
		}
		return nil, fmt.Errorf("load prompt version: %w", err)
	}

	var resolved string
	if version.PromptType == promptmodel.PromptTypeText {
		if err := json.Unmarshal(version.Content, &resolved); err != nil {
			return nil, fmt.Errorf("decode text prompt content: %w", err)
		}
	}

	return &ResolvedRuntimePrompt{
		Prompt:   prompt,
		Version:  &version,
		Resolved: resolved,
	}, nil
}

func (s *promptService) getAccessiblePrompt(ctx context.Context, organizationID, accountID, id string, permissionCodes ...workspace_model.WorkspacePermissionCode) (*promptmodel.Prompt, error) {
	prompt, err := s.repo.FindByID(ctx, id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("prompt not found")
		}
		return nil, fmt.Errorf("load prompt: %w", err)
	}
	if prompt.Source == promptmodel.PromptSourceOfficial {
		scope, err := shared_visibility.ResolveVisibleWorkspaceScope(
			ctx,
			s.organizationService,
			organizationID,
			accountID,
			"",
			permissionCodes...,
		)
		if err != nil {
			return nil, fmt.Errorf("resolve prompt access: %w", err)
		}
		if len(scope.WorkspaceIDs) == 0 && !scope.AllowOrganizationScoped {
			return nil, fmt.Errorf("prompt not found")
		}
		return prompt, nil
	}
	scope, err := shared_visibility.ResolveVisibleWorkspaceScope(
		ctx,
		s.organizationService,
		organizationID,
		accountID,
		derefString(prompt.WorkspaceID),
		permissionCodes...,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve prompt access: %w", err)
	}
	if prompt.WorkspaceID == nil || !slices.Contains(scope.WorkspaceIDs, *prompt.WorkspaceID) {
		return nil, fmt.Errorf("prompt not found")
	}
	if prompt.Source == promptmodel.PromptSourcePersonal && (prompt.OwnerAccountID == nil || *prompt.OwnerAccountID != accountID) {
		return nil, fmt.Errorf("prompt not found")
	}
	return prompt, nil
}

func (s *promptService) ensureWorkspaceAccess(ctx context.Context, organizationID, accountID, workspaceID string, permissionCodes ...workspace_model.WorkspacePermissionCode) error {
	if strings.TrimSpace(workspaceID) == "" {
		return fmt.Errorf("workspace is required")
	}
	scope, err := shared_visibility.ResolveVisibleWorkspaceScope(
		ctx,
		s.organizationService,
		organizationID,
		accountID,
		workspaceID,
		permissionCodes...,
	)
	if err != nil {
		return fmt.Errorf("resolve workspace access: %w", err)
	}
	if !slices.Contains(scope.WorkspaceIDs, workspaceID) {
		return fmt.Errorf("workspace not accessible")
	}
	return nil
}

func (s *promptService) buildDetail(ctx context.Context, prompt *promptmodel.Prompt, accountID string) (*promptdto.PromptDetailResponse, error) {
	versions, err := s.repo.FindVersions(ctx, prompt.ID)
	if err != nil {
		return nil, fmt.Errorf("load prompt versions: %w", err)
	}
	var latest *promptmodel.PromptVersion
	if len(versions) > 0 {
		for _, version := range versions {
			if version.Version == prompt.LatestVersion {
				latest = version
				break
			}
		}
		if latest == nil {
			latest = versions[0]
		}
	}
	resp := &promptdto.PromptDetailResponse{
		PromptSummaryResponse: promptdto.BuildPromptSummary(prompt, latest, firstVersionWithLabel(versions, productionLabel), accountID),
		Versions:              make([]promptdto.PromptVersionResponse, 0, len(versions)),
	}
	for _, version := range versions {
		resp.Versions = append(resp.Versions, promptdto.BuildPromptVersionResponse(version))
	}
	return resp, nil
}

func applyAccessibleQuery(query *gorm.DB, scope shared_visibility.VisibleWorkspaceScope, accountID string) *gorm.DB {
	accessibleScope := query.Session(&gorm.Session{}).
		Where("source = ?", promptmodel.PromptSourceOfficial).
		Or("source = ? AND workspace_id IN ?", promptmodel.PromptSourceWorkspace, scope.WorkspaceIDs).
		Or(
			"source = ? AND owner_account_id = ? AND workspace_id IN ?",
			promptmodel.PromptSourcePersonal,
			accountID,
			scope.WorkspaceIDs,
		)
	return query.Where(accessibleScope)
}

func normalizePageLimit(page, limit int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return page, limit
}

func normalizeVersionInput(input promptdto.PromptVersionInput, ensureLatest bool) (datatypes.JSON, datatypes.JSON, promptmodel.PromptType, []string, error) {
	promptType := promptmodel.PromptType(strings.TrimSpace(input.PromptType))
	if promptType != promptmodel.PromptTypeText && promptType != promptmodel.PromptTypeChat {
		return nil, nil, "", nil, fmt.Errorf("unsupported prompt type")
	}
	content := bytesOrNull(input.Content)
	if !json.Valid(content) {
		return nil, nil, "", nil, fmt.Errorf("content must be valid JSON")
	}
	if promptType == promptmodel.PromptTypeText {
		var text string
		if err := json.Unmarshal(content, &text); err != nil {
			return nil, nil, "", nil, fmt.Errorf("text prompt content must be a JSON string")
		}
		if strings.TrimSpace(text) == "" {
			return nil, nil, "", nil, fmt.Errorf("text prompt content cannot be empty")
		}
	} else {
		var messages []promptChatMessageInput
		if err := json.Unmarshal(content, &messages); err != nil {
			return nil, nil, "", nil, fmt.Errorf("chat prompt content must be a JSON array of messages")
		}
		if len(messages) == 0 {
			return nil, nil, "", nil, fmt.Errorf("chat prompt content cannot be empty")
		}
		for i, message := range messages {
			role := strings.TrimSpace(message.Role)
			if role != "system" && role != "user" && role != "assistant" {
				return nil, nil, "", nil, fmt.Errorf("chat prompt message %d must use role system, user, or assistant", i+1)
			}
			if strings.TrimSpace(message.Content) == "" {
				return nil, nil, "", nil, fmt.Errorf("chat prompt message %d content cannot be empty", i+1)
			}
		}
	}

	configJSON, err := json.Marshal(input.Config)
	if err != nil {
		return nil, nil, "", nil, fmt.Errorf("marshal prompt config: %w", err)
	}
	labels := uniqueLabels(input.Labels, ensureLatest)
	return datatypes.JSON(content), datatypes.JSON(configJSON), promptType, labels, nil
}

func uniqueLabels(input []string, ensureLatest bool) []string {
	labels := make([]string, 0, len(input)+1)
	seen := map[string]struct{}{}
	for _, label := range input {
		normalized := strings.TrimSpace(label)
		if normalized == "" || normalized == latestLabel {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		labels = append(labels, normalized)
	}
	if ensureLatest {
		labels = append(labels, latestLabel)
	}
	return labels
}

func runtimeReferenceLabel(ref RuntimePromptReference) string {
	if ref.Label != nil && strings.TrimSpace(*ref.Label) != "" {
		return strings.TrimSpace(*ref.Label)
	}
	return latestLabel
}

func versionsByLabel(versions []*promptmodel.PromptVersion, label string) map[string]*promptmodel.PromptVersion {
	result := map[string]*promptmodel.PromptVersion{}
	for _, version := range versions {
		if !slices.Contains(version.Labels, label) {
			continue
		}
		if existing := result[version.PromptID]; existing == nil || version.Version > existing.Version {
			result[version.PromptID] = version
		}
	}
	return result
}

func firstVersionWithLabel(versions []*promptmodel.PromptVersion, label string) *promptmodel.PromptVersion {
	for _, version := range versions {
		if slices.Contains(version.Labels, label) {
			return version
		}
	}
	return nil
}

func reassignLabels(tx *gorm.DB, versions []*promptmodel.PromptVersion, targetVersion int, targetLabels []string, requireTarget bool) error {
	targetSet := map[string]struct{}{}
	for _, label := range targetLabels {
		targetSet[label] = struct{}{}
	}
	targetFound := false
	for _, version := range versions {
		if version.Version == targetVersion {
			targetFound = true
			break
		}
	}
	if requireTarget && !targetFound {
		return fmt.Errorf("prompt version not found")
	}
	for _, version := range versions {
		nextLabels := reassignedVersionLabels(version, targetVersion, targetLabels, targetSet)
		labelsJSON, err := json.Marshal(nextLabels)
		if err != nil {
			return err
		}
		if err := tx.Model(&promptmodel.PromptVersion{}).Where("id = ?", version.ID).Update("labels", datatypes.JSON(labelsJSON)).Error; err != nil {
			return err
		}
	}
	return nil
}

func reassignedVersionLabels(version *promptmodel.PromptVersion, targetVersion int, targetLabels []string, targetSet map[string]struct{}) []string {
	if version.Version == targetVersion {
		return uniqueLabels(targetLabels, slices.Contains(targetLabels, latestLabel))
	}
	nextLabels := make([]string, 0, len(version.Labels))
	for _, label := range version.Labels {
		if hasLabel(targetSet, label) {
			continue
		}
		nextLabels = append(nextLabels, label)
	}
	return nextLabels
}

func hasLabel(labels map[string]struct{}, label string) bool {
	_, ok := labels[label]
	return ok
}

func ensurePromptSlug(raw, fallback string) string {
	slug := strings.TrimSpace(strings.ToLower(raw))
	if slug == "" {
		slug = strings.TrimSpace(strings.ToLower(fallback))
	}
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = slugSanitizer.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "prompt"
	}
	return slug
}

func uniqueNonEmpty(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func normalizeLocale(locale string) string {
	trimmed := strings.TrimSpace(locale)
	if trimmed == "" {
		return "zh-Hans"
	}
	return trimmed
}

func trimOptional(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func stringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func bytesOrNull(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte("null")
	}
	return raw
}

func promptVisibleAtRuntime(prompt *promptmodel.Prompt, organizationID, workspaceID string) bool {
	if prompt == nil {
		return false
	}
	switch prompt.Source {
	case promptmodel.PromptSourceOfficial:
		return true
	default:
		if prompt.OrganizationID != nil && strings.TrimSpace(*prompt.OrganizationID) != "" && *prompt.OrganizationID != organizationID {
			return false
		}
		if prompt.WorkspaceID != nil && strings.TrimSpace(*prompt.WorkspaceID) != "" && *prompt.WorkspaceID != workspaceID {
			return false
		}
		return true
	}
}
