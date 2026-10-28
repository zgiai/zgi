package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

func TestApplyModelCapabilitiesFallsBackWhenModelSpecIsUnavailable(t *testing.T) {
	svc := &service{modelSpecResolver: modelSpecResolverFunc(func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
		return ModelSpec{}, false, nil
	})}
	parts := &chatRequestParts{Provider: "private", ModelName: "new-model"}

	if err := svc.applyModelCapabilities(context.Background(), Scope{OrganizationID: uuid.New()}, Caller{}, parts); err != nil {
		t.Fatalf("applyModelCapabilities() error = %v", err)
	}
	if parts.ModelCapabilityStatus != "fallback" || parts.FunctionCallingKnown || !parts.FunctionCallingAssumed {
		t.Fatalf("fallback capabilities = %#v", parts)
	}
}

func TestApplyModelCapabilitiesFallsBackToDirectChatWithoutFunctionCalling(t *testing.T) {
	svc := &service{modelSpecResolver: modelSpecResolverFunc(func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
		return ModelSpec{SupportsToolCall: false}, true, nil
	})}
	parts := &chatRequestParts{Provider: "private", ModelName: "legacy-model", Surface: aiChatSurfaceContextualSidebar}

	if err := svc.applyModelCapabilities(context.Background(), Scope{OrganizationID: uuid.New()}, Caller{}, parts); err != nil {
		t.Fatalf("applyModelCapabilities() error = %v", err)
	}
	if err := finalizeExecutionMode(Caller{}, parts); err != nil {
		t.Fatalf("finalizeExecutionMode() error = %v", err)
	}
	if parts.ExecutionMode != executionModeDirectChat || parts.ExecutionRouteReason != executionRouteFunctionCallingUnavailable {
		t.Fatalf("mode = %q reason = %q, want direct fallback", parts.ExecutionMode, parts.ExecutionRouteReason)
	}
}

func TestApplyModelCapabilitiesSelectsWorkChatExecutionMode(t *testing.T) {
	tests := []struct {
		name     string
		spec     ModelSpec
		skillIDs []string
		want     string
	}{
		{name: "native tools for agent-capable model", spec: ModelSpec{UseCases: []string{"text-chat", "agent"}, SupportsToolCall: true}, skillIDs: []string{"calculator"}, want: executionModeNativeToolLoop},
		{name: "native tools for text-chat model", spec: ModelSpec{UseCases: []string{"text-chat"}, SupportsToolCall: true}, skillIDs: []string{"calculator"}, want: executionModeNativeToolLoop},
		{name: "direct chat without skills", spec: ModelSpec{UseCases: []string{"text-chat"}, SupportsToolCall: true}, want: executionModeDirectChat},
		{name: "direct chat", spec: ModelSpec{UseCases: []string{"text-chat"}}, want: executionModeDirectChat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &service{modelSpecResolver: modelSpecResolverFunc(func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
				return tt.spec, true, nil
			})}
			parts := &chatRequestParts{Provider: "private", ModelName: "model", Surface: aiChatSurfaceWorkChat, SkillIDs: tt.skillIDs}
			if err := svc.applyModelCapabilities(context.Background(), Scope{OrganizationID: uuid.New()}, Caller{}, parts); err != nil {
				t.Fatalf("applyModelCapabilities() error = %v", err)
			}
			if err := finalizeExecutionMode(Caller{}, parts); err != nil {
				t.Fatalf("finalizeExecutionMode() error = %v", err)
			}
			if parts.ExecutionMode != tt.want {
				t.Fatalf("ExecutionMode = %q, want %q", parts.ExecutionMode, tt.want)
			}
		})
	}
}

func TestApplyModelCapabilitiesUsesNativeLoopForNewAgentCaller(t *testing.T) {
	svc := &service{modelSpecResolver: modelSpecResolverFunc(func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
		return ModelSpec{UseCases: []string{"text-chat"}, SupportsToolCall: true}, true, nil
	})}
	parts := &chatRequestParts{Provider: "private", ModelName: "legacy-model", Surface: aiChatSurfaceExternalPageChat}
	caller := Caller{Type: runtimemodel.ConversationCallerAgent}
	if err := svc.applyModelCapabilities(context.Background(), Scope{OrganizationID: uuid.New()}, caller, parts); err != nil {
		t.Fatalf("applyModelCapabilities() error = %v", err)
	}
	if err := finalizeExecutionMode(caller, parts); err != nil {
		t.Fatalf("finalizeExecutionMode() error = %v", err)
	}
	if parts.ExecutionMode != executionModeNativeAgentLoop {
		t.Fatalf("ExecutionMode = %q, want %q", parts.ExecutionMode, executionModeNativeAgentLoop)
	}
}

func TestApplyModelCapabilitiesFallsBackOnResolverFailure(t *testing.T) {
	svc := &service{modelSpecResolver: modelSpecResolverFunc(func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error) {
		return ModelSpec{}, false, errModelSpecResolver
	})}
	parts := &chatRequestParts{Provider: "private", ModelName: "new-model"}

	if err := svc.applyModelCapabilities(context.Background(), Scope{OrganizationID: uuid.New()}, Caller{}, parts); err != nil {
		t.Fatalf("applyModelCapabilities() error = %v", err)
	}
	if parts.ModelCapabilityStatus != "fallback" || !strings.Contains(parts.ModelCapabilityError, errModelSpecResolver.Error()) {
		t.Fatalf("fallback capabilities = %#v", parts)
	}
}

var errModelSpecResolver = errors.New("metadata store unavailable")

type modelSpecResolverFunc func(context.Context, uuid.UUID, string, string) (ModelSpec, bool, error)

func (fn modelSpecResolverFunc) Resolve(ctx context.Context, organizationID uuid.UUID, provider string, modelName string) (ModelSpec, bool, error) {
	return fn(ctx, organizationID, provider, modelName)
}
