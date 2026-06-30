package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// OrganizationStatus enterprise group status enum
type OrganizationStatus string

const (
	OrganizationStatusActive   OrganizationStatus = "active"
	OrganizationStatusInactive OrganizationStatus = "inactive"
	OrganizationStatusArchived OrganizationStatus = "archived"
	OrganizationStatusDeleted  OrganizationStatus = "deleted"
)

// OrganizationRole represents a role in an organization
type OrganizationRole string

const (
	OrganizationRoleOwner  OrganizationRole = "owner"
	OrganizationRoleAdmin  OrganizationRole = "admin"
	OrganizationRoleNormal OrganizationRole = "normal"
)

// Organization enterprise group model
type Organization struct {
	ID        string             `gorm:"type:varchar(255);primaryKey" json:"id"`
	Name      string             `gorm:"type:varchar(255);not null" json:"name"`
	ShortName *string            `gorm:"type:varchar(255)" json:"short_name"`
	Status    OrganizationStatus `gorm:"type:varchar(16);not null;default:'active'" json:"status"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`

	// Relationships - commented out for modular architecture
	// TenantJoins  []EnterpriseGroupTenantJoin  `gorm:"foreignKey:GroupID" json:"-"`
	// Members      []OrganizationMember         `gorm:"foreignKey:GroupID" json:"-"`
}

// TableName sets the table name for Organization
func (Organization) TableName() string {
	return "organizations"
}

// IsActive checks if enterprise group is active
func (org *Organization) IsActive() bool {
	return org.Status == OrganizationStatusActive
}

// BeforeCreate hook to set ID and timestamps
func (org *Organization) BeforeCreate(tx *gorm.DB) error {
	// Generate UUID if ID is empty
	if org.ID == "" {
		org.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	if org.CreatedAt.IsZero() {
		org.CreatedAt = now
	}
	if org.UpdatedAt.IsZero() {
		org.UpdatedAt = now
	}

	return nil
}

const (
	WorkspaceBuiltinRoleOwnerID  = "00000000-0000-0000-0000-000000000001"
	WorkspaceBuiltinRoleAdminID  = "00000000-0000-0000-0000-000000000002"
	WorkspaceBuiltinRoleMemberID = "00000000-0000-0000-0000-000000000003"
	WorkspaceBuiltinRoleViewerID = "00000000-0000-0000-0000-000000000004"
)

type WorkspacePermissionCode string

const (
	WorkspacePermissionWorkspaceView             WorkspacePermissionCode = "workspace.view"
	WorkspacePermissionWorkspaceManage           WorkspacePermissionCode = "workspace.manage"
	WorkspacePermissionWorkspaceBillingAudit     WorkspacePermissionCode = "workspace.billing_audit"
	WorkspacePermissionWorkspaceTransferArchive  WorkspacePermissionCode = "workspace.transfer_archive"
	WorkspacePermissionWorkspaceSettingsManage   WorkspacePermissionCode = "workspace.settings.manage"
	WorkspacePermissionWorkspaceMemberView       WorkspacePermissionCode = "workspace.member.view"
	WorkspacePermissionWorkspaceMemberManage     WorkspacePermissionCode = "workspace.member.manage"
	WorkspacePermissionWorkspacePermissionManage WorkspacePermissionCode = "workspace.permission.manage"
	WorkspacePermissionWorkspaceTransfer         WorkspacePermissionCode = "workspace.transfer"
	WorkspacePermissionWorkspaceArchive          WorkspacePermissionCode = "workspace.archive"

	WorkspacePermissionAgentView                WorkspacePermissionCode = "agent.view"
	WorkspacePermissionAgentManage              WorkspacePermissionCode = "agent.manage"
	WorkspacePermissionAgentCreate              WorkspacePermissionCode = "agent.create"
	WorkspacePermissionAgentUpdate              WorkspacePermissionCode = "agent.update"
	WorkspacePermissionAgentDelete              WorkspacePermissionCode = "agent.delete"
	WorkspacePermissionAgentMove                WorkspacePermissionCode = "agent.move"
	WorkspacePermissionAgentPublish             WorkspacePermissionCode = "agent.publish"
	WorkspacePermissionAgentRuntimeAccessManage WorkspacePermissionCode = "agent.runtime_access.manage"
	WorkspacePermissionAgentLogsView            WorkspacePermissionCode = "agent.logs.view"

	WorkspacePermissionWorkflowView                WorkspacePermissionCode = "workflow.view"
	WorkspacePermissionWorkflowCreate              WorkspacePermissionCode = "workflow.create"
	WorkspacePermissionWorkflowUpdate              WorkspacePermissionCode = "workflow.update"
	WorkspacePermissionWorkflowDelete              WorkspacePermissionCode = "workflow.delete"
	WorkspacePermissionWorkflowMove                WorkspacePermissionCode = "workflow.move"
	WorkspacePermissionWorkflowImport              WorkspacePermissionCode = "workflow.import"
	WorkspacePermissionWorkflowRunDraft            WorkspacePermissionCode = "workflow.run.draft"
	WorkspacePermissionWorkflowPublish             WorkspacePermissionCode = "workflow.publish"
	WorkspacePermissionWorkflowRuntimeAccessManage WorkspacePermissionCode = "workflow.runtime_access.manage"
	WorkspacePermissionWorkflowLogsView            WorkspacePermissionCode = "workflow.logs.view"

	WorkspacePermissionKnowledgeBaseView           WorkspacePermissionCode = "knowledge_base.view"
	WorkspacePermissionKnowledgeBaseManage         WorkspacePermissionCode = "knowledge_base.manage"
	WorkspacePermissionKnowledgeBaseRetrievalTest  WorkspacePermissionCode = "knowledge_base.retrieval_test"
	WorkspacePermissionKnowledgeBaseFolderManage   WorkspacePermissionCode = "knowledge_base.folder_manage"
	WorkspacePermissionKnowledgeBaseCreate         WorkspacePermissionCode = "knowledge_base.create"
	WorkspacePermissionKnowledgeBaseUpdate         WorkspacePermissionCode = "knowledge_base.update"
	WorkspacePermissionKnowledgeBaseDelete         WorkspacePermissionCode = "knowledge_base.delete"
	WorkspacePermissionKnowledgeBaseMove           WorkspacePermissionCode = "knowledge_base.move"
	WorkspacePermissionKnowledgeBaseDocumentView   WorkspacePermissionCode = "knowledge_base.document.view"
	WorkspacePermissionKnowledgeBaseDocumentCreate WorkspacePermissionCode = "knowledge_base.document.create"
	WorkspacePermissionKnowledgeBaseDocumentUpdate WorkspacePermissionCode = "knowledge_base.document.update"
	WorkspacePermissionKnowledgeBaseDocumentDelete WorkspacePermissionCode = "knowledge_base.document.delete"
	WorkspacePermissionKnowledgeBaseSegmentUpdate  WorkspacePermissionCode = "knowledge_base.segment.update"
	WorkspacePermissionKnowledgeBaseSegmentDelete  WorkspacePermissionCode = "knowledge_base.segment.delete"
	WorkspacePermissionKnowledgeBaseIndexManage    WorkspacePermissionCode = "knowledge_base.index.manage"
	WorkspacePermissionKnowledgeBaseGraphView      WorkspacePermissionCode = "knowledge_base.graph.view"
	WorkspacePermissionKnowledgeBaseGraphManage    WorkspacePermissionCode = "knowledge_base.graph.manage"

	WorkspacePermissionDatabaseView              WorkspacePermissionCode = "database.view"
	WorkspacePermissionDatabaseManage            WorkspacePermissionCode = "database.manage"
	WorkspacePermissionDatabaseDataEdit          WorkspacePermissionCode = "database.data_edit"
	WorkspacePermissionDatabaseAIQuery           WorkspacePermissionCode = "database.ai_query"
	WorkspacePermissionDatabaseCreate            WorkspacePermissionCode = "database.create"
	WorkspacePermissionDatabaseUpdate            WorkspacePermissionCode = "database.update"
	WorkspacePermissionDatabaseDelete            WorkspacePermissionCode = "database.delete"
	WorkspacePermissionDatabaseMove              WorkspacePermissionCode = "database.move"
	WorkspacePermissionDatabaseSchemaView        WorkspacePermissionCode = "database.schema.view"
	WorkspacePermissionDatabaseSchemaManage      WorkspacePermissionCode = "database.schema.manage"
	WorkspacePermissionDatabaseRecordView        WorkspacePermissionCode = "database.record.view"
	WorkspacePermissionDatabaseRecordCreate      WorkspacePermissionCode = "database.record.create"
	WorkspacePermissionDatabaseRecordUpdate      WorkspacePermissionCode = "database.record.update"
	WorkspacePermissionDatabaseRecordDelete      WorkspacePermissionCode = "database.record.delete"
	WorkspacePermissionDatabaseImportAnalyze     WorkspacePermissionCode = "database.import.analyze"
	WorkspacePermissionDatabaseImportExecute     WorkspacePermissionCode = "database.import.execute"
	WorkspacePermissionDatabaseAIQueryRead       WorkspacePermissionCode = "database.ai_query.read"
	WorkspacePermissionDatabaseSQLAuditView      WorkspacePermissionCode = "database.sql_audit.view"
	WorkspacePermissionDatabaseOperationLogsView WorkspacePermissionCode = "database.operation_logs.view"

	WorkspacePermissionFileView         WorkspacePermissionCode = "file.view"
	WorkspacePermissionFileManage       WorkspacePermissionCode = "file.manage"
	WorkspacePermissionFileUploadCreate WorkspacePermissionCode = "file.upload_create"
	WorkspacePermissionFileMoveCreate   WorkspacePermissionCode = "file.move_create"
	WorkspacePermissionFilePreview      WorkspacePermissionCode = "file.preview"
	WorkspacePermissionFileUpload       WorkspacePermissionCode = "file.upload"
	WorkspacePermissionFileTextCreate   WorkspacePermissionCode = "file.text.create"
	WorkspacePermissionFileUpdate       WorkspacePermissionCode = "file.update"
	WorkspacePermissionFileDelete       WorkspacePermissionCode = "file.delete"
	WorkspacePermissionFileMove         WorkspacePermissionCode = "file.move"
	WorkspacePermissionFileFolderManage WorkspacePermissionCode = "file.folder.manage"
)

func IsBuiltinRole(roleID string) bool {
	return roleID == WorkspaceBuiltinRoleOwnerID ||
		roleID == WorkspaceBuiltinRoleAdminID ||
		roleID == WorkspaceBuiltinRoleMemberID ||
		roleID == WorkspaceBuiltinRoleViewerID
}

func AllWorkspacePermissionCodes() []WorkspacePermissionCode {
	return uniqueWorkspacePermissionCodes([]WorkspacePermissionCode{
		WorkspacePermissionAgentView,
		WorkspacePermissionAgentManage,
		WorkspacePermissionAgentCreate,
		WorkspacePermissionAgentUpdate,
		WorkspacePermissionAgentDelete,
		WorkspacePermissionAgentMove,
		WorkspacePermissionAgentPublish,
		WorkspacePermissionAgentRuntimeAccessManage,
		WorkspacePermissionAgentLogsView,
		WorkspacePermissionWorkflowView,
		WorkspacePermissionWorkflowCreate,
		WorkspacePermissionWorkflowUpdate,
		WorkspacePermissionWorkflowDelete,
		WorkspacePermissionWorkflowMove,
		WorkspacePermissionWorkflowImport,
		WorkspacePermissionWorkflowRunDraft,
		WorkspacePermissionWorkflowPublish,
		WorkspacePermissionWorkflowRuntimeAccessManage,
		WorkspacePermissionWorkflowLogsView,
		WorkspacePermissionKnowledgeBaseView,
		WorkspacePermissionKnowledgeBaseManage,
		WorkspacePermissionKnowledgeBaseRetrievalTest,
		WorkspacePermissionKnowledgeBaseFolderManage,
		WorkspacePermissionKnowledgeBaseCreate,
		WorkspacePermissionKnowledgeBaseUpdate,
		WorkspacePermissionKnowledgeBaseDelete,
		WorkspacePermissionKnowledgeBaseMove,
		WorkspacePermissionKnowledgeBaseDocumentView,
		WorkspacePermissionKnowledgeBaseDocumentCreate,
		WorkspacePermissionKnowledgeBaseDocumentUpdate,
		WorkspacePermissionKnowledgeBaseDocumentDelete,
		WorkspacePermissionKnowledgeBaseSegmentUpdate,
		WorkspacePermissionKnowledgeBaseSegmentDelete,
		WorkspacePermissionKnowledgeBaseIndexManage,
		WorkspacePermissionKnowledgeBaseGraphView,
		WorkspacePermissionKnowledgeBaseGraphManage,
		WorkspacePermissionDatabaseView,
		WorkspacePermissionDatabaseManage,
		WorkspacePermissionDatabaseDataEdit,
		WorkspacePermissionDatabaseAIQuery,
		WorkspacePermissionDatabaseCreate,
		WorkspacePermissionDatabaseUpdate,
		WorkspacePermissionDatabaseDelete,
		WorkspacePermissionDatabaseMove,
		WorkspacePermissionDatabaseSchemaView,
		WorkspacePermissionDatabaseSchemaManage,
		WorkspacePermissionDatabaseRecordView,
		WorkspacePermissionDatabaseRecordCreate,
		WorkspacePermissionDatabaseRecordUpdate,
		WorkspacePermissionDatabaseRecordDelete,
		WorkspacePermissionDatabaseImportAnalyze,
		WorkspacePermissionDatabaseImportExecute,
		WorkspacePermissionDatabaseAIQueryRead,
		WorkspacePermissionDatabaseSQLAuditView,
		WorkspacePermissionDatabaseOperationLogsView,
		WorkspacePermissionFileView,
		WorkspacePermissionFileManage,
		WorkspacePermissionFileUploadCreate,
		WorkspacePermissionFileMoveCreate,
		WorkspacePermissionFilePreview,
		WorkspacePermissionFileUpload,
		WorkspacePermissionFileTextCreate,
		WorkspacePermissionFileUpdate,
		WorkspacePermissionFileDelete,
		WorkspacePermissionFileMove,
		WorkspacePermissionFileFolderManage,
	})
}

var knownWorkspacePermissionCodes = func() map[WorkspacePermissionCode]struct{} {
	codes := AllWorkspacePermissionCodes()
	known := make(map[WorkspacePermissionCode]struct{}, len(codes))
	for _, code := range codes {
		known[code] = struct{}{}
	}
	return known
}()

func IsKnownWorkspacePermissionCode(code WorkspacePermissionCode) bool {
	_, ok := knownWorkspacePermissionCodes[code]
	return ok
}

// ExpandWorkspacePermissionCodesForCompatibility expands coarse legacy workspace permissions
// into their fine-grained equivalents while preserving the original codes.
func ExpandWorkspacePermissionCodesForCompatibility(codes []WorkspacePermissionCode) []WorkspacePermissionCode {
	if len(codes) == 0 {
		return []WorkspacePermissionCode{}
	}

	expanded := make([]WorkspacePermissionCode, 0, len(codes))
	for _, code := range codes {
		expanded = appendWorkspacePermissionCode(expanded, code)
		for _, mapped := range legacyWorkspacePermissionExpansions[code] {
			expanded = appendWorkspacePermissionCode(expanded, mapped)
		}
	}
	return expanded
}

// ExpandWorkspacePermissionStringsForCompatibility expands string permission codes for API responses.
func ExpandWorkspacePermissionStringsForCompatibility(codes []string) []string {
	if len(codes) == 0 {
		return []string{}
	}

	typedCodes := make([]WorkspacePermissionCode, 0, len(codes))
	for _, code := range codes {
		typedCodes = append(typedCodes, WorkspacePermissionCode(code))
	}

	expandedCodes := ExpandWorkspacePermissionCodesForCompatibility(typedCodes)
	expanded := make([]string, 0, len(expandedCodes))
	for _, code := range expandedCodes {
		expanded = append(expanded, string(code))
	}
	return expanded
}

var deprecatedWorkspacePermissionSnapshotExpansions = map[WorkspacePermissionCode][]WorkspacePermissionCode{
	WorkspacePermissionAgentManage:         legacyWorkspacePermissionExpansions[WorkspacePermissionAgentManage],
	WorkspacePermissionKnowledgeBaseView:   legacyWorkspacePermissionExpansions[WorkspacePermissionKnowledgeBaseView],
	WorkspacePermissionKnowledgeBaseManage: legacyWorkspacePermissionExpansions[WorkspacePermissionKnowledgeBaseManage],
	WorkspacePermissionDatabaseView:        legacyWorkspacePermissionExpansions[WorkspacePermissionDatabaseView],
	WorkspacePermissionDatabaseManage:      legacyWorkspacePermissionExpansions[WorkspacePermissionDatabaseManage],
	WorkspacePermissionFileView:            legacyWorkspacePermissionExpansions[WorkspacePermissionFileView],
	WorkspacePermissionFileManage:          legacyWorkspacePermissionExpansions[WorkspacePermissionFileManage],
}

var compatibilityWorkspacePermissionSnapshotExpansions = map[WorkspacePermissionCode][]WorkspacePermissionCode{
	WorkspacePermissionDatabaseDataEdit: legacyWorkspacePermissionExpansions[WorkspacePermissionDatabaseDataEdit],
	WorkspacePermissionDatabaseAIQuery:  legacyWorkspacePermissionExpansions[WorkspacePermissionDatabaseAIQuery],
	WorkspacePermissionFileUploadCreate: legacyWorkspacePermissionExpansions[WorkspacePermissionFileUploadCreate],
	WorkspacePermissionFileMoveCreate:   legacyWorkspacePermissionExpansions[WorkspacePermissionFileMoveCreate],
}

// CanonicalWorkspacePermissionSnapshotStrings normalizes member or template snapshots.
// Deprecated and compatibility-only coarse asset permissions are replaced by
// fine-grained equivalents so new snapshots do not keep reintroducing legacy
// asset permission codes.
func CanonicalWorkspacePermissionSnapshotStrings(permissions []string) []string {
	normalized := NormalizeWorkspacePermissionStrings(permissions)
	if len(normalized) == 0 {
		return []string{}
	}

	codes := make([]WorkspacePermissionCode, 0, len(normalized))
	for _, permission := range normalized {
		code := WorkspacePermissionCode(permission)
		if !IsKnownWorkspacePermissionCode(code) {
			continue
		}
		if mapped, ok := deprecatedWorkspacePermissionSnapshotExpansions[code]; ok {
			for _, mappedCode := range mapped {
				codes = appendWorkspacePermissionCode(codes, mappedCode)
			}
			continue
		}
		if mapped, ok := compatibilityWorkspacePermissionSnapshotExpansions[code]; ok {
			for _, mappedCode := range mapped {
				codes = appendWorkspacePermissionCode(codes, mappedCode)
			}
			continue
		}
		codes = appendWorkspacePermissionCode(codes, code)
	}

	return WorkspacePermissionStringsFromCodes(codes)
}

// WorkspacePermissionCodesAllow reports whether granted permissions include the requested permission.
// Legacy coarse grants allow their mapped fine-grained permissions, but fine-grained grants do not
// imply the old coarse permission.
func WorkspacePermissionCodesAllow(granted []WorkspacePermissionCode, requested WorkspacePermissionCode) bool {
	for _, code := range ExpandWorkspacePermissionCodesForCompatibility(granted) {
		if code == requested {
			return true
		}
	}
	return false
}

// WorkspacePermissionStringsAllow reports whether string permissions include the requested permission.
func WorkspacePermissionStringsAllow(granted []string, requested WorkspacePermissionCode) bool {
	if len(granted) == 0 {
		return false
	}

	codes := make([]WorkspacePermissionCode, 0, len(granted))
	for _, code := range granted {
		codes = append(codes, WorkspacePermissionCode(code))
	}
	return WorkspacePermissionCodesAllow(codes, requested)
}

func IsWorkspaceGovernancePermission(code WorkspacePermissionCode) bool {
	switch code {
	case WorkspacePermissionWorkspaceManage,
		WorkspacePermissionWorkspaceBillingAudit,
		WorkspacePermissionWorkspaceTransferArchive,
		WorkspacePermissionWorkspaceSettingsManage,
		WorkspacePermissionWorkspaceMemberView,
		WorkspacePermissionWorkspaceMemberManage,
		WorkspacePermissionWorkspacePermissionManage,
		WorkspacePermissionWorkspaceTransfer,
		WorkspacePermissionWorkspaceArchive:
		return true
	default:
		return false
	}
}

func IsWorkspaceMembershipPermission(code WorkspacePermissionCode) bool {
	switch code {
	case WorkspacePermissionWorkspaceView:
		return true
	default:
		return false
	}
}

func IsWorkspaceCompatibilityPermission(code WorkspacePermissionCode) bool {
	switch code {
	case WorkspacePermissionDatabaseDataEdit,
		WorkspacePermissionDatabaseAIQuery,
		WorkspacePermissionFileUploadCreate,
		WorkspacePermissionFileMoveCreate:
		return true
	default:
		return false
	}
}

func isRetiredWorkspacePermission(code WorkspacePermissionCode) bool {
	value := string(code)
	return strings.HasPrefix(value, "prompt.") ||
		strings.HasPrefix(value, "content_parse.") ||
		strings.HasPrefix(value, "dashboard.") ||
		strings.HasPrefix(value, "workspace.")
}

func WorkspacePermissionRequiresGovernanceRole(code WorkspacePermissionCode) bool {
	return IsWorkspaceGovernancePermission(code)
}

func WorkspaceMemberRoleHasGovernanceAuthority(role WorkspaceMemberRole) bool {
	return role == WorkspaceRoleOwner || role == WorkspaceRoleAdmin
}

func CanonicalAssignableWorkspacePermissionSnapshotStrings(permissions []string) []string {
	canonical := CanonicalWorkspacePermissionSnapshotStrings(permissions)
	if len(canonical) == 0 {
		return []string{}
	}

	expanded := ExpandWorkspacePermissionStringsForCompatibility(canonical)
	result := make([]string, 0, len(expanded))
	for _, permission := range expanded {
		code := WorkspacePermissionCode(permission)
		if !IsKnownWorkspacePermissionCode(code) ||
			IsWorkspaceGovernancePermission(code) ||
			IsWorkspaceCompatibilityPermission(code) ||
			isRetiredWorkspacePermission(code) {
			continue
		}
		result = append(result, permission)
	}
	return WorkspacePermissionStringsFromCodes(permissionStringsToCodes(result))
}

func permissionStringsToCodes(permissions []string) []WorkspacePermissionCode {
	codes := make([]WorkspacePermissionCode, 0, len(permissions))
	for _, permission := range permissions {
		permission = strings.TrimSpace(permission)
		if permission == "" {
			continue
		}
		codes = append(codes, WorkspacePermissionCode(permission))
	}
	return codes
}

func WorkspaceMemberAllowsPermission(
	role WorkspaceMemberRole,
	roleID *string,
	permissions []string,
	permissionSource WorkspaceMemberPermissionSource,
	permissionCode WorkspacePermissionCode,
) bool {
	if role == WorkspaceRoleOwner {
		return true
	}
	if IsWorkspaceMembershipPermission(permissionCode) {
		return true
	}
	if IsWorkspaceGovernancePermission(permissionCode) {
		return role == WorkspaceRoleAdmin
	}
	if role == WorkspaceRoleAdmin {
		return WorkspacePermissionStringsAllow(
			DefaultWorkspaceMemberPermissionStrings(role, stringPtr(WorkspaceBuiltinRoleAdminID)),
			permissionCode,
		)
	}
	return WorkspacePermissionStringsAllow(
		EffectiveWorkspaceMemberPermissionStrings(role, roleID, permissions, permissionSource),
		permissionCode,
	)
}

func stringPtr(value string) *string {
	return &value
}

func GetBuiltinGroupRolePermissionsByID(roleID string) []WorkspacePermissionCode {
	switch roleID {
	case WorkspaceBuiltinRoleOwnerID:
		return AllWorkspacePermissionCodes()
	case WorkspaceBuiltinRoleAdminID:
		return ExpandWorkspacePermissionCodesForCompatibility([]WorkspacePermissionCode{
			WorkspacePermissionAgentView,
			WorkspacePermissionAgentLogsView,
			WorkspacePermissionAgentManage,
			WorkspacePermissionWorkflowView,
			WorkspacePermissionWorkflowLogsView,
			WorkspacePermissionKnowledgeBaseView,
			WorkspacePermissionKnowledgeBaseManage,
			WorkspacePermissionKnowledgeBaseRetrievalTest,
			WorkspacePermissionKnowledgeBaseFolderManage,
			WorkspacePermissionDatabaseView,
			WorkspacePermissionDatabaseManage,
			WorkspacePermissionDatabaseDataEdit,
			WorkspacePermissionDatabaseAIQuery,
			WorkspacePermissionFileView,
			WorkspacePermissionFileManage,
			WorkspacePermissionFileUploadCreate,
			WorkspacePermissionFileMoveCreate,
		})
	case WorkspaceBuiltinRoleMemberID:
		return ExpandWorkspacePermissionCodesForCompatibility([]WorkspacePermissionCode{
			WorkspacePermissionAgentView,
			WorkspacePermissionAgentLogsView,
			WorkspacePermissionWorkflowView,
			WorkspacePermissionWorkflowLogsView,
			WorkspacePermissionKnowledgeBaseView,
			WorkspacePermissionKnowledgeBaseRetrievalTest,
			WorkspacePermissionDatabaseView,
			WorkspacePermissionDatabaseAIQuery,
			WorkspacePermissionFileView,
			WorkspacePermissionFileUploadCreate,
		})
	case WorkspaceBuiltinRoleViewerID:
		return ExpandWorkspacePermissionCodesForCompatibility([]WorkspacePermissionCode{
			WorkspacePermissionAgentView,
			WorkspacePermissionAgentLogsView,
			WorkspacePermissionWorkflowView,
			WorkspacePermissionWorkflowLogsView,
			WorkspacePermissionKnowledgeBaseView,
			WorkspacePermissionDatabaseView,
			WorkspacePermissionFileView,
		})
	default:
		return nil
	}
}

func appendWorkspacePermissionCode(codes []WorkspacePermissionCode, code WorkspacePermissionCode) []WorkspacePermissionCode {
	for _, existing := range codes {
		if existing == code {
			return codes
		}
	}
	return append(codes, code)
}

func uniqueWorkspacePermissionCodes(codes []WorkspacePermissionCode) []WorkspacePermissionCode {
	unique := make([]WorkspacePermissionCode, 0, len(codes))
	for _, code := range codes {
		unique = appendWorkspacePermissionCode(unique, code)
	}
	return unique
}

var legacyWorkspacePermissionExpansions = map[WorkspacePermissionCode][]WorkspacePermissionCode{
	WorkspacePermissionAgentManage: {
		WorkspacePermissionAgentView,
		WorkspacePermissionAgentCreate,
		WorkspacePermissionAgentUpdate,
		WorkspacePermissionAgentDelete,
		WorkspacePermissionAgentMove,
		WorkspacePermissionAgentPublish,
		WorkspacePermissionAgentRuntimeAccessManage,
		WorkspacePermissionAgentLogsView,
		WorkspacePermissionWorkflowView,
		WorkspacePermissionWorkflowCreate,
		WorkspacePermissionWorkflowUpdate,
		WorkspacePermissionWorkflowDelete,
		WorkspacePermissionWorkflowMove,
		WorkspacePermissionWorkflowImport,
		WorkspacePermissionWorkflowRunDraft,
		WorkspacePermissionWorkflowPublish,
		WorkspacePermissionWorkflowRuntimeAccessManage,
		WorkspacePermissionWorkflowLogsView,
	},
	WorkspacePermissionKnowledgeBaseView: {
		WorkspacePermissionKnowledgeBaseDocumentView,
		WorkspacePermissionKnowledgeBaseGraphView,
	},
	WorkspacePermissionKnowledgeBaseManage: {
		WorkspacePermissionKnowledgeBaseCreate,
		WorkspacePermissionKnowledgeBaseUpdate,
		WorkspacePermissionKnowledgeBaseDelete,
		WorkspacePermissionKnowledgeBaseMove,
		WorkspacePermissionKnowledgeBaseDocumentCreate,
		WorkspacePermissionKnowledgeBaseDocumentUpdate,
		WorkspacePermissionKnowledgeBaseDocumentDelete,
		WorkspacePermissionKnowledgeBaseFolderManage,
		WorkspacePermissionKnowledgeBaseSegmentUpdate,
		WorkspacePermissionKnowledgeBaseSegmentDelete,
		WorkspacePermissionKnowledgeBaseIndexManage,
		WorkspacePermissionKnowledgeBaseGraphManage,
	},
	WorkspacePermissionKnowledgeBaseFolderManage: {
		WorkspacePermissionKnowledgeBaseFolderManage,
	},
	WorkspacePermissionDatabaseView: {
		WorkspacePermissionDatabaseSchemaView,
		WorkspacePermissionDatabaseRecordView,
		WorkspacePermissionDatabaseOperationLogsView,
	},
	WorkspacePermissionDatabaseManage: {
		WorkspacePermissionDatabaseCreate,
		WorkspacePermissionDatabaseUpdate,
		WorkspacePermissionDatabaseDelete,
		WorkspacePermissionDatabaseMove,
		WorkspacePermissionDatabaseSchemaManage,
		WorkspacePermissionDatabaseImportAnalyze,
		WorkspacePermissionDatabaseSQLAuditView,
	},
	WorkspacePermissionDatabaseDataEdit: {
		WorkspacePermissionDatabaseRecordCreate,
		WorkspacePermissionDatabaseRecordUpdate,
		WorkspacePermissionDatabaseRecordDelete,
		WorkspacePermissionDatabaseImportExecute,
	},
	WorkspacePermissionDatabaseAIQuery: {
		WorkspacePermissionDatabaseAIQueryRead,
	},
	WorkspacePermissionFileView: {
		WorkspacePermissionFilePreview,
	},
	WorkspacePermissionFileManage: {
		WorkspacePermissionFileUpdate,
		WorkspacePermissionFileDelete,
		WorkspacePermissionFileMove,
		WorkspacePermissionFileFolderManage,
	},
	WorkspacePermissionFileUploadCreate: {
		WorkspacePermissionFileUpload,
		WorkspacePermissionFileTextCreate,
	},
	WorkspacePermissionFileMoveCreate: {
		WorkspacePermissionFileMove,
		WorkspacePermissionFileFolderManage,
	},
}

// WorkspaceCustomRoleStatus workspace custom role status enum
type WorkspaceCustomRoleStatus string

const (
	WorkspaceCustomRoleStatusActive   WorkspaceCustomRoleStatus = "active"
	WorkspaceCustomRoleStatusInactive WorkspaceCustomRoleStatus = "inactive"
	WorkspaceCustomRoleStatusArchived WorkspaceCustomRoleStatus = "archived"
	WorkspaceCustomRoleStatusDeleted  WorkspaceCustomRoleStatus = "deleted"
)

type WorkspaceRoleTemplateOrigin string

const (
	WorkspaceRoleTemplateOriginCustom        WorkspaceRoleTemplateOrigin = "custom"
	WorkspaceRoleTemplateOriginSystemDefault WorkspaceRoleTemplateOrigin = "system_default"
)

const (
	WorkspaceDefaultRoleTemplateAdvancedKey = "default_advanced"
	WorkspaceDefaultRoleTemplateBasicKey    = "default_basic"
	WorkspaceDefaultRoleTemplateReadonlyKey = "default_readonly"
)

type WorkspaceDefaultRoleTemplateDefinition struct {
	SystemKey    string
	NameZhHans   string
	NameEnUS     string
	DescZhHans   string
	DescEnUS     string
	Permissions  []string
	DisplayOrder int
}

func DefaultWorkspaceRoleTemplateDefinitions() []WorkspaceDefaultRoleTemplateDefinition {
	return []WorkspaceDefaultRoleTemplateDefinition{
		{
			SystemKey:    WorkspaceDefaultRoleTemplateAdvancedKey,
			NameZhHans:   "高级成员",
			NameEnUS:     "Advanced Member",
			DescZhHans:   "具备大部分资产创建、编辑、发布和内容管理权限，适用于非管理角色中的技术骨干。",
			DescEnUS:     "Can create, edit, publish, and manage most workspace assets without workspace governance permissions.",
			Permissions:  advancedWorkspaceMemberPermissionStrings(),
			DisplayOrder: 10,
		},
		{
			SystemKey:    WorkspaceDefaultRoleTemplateBasicKey,
			NameZhHans:   "基础成员",
			NameEnUS:     "Basic Member",
			DescZhHans:   "可创建和编辑智能体与工作流，并查看工作空间内基础资产，适用于新人和常规开发成员。",
			DescEnUS:     "Can create and edit agents or workflows while viewing supporting workspace assets.",
			Permissions:  basicWorkspaceMemberPermissionStrings(),
			DisplayOrder: 20,
		},
		{
			SystemKey:    WorkspaceDefaultRoleTemplateReadonlyKey,
			NameZhHans:   "只读成员",
			NameEnUS:     "Read-only Member",
			DescZhHans:   "可查看资产并使用或调试已有智能体，不能创建、编辑或删除资产。",
			DescEnUS:     "Can view assets and use or debug existing agents without changing workspace assets.",
			Permissions:  readonlyWorkspaceMemberPermissionStrings(),
			DisplayOrder: 30,
		},
	}
}

func DefaultWorkspaceRoleTemplateDefinition(systemKey string) (WorkspaceDefaultRoleTemplateDefinition, bool) {
	for _, definition := range DefaultWorkspaceRoleTemplateDefinitions() {
		if definition.SystemKey == systemKey {
			return definition, true
		}
	}
	return WorkspaceDefaultRoleTemplateDefinition{}, false
}

func advancedWorkspaceMemberPermissionStrings() []string {
	codes := AllWorkspacePermissionCodes()
	permissions := make([]string, 0, len(codes))
	for _, code := range codes {
		if IsWorkspaceGovernancePermission(code) {
			continue
		}
		permissions = append(permissions, string(code))
	}
	return CanonicalAssignableWorkspacePermissionSnapshotStrings(permissions)
}

func basicWorkspaceMemberPermissionStrings() []string {
	return CanonicalAssignableWorkspacePermissionSnapshotStrings([]string{
		string(WorkspacePermissionAgentView),
		string(WorkspacePermissionAgentCreate),
		string(WorkspacePermissionAgentUpdate),
		string(WorkspacePermissionAgentLogsView),
		string(WorkspacePermissionWorkflowView),
		string(WorkspacePermissionWorkflowCreate),
		string(WorkspacePermissionWorkflowUpdate),
		string(WorkspacePermissionWorkflowImport),
		string(WorkspacePermissionWorkflowRunDraft),
		string(WorkspacePermissionWorkflowLogsView),
		string(WorkspacePermissionKnowledgeBaseView),
		string(WorkspacePermissionKnowledgeBaseRetrievalTest),
		string(WorkspacePermissionKnowledgeBaseDocumentView),
		string(WorkspacePermissionKnowledgeBaseGraphView),
		string(WorkspacePermissionDatabaseView),
		string(WorkspacePermissionDatabaseSchemaView),
		string(WorkspacePermissionDatabaseRecordView),
		string(WorkspacePermissionDatabaseAIQuery),
		string(WorkspacePermissionDatabaseAIQueryRead),
		string(WorkspacePermissionFileView),
		string(WorkspacePermissionFileUploadCreate),
		string(WorkspacePermissionFilePreview),
		string(WorkspacePermissionFileUpload),
		string(WorkspacePermissionFileTextCreate),
	})
}

func readonlyWorkspaceMemberPermissionStrings() []string {
	return CanonicalAssignableWorkspacePermissionSnapshotStrings([]string{
		string(WorkspacePermissionAgentView),
		string(WorkspacePermissionAgentLogsView),
		string(WorkspacePermissionWorkflowView),
		string(WorkspacePermissionWorkflowRunDraft),
		string(WorkspacePermissionWorkflowLogsView),
		string(WorkspacePermissionKnowledgeBaseView),
		string(WorkspacePermissionKnowledgeBaseRetrievalTest),
		string(WorkspacePermissionKnowledgeBaseDocumentView),
		string(WorkspacePermissionKnowledgeBaseGraphView),
		string(WorkspacePermissionDatabaseView),
		string(WorkspacePermissionDatabaseSchemaView),
		string(WorkspacePermissionDatabaseRecordView),
		string(WorkspacePermissionDatabaseAIQuery),
		string(WorkspacePermissionDatabaseAIQueryRead),
		string(WorkspacePermissionFileView),
		string(WorkspacePermissionFilePreview),
	})
}

type WorkspaceCustomRole struct {
	ID              string                      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID  string                      `gorm:"column:group_id;type:uuid;not null;index" json:"organization_id"`
	Name            string                      `gorm:"type:varchar(255);not null" json:"name"`
	NameI18n        map[string]string           `gorm:"column:name_i18n;type:jsonb;serializer:json;not null;default:'{}'" json:"name_i18n,omitempty"`
	Description     *string                     `gorm:"type:text" json:"description,omitempty"`
	DescriptionI18n map[string]string           `gorm:"column:description_i18n;type:jsonb;serializer:json;not null;default:'{}'" json:"description_i18n,omitempty"`
	Status          WorkspaceCustomRoleStatus   `gorm:"type:varchar(16);not null;default:'active'" json:"status"`
	Permissions     []string                    `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"permissions"`
	SystemKey       *string                     `gorm:"column:system_key;type:varchar(64)" json:"system_key,omitempty"`
	TemplateOrigin  WorkspaceRoleTemplateOrigin `gorm:"column:template_origin;type:varchar(32);not null;default:'custom'" json:"template_origin"`
	CreatedBy       string                      `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt       time.Time                   `json:"created_at"`
	UpdatedAt       time.Time                   `json:"updated_at"`
}

func (WorkspaceCustomRole) TableName() string {
	return "roles"
}

func (r *WorkspaceCustomRole) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	normalizeWorkspaceCustomRoleMetadata(r)
	now := time.Now()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = now
	}
	return nil
}

func (r *WorkspaceCustomRole) BeforeUpdate(tx *gorm.DB) error {
	normalizeWorkspaceCustomRoleMetadata(r)
	r.UpdatedAt = time.Now()
	return nil
}

func normalizeWorkspaceCustomRoleMetadata(role *WorkspaceCustomRole) {
	if role == nil {
		return
	}
	if role.NameI18n == nil {
		role.NameI18n = map[string]string{}
	}
	if role.DescriptionI18n == nil {
		role.DescriptionI18n = map[string]string{}
	}
	if role.TemplateOrigin == "" {
		role.TemplateOrigin = WorkspaceRoleTemplateOriginCustom
	}
	if role.SystemKey != nil {
		systemKey := strings.TrimSpace(*role.SystemKey)
		if systemKey == "" {
			role.SystemKey = nil
		} else if systemKey != *role.SystemKey {
			role.SystemKey = &systemKey
		}
	}
	role.Permissions = CanonicalAssignableWorkspacePermissionSnapshotStrings(role.Permissions)
}

// WorkspaceCustomRolePermission removed as permissions are now in roles table

// OrganizationMemberStatus enterprise group account status enum
type OrganizationMemberStatus string

const (
	OrganizationMemberStatusActive   OrganizationMemberStatus = "active"
	OrganizationMemberStatusInactive OrganizationMemberStatus = "inactive"
)

// OrganizationMember enterprise group account association
type OrganizationMember struct {
	OrganizationID string                   `gorm:"type:varchar(255);not null;primaryKey;index" json:"organization_id"`
	AccountID      string                   `gorm:"type:varchar(255);not null;primaryKey;index" json:"account_id"`
	Role           OrganizationRole         `gorm:"type:varchar(16);not null" json:"role"`
	Name           *string                  `gorm:"type:varchar(255)" json:"name"`
	Status         OrganizationMemberStatus `gorm:"type:varchar(16);not null;default:'active'" json:"status"`
	CreatedAt      time.Time                `json:"created_at"`
	UpdatedAt      time.Time                `json:"updated_at"`

	// Relationships - commented out for modular architecture
	// Group   Organization `gorm:"foreignKey:GroupID" json:"-"`
	// Account Account      `gorm:"foreignKey:AccountID" json:"-"`
}

// TableName specifies table name
func (OrganizationMember) TableName() string {
	return "members"
}

// IsAdmin checks if it's an admin role
func (om *OrganizationMember) IsAdmin() bool {
	return om.Role == OrganizationRoleAdmin
}

// BeforeCreate hook to set timestamps
func (om *OrganizationMember) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	if om.CreatedAt.IsZero() {
		om.CreatedAt = now
	}
	if om.UpdatedAt.IsZero() {
		om.UpdatedAt = now
	}
	return nil
}

// OrganizationInviteLink organization invite link model
type OrganizationInviteLink struct {
	ID             string `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID string `gorm:"column:group_id;type:uuid;not null;index" json:"organization_id"`

	DepartmentID *string `gorm:"type:uuid" json:"department_id,omitempty"`
	WorkspaceID  *string `gorm:"column:tenant_id;type:uuid" json:"workspace_id,omitempty"`

	Token string `gorm:"type:varchar(255);not null;uniqueIndex" json:"token"`

	Status string `gorm:"type:varchar(32);not null" json:"status"`

	RequireApproval         bool   `gorm:"not null;default:true" json:"require_approval"`
	DefaultOrganizationRole string `gorm:"column:default_group_role;type:varchar(32);not null;default:'normal'" json:"default_organization_role"`
	DefaultWorkspaceRole    string `gorm:"column:default_tenant_role;type:varchar(32);not null;default:'normal'" json:"default_workspace_role"`

	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	CreatedBy string    `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName specifies table name
func (OrganizationInviteLink) TableName() string {
	return "organization_invite_links"
}

// BeforeCreate hook to set ID and timestamps
func (e *OrganizationInviteLink) BeforeCreate(tx *gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	now := time.Now()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	if e.UpdatedAt.IsZero() {
		e.UpdatedAt = now
	}
	return nil
}

// OrganizationJoinRequestStatus join request status enum
type OrganizationJoinRequestStatus string

const (
	OrganizationJoinRequestStatusPending  OrganizationJoinRequestStatus = "pending"
	OrganizationJoinRequestStatusApproved OrganizationJoinRequestStatus = "approved"
	OrganizationJoinRequestStatusRejected OrganizationJoinRequestStatus = "rejected"
	OrganizationJoinRequestStatusExpired  OrganizationJoinRequestStatus = "expired"
)

// OrganizationJoinRequest organization join request model
type OrganizationJoinRequest struct {
	ID string `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`

	OrganizationID string  `gorm:"column:group_id;type:uuid;not null;index" json:"organization_id"`
	InviteLinkID   *string `gorm:"type:uuid" json:"invite_link_id,omitempty"`
	AccountID      string  `gorm:"type:uuid;not null;index" json:"account_id"`

	DepartmentID *string `gorm:"type:uuid" json:"department_id,omitempty"`
	WorkspaceID  *string `gorm:"column:tenant_id;type:uuid" json:"workspace_id,omitempty"`

	DefaultOrganizationRole string `gorm:"column:default_group_role;type:varchar(32);not null" json:"default_organization_role"`
	DefaultWorkspaceRole    string `gorm:"column:default_tenant_role;type:varchar(32);not null" json:"default_workspace_role"`

	Name       *string                       `gorm:"type:varchar(255)" json:"name"`
	Status     OrganizationJoinRequestStatus `gorm:"type:varchar(32);not null" json:"status"`
	Reason     *string                       `gorm:"type:text" json:"reason,omitempty"`
	ReviewerID *string                       `gorm:"type:uuid" json:"reviewer_id,omitempty"`

	CreatedAt  time.Time  `json:"created_at"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
}

// TableName specifies table name
func (OrganizationJoinRequest) TableName() string {
	return "organization_join_requests"
}

// BeforeCreate hook to set ID and timestamps
func (e *OrganizationJoinRequest) BeforeCreate(tx *gorm.DB) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	return nil
}
