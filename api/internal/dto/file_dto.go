package dto

import (
	"time"
)

// FileListRequest represents request for file list
type FileListRequest struct {
	Page        int       `form:"page" binding:"omitempty,min=1"`
	Limit       int       `form:"limit" binding:"omitempty,min=1,max=100"`
	Keyword     string    `form:"keyword"`
	Sort        string    `form:"sort"`
	StartTime   time.Time `form:"start_time"`
	EndTime     time.Time `form:"end_time"`
	Extension   string    `form:"extension"` // File extension filter
	WorkspaceID string    `form:"workspace_id"`
}

// FileListResponse represents response for file list
type FileListResponse struct {
	Data    []UploadFile `json:"data"`
	HasMore bool         `json:"has_more"`
	Limit   int          `json:"limit"`
	Total   int64        `json:"total"`
	Page    int          `json:"page"`
}

type FileMetadataListResponse struct {
	Data []UploadFile `json:"data"`
}

// FileFolderCreateRequest represents request for creating file folder
type FileFolderCreateRequest struct {
	Name                 string   `json:"name" binding:"required"`
	TenantID             string   `json:"tenant_id"`
	TeamTenantID         *string  `json:"team_tenant_id"`
	WorkspaceID          *string  `json:"workspace_id"`
	Description          *string  `json:"description"`
	ParentID             *string  `json:"parent_id"`
	Icon                 *string  `json:"icon"`
	IconType             *string  `json:"icon_type"`
	IconBackground       *string  `json:"icon_background"`
	Position             *int     `json:"position"`
	Permission           *string  `json:"permission"`
	PartialWorkspaceList []string `json:"partial_workspace_list"`
}

// FileFolderUpdateRequest represents request for updating file folder
type FileFolderUpdateRequest struct {
	Name                 *string  `json:"name"`
	Description          *string  `json:"description"`
	ParentID             *string  `json:"parent_id"`
	Icon                 *string  `json:"icon"`
	IconType             *string  `json:"icon_type"`
	IconBackground       *string  `json:"icon_background"`
	Position             *int     `json:"position"`
	Permission           *string  `json:"permission"`
	PartialWorkspaceList []string `json:"partial_workspace_list"` // Workspace IDs for partial_workspace permission
}

// FileFolderListRequest represents request for file folder list
type FileFolderListRequest struct {
	Page        int    `form:"page" binding:"omitempty,min=1"`
	Limit       int    `form:"limit" binding:"omitempty,min=1,max=100"`
	Keyword     string `form:"keyword"`
	Sort        string `form:"sort"`
	ParentID    string `form:"parent_id"`
	WorkspaceID string `form:"workspace_id"`
}

// FileListInFolderRequest represents request for listing files in a folder
type FileListInFolderRequest struct {
	Page        int       `form:"page" binding:"omitempty,min=1"`
	Limit       int       `form:"limit" binding:"omitempty,min=1,max=100"`
	Keyword     string    `form:"keyword"`
	Sort        string    `form:"sort"`
	StartTime   time.Time `form:"start_time"`
	EndTime     time.Time `form:"end_time"`
	Extension   string    `form:"extension"` // File extension filter
	FolderID    string    `form:"folder_id"` // Folder ID, empty or not provided means root folder
	WorkspaceID string    `form:"workspace_id"`
}

// FileFolderResponse represents response for file folder
type FileFolderResponse struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	OrganizationID  string    `json:"organization_id"`
	TeamTenantID    *string   `json:"team_tenant_id,omitempty"`
	WorkspaceID     *string   `json:"workspace_id,omitempty"`
	Name            string    `json:"name"`
	Description     *string   `json:"description"`
	ParentID        *string   `json:"parent_id"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedBy       *string   `json:"updated_by"`
	UpdatedAt       time.Time `json:"updated_at"`
	IconType        *string   `json:"icon_type"`
	Icon            *string   `json:"icon"`
	IconBackground  *string   `json:"icon_background"`
	Position        int       `json:"position"`
	Permission      string    `json:"permission"`
	FileCount       int64     `json:"file_count"`
	PartialTeamList []string  `json:"partial_team_list,omitempty"`
}

// FileFolderListResponse represents response for file folder list
type FileFolderListResponse struct {
	Data    []FileFolderResponse `json:"data"`
	HasMore bool                 `json:"has_more"`
	Limit   int                  `json:"limit"`
	Total   int64                `json:"total"`
	Page    int                  `json:"page"`
}

// MoveFilesToFolderRequest represents request for moving multiple files to folder
type MoveFilesToFolderRequest struct {
	FileIDs  []string `json:"file_ids" binding:"required"`
	FolderID string   `json:"folder_id" binding:"omitempty"`
}

// ArchiveFilesRequest represents request for archiving/unarchiving multiple files
type ArchiveFilesRequest struct {
	FileIDs []string `json:"file_ids" binding:"required"`
}

// MoveFolderToFolderRequest represents request for moving a folder to another folder
type MoveFolderToFolderRequest struct {
	FolderID string `json:"folder_id" binding:"required"`  // The folder to move
	TargetID string `json:"target_id" binding:"omitempty"` // The target folder (empty means root)
}

type FileUploadRequest struct {
	FolderID *string `form:"folder_id"` // Target folder ID for the uploaded file
	Source   string  `form:"source"`    // File source: datasets or empty
}

type CreateTextFileRequest struct {
	Filename     string  `json:"filename" binding:"required"`
	Content      string  `json:"content" binding:"required"`
	FolderID     *string `json:"folder_id"` // Target folder ID for the uploaded file
	Source       string  `json:"source"`    // File source: datasets or empty
	TeamTenantID *string `json:"team_tenant_id"`
	WorkspaceID  *string `json:"workspace_id"`
}

type FileUploadResponse struct {
	ID                  string    `json:"id"`
	Name                string    `json:"name"`
	Size                int64     `json:"size"`
	Extension           string    `json:"extension"`
	MimeType            string    `json:"mime_type"`
	CreatedBy           string    `json:"created_by"`
	CreatedAt           time.Time `json:"created_at"`
	Hash                string    `json:"hash,omitempty"`
	SourceURL           string    `json:"source_url,omitempty"`
	TeamTenantID        *string   `json:"team_tenant_id,omitempty"`
	WorkspaceID         *string   `json:"workspace_id,omitempty"`
	AssetID             string    `json:"asset_id,omitempty"`
	ProcessingMode      string    `json:"processing_mode,omitempty"`
	ProcessingStatus    string    `json:"processing_status,omitempty"`
	ProcessingRequestID string    `json:"processing_request_id,omitempty"`
	ProcessingRunID     string    `json:"processing_run_id,omitempty"`
	GenerationNo        int64     `json:"generation_no,omitempty"`
}

type FileUploadConfigResponse struct {
	FileSizeLimit           int64 `json:"file_size_limit"`
	BatchCountLimit         int   `json:"batch_count_limit"`
	ImageFileSizeLimit      int64 `json:"image_file_size_limit"`
	VideoFileSizeLimit      int64 `json:"video_file_size_limit"`
	AudioFileSizeLimit      int64 `json:"audio_file_size_limit"`
	WorkflowFileUploadLimit int   `json:"workflow_file_upload_limit"`
}

type FilePreviewResponse struct {
	Content string `json:"content"`
}

type FileOriginalPreviewURLResponse struct {
	URL       string `json:"url"`
	FileID    string `json:"file_id"`
	Name      string `json:"name"`
	Extension string `json:"extension"`
	MimeType  string `json:"mime_type"`
}

type FileSupportTypeResponse struct {
	AllowedExtensions []string `json:"allowed_extensions"`
}

type FileParseResponse struct {
	UploadFileID string `json:"upload_file_id"`
	Status       string `json:"status"`
	Content      string `json:"content,omitempty"`
	Message      string `json:"message,omitempty"`
}

// StorageUsageResponse represents response for storage usage
type StorageUsageResponse struct {
	Used  float64 `json:"used"`  // Used storage in specified unit
	Total float64 `json:"total"` // Total storage quota in specified unit
	Unit  string  `json:"unit"`  // Unit for display (e.g., "GB")
}

func NewFileUploadResponse(uploadFile *UploadFile) *FileUploadResponse {
	return &FileUploadResponse{
		ID:           uploadFile.ID,
		Name:         uploadFile.Name,
		Size:         uploadFile.Size,
		Extension:    uploadFile.Extension,
		MimeType:     uploadFile.MimeType,
		CreatedBy:    uploadFile.CreatedBy,
		CreatedAt:    uploadFile.CreatedAt,
		Hash:         uploadFile.Hash,
		SourceURL:    uploadFile.SourceURL,
		TeamTenantID: uploadFile.TeamTenantID,
		WorkspaceID:  uploadFile.WorkspaceID,
	}
}

// CreatedByRole creator role enum
type CreatedByRole string

const (
	CreatedByRoleAccount CreatedByRole = "account"
	CreatedByRoleEndUser CreatedByRole = "end_user"
)

// UploadFile upload file model
type UploadFile struct {
	ID                  string        `json:"id"`
	TenantID            string        `json:"tenant_id"`
	OrganizationID      string        `json:"organization_id"`
	TeamTenantID        *string       `json:"team_tenant_id,omitempty"`
	WorkspaceID         *string       `json:"workspace_id,omitempty"`
	StorageType         string        `json:"storage_type"`
	Key                 string        `json:"key"`
	Name                string        `json:"name"`
	Size                int64         `json:"size"`
	Extension           string        `json:"extension"`
	MimeType            string        `json:"mime_type"`
	CreatedByRole       CreatedByRole `json:"created_by_role"`
	CreatedBy           string        `json:"created_by"`
	CreatedAt           time.Time     `json:"created_at"`
	Used                bool          `json:"used"`
	UsedBy              *string       `json:"used_by"`
	UsedAt              *time.Time    `json:"used_at"`
	Hash                string        `json:"hash"`
	SourceURL           string        `json:"source_url"`
	ContentText         *string       `json:"content_text"`
	IsTemporary         bool          `json:"is_temporary"`
	RelatedDatasetCount int           `json:"related_dataset_count"`
	RelatedCount        int           `json:"related_count"`
	AssetID             string        `json:"asset_id,omitempty"`
	ProcessingStatus    string        `json:"processing_status,omitempty"`
	ProcessingStage     string        `json:"processing_stage,omitempty"`
	ProcessingProgress  int           `json:"processing_progress,omitempty"`
	ProcessingRequestID string        `json:"processing_request_id,omitempty"`
	ProcessingRunID     string        `json:"processing_run_id,omitempty"`
	GenerationNo        int64         `json:"generation_no,omitempty"`
	PendingConfirmCount int64         `json:"pending_confirmation_count,omitempty"`
	ChunkCount          int64         `json:"chunk_count,omitempty"`
	EmbeddingCount      int64         `json:"embedding_count,omitempty"`
	VectorStatus        string        `json:"vector_status,omitempty"`
	LastErrorCode       string        `json:"last_error_code,omitempty"`
	LastErrorMessage    string        `json:"last_error_message,omitempty"`

	// Favorite field
	IsFavorite bool `json:"is_favorite"`

	// Archive fields
	IsArchived bool       `json:"is_archived"`
	ArchivedAt *time.Time `json:"archived_at"`
	ArchivedBy *string    `json:"archived_by"`
}

type ChildDocument struct {
	PageContent string                 `json:"page_content"`
	Vector      []float64              `json:"vector,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type Document struct {
	PageContent string                 `json:"page_content"`
	Vector      []float64              `json:"vector,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Provider    string                 `json:"provider,omitempty"`
	Children    []ChildDocument        `json:"children,omitempty"`
}

// FileStatisticsResponse represents response for file statistics
type FileStatisticsResponse struct {
	TotalCount      int64 `json:"total_count"`       // All files count
	RecentCount     int64 `json:"recent_count"`      // Recent files count (within last 3 months)
	FavoriteCount   int64 `json:"favorite_count"`    // Favorite files count
	RootFolderCount int64 `json:"root_folder_count"` // Files in root folder count
	ArchivedCount   int64 `json:"archived_count"`    // Archived files count
}

// FileFolderPermissionTenantResponse represents a tenant in the file folder permission list
type FileFolderPermissionTenantResponse struct {
	WorkspaceID string `json:"workspace_id"`
}

// FileFolderPermissionTenantListResponse represents response for file folder permission tenant list
type FileFolderPermissionTenantListResponse struct {
	Data []string `json:"data"` // List of tenant IDs that have permission to access the folder
}

// FileFolderPermissionTenantDetail represents a tenant with details in the file folder permission list
type FileFolderPermissionTenantDetail struct {
	TenantID      string `json:"tenant_id"`
	TenantName    string `json:"tenant_name"`
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
}

// FileFolderPermissionTenantDetailListResponse represents detailed response for file folder permission tenant list
type FileFolderPermissionTenantDetailListResponse struct {
	Data []FileFolderPermissionTenantDetail `json:"data"` // List of tenants with details that have permission to access the folder
}
