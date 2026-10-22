package digitalhuman

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"gorm.io/gorm"
)

// Instance memory (ADR 0006) keeps the DigitalHuman's working memory of one
// end-user relationship in a business scope of type instance. Capacity and
// expiry are optional and off unless their environment settings are given.

const memoryAdmissionReasonInstanceFull = "instance_memory_full"

var errInstanceMemoryFull = fmt.Errorf("%w: %s", ErrMemoryPolicyRejected, memoryAdmissionReasonInstanceFull)

// InstanceMemoryEnabled switches on instance memory for end users' runs.
func InstanceMemoryEnabled() bool {
	return envFlagEnabled("ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_ENABLED")
}

// instanceMemoryMaxEntries bounds the active entries per instance; zero means
// no limit, the default.
func instanceMemoryMaxEntries() int64 {
	value, err := strconv.ParseInt(strings.TrimSpace(os.Getenv("ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_MAX_ENTRIES")), 10, 64)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

// instanceMemoryTTL is how long an entry may go unwritten before it expires;
// zero means entries never expire, the default.
func instanceMemoryTTL() time.Duration {
	value, err := time.ParseDuration(strings.TrimSpace(os.Getenv("ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_TTL")))
	if err != nil || value < 0 {
		return 0
	}
	return value
}

// runInstanceScopeID returns the end-user instance whose memory an external
// end user's run uses, when instance memory is on.
func runInstanceScopeID(run *Run) (uuid.UUID, bool) {
	if run == nil || !InstanceMemoryEnabled() || RunSourceTrust(run) != SourceTrustExternalEndUser ||
		run.EndUserInstanceID == nil || *run.EndUserInstanceID == uuid.Nil {
		return uuid.Nil, false
	}
	return *run.EndUserInstanceID, true
}

// checkInstanceMemoryCapacity rejects a new key in a full instance scope. An
// update to an existing key is always allowed, so the model can make room.
func checkInstanceMemoryCapacity(ctx context.Context, tx *GormRepository, scope Scope, digitalHumanID uuid.UUID, request MemoryProposalRequest) error {
	limit := instanceMemoryMaxEntries()
	if limit == 0 || request.Action != memoryruntime.MutationActionUpsert || request.Scope == nil || request.Scope.Type != memoryruntime.MemoryScopeInstance {
		return nil
	}
	base := tx.db.WithContext(ctx).Model(&DigitalHumanMemory{}).
		Where("organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND memory_class = ? AND scope_type = ? AND scope_id = ? AND status = ?",
			scope.OrganizationID, scope.WorkspaceID, digitalHumanID, memoryruntime.MemoryClassBusiness, memoryruntime.MemoryScopeInstance, request.Scope.ID, MemoryStatusActive)
	var existing int64
	if err := base.Session(&gorm.Session{}).Where("LOWER(memory_key) = ?", memoryIdentity(request.Key)).Count(&existing).Error; err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}
	var count int64
	if err := base.Session(&gorm.Session{}).Count(&count).Error; err != nil {
		return err
	}
	if count >= limit {
		return errInstanceMemoryFull
	}
	return nil
}

// ArchiveExpiredInstanceMemories archives instance memory entries that have
// not been written within the configured expiry. It is a no-op without one.
func (s *Service) ArchiveExpiredInstanceMemories(ctx context.Context) (int64, error) {
	ttl := instanceMemoryTTL()
	if s == nil || s.repo == nil || ttl == 0 {
		return 0, nil
	}
	now := s.now()
	result := s.repo.db.WithContext(ctx).Model(&DigitalHumanMemory{}).
		Where("memory_class = ? AND scope_type = ? AND status = ? AND updated_at < ?", memoryruntime.MemoryClassBusiness, memoryruntime.MemoryScopeInstance, MemoryStatusActive, now.Add(-ttl)).
		Updates(map[string]any{"status": MemoryStatusArchived, "updated_at": now})
	return result.RowsAffected, result.Error
}
