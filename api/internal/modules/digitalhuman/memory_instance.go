package digitalhuman

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
)

// Instance memory (ADR 0006) is the DigitalHuman's notes on one external end
// user, kept in that person's relationship memory under notes/ (ADR 0015).
// Capacity and expiry are optional and off unless their environment settings
// are given.

const memoryAdmissionReasonInstanceFull = "instance_memory_full"

var errInstanceMemoryFull = fmt.Errorf("%w: %s", ErrMemoryPolicyRejected, memoryAdmissionReasonInstanceFull)

// InstanceMemoryEnabled switches on instance memory for end users' runs. It
// is on unless set to false.
func InstanceMemoryEnabled() bool {
	return envFlagDefaultOn("ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_ENABLED")
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

// ArchiveExpiredInstanceMemories archives instance memory entries that have
// not been written within the configured expiry. It is a no-op without one.
func (s *Service) ArchiveExpiredInstanceMemories(ctx context.Context) (int64, error) {
	ttl := instanceMemoryTTL()
	if s == nil || s.repo == nil || ttl == 0 {
		return 0, nil
	}
	// Notes now live in relationship memory (ADR 0015); an expired one moves
	// into the person's archive, where it can still be found.
	moved, err := relationshipmemory.NewService(s.repo.db).ExpireNotes(ctx, s.now().Add(-ttl), noteExpiryBatch)
	return int64(moved), err
}
