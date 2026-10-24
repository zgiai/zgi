package artifacts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// A share is a link to one version of an artifact that anyone holding it may
// open, without signing in, until it expires or is revoked. Only a hash of
// its token is kept, so a link is shown once, when it is made.

// DefaultShareLifetime is how long a share lasts when no lifetime is chosen.
const DefaultShareLifetime = "7d"

// shareLifetimes are the lifetimes a share may have; zero never expires.
var shareLifetimes = map[string]time.Duration{
	"1d": 24 * time.Hour, "7d": 7 * 24 * time.Hour, "30d": 30 * 24 * time.Hour, "never": 0,
}

// Share is a link to one version of an artifact.
type Share struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	ArtifactID     uuid.UUID  `gorm:"type:uuid;not null" json:"artifact_id"`
	OrganizationID uuid.UUID  `gorm:"type:uuid;not null" json:"-"`
	VersionNumber  int        `gorm:"column:version_number;not null" json:"version"`
	TokenHash      string     `gorm:"type:varchar(64);not null" json:"-"`
	CreatedByKind  string     `gorm:"type:varchar(32);not null" json:"-"`
	CreatedByID    string     `gorm:"type:varchar(64);not null" json:"created_by_id"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	RevokedAt      *time.Time `json:"revoked_at,omitempty"`
	AccessCount    int64      `gorm:"not null" json:"access_count"`
	LastAccessedAt *time.Time `json:"last_accessed_at,omitempty"`
	CreatedAt      time.Time  `gorm:"not null" json:"created_at"`
}

func (Share) TableName() string { return "artifact_shares" }

// Active reports whether the share still opens.
func (s *Share) Active(now time.Time) bool {
	return s.RevokedAt == nil && (s.ExpiresAt == nil || s.ExpiresAt.After(now))
}

// NewShare is a share as it is made: the only time its token is known.
type NewShare struct {
	*Share
	Token string `json:"token"`
}

// Shared is what a share opens: the artifact and the shared version.
type Shared struct {
	Artifact *Artifact
	Version  *Version
	Share    *Share
}

// canShare reports whether reader may share artifact outside the
// application: its owner, or a member for a team artifact. Reading another
// person's private artifact as an operator does not let anyone share it.
func canShare(reader Reader, artifact *Artifact) bool {
	own := artifact.OwnerKind == reader.Principal.Kind && artifact.OwnerID == reader.Principal.ID
	return own || (reader.Member && artifact.Visibility == VisibilityTeam)
}

// CreateShare makes a link to a version of an artifact; number 0 is the
// current one, and lifetime is one of 1d, 7d, 30d, or never.
func (s *Service) CreateShare(ctx context.Context, reader Reader, id uuid.UUID, number int, lifetime string, actor Actor) (*NewShare, error) {
	artifact, err := s.load(ctx, reader, id)
	if err != nil {
		return nil, err
	}
	if !canShare(reader, artifact) || artifact.ArchivedAt != nil {
		return nil, ErrNotFound
	}
	lifetime = strings.TrimSpace(lifetime)
	if lifetime == "" {
		lifetime = DefaultShareLifetime
	}
	duration, ok := shareLifetimes[lifetime]
	if !ok || strings.TrimSpace(actor.Kind) == "" || strings.TrimSpace(actor.ID) == "" {
		return nil, ErrInvalidRequest
	}
	if number <= 0 {
		number = artifact.CurrentVersion
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&Version{}).Where("artifact_id = ? AND number = ?", artifact.ID, number).Count(&count).Error; err != nil {
		return nil, fmt.Errorf("check shared artifact version: %w", err)
	}
	if count == 0 {
		return nil, ErrNotFound
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("make share token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	now := s.now()
	share := &Share{ID: uuid.New(), ArtifactID: artifact.ID, OrganizationID: artifact.OrganizationID, VersionNumber: number,
		TokenHash: shareTokenHash(token), CreatedByKind: actor.Kind, CreatedByID: actor.ID, CreatedAt: now}
	if duration > 0 {
		expires := now.Add(duration)
		share.ExpiresAt = &expires
	}
	if err := s.db.WithContext(ctx).Create(share).Error; err != nil {
		return nil, fmt.Errorf("create artifact share: %w", err)
	}
	return &NewShare{Share: share, Token: token}, nil
}

// Shares lists an artifact's shares, newest first, for a reader who may
// share it.
func (s *Service) Shares(ctx context.Context, reader Reader, id uuid.UUID) ([]*Share, error) {
	artifact, err := s.load(ctx, reader, id)
	if err != nil {
		return nil, err
	}
	if !canShare(reader, artifact) {
		return []*Share{}, nil
	}
	shares := []*Share{}
	if err := s.db.WithContext(ctx).Where("artifact_id = ?", artifact.ID).Order("created_at DESC").Find(&shares).Error; err != nil {
		return nil, fmt.Errorf("list artifact shares: %w", err)
	}
	return shares, nil
}

// RevokeShare stops a share from opening.
func (s *Service) RevokeShare(ctx context.Context, reader Reader, id, shareID uuid.UUID) error {
	artifact, err := s.load(ctx, reader, id)
	if err != nil {
		return err
	}
	if !canShare(reader, artifact) {
		return ErrNotFound
	}
	result := s.db.WithContext(ctx).Model(&Share{}).Where("id = ? AND artifact_id = ? AND revoked_at IS NULL", shareID, artifact.ID).
		UpdateColumn("revoked_at", s.now())
	if result.Error != nil {
		return fmt.Errorf("revoke artifact share: %w", result.Error)
	}
	return nil
}

// OpenShare is what a share token opens, or ErrNotFound once it is
// revoked, expired, or its artifact archived. Counting marks each open.
func (s *Service) OpenShare(ctx context.Context, token string, count bool) (*Shared, error) {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 128 {
		return nil, ErrNotFound
	}
	var share Share
	if err := s.db.WithContext(ctx).Where("token_hash = ?", shareTokenHash(token)).Take(&share).Error; err != nil {
		return nil, notFound(err)
	}
	now := s.now()
	if !share.Active(now) {
		return nil, ErrNotFound
	}
	var artifact Artifact
	if err := s.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL AND archived_at IS NULL", share.ArtifactID).Take(&artifact).Error; err != nil {
		return nil, notFound(err)
	}
	var version Version
	if err := s.db.WithContext(ctx).Where("artifact_id = ? AND number = ?", artifact.ID, share.VersionNumber).Take(&version).Error; err != nil {
		return nil, notFound(err)
	}
	if count {
		if err := s.db.WithContext(ctx).Model(&Share{}).Where("id = ?", share.ID).
			Updates(map[string]any{"access_count": gorm.Expr("access_count + 1"), "last_accessed_at": now}).Error; err != nil {
			return nil, fmt.Errorf("count artifact share: %w", err)
		}
	}
	return &Shared{Artifact: &artifact, Version: &version, Share: &share}, nil
}

// SharedContent is the bytes of a share's version.
func (s *Service) SharedContent(ctx context.Context, shared *Shared) ([]byte, error) {
	if shared == nil || shared.Version == nil {
		return nil, ErrNotFound
	}
	if shared.Version.Content != nil {
		return []byte(*shared.Version.Content), nil
	}
	if shared.Version.FileID == nil {
		return nil, ErrNotFound
	}
	data, err := s.blobs.Read(ctx, shared.Artifact.OrganizationID, *shared.Version.FileID)
	if err != nil {
		return nil, fmt.Errorf("read shared artifact file: %w", err)
	}
	return data, nil
}

func shareTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
