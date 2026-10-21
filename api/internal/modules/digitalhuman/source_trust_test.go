package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
)

func TestClassifySourceTrustBySurface(t *testing.T) {
	org, ws, actor := uuid.New(), uuid.New(), uuid.New()
	allow := MemoryPromotionAuthorizerFunc(func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error) { return true, nil })
	deny := MemoryPromotionAuthorizerFunc(func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error) { return false, nil })
	broken := MemoryPromotionAuthorizerFunc(func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error) {
		return true, errors.New("permission backend unavailable")
	})
	scheduled := func(origin string) invocation.Event {
		return invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginSchedule}, Context: map[string]any{"origin_surface": origin}}
	}
	cases := []struct {
		name       string
		authorizer MemoryPromotionAuthorizer
		surface    string
		actor      *uuid.UUID
		event      invocation.Event
		want       SourceTrust
	}{
		{name: "console operator", authorizer: allow, surface: SurfaceConsole, actor: &actor, want: SourceTrustAuthorizedOperator},
		{name: "console member", authorizer: deny, surface: SurfaceConsole, actor: &actor, want: SourceTrustWorkspaceMember},
		{name: "console authorizer error degrades", authorizer: broken, surface: SurfaceConsole, actor: &actor, want: SourceTrustWorkspaceMember},
		{name: "console without authorizer", surface: SurfaceConsole, actor: &actor, want: SourceTrustWorkspaceMember},
		{name: "console without actor", authorizer: allow, surface: SurfaceConsole, want: SourceTrustWorkspaceMember},
		{name: "webapp member is external", authorizer: allow, surface: SurfaceWebApp, actor: &actor, want: SourceTrustExternalEndUser},
		{name: "api", authorizer: allow, surface: SurfaceAPI, want: SourceTrustExternalEndUser},
		{name: "channel ignores installer permission", authorizer: allow, surface: SurfaceChannel, actor: &actor, want: SourceTrustChannelParticipant},
		{name: "scheduled console task", authorizer: allow, surface: SurfaceTask, event: scheduled(SurfaceConsole), want: SourceTrustWorkspaceMember},
		{name: "scheduled channel task", surface: SurfaceTask, event: scheduled(SurfaceChannel), want: SourceTrustChannelParticipant},
		{name: "scheduled api task", surface: SurfaceTask, event: scheduled(SurfaceAPI), want: SourceTrustExternalEndUser},
		{name: "scheduled unknown origin", surface: SurfaceTask, event: scheduled(""), want: SourceTrustExternalEndUser},
		{name: "unknown surface", surface: "other", want: SourceTrustExternalEndUser},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			service := &RuntimeService{trustAuthorizer: tc.authorizer}
			if got := service.classifySourceTrust(context.Background(), org, ws, tc.surface, tc.actor, tc.event); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestRunSourceTrustDerivesLegacyRunsWithoutOperatorLevel(t *testing.T) {
	stored := string(SourceTrustAuthorizedOperator)
	invalid := "administrator"
	scheduledEvent, err := json.Marshal(invocation.Event{Context: map[string]any{"origin_surface": SurfaceChannel}})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		run  *Run
		want SourceTrust
	}{
		{name: "persisted", run: &Run{Surface: SurfaceConsole, SourceTrust: &stored}, want: SourceTrustAuthorizedOperator},
		{name: "invalid persisted value falls back", run: &Run{Surface: SurfaceChannel, SourceTrust: &invalid}, want: SourceTrustChannelParticipant},
		{name: "legacy console", run: &Run{Surface: SurfaceConsole}, want: SourceTrustWorkspaceMember},
		{name: "legacy webapp", run: &Run{Surface: SurfaceWebApp}, want: SourceTrustExternalEndUser},
		{name: "legacy scheduled channel task", run: &Run{Surface: SurfaceTask, InvocationEvent: scheduledEvent}, want: SourceTrustChannelParticipant},
		{name: "legacy scheduled undecodable", run: &Run{Surface: SurfaceTask}, want: SourceTrustExternalEndUser},
		{name: "nil", want: SourceTrustExternalEndUser},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RunSourceTrust(tc.run); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestRunChannelScopeIDIsStablePerConversation(t *testing.T) {
	run := &Run{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), DigitalHumanID: uuid.New()}
	reply := func(conversation string) *invocation.ReplyAddress {
		return &invocation.ReplyAddress{Provider: "feishu", InstallationID: "installation", ConversationID: conversation}
	}
	channel := invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginChannel}, ReplyTo: reply("group-a")}
	first, ok := runChannelScopeID(run, channel)
	if !ok {
		t.Fatal("channel run has no scope")
	}
	scheduled := invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginSchedule}, ReplyTo: reply("group-a"), Context: map[string]any{"origin_surface": SurfaceChannel}}
	if second, ok := runChannelScopeID(run, scheduled); !ok || second != first {
		t.Fatalf("scheduled channel task scope = %s, %v; want %s", second, ok, first)
	}
	if other, _ := runChannelScopeID(run, invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginChannel}, ReplyTo: reply("group-b")}); other == first {
		t.Fatal("different conversations share a scope")
	}
	consoleTask := invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginSchedule}, ReplyTo: reply("group-a"), Context: map[string]any{"origin_surface": SurfaceConsole}}
	if _, ok := runChannelScopeID(run, consoleTask); ok {
		t.Fatal("console task resolved a channel scope")
	}
	if _, ok := runChannelScopeID(run, invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginAPI}, ReplyTo: reply("group-a")}); ok {
		t.Fatal("api run resolved a channel scope")
	}
}

func TestRunChannelScopeIDUsesVerifiedOwnerWithoutDeliveryAddress(t *testing.T) {
	run := &Run{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), DigitalHumanID: uuid.New()}
	channel := invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginChannel},
		ReplyTo: &invocation.ReplyAddress{Provider: "feishu", InstallationID: "installation", ConversationID: "group-a"}}
	conversationScope, ok := runChannelScopeID(run, channel)
	if !ok {
		t.Fatal("channel run has no scope")
	}
	// A task_only channel task is scheduled without a reply address.
	taskOnly := invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginSchedule}, Context: map[string]any{
		"origin_surface": SurfaceChannel, TaskOwnerConversationContextKey: conversationScope.String(),
	}}
	if got, ok := runChannelScopeID(run, taskOnly); !ok || got != conversationScope {
		t.Fatalf("task_only scheduled scope = %s, %v; want %s", got, ok, conversationScope)
	}
	invalid := invocation.Event{Origin: invocation.Origin{Kind: invocation.OriginSchedule}, ReplyTo: channel.ReplyTo, Context: map[string]any{
		"origin_surface": SurfaceChannel, TaskOwnerConversationContextKey: "not-a-uuid",
	}}
	if _, ok := runChannelScopeID(run, invalid); ok {
		t.Fatal("an invalid verified owner fell back to the delivery address")
	}
	admission := admitMemoryWrite(trustedRunWithIDs(run, SourceTrustChannelParticipant), taskOnly, memoryWriteBackground, true)
	if !admission.Allowed || admission.Scope == nil || admission.Scope.ID != conversationScope.String() {
		t.Fatalf("task_only scheduled admission = %+v", admission)
	}
}

func trustedRunWithIDs(base *Run, trust SourceTrust) *Run {
	run := *base
	run.Surface, run.SourceTrust = SurfaceTask, sourceTrustPointer(trust)
	return &run
}
