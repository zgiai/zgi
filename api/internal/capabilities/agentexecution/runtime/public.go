// Package executionruntime provides conversation-owned Linux tools and the
// platform relay. Entry adapters supply verified identity and authorization;
// this package never assumes that an application owner's account is the user.
package executionruntime

import (
	"context"

	"github.com/google/uuid"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	"gorm.io/gorm"
)

type Gateway = agentExecutionGateway
type Binding = agentExecutionBinding
type Job = agentExecutionJob
type OutputChunk = agentExecutionOutputChunk
type Capabilities = agentExecutionCapabilities
type ArtifactPersister = agentExecutionArtifactPersister
type AttachmentReader = agentRuntimeAttachmentReader
type SkillPackageResolver = agentSkillPackageResolver
type DefaultArtifactPersister = defaultAgentExecutionArtifactPersister

const (
	agentExecutionDependencyPreparationModeOffline        = "offline-bundled"
	agentExecutionDependencyPreparationModeControlledHTTP = "controlled-http"
	agentExecutionPackageNetworkMode                      = "package-install"
)

func GatewayFromEnv() Gateway                      { return agentExecutionGatewayFromEnv() }
func EnvBool(name string, fallback bool) bool      { return agentExecutionEnvBool(name, fallback) }
func ValidGatewayURL(raw string) bool              { return validAgentRuntimeGatewayURL(raw) }
func JobErrorDefinitions() []appcatalog.Definition { return agentExecutionJobErrorDefinitions() }

func NewSkillPackageResolver(db *gorm.DB, organizationID uuid.UUID, enabledIDs []string) SkillPackageResolver {
	return newAgentSkillPackageResolver(db, organizationID, enabledIDs)
}

// AuthorizeExecution is checked before every tool invocation. The platform
// authorizer is additionally checked on every relayed action and cached result.
type AuthorizeExecution func(context.Context) error
