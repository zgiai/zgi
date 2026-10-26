package digitalhuman

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
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
