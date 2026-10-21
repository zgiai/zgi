package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrInvalidRequest = errors.New("invalid artifact request")
	ErrNotFound       = errors.New("artifact not found")
)

const (
	// MaxTitleRunes bounds an artifact title.
	MaxTitleRunes = 200
	// MaxDocumentBytes bounds one document version.
	MaxDocumentBytes = 512 << 10
	// MaxFileBytes bounds one file version, matching agent execution files.
	MaxFileBytes = 64 << 20
	defaultPage  = 20
	maxPageSize  = 100
)

// BlobStore keeps the bytes of file versions. A version owns its copy, so
// it outlives the run or conversation file it came from.
type BlobStore interface {
	Store(ctx context.Context, organizationID uuid.UUID, name, mimeType string, data []byte) (string, error)
	Read(ctx context.Context, organizationID uuid.UUID, fileID string) ([]byte, error)
}

// Scope is the application an artifact belongs to.
type Scope struct {
	OrganizationID  uuid.UUID
	WorkspaceID     uuid.UUID
	ApplicationType string
	ApplicationID   uuid.UUID
}

func (s Scope) valid() bool {
	return s.OrganizationID != uuid.Nil && s.WorkspaceID != uuid.Nil && s.ApplicationType != "" && s.ApplicationID != uuid.Nil
}

// Owner is who an artifact belongs to.
type Owner struct {
	Kind OwnerKind
	ID   uuid.UUID
}

func (o Owner) valid() bool {
	switch o.Kind {
	case OwnerAccount, OwnerAPIEndUser, OwnerWorkspace, OwnerApplication:
		return o.ID != uuid.Nil
	default:
		return false
	}
}

// Actor records who published a version: a person, or an agent run.
type Actor struct {
	Kind string
	ID   string
}

// Reader is an authenticated caller, described by the product adapter that
// authorized it. The service decides from it which artifacts it may read.
type Reader struct {
	Scope
	// Principal is the reader's own identity; its artifacts are always readable.
	Principal Owner
	// Member reads the team's artifacts. External end users never do.
	Member bool
	// Operator reads owner-only artifacts of others in the application, such
	// as for support and audit, except end users' while Privacy is on.
	Operator bool
	Privacy  bool
}

// Blob is the content of a file version.
type Blob struct {
	Name     string
	MimeType string
	Data     []byte
}

// PublishRequest creates an artifact or adds a version to one.
type PublishRequest struct {
	Scope
	Owner      Owner
	Visibility Visibility
	Kind       Kind
	Title      string
	Note       string
	// ArtifactID adds a version to that artifact, which must belong to Owner.
	ArtifactID *uuid.UUID
	// Text is a document's Markdown; File is any other kind's content.
	Text string
	File *Blob
	// Source says what produced the version. A task's artifact with the same
	// title gains versions instead of being duplicated.
	Source         SourceKind
	SourceTaskID   *uuid.UUID
	SourceRunID    *uuid.UUID
	SourceReportID *uuid.UUID
	Actor          Actor
}

// Published is the result of a publish. Created is false when the content
// matched the current version, which is then returned unchanged.
type Published struct {
	Artifact *Artifact
	Version  *Version
	Created  bool
}

// Summary is an artifact with its current version.
type Summary struct {
	Artifact
	Latest *Version `json:"latest,omitempty"`
}

// ListQuery filters the artifacts a reader can see.
type ListQuery struct {
	Kind Kind
	// Query matches titles.
	Query string
	// Mine keeps only the reader's own artifacts.
	Mine     bool
	TaskID   *uuid.UUID
	Page     int
	PageSize int
}

// ListResult is a page of artifacts.
type ListResult struct {
	Items    []*Summary `json:"items"`
	Total    int64      `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
}

// Service keeps artifacts and their versions.
type Service struct {
	db    *gorm.DB
	blobs BlobStore
	now   func() time.Time
}

// NewService builds the artifact service.
func NewService(db *gorm.DB, blobs BlobStore) *Service {
	if db == nil || blobs == nil {
		panic("artifact service requires db and blob store")
	}
	return &Service{db: db, blobs: blobs, now: func() time.Time { return time.Now().UTC() }}
}

// Publish stores new content as a version. Content equal to the current
// version is not stored again.
func (s *Service) Publish(ctx context.Context, req PublishRequest) (*Published, error) {
	title := strings.TrimSpace(req.Title)
	if !req.Scope.valid() || !req.Owner.valid() || title == "" || utf8.RuneCountInString(title) > MaxTitleRunes || !validVisibility(req.Owner, req.Visibility) {
		return nil, ErrInvalidRequest
	}
	content, mimeType, name, err := publishContent(req)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(content)
	checksum := hex.EncodeToString(sum[:])

	artifact, err := s.target(ctx, req, title)
	if err != nil {
		return nil, err
	}
	if artifact != nil {
		if artifact.Kind != req.Kind {
			return nil, ErrInvalidRequest
		}
		var current Version
		if err := s.db.WithContext(ctx).Where("artifact_id = ? AND number = ?", artifact.ID, artifact.CurrentVersion).First(&current).Error; err == nil && current.Checksum == checksum {
			return &Published{Artifact: artifact, Version: &current}, nil
		}
	}

	version := &Version{ID: uuid.New(), MimeType: mimeType, FileName: name, Size: int64(len(content)), Checksum: checksum, Note: strings.TrimSpace(req.Note),
		SourceKind: req.Source, SourceTaskID: req.SourceTaskID, SourceRunID: req.SourceRunID, SourceReportID: req.SourceReportID,
		CreatedByKind: req.Actor.Kind, CreatedByID: req.Actor.ID, CreatedAt: s.now()}
	if version.SourceKind == "" {
		version.SourceKind = SourceManual
	}
	if req.Kind == KindDocument {
		text := req.Text
		version.Content = &text
	} else {
		fileID, err := s.blobs.Store(ctx, req.OrganizationID, name, mimeType, content)
		if err != nil {
			return nil, fmt.Errorf("store artifact file: %w", err)
		}
		version.FileID = &fileID
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := s.now()
		if artifact == nil {
			artifact = &Artifact{ID: uuid.New(), OrganizationID: req.OrganizationID, WorkspaceID: req.WorkspaceID, ApplicationType: req.ApplicationType, ApplicationID: req.ApplicationID,
				OwnerKind: req.Owner.Kind, OwnerID: req.Owner.ID, Visibility: req.Visibility, Kind: req.Kind, Title: title, SourceTaskID: req.SourceTaskID,
				CurrentVersion: 1, CreatedByKind: req.Actor.Kind, CreatedByID: req.Actor.ID, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(artifact).Error; err != nil {
				return err
			}
			version.Number = 1
		} else {
			// Advance only from the version this publish was based on, so two
			// concurrent publishes never claim the same number.
			result := tx.Model(&Artifact{}).Where("id = ? AND current_version = ?", artifact.ID, artifact.CurrentVersion).
				Updates(map[string]any{"current_version": artifact.CurrentVersion + 1, "updated_at": now})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrInvalidRequest
			}
			artifact.CurrentVersion++
			artifact.UpdatedAt = now
			version.Number = artifact.CurrentVersion
		}
		version.ArtifactID = artifact.ID
		return tx.Create(version).Error
	})
	if err != nil {
		return nil, err
	}
	return &Published{Artifact: artifact, Version: version, Created: true}, nil
}

// target finds the artifact a publish adds a version to, if any.
func (s *Service) target(ctx context.Context, req PublishRequest, title string) (*Artifact, error) {
	query := s.db.WithContext(ctx).Where("organization_id = ? AND workspace_id = ? AND application_type = ? AND application_id = ? AND owner_kind = ? AND owner_id = ? AND deleted_at IS NULL",
		req.OrganizationID, req.WorkspaceID, req.ApplicationType, req.ApplicationID, req.Owner.Kind, req.Owner.ID)
	var artifact Artifact
	switch {
	case req.ArtifactID != nil:
		if err := query.Where("id = ?", *req.ArtifactID).First(&artifact).Error; err != nil {
			return nil, notFound(err)
		}
	case req.SourceTaskID != nil:
		err := query.Where("source_task_id = ? AND title = ? AND kind = ?", *req.SourceTaskID, title, req.Kind).Order("created_at").First(&artifact).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
	default:
		return nil, nil
	}
	return &artifact, nil
}

// List returns the artifacts reader may see, newest first.
func (s *Service) List(ctx context.Context, reader Reader, query ListQuery) (*ListResult, error) {
	base, err := s.visible(ctx, reader)
	if err != nil {
		return nil, err
	}
	if query.Kind != "" {
		base = base.Where("kind = ?", query.Kind)
	}
	if keyword := strings.TrimSpace(query.Query); keyword != "" {
		base = base.Where("LOWER(title) LIKE LOWER(?)", "%"+keyword+"%")
	}
	if query.Mine {
		base = base.Where("owner_kind = ? AND owner_id = ?", reader.Principal.Kind, reader.Principal.ID)
	}
	if query.TaskID != nil {
		base = base.Where("source_task_id = ?", *query.TaskID)
	}
	page, size := query.Page, query.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 || size > maxPageSize {
		size = defaultPage
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count artifacts: %w", err)
	}
	var rows []*Artifact
	if err := base.Order("updated_at DESC").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list artifacts: %w", err)
	}
	latest, err := s.latestVersions(ctx, rows)
	if err != nil {
		return nil, err
	}
	items := make([]*Summary, 0, len(rows))
	for _, row := range rows {
		items = append(items, &Summary{Artifact: *row, Latest: latest[row.ID]})
	}
	return &ListResult{Items: items, Total: total, Page: page, PageSize: size}, nil
}

// Get returns one artifact reader may see, with its current version.
func (s *Service) Get(ctx context.Context, reader Reader, id uuid.UUID) (*Summary, error) {
	artifact, err := s.load(ctx, reader, id)
	if err != nil {
		return nil, err
	}
	latest, err := s.latestVersions(ctx, []*Artifact{artifact})
	if err != nil {
		return nil, err
	}
	return &Summary{Artifact: *artifact, Latest: latest[artifact.ID]}, nil
}

// Versions lists an artifact's versions, newest first.
func (s *Service) Versions(ctx context.Context, reader Reader, id uuid.UUID) ([]*Version, error) {
	if _, err := s.load(ctx, reader, id); err != nil {
		return nil, err
	}
	var versions []*Version
	if err := s.db.WithContext(ctx).Where("artifact_id = ?", id).Order("number DESC").Find(&versions).Error; err != nil {
		return nil, fmt.Errorf("list artifact versions: %w", err)
	}
	return versions, nil
}

// Content returns a version's content: a document's Markdown, or a file's
// bytes. Number 0 means the current version.
func (s *Service) Content(ctx context.Context, reader Reader, id uuid.UUID, number int) (*Artifact, *Version, []byte, error) {
	artifact, err := s.load(ctx, reader, id)
	if err != nil {
		return nil, nil, nil, err
	}
	if number <= 0 {
		number = artifact.CurrentVersion
	}
	var version Version
	if err := s.db.WithContext(ctx).Where("artifact_id = ? AND number = ?", id, number).First(&version).Error; err != nil {
		return nil, nil, nil, notFound(err)
	}
	if version.Content != nil {
		return artifact, &version, []byte(*version.Content), nil
	}
	if version.FileID == nil {
		return nil, nil, nil, ErrNotFound
	}
	data, err := s.blobs.Read(ctx, artifact.OrganizationID, *version.FileID)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("read artifact file: %w", err)
	}
	return artifact, &version, data, nil
}

func (s *Service) load(ctx context.Context, reader Reader, id uuid.UUID) (*Artifact, error) {
	query, err := s.visible(ctx, reader)
	if err != nil {
		return nil, err
	}
	var artifact Artifact
	if err := query.Where("id = ?", id).First(&artifact).Error; err != nil {
		return nil, notFound(err)
	}
	return &artifact, nil
}

// visible limits a query to the artifacts reader may read. Every read goes
// through it, so an external end user can never reach the team's artifacts
// or another user's.
func (s *Service) visible(ctx context.Context, reader Reader) (*gorm.DB, error) {
	if !reader.Scope.valid() {
		return nil, ErrInvalidRequest
	}
	clause := "(owner_kind = ? AND owner_id = ?) OR visibility = ?"
	args := []any{reader.Principal.Kind, reader.Principal.ID, VisibilityApplication}
	if reader.Member {
		clause += " OR visibility = ?"
		args = append(args, VisibilityTeam)
	}
	if reader.Operator {
		if reader.Privacy {
			clause += " OR (visibility = ? AND owner_kind <> ?)"
			args = append(args, VisibilityOwner, OwnerAPIEndUser)
		} else {
			clause += " OR visibility = ?"
			args = append(args, VisibilityOwner)
		}
	}
	return s.db.WithContext(ctx).Model(&Artifact{}).
		Where("organization_id = ? AND workspace_id = ? AND application_type = ? AND application_id = ? AND deleted_at IS NULL",
			reader.OrganizationID, reader.WorkspaceID, reader.ApplicationType, reader.ApplicationID).
		Where("("+clause+")", args...), nil
}

func (s *Service) latestVersions(ctx context.Context, rows []*Artifact) (map[uuid.UUID]*Version, error) {
	result := make(map[uuid.UUID]*Version, len(rows))
	if len(rows) == 0 {
		return result, nil
	}
	ids := make([]uuid.UUID, 0, len(rows))
	current := make(map[uuid.UUID]int, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
		current[row.ID] = row.CurrentVersion
	}
	var versions []*Version
	if err := s.db.WithContext(ctx).Where("artifact_id IN ?", ids).Find(&versions).Error; err != nil {
		return nil, fmt.Errorf("load artifact versions: %w", err)
	}
	for _, version := range versions {
		if version.Number == current[version.ArtifactID] {
			result[version.ArtifactID] = version
		}
	}
	return result, nil
}

// validVisibility keeps the team tier to workspace members' artifacts: an
// end user's artifact is theirs, or offered to the whole application.
func validVisibility(owner Owner, visibility Visibility) bool {
	switch visibility {
	case VisibilityOwner, VisibilityApplication:
		return true
	case VisibilityTeam:
		return owner.Kind != OwnerAPIEndUser
	default:
		return false
	}
}

func publishContent(req PublishRequest) ([]byte, string, string, error) {
	switch req.Kind {
	case KindDocument:
		if req.File != nil || strings.TrimSpace(req.Text) == "" || len(req.Text) > MaxDocumentBytes {
			return nil, "", "", ErrInvalidRequest
		}
		return []byte(req.Text), "text/markdown", "", nil
	case KindFile, KindImage, KindTable:
		if req.File == nil || len(req.File.Data) == 0 || len(req.File.Data) > MaxFileBytes || strings.TrimSpace(req.File.Name) == "" || req.Text != "" {
			return nil, "", "", ErrInvalidRequest
		}
		mimeType := strings.TrimSpace(req.File.MimeType)
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		if req.Kind == KindImage && !strings.HasPrefix(mimeType, "image/") {
			return nil, "", "", ErrInvalidRequest
		}
		return req.File.Data, mimeType, strings.TrimSpace(req.File.Name), nil
	default:
		return nil, "", "", ErrInvalidRequest
	}
}

func notFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}
