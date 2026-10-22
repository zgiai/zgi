package digitalhuman

import (
	"context"
	"encoding/json"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
)

// SourceTrust is the server-classified trust level of the source that woke a
// DigitalHuman run. It is decided once at admission from verified run data and
// never from model output or client-supplied fields. See ADR 0004.
type SourceTrust string

const (
	SourceTrustAuthorizedOperator SourceTrust = "authorized_operator"
	SourceTrustWorkspaceMember    SourceTrust = "workspace_member"
	SourceTrustChannelParticipant SourceTrust = "channel_participant"
	SourceTrustExternalEndUser    SourceTrust = "external_end_user"
)

func (t SourceTrust) valid() bool {
	switch t {
	case SourceTrustAuthorizedOperator, SourceTrustWorkspaceMember, SourceTrustChannelParticipant, SourceTrustExternalEndUser:
		return true
	default:
		return false
	}
}

// MemoryTrustPolicyEnabled switches DigitalHuman memory admission and
// retrieval to the source trust policy. It is fail-closed: unset keeps the
// legacy origin-based checks.
func MemoryTrustPolicyEnabled() bool {
	return envFlagEnabled("ZGI_DIGITAL_HUMAN_MEMORY_TRUST_POLICY_ENABLED")
}

// TaskVisibilityEnabled switches DigitalHuman task reads from owner
// namespaces to requester-based visibility. It is fail-closed.
func TaskVisibilityEnabled() bool {
	return envFlagEnabled("ZGI_DIGITAL_HUMAN_TASK_VISIBILITY_ENABLED")
}

func envFlagEnabled(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

// WithSourceTrustAuthorizer lets console admission recognize workspace members
// who may promote DigitalHuman memory. Without it every console run is
// classified as a plain workspace member.
func WithSourceTrustAuthorizer(authorizer MemoryPromotionAuthorizer) RuntimeServiceOption {
	return func(service *RuntimeService) {
		service.trustAuthorizer = authorizer
	}
}

// classifySourceTrust decides the trust level of a newly admitted run. An
// authorization failure degrades to the lower level instead of rejecting the
// run, because the level only narrows what the run may change.
func (s *RuntimeService) classifySourceTrust(ctx context.Context, organizationID, workspaceID uuid.UUID, surface string, actorID *uuid.UUID, event invocation.Event) SourceTrust {
	switch surface {
	case SurfaceConsole:
		if s.trustAuthorizer != nil && actorID != nil && *actorID != uuid.Nil {
			if allowed, err := s.trustAuthorizer.CanPromoteDigitalHumanMemory(ctx, organizationID, workspaceID, *actorID); err == nil && allowed {
				return SourceTrustAuthorizedOperator
			}
		}
		return SourceTrustWorkspaceMember
	case SurfaceTask:
		origin, _ := event.Context["origin_surface"].(string)
		return scheduledSourceTrust(origin)
	case SurfacePreview:
		// A preview never promotes memory, whoever runs it.
		return SourceTrustWorkspaceMember
	default:
		return surfaceSourceTrust(surface)
	}
}

// RunSourceTrust returns the persisted trust level, deriving one for runs
// admitted before classification existed. Derivation never yields
// authorized_operator because the historical actor permission is unknown.
func RunSourceTrust(run *Run) SourceTrust {
	if run == nil {
		return SourceTrustExternalEndUser
	}
	if run.SourceTrust != nil {
		if trust := SourceTrust(strings.TrimSpace(*run.SourceTrust)); trust.valid() {
			return trust
		}
	}
	switch run.Surface {
	case SurfaceConsole:
		return SourceTrustWorkspaceMember
	case SurfaceTask:
		var event invocation.Event
		if json.Unmarshal(run.InvocationEvent, &event) != nil {
			return SourceTrustExternalEndUser
		}
		origin, _ := event.Context["origin_surface"].(string)
		return scheduledSourceTrust(origin)
	default:
		return surfaceSourceTrust(run.Surface)
	}
}

func sourceTrustPointer(trust SourceTrust) *string {
	value := string(trust)
	return &value
}

// scheduledSourceTrust maps a task's origin surface to the level its scheduled
// runs inherit. A scheduled run never acts as an authorized operator.
func scheduledSourceTrust(originSurface string) SourceTrust {
	switch strings.TrimSpace(originSurface) {
	case SurfaceConsole:
		return SourceTrustWorkspaceMember
	case SurfaceChannel:
		return SourceTrustChannelParticipant
	default:
		return SourceTrustExternalEndUser
	}
}

func surfaceSourceTrust(surface string) SourceTrust {
	switch surface {
	case SurfaceChannel:
		return SourceTrustChannelParticipant
	default:
		// WebApp, API, and unknown surfaces fail closed.
		return SourceTrustExternalEndUser
	}
}

// TaskOwnerConversationContextKey carries a channel task's verified owner
// conversation into its scheduled invocations.
const TaskOwnerConversationContextKey = "task_owner_conversation_id"

// runChannelScopeID returns the stable channel conversation identifier for a
// channel run or a scheduled run whose task was created in a channel.
func runChannelScopeID(run *Run, event invocation.Event) (uuid.UUID, bool) {
	if run == nil {
		return uuid.Nil, false
	}
	if event.Origin.Kind != invocation.OriginChannel && event.Origin.Kind != invocation.OriginSchedule {
		return uuid.Nil, false
	}
	if event.Origin.Kind == invocation.OriginSchedule {
		if origin, _ := event.Context["origin_surface"].(string); strings.TrimSpace(origin) != SurfaceChannel {
			return uuid.Nil, false
		}
		// The scheduler supplies the task's verified owner conversation, which
		// is the same identifier ChannelTaskConversationID derives below. A
		// task_only task has no reply address, so the owner must come first.
		if raw, _ := event.Context[TaskOwnerConversationContextKey].(string); raw != "" {
			if id, err := uuid.Parse(raw); err == nil && id != uuid.Nil {
				return id, true
			}
			return uuid.Nil, false
		}
	}
	if event.ReplyTo == nil {
		return uuid.Nil, false
	}
	id, err := ChannelTaskConversationID(run.OrganizationID, run.WorkspaceID, run.DigitalHumanID,
		event.ReplyTo.Provider, event.ReplyTo.InstallationID, event.ReplyTo.ConversationID)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}
