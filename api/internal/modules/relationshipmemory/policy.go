package relationshipmemory

import (
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
)

type MemorySurface string
type ConversationType string

const (
	SurfaceAgentConsole        MemorySurface = "agent_console"
	SurfaceAgentWebApp         MemorySurface = "agent_webapp"
	SurfaceAgentAPI            MemorySurface = "agent_api"
	SurfaceDigitalHumanConsole MemorySurface = "digital_human_console"
	SurfaceDigitalHumanWebApp  MemorySurface = "digital_human_webapp"
	SurfaceDigitalHumanAPI     MemorySurface = "digital_human_api"
	SurfaceDigitalHumanTask    MemorySurface = "digital_human_task"
	SurfaceChannel             MemorySurface = "channel"
	SurfaceAIChat              MemorySurface = "aichat"

	ConversationPrivate ConversationType = "private"
	ConversationGroup   ConversationType = "group"
	ConversationTask    ConversationType = "task"
)

type PrincipalRef struct {
	Type string
	ID   uuid.UUID
}

type ExternalIdentityHint struct {
	Provider string
	Tenant   string
	User     string
}

type InvocationIdentity struct {
	Principal        PrincipalRef
	UserSubject      *usersubject.Ref
	Surface          MemorySurface
	ConversationType ConversationType
	ExternalHint     *ExternalIdentityHint
}

type AccessDecision struct {
	Read, Mutate, Tools, AutoCapture bool
	Reason                           string
}

func DecideAccess(identity InvocationIdentity) AccessDecision {
	if identity.Surface == SurfaceChannel {
		return AccessDecision{Reason: "external_identity_unresolved"}
	}
	if identity.Surface == SurfaceAIChat {
		return AccessDecision{Reason: "aichat_memory_disabled"}
	}
	if identity.Surface == SurfaceDigitalHumanTask || identity.ConversationType == ConversationTask {
		return AccessDecision{Reason: "automated_invocation_has_no_user_subject"}
	}
	if identity.UserSubject == nil || identity.UserSubject.ID == uuid.Nil {
		return AccessDecision{Reason: "user_subject_missing"}
	}
	switch identity.Surface {
	case SurfaceAgentConsole, SurfaceAgentWebApp, SurfaceDigitalHumanConsole, SurfaceDigitalHumanWebApp:
		if identity.UserSubject.Kind != usersubject.Account {
			return AccessDecision{Reason: "subject_kind_mismatch"}
		}
	case SurfaceAgentAPI, SurfaceDigitalHumanAPI:
		if identity.UserSubject.Kind != usersubject.APIEndUser {
			return AccessDecision{Reason: "subject_kind_mismatch"}
		}
	default:
		return AccessDecision{Reason: "surface_not_allowed"}
	}
	return AccessDecision{Read: true, Mutate: true, Tools: true, AutoCapture: true, Reason: "allowed"}
}

type MemoryDefinition struct {
	Key         string `json:"key"`
	DisplayName string `json:"display_name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	MaxChars    int    `json:"max_chars,omitempty"`
	Enabled     bool   `json:"enabled"`
}

type ConsumerMemoryContract struct {
	Enabled            bool               `json:"enabled"`
	DefinitionMode     string             `json:"definition_mode"`
	Definitions        []MemoryDefinition `json:"definitions,omitempty"`
	AllowedCategories  []string           `json:"allowed_categories,omitempty"`
	AutoCaptureAllowed bool               `json:"auto_capture_allowed"`
	ConfigRevision     string             `json:"config_revision"`
	RenderBudgetChars  int                `json:"render_budget_chars"`
}
