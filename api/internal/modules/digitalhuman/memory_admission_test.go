package digitalhuman

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"gorm.io/datatypes"
)

func trustedRun(trust SourceTrust, surface string) *Run {
	value := string(trust)
	return &Run{
		ID: uuid.New(), OrganizationID: uuid.New(), WorkspaceID: uuid.New(), DigitalHumanID: uuid.New(),
		Surface: surface, SourceTrust: &value,
	}
}

func channelEvent(conversation string) invocation.Event {
	return invocation.Event{
		Origin:  invocation.Origin{Kind: invocation.OriginChannel, Provider: "feishu", InstallationID: "installation"},
		ReplyTo: &invocation.ReplyAddress{Provider: "feishu", InstallationID: "installation", ConversationID: conversation},
	}
}

func TestAdmitMemoryWriteFollowsSourceTrust(t *testing.T) {
	manual := invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginManual}}
	api := invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginAPI}}
	cases := []struct {
		name    string
		run     *Run
		event   invocation.Event
		mode    memoryWriteMode
		auto    bool
		allowed bool
		class   memoryruntime.MemoryClass
		scope   memoryruntime.MemoryScopeType
		reason  string
	}{
		{name: "operator explicit", run: trustedRun(SourceTrustAuthorizedOperator, SurfaceConsole), event: manual, mode: memoryWriteExplicit, allowed: true, class: memoryruntime.MemoryClassSelf},
		{name: "operator proactive needs auto extraction", run: trustedRun(SourceTrustAuthorizedOperator, SurfaceConsole), event: manual, mode: memoryWriteProactive, reason: memoryAdmissionReasonTrust},
		{name: "operator proactive", run: trustedRun(SourceTrustAuthorizedOperator, SurfaceConsole), event: manual, mode: memoryWriteProactive, auto: true, allowed: true, class: memoryruntime.MemoryClassSelf},
		{name: "operator background stays excluded", run: trustedRun(SourceTrustAuthorizedOperator, SurfaceConsole), event: manual, mode: memoryWriteBackground, auto: true, reason: memoryAdmissionReasonTrust},
		{name: "member explicit", run: trustedRun(SourceTrustWorkspaceMember, SurfaceConsole), event: manual, mode: memoryWriteExplicit, auto: true, reason: memoryAdmissionReasonTrust},
		{name: "external explicit", run: trustedRun(SourceTrustExternalEndUser, SurfaceAPI), event: api, mode: memoryWriteExplicit, auto: true, reason: memoryAdmissionReasonTrust},
		{name: "external background", run: trustedRun(SourceTrustExternalEndUser, SurfaceAPI), event: api, mode: memoryWriteBackground, auto: true, reason: memoryAdmissionReasonTrust},
		{name: "channel without auto extraction", run: trustedRun(SourceTrustChannelParticipant, SurfaceChannel), event: channelEvent("group"), mode: memoryWriteExplicit, reason: memoryAdmissionReasonTrust},
		{name: "channel explicit", run: trustedRun(SourceTrustChannelParticipant, SurfaceChannel), event: channelEvent("group"), mode: memoryWriteExplicit, auto: true, allowed: true, class: memoryruntime.MemoryClassBusiness, scope: memoryruntime.MemoryScopeChannel},
		{name: "channel background", run: trustedRun(SourceTrustChannelParticipant, SurfaceChannel), event: channelEvent("group"), mode: memoryWriteBackground, auto: true, allowed: true, class: memoryruntime.MemoryClassBusiness, scope: memoryruntime.MemoryScopeChannel},
		{name: "channel without conversation", run: trustedRun(SourceTrustChannelParticipant, SurfaceChannel), event: invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginChannel}}, mode: memoryWriteBackground, auto: true, reason: memoryAdmissionReasonChannelScope},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := admitMemoryWrite(tc.run, tc.event, tc.mode, tc.auto)
			if got.Allowed != tc.allowed || got.Trust != RunSourceTrust(tc.run) {
				t.Fatalf("admission = %+v", got)
			}
			if !tc.allowed {
				if got.Reason != tc.reason {
					t.Fatalf("reason = %q, want %q", got.Reason, tc.reason)
				}
				if memoryRejectionReason(got.err()) != tc.reason {
					t.Fatalf("recorded reason = %q", memoryRejectionReason(got.err()))
				}
				return
			}
			if got.Class != tc.class {
				t.Fatalf("class = %s, want %s", got.Class, tc.class)
			}
			if tc.scope == "" {
				if got.Scope != nil {
					t.Fatalf("unexpected scope %+v", got.Scope)
				}
			} else if got.Scope == nil || got.Scope.Type != tc.scope || got.Scope.ID == "" {
				t.Fatalf("scope = %+v", got.Scope)
			}
		})
	}
}

func TestChannelMemoryAdmissionScopesEachConversationSeparately(t *testing.T) {
	run := trustedRun(SourceTrustChannelParticipant, SurfaceChannel)
	first := admitMemoryWrite(run, channelEvent("group-a"), memoryWriteExplicit, true)
	second := admitMemoryWrite(run, channelEvent("group-b"), memoryWriteExplicit, true)
	if first.Scope == nil || second.Scope == nil || first.Scope.ID == second.Scope.ID {
		t.Fatalf("conversations share a memory scope: %+v %+v", first.Scope, second.Scope)
	}
}

func TestMemoryToolAvailabilityMirrorsAdmission(t *testing.T) {
	for _, tc := range []struct {
		trust SourceTrust
		auto  bool
		want  bool
	}{
		{SourceTrustAuthorizedOperator, false, true},
		{SourceTrustWorkspaceMember, true, false},
		{SourceTrustChannelParticipant, false, false},
		{SourceTrustChannelParticipant, true, true},
		{SourceTrustExternalEndUser, true, false},
	} {
		if got := memoryToolAvailable(trustedRun(tc.trust, ""), tc.auto); got != tc.want {
			t.Fatalf("%s auto=%v: got %v", tc.trust, tc.auto, got)
		}
	}
	if !backgroundExtractionAdmitted(trustedRun(SourceTrustChannelParticipant, SurfaceChannel)) ||
		backgroundExtractionAdmitted(trustedRun(SourceTrustAuthorizedOperator, SurfaceConsole)) ||
		backgroundExtractionAdmitted(trustedRun(SourceTrustExternalEndUser, SurfaceAPI)) {
		t.Fatal("background extraction admission diverges from the policy")
	}
}

func TestMemoryReadScopeFiltersMemoriesAndQueries(t *testing.T) {
	_, repo := newTestService(t)
	if err := repo.db.AutoMigrate(&DigitalHumanMemory{}); err != nil {
		t.Fatal(err)
	}
	scope := testScope()
	digitalHumanID := uuid.New()
	run := trustedRun(SourceTrustChannelParticipant, SurfaceChannel)
	run.OrganizationID, run.WorkspaceID, run.DigitalHumanID = scope.OrganizationID, scope.WorkspaceID, digitalHumanID
	event := channelEvent("group-a")
	ownChannel, _ := runChannelScopeID(run, event)
	otherChannel, _ := runChannelScopeID(run, channelEvent("group-b"))
	workspace := scope.WorkspaceID.String()

	rows := map[string]*DigitalHumanMemory{
		"self":                 {MemoryClass: memoryruntime.MemoryClassSelf},
		"workspace":            {MemoryClass: memoryruntime.MemoryClassBusiness, ScopeType: memoryruntime.MemoryScopeWorkspace, ScopeID: workspace},
		"workspace-restricted": {MemoryClass: memoryruntime.MemoryClassBusiness, ScopeType: memoryruntime.MemoryScopeWorkspace, ScopeID: "other-workspace-key", Visibility: MemoryVisibilityRestricted},
		"own-channel":          {MemoryClass: memoryruntime.MemoryClassBusiness, ScopeType: memoryruntime.MemoryScopeChannel, ScopeID: ownChannel.String()},
		"other-channel":        {MemoryClass: memoryruntime.MemoryClassBusiness, ScopeType: memoryruntime.MemoryScopeChannel, ScopeID: otherChannel.String()},
		"project":              {MemoryClass: memoryruntime.MemoryClassBusiness, ScopeType: memoryruntime.MemoryScopeType("project"), ScopeID: "project-1"},
	}
	now := time.Now().UTC()
	for key, row := range rows {
		row.ID, row.OrganizationID, row.WorkspaceID, row.DigitalHumanID = uuid.New(), scope.OrganizationID, scope.WorkspaceID, digitalHumanID
		row.MemoryKey, row.MemoryType, row.Content, row.Status = key, memoryruntime.MemoryTypeFact, key, MemoryStatusActive
		if row.Visibility == "" {
			row.Visibility = MemoryVisibilityWorkspace
		}
		row.Evidence, row.CreatedAt, row.UpdatedAt = datatypes.JSON(`[]`), now, now
		if err := repo.db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	check := func(name string, readScope memoryReadScope, want ...string) {
		t.Helper()
		expected := make(map[string]bool, len(want))
		for _, key := range want {
			expected[key] = true
		}
		for key, row := range rows {
			if readScope.allows(row) != expected[key] {
				t.Fatalf("%s: allows(%s) = %v", name, key, !expected[key])
			}
		}
		items, err := repo.ActiveMemories(t.Context(), scope, digitalHumanID, 50, &readScope)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != len(want) {
			t.Fatalf("%s: query returned %d memories, want %d", name, len(items), len(want))
		}
		for _, item := range items {
			if !expected[item.MemoryKey] {
				t.Fatalf("%s: query returned %s", name, item.MemoryKey)
			}
		}
	}
	check("channel participant", memoryReadScopeForRun(run, event), "self", "workspace", "own-channel")
	operator := trustedRun(SourceTrustAuthorizedOperator, SurfaceConsole)
	operator.WorkspaceID = scope.WorkspaceID
	check("console operator", memoryReadScopeForRun(operator, invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginManual}}), "self", "workspace")
	external := trustedRun(SourceTrustExternalEndUser, SurfaceAPI)
	external.WorkspaceID = scope.WorkspaceID
	check("external end user", memoryReadScopeForRun(external, invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginAPI}}), "self")

	all, err := repo.ActiveMemories(t.Context(), scope, digitalHumanID, 50, nil)
	if err != nil || len(all) != len(rows) {
		t.Fatalf("unscoped recall returned %d memories, err %v", len(all), err)
	}
}

func TestMemoryInWriteTargetMatchesClassAndScope(t *testing.T) {
	channel := memoryWriteAdmission{Allowed: true, Class: memoryruntime.MemoryClassBusiness, Scope: &memoryruntime.MemoryScopeRef{Type: memoryruntime.MemoryScopeChannel, ID: "a"}}
	self := memoryWriteAdmission{Allowed: true, Class: memoryruntime.MemoryClassSelf}
	selfMemory := &DigitalHumanMemory{MemoryClass: memoryruntime.MemoryClassSelf}
	channelMemory := &DigitalHumanMemory{MemoryClass: memoryruntime.MemoryClassBusiness, ScopeType: memoryruntime.MemoryScopeChannel, ScopeID: "a"}
	otherChannel := &DigitalHumanMemory{MemoryClass: memoryruntime.MemoryClassBusiness, ScopeType: memoryruntime.MemoryScopeChannel, ScopeID: "b"}
	if !memoryInWriteTarget(selfMemory, self) || memoryInWriteTarget(channelMemory, self) {
		t.Fatal("self target mismatch")
	}
	if !memoryInWriteTarget(channelMemory, channel) || memoryInWriteTarget(selfMemory, channel) || memoryInWriteTarget(otherChannel, channel) {
		t.Fatal("channel target mismatch")
	}
}
