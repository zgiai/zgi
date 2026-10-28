package runtimeworkspace

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/zgiai/zgi/api/internal/capabilities/externalapikey"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/util"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrUnauthorized              = errors.New("runtime workspace credential is invalid")
	ErrWorkspaceDisabled         = errors.New("runtime workspace is disabled")
	ErrAppNotFound               = errors.New("runtime workspace app not found")
	ErrAppDisabled               = errors.New("runtime workspace app is disabled")
	ErrSubjectRequired           = errors.New("runtime workspace external user id is required")
	ErrFileNotFound              = errors.New("runtime workspace file not found")
	ErrInvalidWorkspace          = errors.New("runtime workspace update is invalid")
	ErrInvalidCredential         = errors.New("runtime workspace credential is invalid")
	ErrInvalidBinding            = errors.New("runtime workspace agent binding is invalid")
	ErrAppKeyConflict            = errors.New("runtime workspace app key already exists")
	ErrBindingRevisionConflict   = errors.New("runtime workspace agent bindings changed")
	ErrSourceWorkspacePermission = errors.New("runtime workspace source Workspace permission is required")
	ErrIdempotencyConflict       = errors.New("runtime workspace idempotency key was reused for another request")
	ErrOperationInProgress       = errors.New("runtime workspace credential operation is in progress")
	ErrSecretReplayExpired       = errors.New("runtime workspace credential secret replay window expired")
	ErrRequestDrainTimeout       = errors.New("runtime workspace requests did not drain before teardown")
	// ErrAppLimitExceeded reports more bound apps than a Runtime Workspace
	// may have.
	ErrAppLimitExceeded = errors.New("runtime workspace app limit exceeded")
	// ErrCredentialLimitExceeded reports too many active access keys.
	ErrCredentialLimitExceeded = errors.New("runtime workspace credential limit exceeded")
)

// MaxRuntimeWorkspaceApps is how many apps a Runtime Workspace may bind, of
// all types together: ZGI_RUNTIME_WORKSPACE_MAX_APPS, 500 by default.
func MaxRuntimeWorkspaceApps() int {
	if value, err := strconv.Atoi(strings.TrimSpace(os.Getenv("ZGI_RUNTIME_WORKSPACE_MAX_APPS"))); err == nil && value > 0 {
		return value
	}
	return defaultRuntimeWorkspaceApps
}

const (
	credentialSecretReplayTTL    = 30 * time.Minute
	credentialOperationRecordTTL = 24 * time.Hour
	defaultRuntimeWorkspaceApps  = 500
	// MaxActiveCredentials bounds the usable access keys of a Runtime
	// Workspace; revoked and expired keys do not count.
	MaxActiveCredentials       = 50
	runtimeRequestDrainTimeout = 30 * time.Second
	runtimeRequestDrainPoll    = 50 * time.Millisecond
)

type Service struct {
	db              *gorm.DB
	replayProtector CredentialReplayProtector
	webhookCipher   WebhookCipher
	now             func() time.Time
}

type ServiceOption func(*Service)

func WithCredentialReplayProtector(protector CredentialReplayProtector) ServiceOption {
	return func(service *Service) {
		service.replayProtector = protector
	}
}

func WithClock(now func() time.Time) ServiceOption {
	return func(service *Service) {
		if now != nil {
			service.now = now
		}
	}
}

func NewService(db *gorm.DB, options ...ServiceOption) *Service {
	if db == nil {
		panic("runtime workspace service requires db")
	}
	service := &Service{db: db, now: time.Now}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

// AcquireRequestLease admits one external Runtime request. The update is
// intentionally conditional on the Runtime Workspace still being active. It
// serializes with the deletion-side disable update, so a request that passed
// credential authentication but has not entered the request pipeline cannot
// start after teardown begins.
func (s *Service) AcquireRequestLease(ctx context.Context, runtimeWorkspaceID uuid.UUID) error {
	if s == nil || s.db == nil || runtimeWorkspaceID == uuid.Nil {
		return ErrUnauthorized
	}
	result := s.db.WithContext(ctx).Model(&RuntimeWorkspace{}).
		Where("runtime_id = ? AND status = ?", runtimeWorkspaceID, StatusActive).
		UpdateColumn("in_flight_requests", gorm.Expr("in_flight_requests + 1"))
	if result.Error != nil {
		return fmt.Errorf("acquire Runtime Workspace request lease: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return ErrWorkspaceDisabled
	}
	return nil
}

// ReleaseRequestLease releases an external Runtime request admitted by
// AcquireRequestLease. A detached context is used by the HTTP middleware so
// client disconnects cannot strand the counter.
func (s *Service) ReleaseRequestLease(ctx context.Context, runtimeWorkspaceID uuid.UUID) error {
	if s == nil || s.db == nil || runtimeWorkspaceID == uuid.Nil {
		return nil
	}
	result := s.db.WithContext(ctx).Model(&RuntimeWorkspace{}).
		Where("runtime_id = ? AND in_flight_requests > 0", runtimeWorkspaceID).
		UpdateColumn("in_flight_requests", gorm.Expr("in_flight_requests - 1"))
	if result.Error != nil {
		return fmt.Errorf("release Runtime Workspace request lease: %w", result.Error)
	}
	return nil
}

// WaitForRequestDrain prevents teardown from deleting Runtime-owned data while
// an already-admitted external request can still persist a Subject,
// conversation, or message. New requests are rejected by the disabled status
// before this drain begins.
func (s *Service) WaitForRequestDrain(ctx context.Context, identifier uuid.UUID) error {
	if s == nil || s.db == nil || identifier == uuid.Nil {
		return ErrInvalidWorkspace
	}
	drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runtimeRequestDrainTimeout)
	defer cancel()
	for {
		var workspace RuntimeWorkspace
		err := s.db.WithContext(drainCtx).
			Where("runtime_id = ? OR workspace_id = ?", identifier, identifier).
			First(&workspace).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect Runtime Workspace request leases: %w", err)
		}
		if workspace.InFlightRequests <= 0 {
			return nil
		}

		timer := time.NewTimer(runtimeRequestDrainPoll)
		select {
		case <-drainCtx.Done():
			if errors.Is(drainCtx.Err(), context.DeadlineExceeded) {
				return ErrRequestDrainTimeout
			}
			return drainCtx.Err()
		case <-timer.C:
		}
	}
}

// loadRuntimeWorkspace accepts either the stable Runtime Workspace ID or the
// attached resource Workspace ID. Keeping this compatibility lookup at the
// service boundary lets existing console routes continue to pass workspace_id
// while future isolated routes can address runtime_id directly.
type resourceWorkspaceState struct {
	ID             uuid.UUID
	OrganizationID uuid.UUID                       `gorm:"column:organization_id"`
	Status         workspace_model.WorkspaceStatus `gorm:"column:status"`
}

func loadResourceWorkspaceState(ctx context.Context, db *gorm.DB, workspaceID uuid.UUID, lock bool) (*resourceWorkspaceState, error) {
	if db == nil || workspaceID == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}
	query := db.WithContext(ctx).Table("workspaces").Select("id, organization_id, status").Where("id = ?", workspaceID)
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var workspace resourceWorkspaceState
	if err := query.Take(&workspace).Error; err != nil {
		return nil, err
	}
	return &workspace, nil
}

func requireNormalResourceWorkspace(ctx context.Context, db *gorm.DB, workspaceID uuid.UUID, lock bool) (*resourceWorkspaceState, error) {
	workspace, err := loadResourceWorkspaceState(ctx, db, workspaceID, lock)
	if err != nil {
		return nil, err
	}
	if workspace.Status != workspace_model.WorkspaceStatusNormal {
		return nil, ErrWorkspaceDisabled
	}
	return workspace, nil
}

func (s *Service) loadRuntimeWorkspace(ctx context.Context, identifier uuid.UUID) (*RuntimeWorkspace, error) {
	if identifier == uuid.Nil {
		return nil, gorm.ErrRecordNotFound
	}
	var workspace RuntimeWorkspace
	err := s.db.WithContext(ctx).Where("runtime_id = ?", identifier).First(&workspace).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = s.db.WithContext(ctx).Where("workspace_id = ?", identifier).First(&workspace).Error
	}
	if err != nil {
		return nil, err
	}
	return &workspace, nil
}

// ResolveRuntimeID returns the logical Runtime Workspace ID for either the
// current attached workspace identifier or a future isolated runtime ID.
func (s *Service) ResolveRuntimeID(ctx context.Context, identifier uuid.UUID) (uuid.UUID, error) {
	workspace, err := s.loadRuntimeWorkspace(ctx, identifier)
	if err != nil {
		return uuid.Nil, err
	}
	return workspace.LogicalID(), nil
}

type runtimeAgentRecord struct {
	ID              uuid.UUID
	AppType         string                          `gorm:"-"`
	TenantID        uuid.UUID                       `gorm:"column:tenant_id"`
	OrganizationID  uuid.UUID                       `gorm:"column:organization_id"`
	WorkspaceName   string                          `gorm:"column:workspace_name"`
	WorkspaceStatus workspace_model.WorkspaceStatus `gorm:"column:workspace_status"`
	Name            string
	Description     string
	Icon            string    `gorm:"column:icon"`
	IconType        string    `gorm:"column:icon_type"`
	EnableAPI       bool      `gorm:"column:enable_api"`
	WebAppStatus    string    `gorm:"column:web_app_status"`
	Published       bool      `gorm:"column:published"`
	Version         string    `gorm:"column:version"`
	VersionUUID     uuid.UUID `gorm:"column:version_uuid"`
	PublishedAt     time.Time `gorm:"column:published_at"`
	ConfigSnapshot  []byte    `gorm:"column:config_snapshot"`
}

type runtimePublishedPresentation struct {
	HomeTitle          string   `json:"home_title"`
	OpeningStatement   string   `json:"opening_statement"`
	InputPlaceholder   string   `json:"input_placeholder"`
	ThemeColor         string   `json:"theme_color"`
	SuggestedQuestions []string `json:"suggested_questions"`
	FileUploadEnabled  bool     `json:"file_upload_enabled"`
	SupportsVision     bool     `json:"supports_vision"`
	AgentMemoryEnabled bool     `json:"agent_memory_enabled"`
	AgentMemorySlots   []struct {
		Key     string `json:"key"`
		Enabled bool   `json:"enabled"`
	} `json:"agent_memory_slots"`
	EnabledSkillIDs     []string          `json:"enabled_skill_ids"`
	KnowledgeDatasetIDs []string          `json:"knowledge_dataset_ids"`
	DatabaseBindings    []json.RawMessage `json:"database_bindings"`
	WorkflowBindings    []json.RawMessage `json:"workflow_bindings"`
	ConnectorBindings   []json.RawMessage `json:"connector_bindings"`
}

func runtimeAgentPresentation(record runtimeAgentRecord) runtimePublishedPresentation {
	presentation := runtimePublishedPresentation{SuggestedQuestions: []string{}}
	if len(record.ConfigSnapshot) > 0 {
		_ = json.Unmarshal(record.ConfigSnapshot, &presentation)
	}
	if presentation.SuggestedQuestions == nil {
		presentation.SuggestedQuestions = []string{}
	}
	return presentation
}

func runtimeAgentIconURL(record runtimeAgentRecord) string {
	if strings.TrimSpace(record.IconType) != "image" || strings.TrimSpace(record.Icon) == "" {
		return ""
	}
	url, err := util.GetSignedFileURL(record.Icon)
	if err != nil {
		return ""
	}
	return url
}

func runtimeMemoryRequiresUser(presentation runtimePublishedPresentation) bool {
	if !presentation.AgentMemoryEnabled {
		return false
	}
	for _, slot := range presentation.AgentMemorySlots {
		if slot.Enabled && strings.TrimSpace(slot.Key) != "" {
			return true
		}
	}
	return false
}

func runtimeAgentCapabilities(presentation runtimePublishedPresentation) []string {
	capabilities := []string{"chat", "conversations"}
	if presentation.FileUploadEnabled {
		capabilities = append(capabilities, "files")
	}
	if presentation.AgentMemoryEnabled {
		capabilities = append(capabilities, "memory")
	}
	if presentation.SupportsVision {
		capabilities = append(capabilities, "vision")
	}
	if len(presentation.EnabledSkillIDs) > 0 {
		capabilities = append(capabilities, "skills")
	}
	return capabilities
}

// loadBoundAgents loads the applications of bindings, Agents and
// DigitalHumans, and reports which of them the Runtime Workspace may call.
func (s *Service) loadBoundAgents(ctx context.Context, runtimeWorkspace *RuntimeWorkspace, bindings []AgentBinding) (map[uuid.UUID]runtimeAgentRecord, map[uuid.UUID]struct{}, error) {
	agentsByID := make(map[uuid.UUID]runtimeAgentRecord, len(bindings))
	if runtimeWorkspace == nil || len(bindings) == 0 {
		return agentsByID, map[uuid.UUID]struct{}{}, nil
	}
	agentIDs := make([]uuid.UUID, 0, len(bindings))
	var digitalHumanIDs []uuid.UUID
	for _, binding := range bindings {
		switch bindingAppType(binding) {
		case AppTypeDigitalHuman:
			digitalHumanIDs = append(digitalHumanIDs, binding.AgentID)
		case AppTypeAgent:
			agentIDs = append(agentIDs, binding.AgentID)
		}
		// Bindings of other types load nothing and read as not found.
	}
	digitalHumans, authorizedDigitalHumans, err := s.loadBoundDigitalHumans(ctx, runtimeWorkspace, digitalHumanIDs)
	if err != nil {
		return nil, nil, err
	}
	if len(agentIDs) == 0 {
		return digitalHumans, authorizedDigitalHumans, nil
	}
	var records []runtimeAgentRecord
	if err := s.db.WithContext(ctx).Table("agents AS agents").
		Select("agents.id, agents.tenant_id, workspaces.organization_id, workspaces.name AS workspace_name, workspaces.status AS workspace_status, agents.name, agents.description, COALESCE(agents.icon, '') AS icon, COALESCE(agents.icon_type, '') AS icon_type, agents.enable_api, agents.web_app_status, latest.version_uuid IS NOT NULL AS published, COALESCE(latest.version, '') AS version, latest.version_uuid, latest.created_at AS published_at, latest.config_snapshot").
		Joins("JOIN workspaces AS workspaces ON workspaces.id = agents.tenant_id").
		Joins("LEFT JOIN LATERAL (SELECT versions.version, versions.version_uuid, versions.created_at, versions.config_snapshot FROM agent_published_versions AS versions WHERE versions.agent_id = agents.id AND versions.workspace_id = agents.tenant_id AND versions.deleted_at IS NULL ORDER BY versions.created_at DESC, versions.id DESC LIMIT 1) AS latest ON TRUE").
		Where("agents.id IN ? AND agents.deleted_at IS NULL", agentIDs).Find(&records).Error; err != nil {
		return nil, nil, fmt.Errorf("load runtime workspace agents: %w", err)
	}
	candidates := make([]runtimeauth.ResourceAuthorizationCandidate, 0, len(records))
	for _, record := range records {
		record.AppType = AppTypeAgent
		agentsByID[record.ID] = record
		if record.OrganizationID != runtimeWorkspace.OrganizationID || record.WorkspaceStatus != workspace_model.WorkspaceStatusNormal || !record.Published || !record.EnableAPI || strings.TrimSpace(record.WebAppStatus) != runtimeauth.WebAppStatusActive {
			continue
		}
		candidates = append(candidates, runtimeauth.ResourceAuthorizationCandidate{ResourceID: record.ID, Fallback: runtimeauth.PolicyFromAgentFields(record.WebAppStatus, record.EnableAPI)})
	}
	audience := runtimeauth.RuntimeAudience{OrganizationID: runtimeWorkspace.OrganizationID, WorkspaceIDs: []uuid.UUID{runtimeWorkspace.ResourceWorkspaceID()}}
	authorizedIDs, err := runtimeauth.NewStore(s.db).FilterAuthorizedResourceIDs(ctx, runtimeauth.PublishedRuntimeResourceAgent, runtimeauth.PublishedRuntimeSurfaceAPI, runtimeWorkspace.OrganizationID, candidates, audience)
	if err != nil {
		return nil, nil, fmt.Errorf("authorize runtime workspace agents: %w", err)
	}
	authorized := make(map[uuid.UUID]struct{}, len(authorizedIDs)+len(authorizedDigitalHumans))
	for _, agentID := range authorizedIDs {
		authorized[agentID] = struct{}{}
	}
	for id, record := range digitalHumans {
		agentsByID[id] = record
	}
	for id := range authorizedDigitalHumans {
		authorized[id] = struct{}{}
	}
	return agentsByID, authorized, nil
}

// runtimeAppCapabilities lists what /api/connect/v1 serves for an app.
func runtimeAppCapabilities(record runtimeAgentRecord, presentation runtimePublishedPresentation) []string {
	switch record.AppType {
	case AppTypeDigitalHuman:
		return append([]string(nil), digitalHumanCapabilities...)
	case AppTypeAgent, "":
		return runtimeAgentCapabilities(presentation)
	default:
		return []string{}
	}
}

// runtimeAppType reports a record's application type; records loaded
// before types were kept are Agents.
func runtimeAppType(record runtimeAgentRecord) string {
	if record.AppType == "" {
		return AppTypeAgent
	}
	return record.AppType
}

func runtimeAgentUnavailableReason(record runtimeAgentRecord, found, authorized bool) string {
	if found && record.AppType == AppTypeDigitalHuman {
		switch {
		case record.WorkspaceStatus != workspace_model.WorkspaceStatusNormal:
			return "source_workspace_archived"
		case !record.Published:
			return "digital_human_not_published"
		case !authorized:
			return "digital_human_api_disabled"
		default:
			return ""
		}
	}
	switch {
	case !found:
		return "agent_not_found"
	case record.WorkspaceStatus != workspace_model.WorkspaceStatusNormal:
		return "source_workspace_archived"
	case !record.Published:
		return "agent_not_published"
	case !record.EnableAPI || strings.TrimSpace(record.WebAppStatus) != runtimeauth.WebAppStatusActive:
		return "agent_disabled"
	case !authorized:
		return "agent_not_authorized"
	default:
		return ""
	}
}

func (s *Service) AuthenticateCredential(ctx context.Context, rawKey string) (*Principal, error) {
	key := strings.TrimSpace(rawKey)
	if key == "" {
		return nil, ErrUnauthorized
	}
	var credential Credential
	query := s.db.WithContext(ctx).Where("key_hash = ? AND status = ?", externalapikey.Hash(key), CredentialActive).
		Where("expires_at IS NULL OR expires_at > ?", s.now())
	if err := query.First(&credential).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUnauthorized
		}
		return nil, fmt.Errorf("authenticate runtime workspace: %w", err)
	}
	workspace, err := s.loadRuntimeWorkspace(ctx, credential.RuntimeWorkspaceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWorkspaceDisabled
		}
		return nil, fmt.Errorf("load runtime workspace: %w", err)
	}
	if workspace.Status != StatusActive || workspace.Mode != ModeAttached && workspace.Mode != ModeIsolated {
		return nil, ErrWorkspaceDisabled
	}
	if _, err := requireNormalResourceWorkspace(ctx, s.db, workspace.ResourceWorkspaceID(), false); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, ErrWorkspaceDisabled) {
			return nil, ErrWorkspaceDisabled
		}
		return nil, fmt.Errorf("load runtime resource workspace: %w", err)
	}
	return &Principal{RuntimeWorkspace: workspace, Credential: &credential}, nil
}

func (s *Service) ResolveRequestPrincipal(ctx context.Context, principal *Principal, externalUserID, appKey string) error {
	if principal == nil || principal.RuntimeWorkspace == nil || principal.Credential == nil {
		return ErrUnauthorized
	}
	principal.ExternalUserID = strings.TrimSpace(externalUserID)
	principal.SubjectID = uuid.Nil
	principal.Agent = nil
	if principal.ExternalUserID != "" {
		subject, err := usersubject.RuntimeWorkspaceEndUserSubject(principal.RuntimeWorkspace.LogicalID(), principal.ExternalUserID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrSubjectRequired, err)
		}
		logicalID := principal.RuntimeWorkspace.LogicalID()
		if _, err := usersubject.NewService(s.db).Ensure(ctx, usersubject.EnsureRequest{
			Subject: subject, IssuerType: SubjectIssuerType, IssuerID: &logicalID,
		}); err != nil {
			return fmt.Errorf("ensure runtime workspace subject: %w", err)
		}
		if err := s.touchEndUser(ctx, logicalID, subject.ID, principal.ExternalUserID); err != nil {
			return fmt.Errorf("record runtime workspace end user: %w", err)
		}
		principal.SubjectID = subject.ID
	}
	appKey = strings.TrimSpace(appKey)
	if appKey != "" && principal.SubjectID == uuid.Nil {
		return ErrSubjectRequired
	}
	if appKey != "" {
		binding, err := s.ResolveApp(ctx, principal.RuntimeWorkspace.LogicalID(), appKey)
		if err != nil {
			return err
		}
		principal.Agent = binding
	}
	_ = s.db.WithContext(ctx).Model(&Credential{}).Where("id = ?", principal.Credential.ID).Updates(map[string]any{
		"last_used_at": s.now(), "usage_count": gorm.Expr("usage_count + 1"),
	})
	return nil
}

func (s *Service) Authenticate(ctx context.Context, rawKey, externalUserID, appKey string) (*Principal, error) {
	principal, err := s.AuthenticateCredential(ctx, rawKey)
	if err != nil {
		return nil, err
	}
	if err := s.ResolveRequestPrincipal(ctx, principal, externalUserID, appKey); err != nil {
		return nil, err
	}
	return principal, nil
}

// CanReadRuntimeExternalFile authorizes a permanent uploaded file for the
// authenticated Runtime Subject. Runtime Subjects are not Workspace members,
// so this binding is the source of truth for chat attachments as well as the
// Runtime file management endpoints.
func (s *Service) CanReadRuntimeExternalFile(ctx context.Context, runtimeWorkspaceID, subjectID uuid.UUID, fileID string) (bool, error) {
	fileID = strings.TrimSpace(fileID)
	if s == nil || s.db == nil || runtimeWorkspaceID == uuid.Nil || subjectID == uuid.Nil || fileID == "" {
		return false, nil
	}
	if _, err := uuid.Parse(fileID); err != nil {
		return false, nil
	}
	var count int64
	err := s.db.WithContext(ctx).Table("runtime_external_files").
		Where("file_id = ? AND runtime_workspace_id = ? AND subject_id = ?", fileID, runtimeWorkspaceID, subjectID).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check runtime external file access: %w", err)
	}
	return count == 1, nil
}

func (s *Service) ResolveApp(ctx context.Context, runtimeWorkspaceID uuid.UUID, appKey string) (*AgentBinding, error) {
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, runtimeWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("load runtime workspace authorization scope: %w", err)
	}
	return s.resolveApp(ctx, runtimeWorkspace, appKey)
}

func (s *Service) GetApp(ctx context.Context, runtimeWorkspaceID uuid.UUID, appKey string) (*App, error) {
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, runtimeWorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("load runtime workspace app scope: %w", err)
	}
	binding, err := s.resolveApp(ctx, runtimeWorkspace, appKey)
	if err != nil {
		return nil, err
	}
	agentsByID, _, err := s.loadBoundAgents(ctx, runtimeWorkspace, []AgentBinding{*binding})
	if err != nil {
		return nil, err
	}
	agent, found := agentsByID[binding.AgentID]
	if !found {
		return nil, ErrAppNotFound
	}
	presentation := runtimeAgentPresentation(agent)
	return &App{
		AppKey: binding.PublicAppKey, AppType: runtimeAppType(agent), Name: agent.Name, Description: agent.Description,
		Icon: agent.Icon, IconType: agent.IconType, IconURL: runtimeAgentIconURL(agent), Version: agent.Version, VersionUUID: agent.VersionUUID.String(),
		HomeTitle: presentation.HomeTitle, OpeningStatement: presentation.OpeningStatement, InputPlaceholder: presentation.InputPlaceholder, ThemeColor: presentation.ThemeColor, SuggestedQuestions: presentation.SuggestedQuestions,
		Status: binding.Status, SupportsStream: true, FileUploadEnabled: presentation.FileUploadEnabled, SupportsVision: presentation.SupportsVision,
		AgentMemoryEnabled: presentation.AgentMemoryEnabled, MemoryRequiresUser: runtimeMemoryRequiresUser(presentation), ExternalUserRequired: true, Capabilities: runtimeAppCapabilities(agent, presentation),
	}, nil
}

func (s *Service) resolveApp(ctx context.Context, runtimeWorkspace *RuntimeWorkspace, appKey string) (*AgentBinding, error) {
	if runtimeWorkspace == nil || runtimeWorkspace.LogicalID() == uuid.Nil {
		return nil, ErrAppNotFound
	}
	var binding AgentBinding
	if err := s.db.WithContext(ctx).Where("runtime_workspace_id = ? AND public_app_key = ? AND status = ?", runtimeWorkspace.LogicalID(), strings.TrimSpace(appKey), AgentBindingActive).First(&binding).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAppNotFound
		}
		return nil, fmt.Errorf("resolve runtime workspace app: %w", err)
	}
	agentsByID, authorized, err := s.loadBoundAgents(ctx, runtimeWorkspace, []AgentBinding{binding})
	if err != nil {
		return nil, err
	}
	_, found := agentsByID[binding.AgentID]
	_, allowed := authorized[binding.AgentID]
	if !found {
		return nil, ErrAppNotFound
	}
	if !allowed {
		return nil, ErrAppDisabled
	}
	return &binding, nil
}

func (s *Service) ListApps(ctx context.Context, runtimeWorkspaceID uuid.UUID) ([]App, error) {
	runtimeWorkspace, bindings, agentsByID, authorized, err := s.listBindingState(ctx, runtimeWorkspaceID, true)
	if err != nil {
		return nil, err
	}
	agentIDs := make([]uuid.UUID, 0, len(bindings))
	for _, binding := range bindings {
		agentIDs = append(agentIDs, binding.AgentID)
	}
	tagsByAgent, err := s.catalogTagsByAgentIDs(ctx, agentIDs)
	if err != nil {
		return nil, err
	}
	apps := make([]App, 0, len(bindings))
	for _, binding := range bindings {
		agent, found := agentsByID[binding.AgentID]
		_, allowed := authorized[binding.AgentID]
		if !found || !allowed || agent.OrganizationID != runtimeWorkspace.OrganizationID {
			continue
		}
		presentation := runtimeAgentPresentation(agent)
		apps = append(apps, App{
			AppKey: binding.PublicAppKey, AppType: runtimeAppType(agent), Name: agent.Name, Description: agent.Description,
			Icon: agent.Icon, IconType: agent.IconType, IconURL: runtimeAgentIconURL(agent), Version: agent.Version, VersionUUID: agent.VersionUUID.String(),
			HomeTitle: presentation.HomeTitle, OpeningStatement: presentation.OpeningStatement, InputPlaceholder: presentation.InputPlaceholder, ThemeColor: presentation.ThemeColor, SuggestedQuestions: presentation.SuggestedQuestions,
			Status: binding.Status, SupportsStream: true, FileUploadEnabled: presentation.FileUploadEnabled, SupportsVision: presentation.SupportsVision,
			AgentMemoryEnabled: presentation.AgentMemoryEnabled, MemoryRequiresUser: runtimeMemoryRequiresUser(presentation), ExternalUserRequired: true, Capabilities: runtimeAppCapabilities(agent, presentation),
			Tags: tagsByAgent[binding.AgentID],
		})
	}
	return apps, nil
}

func (s *Service) ListBindings(ctx context.Context, runtimeWorkspaceID uuid.UUID) (*ManagementAppSet, error) {
	runtimeWorkspace, bindings, agentsByID, authorized, err := s.listBindingState(ctx, runtimeWorkspaceID, true)
	if err != nil {
		return nil, err
	}
	agentIDs := make([]uuid.UUID, 0, len(bindings))
	for _, binding := range bindings {
		agentIDs = append(agentIDs, binding.AgentID)
	}
	tagsByAgent, err := s.catalogTagsByAgentIDs(ctx, agentIDs)
	if err != nil {
		return nil, err
	}
	apps := make([]ManagementApp, 0, len(bindings))
	for _, binding := range bindings {
		agent, found := agentsByID[binding.AgentID]
		_, allowed := authorized[binding.AgentID]
		if found && agent.OrganizationID != runtimeWorkspace.OrganizationID {
			allowed = false
		}
		reason := runtimeAgentUnavailableReason(agent, found, allowed)
		if binding.Status != AgentBindingActive {
			reason = "binding_disabled"
		}
		presentation := runtimeAgentPresentation(agent)
		if !found {
			agent.AppType = bindingAppType(binding)
		}
		apps = append(apps, ManagementApp{
			AgentID: binding.AgentID.String(), AppType: bindingAppType(binding), AppKey: binding.PublicAppKey, WorkspaceID: agent.TenantID.String(), WorkspaceName: agent.WorkspaceName,
			Name: agent.Name, Description: agent.Description, Icon: agent.Icon, IconType: agent.IconType, IconURL: runtimeAgentIconURL(agent),
			Version: agent.Version, VersionUUID: agent.VersionUUID.String(), PublishedAt: agent.PublishedAt,
			Status: binding.Status, SupportsStream: true, FileUploadEnabled: presentation.FileUploadEnabled, SupportsVision: presentation.SupportsVision,
			AgentMemoryEnabled: presentation.AgentMemoryEnabled, Capabilities: runtimeAppCapabilities(agent, presentation),
			Tags:     tagsByAgent[binding.AgentID],
			Callable: binding.Status == AgentBindingActive && allowed, UnavailableReason: reason,
		})
	}
	return &ManagementAppSet{Revision: runtimeWorkspace.BindingsRevision, Limit: MaxRuntimeWorkspaceApps(), Items: apps}, nil
}

// ListAgentCandidates lists every app of the scope's workspaces that the
// Runtime Workspace may bind.
func (s *Service) ListAgentCandidates(ctx context.Context, runtimeWorkspaceID, accountID uuid.UUID, scope string, sourceWorkspaceID *uuid.UUID) (*AgentCandidateSet, error) {
	return s.ListAppCandidates(ctx, runtimeWorkspaceID, accountID, scope, sourceWorkspaceID, CandidateQuery{})
}

// candidateWorkspaces are the workspaces a candidate scope covers: the
// Runtime Workspace's own, or the source workspaces the account may
// reference.
func candidateWorkspaces(runtimeWorkspace *RuntimeWorkspace, access *sourceWorkspaceAccess, scope string, sourceWorkspaceID *uuid.UUID) ([]uuid.UUID, string, error) {
	resourceWorkspaceID := runtimeWorkspace.ResourceWorkspaceID()
	if scope == "" {
		scope = "current"
	}
	workspaceIDs := []uuid.UUID{resourceWorkspaceID}
	if scope == "authorized" {
		if !access.CrossEnabled {
			return nil, scope, ErrSourceWorkspacePermission
		}
		workspaceIDs = workspaceIDs[:0]
		if sourceWorkspaceID != nil {
			if *sourceWorkspaceID == resourceWorkspaceID || !access.canReference(resourceWorkspaceID, *sourceWorkspaceID) {
				return nil, scope, ErrSourceWorkspacePermission
			}
			workspaceIDs = append(workspaceIDs, *sourceWorkspaceID)
		} else {
			for workspaceID := range access.AllowedIDs {
				if workspaceID != resourceWorkspaceID {
					workspaceIDs = append(workspaceIDs, workspaceID)
				}
			}
		}
	} else if scope != "current" {
		return nil, scope, ErrInvalidBinding
	}
	return workspaceIDs, scope, nil
}

func (s *Service) listBindingState(ctx context.Context, runtimeWorkspaceID uuid.UUID, activeOnly bool) (*RuntimeWorkspace, []AgentBinding, map[uuid.UUID]runtimeAgentRecord, map[uuid.UUID]struct{}, error) {
	runtimeWorkspace, err := s.loadRuntimeWorkspace(ctx, runtimeWorkspaceID)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("load runtime workspace apps scope: %w", err)
	}
	runtimeID := runtimeWorkspace.LogicalID()
	query := s.db.WithContext(ctx).Where("runtime_workspace_id = ?", runtimeID)
	if activeOnly {
		query = query.Where("status = ?", AgentBindingActive)
	}
	var bindings []AgentBinding
	if err := query.Order("created_at ASC").Find(&bindings).Error; err != nil {
		return nil, nil, nil, nil, fmt.Errorf("list runtime workspace apps: %w", err)
	}
	agentsByID, authorized, err := s.loadBoundAgents(ctx, runtimeWorkspace, bindings)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	return runtimeWorkspace, bindings, agentsByID, authorized, nil
}

func (s *Service) CreateWorkspace(ctx context.Context, workspaceID, accountID uuid.UUID) (*RuntimeWorkspace, error) {
	if workspaceID == uuid.Nil || accountID == uuid.Nil {
		return nil, ErrUnauthorized
	}
	var entry RuntimeWorkspace
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		workspace, err := requireNormalResourceWorkspace(ctx, tx, workspaceID, true)
		if err != nil {
			return err
		}
		// Attached Runtime Workspaces intentionally start with a stable logical ID
		// equal to the current resource Workspace ID. This preserves all existing
		// child rows while making the identity explicit for future isolated mode.
		candidate := RuntimeWorkspace{RuntimeID: workspaceID, WorkspaceID: workspaceID, OrganizationID: workspace.OrganizationID, ManagedByAccountID: accountID, Status: StatusActive, Mode: ModeAttached, PolicyJSON: "{}"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&candidate).Error; err != nil {
			return fmt.Errorf("create runtime workspace: %w", err)
		}
		return tx.First(&entry, "workspace_id = ?", workspaceID).Error
	})
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

func (s *Service) UpdateWorkspace(ctx context.Context, workspaceID uuid.UUID, status *string, policy json.RawMessage) (*RuntimeWorkspace, error) {
	if workspaceID == uuid.Nil {
		return nil, ErrInvalidWorkspace
	}
	updates := map[string]any{"updated_at": time.Now()}
	if status != nil {
		normalized := strings.TrimSpace(*status)
		if normalized != StatusActive && normalized != StatusDisabled {
			return nil, ErrInvalidWorkspace
		}
		updates["status"] = normalized
	}
	if policy != nil {
		var policyObject map[string]json.RawMessage
		if !json.Valid(policy) || json.Unmarshal(policy, &policyObject) != nil || policyObject == nil || ValidateEndUserQuota(policy) != nil {
			return nil, ErrInvalidWorkspace
		}
		updates["policy_json"] = string(policy)
	}
	if len(updates) == 1 {
		return nil, ErrInvalidWorkspace
	}
	var updated RuntimeWorkspace
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txService := NewService(tx, WithClock(s.now))
		runtimeWorkspace, err := txService.loadRuntimeWorkspace(ctx, workspaceID)
		if err != nil {
			return err
		}
		if status != nil && strings.TrimSpace(*status) == StatusActive {
			if _, err := requireNormalResourceWorkspace(ctx, tx, runtimeWorkspace.ResourceWorkspaceID(), true); err != nil {
				return err
			}
		}
		result := tx.Model(&RuntimeWorkspace{}).Where("runtime_id = ?", runtimeWorkspace.LogicalID()).Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("update runtime workspace: %w", result.Error)
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return tx.First(&updated, "runtime_id = ?", runtimeWorkspace.LogicalID()).Error
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

type CreatedCredential struct {
	Credential *Credential `json:"credential"`
	Secret     string      `json:"secret"`
	Replayed   bool        `json:"-"`
}

func (s *Service) CreateCredential(ctx context.Context, runtimeWorkspaceID, accountID uuid.UUID, name string, expiresAt *time.Time, idempotencyKey string) (*CreatedCredential, error) {
	return s.CreateScopedCredential(ctx, runtimeWorkspaceID, accountID, CredentialInput{Name: name, ExpiresAt: expiresAt}, idempotencyKey)
}

// CreateScopedCredential creates an access key, optionally limited to scopes
// and apps (ADR 0012).
func (s *Service) CreateScopedCredential(ctx context.Context, runtimeWorkspaceID, accountID uuid.UUID, input CredentialInput, idempotencyKey string) (*CreatedCredential, error) {
	name, expiresAt := strings.TrimSpace(input.Name), input.ExpiresAt
	now := s.now().UTC()
	if runtimeWorkspaceID == uuid.Nil || accountID == uuid.Nil || utf8.RuneCountInString(name) > 255 || expiresAt != nil && !expiresAt.After(now) {
		return nil, ErrInvalidCredential
	}
	scopes, appKeys, err := normalizeCredentialLimits(input.Scopes, input.AppKeys)
	if err != nil {
		return nil, err
	}
	if s.replayProtector == nil {
		return nil, ErrCredentialReplayUnavailable
	}
	resolvedRuntimeID, err := s.ResolveRuntimeID(ctx, runtimeWorkspaceID)
	if err != nil {
		return nil, err
	}
	runtimeWorkspaceID = resolvedRuntimeID
	idempotencyHash, err := s.replayProtector.IdempotencyKeyHash(runtimeWorkspaceID, accountID, idempotencyKey)
	if err != nil {
		return nil, err
	}
	fingerprint := s.replayProtector.RequestFingerprint(runtimeWorkspaceID, accountID, name, expiresAt, credentialLimitParts(scopes, appKeys)...)
	_ = s.CleanupExpiredCredentialOperations(ctx)

	var created *CreatedCredential
	var terminalErr error
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var workspace RuntimeWorkspace
		if err := tx.Where("runtime_id = ? OR workspace_id = ?", runtimeWorkspaceID, runtimeWorkspaceID).First(&workspace).Error; err != nil {
			return err
		}
		operation := CredentialOperation{
			ID: uuid.New(), RuntimeWorkspaceID: runtimeWorkspaceID, CreatedByAccountID: accountID,
			IdempotencyKeyHash: idempotencyHash, RequestFingerprint: fingerprint, State: CredentialOperationActive,
			ExpiresAt: now.Add(credentialOperationRecordTTL), CreatedAt: now, UpdatedAt: now,
		}
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "runtime_workspace_id"}, {Name: "created_by_account_id"}, {Name: "idempotency_key_hash"}},
			DoNothing: true,
		}).Create(&operation)
		if result.Error != nil {
			return fmt.Errorf("claim runtime workspace credential operation: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			var existing CredentialOperation
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("runtime_workspace_id = ? AND created_by_account_id = ? AND idempotency_key_hash = ?", runtimeWorkspaceID, accountID, idempotencyHash).
				Take(&existing).Error; err != nil {
				return fmt.Errorf("load runtime workspace credential operation: %w", err)
			}
			operation = existing
			if operation.RequestFingerprint != fingerprint {
				terminalErr = ErrIdempotencyConflict
				return nil
			}
			if operation.State != CredentialOperationDone || operation.CredentialID == nil {
				terminalErr = ErrOperationInProgress
				return nil
			}
			var credential Credential
			if err := tx.First(&credential, "id = ? AND runtime_workspace_id = ?", *operation.CredentialID, runtimeWorkspaceID).Error; err != nil {
				return fmt.Errorf("load replayed runtime workspace credential: %w", err)
			}
			if credential.Status != CredentialActive || credential.ExpiresAt != nil && !credential.ExpiresAt.After(now) || operation.SecretEnvelope == nil || operation.SecretExpiresAt == nil || !operation.SecretExpiresAt.After(now) {
				if err := tx.Model(&CredentialOperation{}).Where("id = ?", operation.ID).Updates(map[string]any{"secret_envelope": nil, "secret_expires_at": nil, "updated_at": now}).Error; err != nil {
					return err
				}
				terminalErr = ErrSecretReplayExpired
				return nil
			}
			secret, err := s.replayProtector.Open(*operation.SecretEnvelope, credentialReplayAAD(workspace.OrganizationID, operation, credential.ID))
			if err != nil {
				return fmt.Errorf("open runtime workspace credential replay: %w", err)
			}
			created = &CreatedCredential{Credential: &credential, Secret: secret, Replayed: true}
			return nil
		}

		var activeCredentials int64
		if err := tx.Model(&Credential{}).
			Where("runtime_workspace_id = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)", runtimeWorkspaceID, CredentialActive, now).
			Count(&activeCredentials).Error; err != nil {
			return fmt.Errorf("count runtime workspace credentials: %w", err)
		}
		if activeCredentials >= MaxActiveCredentials {
			// The claimed operation goes with the rolled-back transaction.
			return ErrCredentialLimitExceeded
		}
		if err := requireBoundAppKeys(tx, runtimeWorkspaceID, appKeys); err != nil {
			return err
		}

		secret, prefix, hash, err := externalapikey.Generate()
		if err != nil {
			return fmt.Errorf("generate runtime workspace credential: %w", err)
		}
		credential := &Credential{ID: uuid.New(), RuntimeWorkspaceID: runtimeWorkspaceID, KeyPrefix: prefix, KeyHash: hash, Name: name, Status: CredentialActive, ExpiresAt: expiresAt, CreatedByAccountID: accountID, Scopes: scopes, AppKeys: appKeys, CreatedAt: now, UpdatedAt: now}
		envelope, err := s.replayProtector.Seal(secret, credentialReplayAAD(workspace.OrganizationID, operation, credential.ID))
		if err != nil {
			return fmt.Errorf("seal runtime workspace credential replay: %w", err)
		}
		if err := tx.Create(credential).Error; err != nil {
			return fmt.Errorf("create runtime workspace credential: %w", err)
		}
		secretExpiresAt := now.Add(credentialSecretReplayTTL)
		if err := tx.Model(&CredentialOperation{}).Where("id = ?", operation.ID).Updates(map[string]any{
			"state": CredentialOperationDone, "credential_id": credential.ID, "secret_envelope": envelope,
			"secret_expires_at": secretExpiresAt, "updated_at": now,
		}).Error; err != nil {
			return fmt.Errorf("complete runtime workspace credential operation: %w", err)
		}
		created = &CreatedCredential{Credential: credential, Secret: secret}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if terminalErr != nil {
		return nil, terminalErr
	}
	return created, nil
}

func (s *Service) CleanupExpiredCredentialOperations(ctx context.Context) error {
	now := s.now().UTC()
	if err := s.db.WithContext(ctx).Model(&CredentialOperation{}).
		Where("secret_expires_at IS NOT NULL AND secret_expires_at <= ?", now).
		Updates(map[string]any{"secret_envelope": nil, "secret_expires_at": nil, "updated_at": now}).Error; err != nil {
		return fmt.Errorf("expire runtime workspace credential replay secrets: %w", err)
	}
	if err := s.db.WithContext(ctx).Where("expires_at <= ?", now).Delete(&CredentialOperation{}).Error; err != nil {
		return fmt.Errorf("delete expired runtime workspace credential operations: %w", err)
	}
	return nil
}

func credentialReplayAAD(organizationID uuid.UUID, operation CredentialOperation, credentialID uuid.UUID) CredentialReplayAAD {
	return CredentialReplayAAD{
		OrganizationID: organizationID, RuntimeWorkspaceID: operation.RuntimeWorkspaceID, OperationID: operation.ID,
		CredentialID: credentialID, RequestFingerprint: operation.RequestFingerprint,
	}
}

func (s *Service) RevokeCredential(ctx context.Context, runtimeWorkspaceID, credentialID uuid.UUID) error {
	now := s.now().UTC()
	resolvedRuntimeID, err := s.ResolveRuntimeID(ctx, runtimeWorkspaceID)
	if err != nil {
		return err
	}
	runtimeWorkspaceID = resolvedRuntimeID
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Credential{}).Where("id = ? AND runtime_workspace_id = ?", credentialID, runtimeWorkspaceID).Updates(map[string]any{"status": CredentialRevoked, "revoked_at": now, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return tx.Model(&CredentialOperation{}).Where("credential_id = ?", credentialID).Updates(map[string]any{"secret_envelope": nil, "secret_expires_at": nil, "updated_at": now}).Error
	})
}

func (s *Service) SyncAgentBindings(ctx context.Context, runtimeWorkspaceID, actingAccountID uuid.UUID, expectedRevision int64, requestedAgentIDs []uuid.UUID) (*ManagementAppSet, error) {
	refs := make([]AppRef, 0, len(requestedAgentIDs))
	for _, agentID := range requestedAgentIDs {
		refs = append(refs, AppRef{Type: AppTypeAgent, ID: agentID})
	}
	return s.SyncAppBindings(ctx, runtimeWorkspaceID, actingAccountID, expectedRevision, refs)
}

// SyncAppBindings makes the Runtime Workspace bind exactly the requested
// Agents and DigitalHumans.
func (s *Service) SyncAppBindings(ctx context.Context, runtimeWorkspaceID, actingAccountID uuid.UUID, expectedRevision int64, requested []AppRef) (*ManagementAppSet, error) {
	if runtimeWorkspaceID == uuid.Nil || actingAccountID == uuid.Nil || expectedRevision < 0 {
		return nil, ErrInvalidBinding
	}
	desired := make(map[uuid.UUID]struct{}, len(requested))
	desiredType := make(map[uuid.UUID]string, len(requested))
	for _, ref := range requested {
		if ref.ID == uuid.Nil || (ref.Type != AppTypeAgent && ref.Type != AppTypeDigitalHuman) {
			return nil, ErrInvalidBinding
		}
		if previous, seen := desiredType[ref.ID]; seen && previous != ref.Type {
			return nil, ErrInvalidBinding
		}
		desired[ref.ID] = struct{}{}
		desiredType[ref.ID] = ref.Type
	}
	if len(desired) > MaxRuntimeWorkspaceApps() {
		return nil, ErrAppLimitExceeded
	}
	resolvedRuntimeID, err := s.ResolveRuntimeID(ctx, runtimeWorkspaceID)
	if err != nil {
		return nil, err
	}
	runtimeWorkspaceID = resolvedRuntimeID
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var runtimeWorkspace RuntimeWorkspace
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("runtime_id = ? OR workspace_id = ?", runtimeWorkspaceID, runtimeWorkspaceID).
			First(&runtimeWorkspace).Error; err != nil {
			return err
		}
		if runtimeWorkspace.BindingsRevision != expectedRevision {
			return ErrBindingRevisionConflict
		}
		var existing []AgentBinding
		if err := tx.Where("runtime_workspace_id = ?", runtimeWorkspaceID).Find(&existing).Error; err != nil {
			return fmt.Errorf("load runtime workspace agent bindings for sync: %w", err)
		}
		existingByAgent := make(map[uuid.UUID]AgentBinding, len(existing))
		active := make(map[uuid.UUID]struct{}, len(existing))
		for _, binding := range existing {
			if want, requested := desiredType[binding.AgentID]; requested && want != bindingAppType(binding) {
				return ErrInvalidBinding
			}
			existingByAgent[binding.AgentID] = binding
			if binding.Status == AgentBindingActive {
				active[binding.AgentID] = struct{}{}
			}
		}
		desiredBindings := make([]AgentBinding, 0, len(desired))
		for agentID := range desired {
			desiredBindings = append(desiredBindings, AgentBinding{RuntimeWorkspaceID: runtimeWorkspaceID, AgentID: agentID, AppType: desiredType[agentID]})
		}
		txService := NewService(tx, WithClock(s.now))
		agentsByID, authorized, err := txService.loadBoundAgents(ctx, &runtimeWorkspace, desiredBindings)
		if err != nil {
			return err
		}
		resourceWorkspaceID := runtimeWorkspace.ResourceWorkspaceID()
		var sourceAccess *sourceWorkspaceAccess
		for agentID := range desired {
			agent, found := agentsByID[agentID]
			if !found {
				if _, bound := active[agentID]; bound {
					delete(desired, agentID)
					continue
				}
				return ErrAppNotFound
			}
			if _, allowed := authorized[agentID]; !allowed {
				return ErrAppDisabled
			}
			existingBinding, existed := existingByAgent[agentID]
			requiresSourceAuthorization := !existed || existingBinding.Status != AgentBindingActive
			if requiresSourceAuthorization && agent.TenantID != resourceWorkspaceID {
				if sourceAccess == nil {
					sourceAccess, err = txService.sourceWorkspaceAccess(ctx, &runtimeWorkspace, actingAccountID)
					if err != nil {
						return err
					}
				}
				if !sourceAccess.canReference(resourceWorkspaceID, agent.TenantID) {
					return ErrSourceWorkspacePermission
				}
			}
		}
		// Checked once apps deleted since they were bound are dropped.
		unchanged := len(active) == len(desired)
		if unchanged {
			for agentID := range desired {
				if _, found := active[agentID]; !found {
					unchanged = false
					break
				}
			}
		}
		if unchanged {
			return nil
		}
		now := s.now().UTC()
		for agentID := range desired {
			if binding, found := existingByAgent[agentID]; found {
				if binding.Status != AgentBindingActive {
					if err := tx.Model(&AgentBinding{}).
						Where("runtime_workspace_id = ? AND agent_id = ?", runtimeWorkspaceID, agentID).
						Updates(map[string]any{"status": AgentBindingActive, "updated_at": now}).Error; err != nil {
						return fmt.Errorf("restore runtime workspace agent binding: %w", err)
					}
				}
				continue
			}
			binding := AgentBinding{RuntimeWorkspaceID: runtimeWorkspaceID, AgentID: agentID, AppType: desiredType[agentID], PublicAppKey: "app_" + shortID(), Status: AgentBindingActive, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&binding).Error; err != nil {
				var pgErr *pgconn.PgError
				if errors.Is(err, gorm.ErrDuplicatedKey) || errors.As(err, &pgErr) && pgErr.Code == "23505" {
					return ErrAppKeyConflict
				}
				return fmt.Errorf("create runtime workspace agent binding: %w", err)
			}
		}
		for agentID := range active {
			if _, keep := desired[agentID]; keep {
				continue
			}
			if err := tx.Model(&AgentBinding{}).
				Where("runtime_workspace_id = ? AND agent_id = ?", runtimeWorkspaceID, agentID).
				Updates(map[string]any{"status": AgentBindingDisabled, "updated_at": now}).Error; err != nil {
				return fmt.Errorf("disable runtime workspace agent binding: %w", err)
			}
		}
		if err := tx.Model(&RuntimeWorkspace{}).Where("runtime_id = ?", runtimeWorkspaceID).
			Updates(map[string]any{"bindings_revision": gorm.Expr("bindings_revision + 1"), "updated_at": now}).Error; err != nil {
			return fmt.Errorf("advance runtime workspace agent binding revision: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.ListBindings(ctx, runtimeWorkspaceID)
}

func (s *Service) BindAgent(ctx context.Context, runtimeWorkspaceID, agentID uuid.UUID) (*AgentBinding, error) {
	if runtimeWorkspaceID == uuid.Nil || agentID == uuid.Nil {
		return nil, ErrInvalidBinding
	}
	runtimeID, err := s.ResolveRuntimeID(ctx, runtimeWorkspaceID)
	if err != nil {
		return nil, err
	}
	runtimeWorkspaceID = runtimeID
	var runtimeWorkspace RuntimeWorkspace
	if err := s.db.WithContext(ctx).Where("runtime_id = ?", runtimeWorkspaceID).First(&runtimeWorkspace).Error; err != nil {
		return nil, err
	}
	candidate := AgentBinding{RuntimeWorkspaceID: runtimeWorkspaceID, AgentID: agentID}
	agentsByID, authorized, err := s.loadBoundAgents(ctx, &runtimeWorkspace, []AgentBinding{candidate})
	if err != nil {
		return nil, err
	}
	if _, found := agentsByID[agentID]; !found {
		return nil, ErrAppNotFound
	}
	if _, allowed := authorized[agentID]; !allowed {
		return nil, ErrAppDisabled
	}
	var binding AgentBinding
	if err := s.db.WithContext(ctx).First(&binding, "runtime_workspace_id = ? AND agent_id = ?", runtimeWorkspaceID, agentID).Error; err == nil {
		if err := s.db.WithContext(ctx).Model(&binding).Updates(map[string]any{"status": AgentBindingActive, "updated_at": time.Now()}).Error; err != nil {
			return nil, fmt.Errorf("restore runtime workspace agent binding: %w", err)
		}
		return &binding, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load runtime workspace agent binding: %w", err)
	}
	binding = AgentBinding{RuntimeWorkspaceID: runtimeWorkspaceID, AgentID: agentID, PublicAppKey: "app_" + shortID(), Status: AgentBindingActive}
	if err := s.db.WithContext(ctx).Create(&binding).Error; err != nil {
		var pgErr *pgconn.PgError
		if errors.Is(err, gorm.ErrDuplicatedKey) || errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, ErrAppKeyConflict
		}
		return nil, fmt.Errorf("bind runtime workspace agent: %w", err)
	}
	return &binding, nil
}

func (s *Service) UnbindAgent(ctx context.Context, runtimeWorkspaceID, agentID uuid.UUID) error {
	runtimeID, err := s.ResolveRuntimeID(ctx, runtimeWorkspaceID)
	if err != nil {
		return err
	}
	result := s.db.WithContext(ctx).Model(&AgentBinding{}).Where("runtime_workspace_id = ? AND agent_id = ?", runtimeID, agentID).Updates(map[string]any{"status": AgentBindingDisabled, "updated_at": time.Now()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func shortID() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	}
	return fmt.Sprintf("%x", buf)
}
