package digitalhuman

import (
	"context"
	"maps"

	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	executionruntime "github.com/zgiai/zgi/api/internal/capabilities/agentexecution/runtime"
	"github.com/zgiai/zgi/api/internal/capabilities/artifacts"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"gorm.io/gorm"
)

// LinuxExecutionRuntimeProvider is called only after durable run integrity checks.
// Preparation must fail before ChatRuntime or a Worker executes any user command.
type LinuxExecutionRuntimeProvider interface {
	Prepare(context.Context, *Run, *ResolvedExecution, invocation.Event) (func(*chatruntime.PreparedChat) []skillloop.RuntimeTool, error)
}

// LinuxExecutionAccess rechecks live application/credential and upload ownership.
// Console workspace permission is an additional, independently injected check.
type LinuxExecutionAccess interface {
	AuthorizeLinuxExecution(context.Context, Scope, uuid.UUID, uuid.UUID) error
	AuthorizeExecutionUpload(context.Context, Scope, uuid.UUID, *dto.UploadFile) error
}

// LinuxExecutionOptions contains server-resolved dependencies, never client
// environment settings. EntryReady is supplied by production wiring only for
// entries with protected files and dedicated approval continuation.
type LinuxExecutionOptions struct {
	DB               *gorm.DB
	Approvals        executionruntime.Approvals
	Gateway          executionruntime.Gateway
	Files            executionruntime.AttachmentSource
	Access           LinuxExecutionAccess
	AuthorizeConsole func(context.Context, Scope) error
	ArtifactManager  func() *tool_file.ToolFileManager
	EntryReady       func(applicationType, entry string) bool
	Entries          execution.EntryAdmission
	ErrorProjector   *apptransport.Projector
	PlatformCLI      bool
	// Artifacts keeps exported files in the artifact center when it is set.
	Artifacts *artifacts.Service
}

// LinuxExecutionRuntime attaches identity-scoped tools to approved runs.
type LinuxExecutionRuntime struct{ options LinuxExecutionOptions }

// NewLinuxExecutionRuntime constructs a provider with server-owned admission rules.
func NewLinuxExecutionRuntime(options LinuxExecutionOptions) *LinuxExecutionRuntime {
	return &LinuxExecutionRuntime{options: options}
}

// WithLinuxExecutionRuntime injects the optional interactive execution provider.
func WithLinuxExecutionRuntime(provider LinuxExecutionRuntimeProvider) RuntimeServiceOption {
	return func(service *RuntimeService) { service.linux = provider }
}

// Prepare freezes a verified run's execution identity before preparing chat.
// Unreleased entries keep normal chat behavior without receiving Linux tools.
func (r *LinuxExecutionRuntime) Prepare(ctx context.Context, run *Run, resolved *ResolvedExecution, event invocation.Event) (func(*chatruntime.PreparedChat) []skillloop.RuntimeTool, error) {
	return r.prepare(ctx, run, resolved, event, nil)
}

// PrepareContinuation uses the currently authenticated credential for live
// authorization without changing the durable run or its final-user identity.
func (r *LinuxExecutionRuntime) PrepareContinuation(ctx context.Context, run *Run, resolved *ResolvedExecution, event invocation.Event, caller Scope) (func(*chatruntime.PreparedChat) []skillloop.RuntimeTool, error) {
	return r.prepare(ctx, run, resolved, event, &caller)
}

func (r *LinuxExecutionRuntime) prepare(ctx context.Context, run *Run, resolved *ResolvedExecution, event invocation.Event, current *Scope) (func(*chatruntime.PreparedChat) []skillloop.RuntimeTool, error) {
	if run == nil || resolved == nil || resolved.Snapshot.ExecutionRuntime == nil || !resolved.Snapshot.ExecutionRuntime.Enabled {
		return nil, nil
	}
	var entry string
	switch run.Surface {
	case SurfaceConsole:
		entry = "console"
	case SurfaceWebApp:
		entry = "webapp"
	case SurfaceAPI:
		entry = "external-api"
	case SurfaceChannel:
		entry = SurfaceChannel
	case SurfaceTask:
		marker, _ := stringContextValue(event.Context, backgroundExecutionEntryKey)
		if marker != backgroundTaskEntry {
			return nil, nil // Previously persisted tasks retain their original path.
		}
		entry = backgroundTaskEntry
	default:
		return nil, nil
	}
	o := r.options
	if !o.Approvals.Enabled {
		return nil, nil
	}
	if o.EntryReady == nil || !o.EntryReady("digital_human", entry) {
		return nil, nil
	}
	access, err := executionAccessFor(run, resolved, event)
	if err != nil {
		return nil, executionPreparationDenied(err)
	}
	frozenRun, frozenEvent := *run, event
	frozenRun.ConversationID = cloneUUIDPointer(run.ConversationID)
	frozenRun.ActorID = cloneUUIDPointer(run.ActorID)
	frozenRun.PrincipalID = cloneUUIDPointer(run.PrincipalID)
	frozenRun.UserSubjectID = cloneUUIDPointer(run.UserSubjectID)
	frozenEvent.Context = maps.Clone(event.Context)
	if event.ReplyTo != nil {
		reply := *event.ReplyTo
		frozenEvent.ReplyTo = &reply
	}
	if current != nil && access.background {
		return nil, executionPreparationDenied(execution.ErrInvalidIdentity)
	}
	authorizationScope := access.scope()
	if current != nil {
		if !continuationRunOwnedByScope(run, *current, access.application) || current.ActorID != access.user || current.PrincipalID == uuid.Nil ||
			current.PrincipalType != authorizationScope.PrincipalType {
			return nil, executionPreparationDenied(execution.ErrInvalidIdentity)
		}
		if access.surface == SurfaceAPI {
			authorizationScope.PrincipalID = current.PrincipalID
		}
	}
	if !o.Entries.Allows(access.organization.String(), "digital_human", entry) || o.Gateway == nil || o.Files == nil || o.Access == nil || o.ArtifactManager == nil {
		return nil, executionPreparationDenied(execution.ErrInvalidIdentity)
	}
	manager := o.ArtifactManager()
	if manager == nil {
		return nil, executionPreparationDenied(execution.ErrInvalidIdentity)
	}
	approved, err := o.Approvals.Resolve(access.organization.String(), true)
	if err != nil {
		return nil, executionPreparationDenied(err)
	}
	authorize := func(ctx context.Context) error {
		if access.background {
			background, ok := o.Access.(interface {
				AuthorizeBackgroundLinuxExecution(context.Context, *Run, invocation.Event) error
			})
			if !ok {
				return execution.ErrInvalidIdentity
			}
			if err := background.AuthorizeBackgroundLinuxExecution(ctx, &frozenRun, frozenEvent); err != nil {
				return err
			}
			_, err := o.Approvals.Resolve(access.organization.String(), true)
			return err
		}
		if access.surface == SurfaceConsole {
			if o.AuthorizeConsole == nil {
				return execution.ErrInvalidIdentity
			}
			if err := o.AuthorizeConsole(ctx, access.scope()); err != nil {
				return err
			}
		}
		authorizeExecution := o.Access.AuthorizeLinuxExecution
		if current != nil {
			continuationAccess, ok := o.Access.(interface {
				AuthorizeLinuxContinuation(context.Context, Scope, uuid.UUID, uuid.UUID) error
			})
			if !ok {
				return execution.ErrInvalidIdentity
			}
			authorizeExecution = continuationAccess.AuthorizeLinuxContinuation
		}
		if err := authorizeExecution(ctx, authorizationScope, access.application, access.publication); err != nil {
			return err
		}
		_, err := o.Approvals.Resolve(access.organization.String(), true)
		return err
	}
	if err := authorize(ctx); err != nil {
		return nil, executionPreparationDenied(err)
	}
	var skillIDs []string
	for _, binding := range resolved.Snapshot.Bindings {
		if binding.Type == resourcebindings.BindingTypeSkill {
			skillIDs = append(skillIDs, binding.ResourceID)
		}
	}
	profile := o.Approvals.Profiles[approved.ProfileID]
	options := executionruntime.FactoryOptions{
		Gateway: o.Gateway, Entries: o.Entries, EntryReady: o.EntryReady, ResolveScope: access.resolve, Authorize: authorize,
		ProfileID: approved.ProfileID, Backend: profile.Backend, NetworkPolicy: approved.PolicyID, ErrorProjector: o.ErrorProjector,
		Skills:                executionruntime.NewSkillPackageResolver(o.DB, access.organization, skillIDs),
		DependencyPreparation: profile.DependencyPreparation && !profile.DependencyInstallation,
		ResolvePersister: func(prepared *chatruntime.PreparedChat, scope execution.Scope) executionruntime.ArtifactPersister {
			persister, err := executionruntime.NewProtectedArtifactPersister(manager, scope, prepared.Message.ID.String())
			if err != nil {
				return nil
			}
			return withExportArtifacts(persister, o.Artifacts, frozenRun, frozenEvent.Context)
		},
		ResolveFiles: func(prepared *chatruntime.PreparedChat, scope execution.Scope) (*executionruntime.ScopedAttachmentReader, error) {
			return executionruntime.NewScopedAttachmentReader(o.Files, scope, prepared.RuntimeAttachmentRefs(), func(ctx context.Context, file *dto.UploadFile) error {
				if err := authorize(ctx); err != nil {
					return err
				}
				if access.surface == SurfaceConsole {
					if !consoleFileBelongsToScope(file, access.scope()) {
						return ErrConsoleFileNotAccessible
					}
					return nil
				}
				return o.Access.AuthorizeExecutionUpload(ctx, access.scope(), access.application, file)
			})
		},
	}
	// Background channels/tasks have no interactive platform-CLI approval
	// continuation. Do not inherit that capability from an interactive profile.
	if o.PlatformCLI && !access.background {
		options.PlatformCLI, options.InteractiveApproval = true, true
		options.AuthorizePlatform = authorize
	}
	if approved.HomeSnapshotID != nil {
		options.HomeSnapshotID = *approved.HomeSnapshotID
	}
	return executionruntime.NewToolFactory(options), nil
}

func executionPreparationDenied(cause error) error {
	return apperror.Wrap(cause, catalog.CodePermissionDenied, apperror.WithOperation("digital_human.execution.prepare"))
}
