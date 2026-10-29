package digitalhuman

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	"gorm.io/gorm"
)

// CompositeMemoryRuntimeProvider keeps DigitalHuman-owned memory and
// user-owned relationship memory as separate providers and storage domains.
type CompositeMemoryRuntimeProvider struct {
	providers []MemoryRuntimeProvider
}

func NewCompositeMemoryRuntimeProvider(providers ...MemoryRuntimeProvider) *CompositeMemoryRuntimeProvider {
	filtered := make([]MemoryRuntimeProvider, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			filtered = append(filtered, provider)
		}
	}
	return &CompositeMemoryRuntimeProvider{providers: filtered}
}

func (p *CompositeMemoryRuntimeProvider) Prepare(ctx context.Context, run *Run, resolved *ResolvedExecution, event invocation.Event) (*MemoryRuntimePreparation, error) {
	result := &MemoryRuntimePreparation{}
	for _, provider := range p.providers {
		prepared, err := provider.Prepare(ctx, run, resolved, event)
		if err != nil {
			return nil, err
		}
		if prepared == nil {
			continue
		}
		result.StableGuidance = joinGuidance(result.StableGuidance, prepared.StableGuidance)
		result.SystemPromptAddendum = joinGuidance(result.SystemPromptAddendum, prepared.SystemPromptAddendum)
		result.Tools = append(result.Tools, prepared.Tools...)
		result.fileEditors = append(result.fileEditors, prepared.fileEditors...)
		if prepared.archiveSearch != nil {
			result.archiveSearch = prepared.archiveSearch
		}
		if prepared.forgottenLines != nil {
			result.forgottenLines = prepared.forgottenLines
		}
	}
	if result.archiveSearch != nil {
		result.Tools = withArchiveResults(result.Tools, result.archiveSearch)
	}
	if result.forgottenLines != nil {
		result.Tools = withoutForgottenTurns(result.Tools, result.forgottenLines)
	}
	if len(result.fileEditors) > 0 {
		result.StableGuidance = joinGuidance(result.StableGuidance, memoryEditGuidance(result.fileEditors))
		result.Tools = append(result.Tools, newMemoryEditTool(run, result.fileEditors))
	}
	return result, nil
}

func (p *CompositeMemoryRuntimeProvider) Finalize(ctx context.Context, run *Run, prepared *chatruntime.PreparedChat, result *chatruntime.ChatResult) error {
	var combined error
	for _, provider := range p.providers {
		if finalizer, ok := provider.(memoryRuntimeFinalizer); ok {
			combined = errors.Join(combined, finalizer.Finalize(ctx, run, prepared, result))
		}
	}
	return combined
}

// RelationshipMemoryRuntimeProvider exposes only current-user relationship
// data. Channels, schedules, and anonymous hosted-page sessions stay excluded.
type RelationshipMemoryRuntimeProvider struct {
	service *relationshipmemory.Service
}

func NewRelationshipMemoryRuntimeProvider(db *gorm.DB) *RelationshipMemoryRuntimeProvider {
	if db == nil {
		panic("DigitalHuman relationship memory runtime requires db")
	}
	return &RelationshipMemoryRuntimeProvider{service: relationshipmemory.NewService(db)}
}

func (p *RelationshipMemoryRuntimeProvider) Prepare(ctx context.Context, run *Run, resolved *ResolvedExecution, event invocation.Event) (*MemoryRuntimePreparation, error) {
	if run == nil || resolved == nil || !resolved.Snapshot.UseUserMemory {
		return nil, nil
	}
	identity := relationshipmemory.InvocationIdentity{
		Principal:        relationshipmemory.PrincipalRef{Type: string(event.Sender.Type)},
		Surface:          relationshipMemorySurfaceForRun(run),
		ConversationType: relationshipmemory.ConversationPrivate,
	}
	if identity.Surface == relationshipmemory.SurfaceDigitalHumanTask {
		identity.ConversationType = relationshipmemory.ConversationTask
	}
	if run.PrincipalID != nil {
		identity.Principal.ID = *run.PrincipalID
	}
	if run.UserSubjectID != nil && *run.UserSubjectID != uuid.Nil {
		subject := usersubject.Ref{Kind: usersubject.Kind(run.UserSubjectKind), ID: *run.UserSubjectID}
		identity.UserSubject = &subject
	}
	decision := relationshipmemory.DecideAccess(identity)
	relationshipmemory.RecordAccessDecision(ctx, relationshipmemory.ConsumerDigitalHuman, identity, decision)
	// Policy is rechecked at the product edge so an installation owner can
	// never become a channel end-user by accident.
	if !decision.Read || identity.UserSubject == nil {
		return nil, nil
	}
	ensureRequest := relationshipmemory.EnsureRelationshipRequest{
		OrganizationID:       run.OrganizationID,
		WorkspaceID:          run.WorkspaceID,
		Subject:              *identity.UserSubject,
		ConsumerType:         relationshipmemory.ConsumerDigitalHuman,
		ConsumerID:           run.DigitalHumanID,
		ConsumerNameSnapshot: resolved.Snapshot.IdentityName,
	}
	if identity.UserSubject.Kind == usersubject.APIEndUser {
		issuerID := run.DigitalHumanID
		ensureRequest.IssuerType = digitalHumanAPIIssuerType
		ensureRequest.IssuerID = &issuerID
	}
	relationship, err := p.service.EnsureRelationship(ctx, ensureRequest)
	if err != nil {
		return nil, err
	}
	// The person's memory is files they own, as with an Agent (ADR 0019):
	// named by topic and bounded in total, shown and changed under /user/.
	contract := relationshipmemory.ConsumerMemoryContract{
		Enabled:            true,
		DefinitionMode:     "files",
		AutoCaptureAllowed: resolved.Snapshot.UserMemoryAutoCaptureAllowed && relationshipmemory.AutomaticExtractionEnabled(),
		ConfigRevision:     resolved.Snapshot.Revision,
	}
	writable := relationship.Status == relationshipmemory.RelationshipActive && relationship.MemoryEnabled
	var items []relationshipmemory.UserMemoryItem
	if writable {
		if items, err = p.service.MemoryFiles(ctx, relationship.ID); err != nil {
			return nil, err
		}
	}
	preparation := &MemoryRuntimePreparation{
		StableGuidance:       renderUserMemoryRules(relationship.AutoCaptureEnabled && contract.AutoCaptureAllowed),
		SystemPromptAddendum: renderUserMemoryFilesSection(items),
	}
	if writable {
		preparation.fileEditors = []memoryFileEditor{&relationshipFileEditor{service: p.service, run: run, relationship: relationship, contract: contract}}
	}
	relationshipID := relationship.ID
	preparation.archiveSearch = func(ctx context.Context, query string, limit int) ([]relationshipmemory.UserMemoryArchiveEntry, error) {
		return p.service.SearchArchive(ctx, relationshipID, query, limit)
	}
	preparation.forgottenLines = func(ctx context.Context) ([]string, error) {
		return p.service.ForgottenLines(ctx, relationshipID)
	}
	return preparation, nil
}

func relationshipMemorySurfaceForRun(run *Run) relationshipmemory.MemorySurface {
	if run == nil {
		return relationshipmemory.SurfaceChannel
	}
	switch strings.TrimSpace(run.Surface) {
	case SurfaceConsole:
		return relationshipmemory.SurfaceDigitalHumanConsole
	case SurfaceWebApp:
		return relationshipmemory.SurfaceDigitalHumanWebApp
	case SurfaceAPI:
		return relationshipmemory.SurfaceDigitalHumanAPI
	case SurfaceTask:
		return relationshipmemory.SurfaceDigitalHumanTask
	default:
		return relationshipmemory.SurfaceChannel
	}
}

// renderUserMemoryFilesGuidance is the file view of the user's own memory.
// renderUserMemoryRules is how the DigitalHuman treats the person's own
// memory. It stays the same from run to run and goes in the system prompt.
func renderUserMemoryRules(proactiveAllowed bool) string {
	mode := "Change /user/ files only when the person directly asks you to remember, correct, or forget something about themselves."
	if proactiveAllowed {
		mode = "You may also add to /user/ a stable fact or preference the person states about themselves, without being asked. Use mode explicit only when they asked you to remember, correct, or forget it; anything you save on your own uses mode proactive, so they can tell it apart and undo it."
	}
	return strings.Join([]string{
		"Files under /user/ are the person's own memory: facts about them that they own. They are separate from your memory and your notes, and nothing is ever copied between the two.",
		mode,
		"Never store project, customer, channel, credential, third-party, or document data under /user/.",
	}, "\n\n")
}

// renderUserMemoryFilesSection shows the person's files, which change from
// run to run.
func renderUserMemoryFilesSection(items []relationshipmemory.UserMemoryItem) string {
	if len(items) > 0 {
		return renderUserMemoryFiles(items)
	}
	return "The person has no files under /user/ yet."
}

func joinGuidance(current, next string) string {
	current, next = strings.TrimSpace(current), strings.TrimSpace(next)
	switch {
	case next == "":
		return current
	case current == "":
		return next
	default:
		return current + "\n\n" + next
	}
}

func sourceKindForRelationshipMode(mode string) string {
	if mode == "proactive" {
		return relationshipmemory.SourceAutomatic
	}
	return relationshipmemory.SourceExplicit
}
