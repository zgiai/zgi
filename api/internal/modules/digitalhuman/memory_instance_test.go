package digitalhuman

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"gorm.io/datatypes"
)

func instanceRun(t *testing.T) (*Run, uuid.UUID) {
	t.Helper()
	run := trustedRun(SourceTrustExternalEndUser, SurfaceAPI)
	instanceID := uuid.New()
	run.EndUserInstanceID = &instanceID
	return run, instanceID
}

func TestInstanceMemoryAdmissionWritesOnlyTheOwnInstance(t *testing.T) {
	api := invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginAPI}}
	run, instanceID := instanceRun(t)
	t.Setenv("ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_ENABLED", "false")
	if got := admitMemoryWrite(run, api, memoryWriteExplicit, true); got.Allowed {
		t.Fatalf("instance memory was admitted while it is off: %+v", got)
	}

	t.Setenv("ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_ENABLED", "true")
	explicit := admitMemoryWrite(run, api, memoryWriteExplicit, false)
	if !explicit.Allowed || explicit.Class != memoryruntime.MemoryClassBusiness || explicit.Scope == nil ||
		explicit.Scope.Type != memoryruntime.MemoryScopeInstance || explicit.Scope.ID != instanceID.String() || explicit.Origin != MemoryOriginEndUser {
		t.Fatalf("explicit instance admission = %+v", explicit)
	}
	if got := admitMemoryWrite(run, api, memoryWriteProactive, false); got.Allowed {
		t.Fatal("proactive instance memory was admitted without automatic extraction")
	}
	if got := admitMemoryWrite(run, api, memoryWriteBackground, true); !got.Allowed || got.Scope.ID != instanceID.String() {
		t.Fatalf("background instance admission = %+v", got)
	}
	if !memoryToolAvailable(run, false) || !backgroundExtractionAdmitted(run) {
		t.Fatal("an instance run is not offered its memory")
	}

	anonymous := trustedRun(SourceTrustExternalEndUser, SurfaceWebApp)
	if got := admitMemoryWrite(anonymous, api, memoryWriteExplicit, true); got.Allowed || memoryToolAvailable(anonymous, true) || backgroundExtractionAdmitted(anonymous) {
		t.Fatalf("a run without an instance was admitted: %+v", got)
	}
	member := trustedRun(SourceTrustWorkspaceMember, SurfaceConsole)
	member.EndUserInstanceID = run.EndUserInstanceID
	if got := admitMemoryWrite(member, api, memoryWriteExplicit, true); got.Scope != nil || !got.InternalOnly {
		t.Fatalf("a member run wrote instance memory: %+v", got)
	}
}

func TestInstanceMemoryRecallIsLimitedToTheOwnInstanceAndExpiry(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_ENABLED", "true")
	_, repo := newTestService(t)
	if err := repo.db.AutoMigrate(&DigitalHumanMemory{}); err != nil {
		t.Fatal(err)
	}
	scope := testScope()
	digitalHumanID := uuid.New()
	run, instanceID := instanceRun(t)
	run.OrganizationID, run.WorkspaceID, run.DigitalHumanID = scope.OrganizationID, scope.WorkspaceID, digitalHumanID
	now := time.Now().UTC()
	rows := map[string]*DigitalHumanMemory{
		"self":           {MemoryClass: memoryruntime.MemoryClassSelf, Visibility: MemoryVisibilityEveryone, UpdatedAt: now},
		"internal":       {MemoryClass: memoryruntime.MemoryClassSelf, UpdatedAt: now},
		"own-instance":   {MemoryClass: memoryruntime.MemoryClassBusiness, ScopeType: memoryruntime.MemoryScopeInstance, ScopeID: instanceID.String(), UpdatedAt: now},
		"stale-instance": {MemoryClass: memoryruntime.MemoryClassBusiness, ScopeType: memoryruntime.MemoryScopeInstance, ScopeID: instanceID.String(), UpdatedAt: now.Add(-48 * time.Hour)},
		"other-instance": {MemoryClass: memoryruntime.MemoryClassBusiness, ScopeType: memoryruntime.MemoryScopeInstance, ScopeID: uuid.NewString(), UpdatedAt: now},
	}
	for key, row := range rows {
		row.ID, row.OrganizationID, row.WorkspaceID, row.DigitalHumanID = uuid.New(), scope.OrganizationID, scope.WorkspaceID, digitalHumanID
		row.MemoryKey, row.MemoryType, row.Content, row.Status = key, memoryruntime.MemoryTypeFact, key, MemoryStatusActive
		if row.Visibility == "" {
			row.Visibility = MemoryVisibilityInternal
		}
		row.Evidence, row.CreatedAt = datatypes.JSON(`[]`), row.UpdatedAt
		if err := repo.db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	recall := func(want ...string) {
		t.Helper()
		readScope := memoryReadScopeForRun(run, invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginAPI}})
		items, err := repo.ActiveMemories(t.Context(), scope, digitalHumanID, 50, &readScope)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, item := range items {
			got[item.MemoryKey] = true
			if !readScope.allows(item) {
				t.Fatalf("query and allows disagree on %s", item.MemoryKey)
			}
		}
		if len(got) != len(want) {
			t.Fatalf("recalled %v, want %v", got, want)
		}
		for _, key := range want {
			if !got[key] {
				t.Fatalf("recalled %v, want %v", got, want)
			}
		}
	}
	recall("self", "own-instance", "stale-instance")
	t.Setenv("ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_TTL", "24h")
	recall("self", "own-instance")
}

func TestInstanceMemoryCapacityAndExpiryAreOptional(t *testing.T) {
	service, repo := newTestService(t)
	if err := repo.db.AutoMigrate(&DigitalHumanMemory{}); err != nil {
		t.Fatal(err)
	}
	scope := testScope()
	digitalHumanID, instanceID := uuid.New(), uuid.New()
	now := time.Date(2026, 10, 23, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	instanceScope := &memoryruntime.MemoryScopeRef{Type: memoryruntime.MemoryScopeInstance, ID: instanceID.String()}
	for index, key := range []string{"first", "second"} {
		row := &DigitalHumanMemory{ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID,
			MemoryClass: memoryruntime.MemoryClassBusiness, ScopeType: memoryruntime.MemoryScopeInstance, ScopeID: instanceID.String(), MemoryKey: key,
			MemoryType: memoryruntime.MemoryTypeFact, Content: key, Status: MemoryStatusActive, Visibility: MemoryVisibilityInternal,
			Evidence: datatypes.JSON(`[]`), CreatedAt: now, UpdatedAt: now.Add(-time.Duration(index*72) * time.Hour)}
		if err := repo.db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	upsert := func(key string) MemoryProposalRequest {
		return MemoryProposalRequest{Key: key, Action: memoryruntime.MutationActionUpsert, MemoryClass: memoryruntime.MemoryClassBusiness, Scope: instanceScope}
	}
	if err := checkInstanceMemoryCapacity(t.Context(), repo, scope, digitalHumanID, upsert("third")); err != nil {
		t.Fatalf("an unlimited instance rejected a write: %v", err)
	}
	t.Setenv("ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_MAX_ENTRIES", "2")
	if err := checkInstanceMemoryCapacity(t.Context(), repo, scope, digitalHumanID, upsert("third")); !errors.Is(err, errInstanceMemoryFull) || !errors.Is(err, ErrMemoryPolicyRejected) {
		t.Fatalf("a full instance accepted a new key: %v", err)
	}
	if err := checkInstanceMemoryCapacity(t.Context(), repo, scope, digitalHumanID, upsert("First")); err != nil {
		t.Fatalf("a full instance refused to update an existing key: %v", err)
	}
	if code := memoryPublicErrorCode(applicationError(errInstanceMemoryFull, "digital_human.memory.propose")); code != memoryAdmissionReasonInstanceFull {
		t.Fatalf("full instance error code = %q", code)
	}

	if archived, err := service.ArchiveExpiredInstanceMemories(t.Context()); err != nil || archived != 0 {
		t.Fatalf("entries were archived without an expiry: %d %v", archived, err)
	}
	t.Setenv("ZGI_DIGITAL_HUMAN_INSTANCE_MEMORY_TTL", "48h")
	if archived, err := service.ArchiveExpiredInstanceMemories(t.Context()); err != nil || archived != 1 {
		t.Fatalf("expired entries archived = %d, %v; want 1", archived, err)
	}
	var second DigitalHumanMemory
	if err := repo.db.Where("memory_key = ?", "second").Take(&second).Error; err != nil || second.Status != MemoryStatusArchived {
		t.Fatalf("expired entry = %+v, %v", second, err)
	}
}
