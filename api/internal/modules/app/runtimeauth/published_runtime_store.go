package runtimeauth

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PublishedRuntimeResourceType string

const (
	PublishedRuntimeResourceAgent           PublishedRuntimeResourceType = "agent"
	PublishedRuntimeResourceBuiltinWorkflow PublishedRuntimeResourceType = "builtin_workflow"
)

type PublishedRuntimeSubjectType string

const (
	PublishedRuntimeSubjectPublic       PublishedRuntimeSubjectType = "public"
	PublishedRuntimeSubjectOrganization PublishedRuntimeSubjectType = "organization"
	PublishedRuntimeSubjectDepartment   PublishedRuntimeSubjectType = "department"
	PublishedRuntimeSubjectWorkspace    PublishedRuntimeSubjectType = "workspace"
	PublishedRuntimeSubjectAccount      PublishedRuntimeSubjectType = "account"
	PublishedRuntimeSubjectInternal     PublishedRuntimeSubjectType = "internal"
)

const (
	PublishedRuntimeSourceLegacyAgentFields = "legacy_agent_fields"
	PublishedRuntimeSourceGrant             = "grant"
	PublishedRuntimeSourceSystemDefault     = "system_default"
)

type SurfaceAuthorization struct {
	Surface             PublishedRuntimeSurface
	Enabled             bool
	CompatibilitySource string
	Grants              []SurfaceGrant
}

type SurfaceGrant struct {
	SubjectType PublishedRuntimeSubjectType
	SubjectID   *uuid.UUID
	Enabled     bool
}

type ResourceAuthorization struct {
	ResourceType   PublishedRuntimeResourceType
	ResourceID     uuid.UUID
	OrganizationID uuid.UUID
	WorkspaceID    *uuid.UUID
	Surfaces       []SurfaceAuthorization
}

type ResourceAuthorizationCandidate struct {
	ResourceID uuid.UUID
	Fallback   PublishedRuntimePolicy
}

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

type publishedRuntimeSurfaceRecord struct {
	ID                  uuid.UUID  `gorm:"column:id"`
	ResourceType        string     `gorm:"column:resource_type"`
	ResourceID          uuid.UUID  `gorm:"column:resource_id"`
	OrganizationID      uuid.UUID  `gorm:"column:organization_id"`
	WorkspaceID         *uuid.UUID `gorm:"column:workspace_id"`
	Surface             string     `gorm:"column:surface"`
	Enabled             bool       `gorm:"column:enabled"`
	CompatibilitySource string     `gorm:"column:compatibility_source"`
	CreatedAt           time.Time  `gorm:"column:created_at"`
	UpdatedAt           time.Time  `gorm:"column:updated_at"`
	DeletedAt           *time.Time `gorm:"column:deleted_at"`
}

func (publishedRuntimeSurfaceRecord) TableName() string {
	return "published_runtime_surfaces"
}

type publishedRuntimeGrantRecord struct {
	ID          uuid.UUID  `gorm:"column:id"`
	SurfaceID   uuid.UUID  `gorm:"column:surface_id"`
	SubjectType string     `gorm:"column:subject_type"`
	SubjectID   *uuid.UUID `gorm:"column:subject_id"`
	Enabled     bool       `gorm:"column:enabled"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
	DeletedAt   *time.Time `gorm:"column:deleted_at"`
}

func (publishedRuntimeGrantRecord) TableName() string {
	return "published_runtime_surface_grants"
}

func (s *Store) GetResourceAuthorization(ctx context.Context, resourceType PublishedRuntimeResourceType, resourceID uuid.UUID, fallback PublishedRuntimePolicy) (*ResourceAuthorization, error) {
	if s == nil || s.db == nil {
		return resourceAuthorizationFromFallback(resourceType, resourceID, fallback), nil
	}
	if resourceType == "" || resourceID == uuid.Nil {
		return nil, fmt.Errorf("resource type and resource id are required")
	}

	var surfaceRows []publishedRuntimeSurfaceRecord
	if err := s.db.WithContext(ctx).
		Where("resource_type = ? AND resource_id = ? AND deleted_at IS NULL", string(resourceType), resourceID).
		Order("surface ASC").
		Find(&surfaceRows).Error; err != nil {
		return nil, fmt.Errorf("failed to load published runtime surfaces: %w", err)
	}

	if len(surfaceRows) == 0 {
		return resourceAuthorizationFromFallback(resourceType, resourceID, fallback), nil
	}

	surfaceIDs := make([]uuid.UUID, 0, len(surfaceRows))
	for _, row := range surfaceRows {
		surfaceIDs = append(surfaceIDs, row.ID)
	}

	var grantRows []publishedRuntimeGrantRecord
	if err := s.db.WithContext(ctx).
		Where("surface_id IN ? AND deleted_at IS NULL", surfaceIDs).
		Order("subject_type ASC, subject_id ASC, created_at ASC").
		Find(&grantRows).Error; err != nil {
		return nil, fmt.Errorf("failed to load published runtime surface grants: %w", err)
	}

	grantsBySurfaceID := make(map[uuid.UUID][]SurfaceGrant, len(surfaceRows))
	for _, row := range grantRows {
		grantsBySurfaceID[row.SurfaceID] = append(grantsBySurfaceID[row.SurfaceID], SurfaceGrant{
			SubjectType: PublishedRuntimeSubjectType(row.SubjectType),
			SubjectID:   copyUUIDPtr(row.SubjectID),
			Enabled:     row.Enabled,
		})
	}

	auth := &ResourceAuthorization{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Surfaces:     defaultSurfaceAuthorizations(resourceType, fallback),
	}

	for _, row := range surfaceRows {
		if auth.OrganizationID == uuid.Nil {
			auth.OrganizationID = row.OrganizationID
		}
		if auth.WorkspaceID == nil && row.WorkspaceID != nil {
			auth.WorkspaceID = copyUUIDPtr(row.WorkspaceID)
		}
		setSurfaceAuthorization(auth.Surfaces, SurfaceAuthorization{
			Surface:             PublishedRuntimeSurface(row.Surface),
			Enabled:             row.Enabled,
			CompatibilitySource: row.CompatibilitySource,
			Grants:              slices.Clone(grantsBySurfaceID[row.ID]),
		})
	}
	return auth, nil
}

// ListAuthorizedResourceIDs returns persisted resources for a surface that are enabled
// and allow the supplied audience. Legacy fallback candidate selection remains owned
// by the caller because fallback state may live outside published_runtime_surfaces.
func (s *Store) ListAuthorizedResourceIDs(ctx context.Context, resourceType PublishedRuntimeResourceType, surface PublishedRuntimeSurface, organizationID uuid.UUID, audience RuntimeAudience) ([]uuid.UUID, error) {
	if s == nil || s.db == nil {
		return []uuid.UUID{}, nil
	}
	if resourceType == "" || organizationID == uuid.Nil {
		return nil, fmt.Errorf("resource type and organization id are required")
	}
	if !IsKnownSurface(surface) {
		return nil, fmt.Errorf("unknown published runtime surface %q", surface)
	}

	var surfaceRows []publishedRuntimeSurfaceRecord
	if err := s.db.WithContext(ctx).
		Where("resource_type = ? AND surface = ? AND organization_id = ? AND enabled = ? AND deleted_at IS NULL", string(resourceType), string(surface), organizationID, true).
		Order("resource_id ASC").
		Find(&surfaceRows).Error; err != nil {
		return nil, fmt.Errorf("failed to list published runtime surfaces: %w", err)
	}
	if len(surfaceRows) == 0 {
		return []uuid.UUID{}, nil
	}

	surfaceIDs := make([]uuid.UUID, 0, len(surfaceRows))
	for _, row := range surfaceRows {
		surfaceIDs = append(surfaceIDs, row.ID)
	}

	var grantRows []publishedRuntimeGrantRecord
	if err := s.db.WithContext(ctx).
		Where("surface_id IN ? AND deleted_at IS NULL", surfaceIDs).
		Order("subject_type ASC, subject_id ASC, created_at ASC").
		Find(&grantRows).Error; err != nil {
		return nil, fmt.Errorf("failed to list published runtime surface grants: %w", err)
	}

	grantsBySurfaceID := make(map[uuid.UUID][]SurfaceGrant, len(surfaceRows))
	for _, row := range grantRows {
		grantsBySurfaceID[row.SurfaceID] = append(grantsBySurfaceID[row.SurfaceID], SurfaceGrant{
			SubjectType: PublishedRuntimeSubjectType(row.SubjectType),
			SubjectID:   copyUUIDPtr(row.SubjectID),
			Enabled:     row.Enabled,
		})
	}

	out := make([]uuid.UUID, 0, len(surfaceRows))
	for _, row := range surfaceRows {
		authorization := SurfaceAuthorization{
			Surface: surface,
			Enabled: row.Enabled,
			Grants:  slices.Clone(grantsBySurfaceID[row.ID]),
		}
		if authorization.Allows(audience) {
			out = append(out, row.ResourceID)
		}
	}
	return out, nil
}

// FilterAuthorizedResourceIDs filters caller-owned candidate resources by
// applying persisted surface rows over each candidate's fallback policy.
func (s *Store) FilterAuthorizedResourceIDs(ctx context.Context, resourceType PublishedRuntimeResourceType, surface PublishedRuntimeSurface, organizationID uuid.UUID, candidates []ResourceAuthorizationCandidate, audience RuntimeAudience) ([]uuid.UUID, error) {
	if len(candidates) == 0 {
		return []uuid.UUID{}, nil
	}
	if resourceType == "" || organizationID == uuid.Nil {
		return nil, fmt.Errorf("resource type and organization id are required")
	}
	if !IsKnownSurface(surface) {
		return nil, fmt.Errorf("unknown published runtime surface %q", surface)
	}

	resourceIDs, err := uniqueCandidateResourceIDs(candidates)
	if err != nil {
		return nil, err
	}
	if s == nil || s.db == nil {
		return filterAuthorizedCandidates(resourceType, surface, candidates, nil, audience), nil
	}

	var surfaceRows []publishedRuntimeSurfaceRecord
	if err := s.db.WithContext(ctx).
		Where("resource_type = ? AND surface = ? AND organization_id = ? AND resource_id IN ? AND deleted_at IS NULL", string(resourceType), string(surface), organizationID, resourceIDs).
		Order("resource_id ASC").
		Find(&surfaceRows).Error; err != nil {
		return nil, fmt.Errorf("failed to load published runtime surface candidates: %w", err)
	}
	if len(surfaceRows) == 0 {
		return filterAuthorizedCandidates(resourceType, surface, candidates, nil, audience), nil
	}

	surfaceIDs := make([]uuid.UUID, 0, len(surfaceRows))
	for _, row := range surfaceRows {
		surfaceIDs = append(surfaceIDs, row.ID)
	}

	var grantRows []publishedRuntimeGrantRecord
	if err := s.db.WithContext(ctx).
		Where("surface_id IN ? AND deleted_at IS NULL", surfaceIDs).
		Order("subject_type ASC, subject_id ASC, created_at ASC").
		Find(&grantRows).Error; err != nil {
		return nil, fmt.Errorf("failed to load published runtime surface candidate grants: %w", err)
	}

	grantsBySurfaceID := make(map[uuid.UUID][]SurfaceGrant, len(surfaceRows))
	for _, row := range grantRows {
		grantsBySurfaceID[row.SurfaceID] = append(grantsBySurfaceID[row.SurfaceID], SurfaceGrant{
			SubjectType: PublishedRuntimeSubjectType(row.SubjectType),
			SubjectID:   copyUUIDPtr(row.SubjectID),
			Enabled:     row.Enabled,
		})
	}

	overlays := make(map[uuid.UUID]SurfaceAuthorization, len(surfaceRows))
	for _, row := range surfaceRows {
		overlays[row.ResourceID] = SurfaceAuthorization{
			Surface:             PublishedRuntimeSurface(row.Surface),
			Enabled:             row.Enabled,
			CompatibilitySource: row.CompatibilitySource,
			Grants:              slices.Clone(grantsBySurfaceID[row.ID]),
		}
	}
	return filterAuthorizedCandidates(resourceType, surface, candidates, overlays, audience), nil
}

func (s *Store) SaveResourceAuthorization(ctx context.Context, auth ResourceAuthorization) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("database is required")
	}
	if auth.ResourceType == "" || auth.ResourceID == uuid.Nil || auth.OrganizationID == uuid.Nil {
		return fmt.Errorf("resource type, resource id, and organization id are required")
	}
	if len(auth.Surfaces) == 0 {
		return fmt.Errorf("at least one published runtime surface is required")
	}
	for _, surface := range auth.Surfaces {
		if err := validateSurfaceAuthorization(auth.ResourceType, surface); err != nil {
			return err
		}
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, surface := range auth.Surfaces {
			if err := saveSurfaceAuthorization(ctx, tx, auth, surface); err != nil {
				return err
			}
		}
		return nil
	})
}

func PolicyFromAuthorization(fallback PublishedRuntimePolicy, auth *ResourceAuthorization) PublishedRuntimePolicy {
	policy := fallback
	if auth == nil {
		return policy
	}

	policy.AllowedBuiltinAccountIDs = nil
	policy.AllowedBuiltinDeptIDs = nil
	if auth.ResourceType == PublishedRuntimeResourceAgent {
		policy.BuiltinAppEnabled = false
	}
	for _, surface := range auth.Surfaces {
		switch surface.Surface {
		case PublishedRuntimeSurfaceWebApp:
			if surface.Enabled {
				policy.WebAppStatus = WebAppStatusActive
			} else {
				policy.WebAppStatus = WebAppStatusInactive
			}
		case PublishedRuntimeSurfaceAPI:
			policy.APIEnabled = surface.Enabled
		case PublishedRuntimeSurfaceAppCenter:
			policy.AppCenterEnabled = surface.Enabled
		case PublishedRuntimeSurfaceBuiltinApp:
			if auth.ResourceType == PublishedRuntimeResourceAgent {
				continue
			}
			policy.BuiltinAppEnabled = surface.Enabled
			if surface.Enabled {
				policy.AllowedBuiltinAccountIDs, policy.AllowedBuiltinDeptIDs = builtinAudienceIDs(surface.Grants)
			}
		case PublishedRuntimeSurfaceInternal:
			policy.InternalInvocation = surface.Enabled
		}
	}
	if auth.ResourceType == PublishedRuntimeResourceAgent && NormalizeWebAppStatus(fallback.WebAppStatus) != WebAppStatusActive {
		return disabledAgentPublishedRuntimePolicy(policy)
	}
	return policy
}

func disabledAgentPublishedRuntimePolicy(policy PublishedRuntimePolicy) PublishedRuntimePolicy {
	policy.WebAppStatus = WebAppStatusInactive
	policy.APIEnabled = false
	policy.AppCenterEnabled = false
	policy.BuiltinAppEnabled = false
	policy.InternalInvocation = false
	policy.AllowedBuiltinAccountIDs = nil
	policy.AllowedBuiltinDeptIDs = nil
	return policy
}

func saveSurfaceAuthorization(ctx context.Context, tx *gorm.DB, auth ResourceAuthorization, surface SurfaceAuthorization) error {
	now := time.Now()
	var row publishedRuntimeSurfaceRecord
	err := tx.WithContext(ctx).
		Where("resource_type = ? AND resource_id = ? AND surface = ? AND deleted_at IS NULL", string(auth.ResourceType), auth.ResourceID, string(surface.Surface)).
		Take(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to load published runtime surface: %w", err)
	}

	workspaceID := copyUUIDPtr(auth.WorkspaceID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		row = publishedRuntimeSurfaceRecord{
			ID:                  uuid.New(),
			ResourceType:        string(auth.ResourceType),
			ResourceID:          auth.ResourceID,
			OrganizationID:      auth.OrganizationID,
			WorkspaceID:         workspaceID,
			Surface:             string(surface.Surface),
			Enabled:             surface.Enabled,
			CompatibilitySource: nonEmptySource(surface.CompatibilitySource),
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
			return fmt.Errorf("failed to create published runtime surface: %w", err)
		}
	} else {
		if err := tx.WithContext(ctx).Model(&publishedRuntimeSurfaceRecord{}).
			Where("id = ? AND deleted_at IS NULL", row.ID).
			Updates(map[string]interface{}{
				"organization_id":      auth.OrganizationID,
				"workspace_id":         workspaceID,
				"enabled":              surface.Enabled,
				"compatibility_source": nonEmptySource(surface.CompatibilitySource),
				"updated_at":           now,
			}).Error; err != nil {
			return fmt.Errorf("failed to update published runtime surface: %w", err)
		}
	}

	if err := tx.WithContext(ctx).Model(&publishedRuntimeGrantRecord{}).
		Where("surface_id = ? AND deleted_at IS NULL", row.ID).
		Updates(map[string]interface{}{
			"deleted_at": now,
			"updated_at": now,
		}).Error; err != nil {
		return fmt.Errorf("failed to replace published runtime surface grants: %w", err)
	}

	grantRows := make([]publishedRuntimeGrantRecord, 0, len(surface.Grants))
	for _, grant := range surface.Grants {
		grantRows = append(grantRows, publishedRuntimeGrantRecord{
			ID:          uuid.New(),
			SurfaceID:   row.ID,
			SubjectType: string(grant.SubjectType),
			SubjectID:   copyUUIDPtr(grant.SubjectID),
			Enabled:     grant.Enabled,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}
	if len(grantRows) == 0 {
		return nil
	}
	if err := tx.WithContext(ctx).Create(&grantRows).Error; err != nil {
		return fmt.Errorf("failed to create published runtime surface grants: %w", err)
	}
	return nil
}

func resourceAuthorizationFromFallback(resourceType PublishedRuntimeResourceType, resourceID uuid.UUID, fallback PublishedRuntimePolicy) *ResourceAuthorization {
	return &ResourceAuthorization{
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Surfaces:     defaultSurfaceAuthorizations(resourceType, fallback),
	}
}

func uniqueCandidateResourceIDs(candidates []ResourceAuthorizationCandidate) ([]uuid.UUID, error) {
	ids := make([]uuid.UUID, 0, len(candidates))
	seen := make(map[uuid.UUID]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.ResourceID == uuid.Nil {
			return nil, fmt.Errorf("resource authorization candidate id is required")
		}
		if _, ok := seen[candidate.ResourceID]; ok {
			continue
		}
		seen[candidate.ResourceID] = struct{}{}
		ids = append(ids, candidate.ResourceID)
	}
	return ids, nil
}

func filterAuthorizedCandidates(resourceType PublishedRuntimeResourceType, surface PublishedRuntimeSurface, candidates []ResourceAuthorizationCandidate, overlays map[uuid.UUID]SurfaceAuthorization, audience RuntimeAudience) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(candidates))
	for _, candidate := range candidates {
		if resourceType == PublishedRuntimeResourceAgent && NormalizeWebAppStatus(candidate.Fallback.WebAppStatus) != WebAppStatusActive {
			continue
		}
		auth := resourceAuthorizationFromFallback(resourceType, candidate.ResourceID, candidate.Fallback)
		if overlay, ok := overlays[candidate.ResourceID]; ok {
			setSurfaceAuthorization(auth.Surfaces, overlay)
		}
		if auth.Allows(surface, audience) {
			out = append(out, candidate.ResourceID)
		}
	}
	return out
}

func defaultSurfaceAuthorizations(resourceType PublishedRuntimeResourceType, policy PublishedRuntimePolicy) []SurfaceAuthorization {
	switch resourceType {
	case PublishedRuntimeResourceBuiltinWorkflow:
		return []SurfaceAuthorization{
			{
				Surface:             PublishedRuntimeSurfaceBuiltinApp,
				Enabled:             policy.Allows(PublishedRuntimeSurfaceBuiltinApp),
				CompatibilitySource: PublishedRuntimeSourceSystemDefault,
			},
			{
				Surface:             PublishedRuntimeSurfaceInternal,
				Enabled:             policy.Allows(PublishedRuntimeSurfaceInternal),
				CompatibilitySource: PublishedRuntimeSourceLegacyAgentFields,
			},
		}
	default:
		return []SurfaceAuthorization{
			{
				Surface:             PublishedRuntimeSurfaceWebApp,
				Enabled:             policy.Allows(PublishedRuntimeSurfaceWebApp),
				CompatibilitySource: PublishedRuntimeSourceLegacyAgentFields,
			},
			{
				Surface:             PublishedRuntimeSurfaceAPI,
				Enabled:             policy.Allows(PublishedRuntimeSurfaceAPI),
				CompatibilitySource: PublishedRuntimeSourceLegacyAgentFields,
			},
			{
				Surface:             PublishedRuntimeSurfaceAppCenter,
				Enabled:             policy.Allows(PublishedRuntimeSurfaceAppCenter),
				CompatibilitySource: PublishedRuntimeSourceLegacyAgentFields,
			},
			{
				Surface:             PublishedRuntimeSurfaceInternal,
				Enabled:             policy.Allows(PublishedRuntimeSurfaceInternal),
				CompatibilitySource: PublishedRuntimeSourceLegacyAgentFields,
			},
		}
	}
}

func IsKnownSurface(surface PublishedRuntimeSurface) bool {
	switch surface {
	case PublishedRuntimeSurfaceWebApp,
		PublishedRuntimeSurfaceAPI,
		PublishedRuntimeSurfaceAppCenter,
		PublishedRuntimeSurfaceBuiltinApp,
		PublishedRuntimeSurfaceInternal:
		return true
	default:
		return false
	}
}

func IsKnownSubjectType(subjectType PublishedRuntimeSubjectType) bool {
	switch subjectType {
	case PublishedRuntimeSubjectPublic,
		PublishedRuntimeSubjectOrganization,
		PublishedRuntimeSubjectDepartment,
		PublishedRuntimeSubjectWorkspace,
		PublishedRuntimeSubjectAccount,
		PublishedRuntimeSubjectInternal:
		return true
	default:
		return false
	}
}

func validateSurfaceAuthorization(resourceType PublishedRuntimeResourceType, surface SurfaceAuthorization) error {
	if !IsKnownSurface(surface.Surface) {
		return fmt.Errorf("unknown published runtime surface %q", surface.Surface)
	}
	if !surfaceSupportedForResource(resourceType, surface.Surface) {
		return fmt.Errorf("runtime surface %q is not supported for resource type %q", surface.Surface, resourceType)
	}
	for _, grant := range surface.Grants {
		if !IsKnownSubjectType(grant.SubjectType) {
			return fmt.Errorf("unknown published runtime subject type %q", grant.SubjectType)
		}
		if err := validateSurfaceGrantSubject(surface.Surface, grant.SubjectType); err != nil {
			return err
		}
	}
	return nil
}

func surfaceSupportedForResource(resourceType PublishedRuntimeResourceType, surface PublishedRuntimeSurface) bool {
	switch resourceType {
	case PublishedRuntimeResourceBuiltinWorkflow:
		return surface == PublishedRuntimeSurfaceBuiltinApp || surface == PublishedRuntimeSurfaceInternal
	default:
		return surface == PublishedRuntimeSurfaceWebApp ||
			surface == PublishedRuntimeSurfaceAPI ||
			surface == PublishedRuntimeSurfaceAppCenter ||
			surface == PublishedRuntimeSurfaceInternal
	}
}

func validateSurfaceGrantSubject(surface PublishedRuntimeSurface, subjectType PublishedRuntimeSubjectType) error {
	switch surface {
	case PublishedRuntimeSurfaceWebApp:
		switch subjectType {
		case PublishedRuntimeSubjectPublic,
			PublishedRuntimeSubjectOrganization,
			PublishedRuntimeSubjectAccount,
			PublishedRuntimeSubjectDepartment,
			PublishedRuntimeSubjectWorkspace:
			return nil
		default:
			return fmt.Errorf("webapp runtime grants must target public, organization, account, department, or workspace")
		}
	case PublishedRuntimeSurfaceAPI:
		if subjectType != PublishedRuntimeSubjectPublic {
			return fmt.Errorf("api runtime grants must use public subject")
		}
	case PublishedRuntimeSurfaceAppCenter:
		switch subjectType {
		case PublishedRuntimeSubjectOrganization,
			PublishedRuntimeSubjectAccount,
			PublishedRuntimeSubjectDepartment,
			PublishedRuntimeSubjectWorkspace:
			return nil
		default:
			return fmt.Errorf("app center grants must target organization, account, department, or workspace")
		}
	case PublishedRuntimeSurfaceInternal:
		if subjectType != PublishedRuntimeSubjectInternal {
			return fmt.Errorf("internal runtime grants must use internal subject")
		}
	case PublishedRuntimeSurfaceBuiltinApp:
		switch subjectType {
		case PublishedRuntimeSubjectOrganization,
			PublishedRuntimeSubjectAccount,
			PublishedRuntimeSubjectDepartment,
			PublishedRuntimeSubjectWorkspace:
			return nil
		default:
			return fmt.Errorf("builtin app grants must target organization, account, department, or workspace")
		}
	}
	return nil
}

func NormalizeSurface(surface string) PublishedRuntimeSurface {
	return PublishedRuntimeSurface(strings.TrimSpace(surface))
}

func nonEmptySource(source string) string {
	if strings.TrimSpace(source) == "" {
		return PublishedRuntimeSourceGrant
	}
	return strings.TrimSpace(source)
}

func setSurfaceAuthorization(surfaces []SurfaceAuthorization, next SurfaceAuthorization) {
	for i := range surfaces {
		if surfaces[i].Surface == next.Surface {
			surfaces[i] = next
			return
		}
	}
}

func builtinAudienceIDs(grants []SurfaceGrant) ([]string, []string) {
	var accountIDs []string
	var departmentIDs []string
	for _, grant := range grants {
		if !grant.Enabled || grant.SubjectID == nil {
			continue
		}
		switch grant.SubjectType {
		case PublishedRuntimeSubjectAccount:
			accountIDs = append(accountIDs, grant.SubjectID.String())
		case PublishedRuntimeSubjectDepartment:
			departmentIDs = append(departmentIDs, grant.SubjectID.String())
		}
	}
	return accountIDs, departmentIDs
}

func copyUUIDPtr(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func NormalizeSubjectType(subjectType string) PublishedRuntimeSubjectType {
	return PublishedRuntimeSubjectType(strings.TrimSpace(subjectType))
}
