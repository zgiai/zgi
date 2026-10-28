package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/memoryruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

// Person notes (ADR 0015): what a DigitalHuman keeps on an external end user,
// such as its commitments and open loops, lives in that person's
// relationship memory under notes/, not in the DigitalHuman's own memory.
// Inside this package a note still looks like an instance-scope memory, so
// the file view, the tools, and the worker treat it as before; reads load the
// person's notes and instance-scope writes are routed to them here.

const (
	noteExpiryBatch = 200
	// noteReadLimit bounds how many notes one run reads.
	noteReadLimit = 200
)

// noteRelationship returns the relationship that holds a DigitalHuman's notes
// on the person of one end-user instance, creating it when create is set.
func noteRelationship(ctx context.Context, db *gorm.DB, scope Scope, digitalHumanID, instanceID uuid.UUID, create bool) (*relationshipmemory.Service, *relationshipmemory.UserMemoryRelationship, error) {
	var instance EndUserInstance
	if err := db.WithContext(ctx).Where("id = ? AND digital_human_id = ? AND workspace_id = ?", instanceID, digitalHumanID, scope.WorkspaceID).Take(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrMemoryPolicyRejected
		}
		return nil, nil, fmt.Errorf("load end-user instance: %w", err)
	}
	service := relationshipmemory.NewService(db)
	if !create {
		relationship, err := service.FindRelationship(ctx, instance.WorkspaceID, instance.SubjectID, relationshipmemory.ConsumerDigitalHuman, digitalHumanID)
		if errors.Is(err, relationshipmemory.ErrRelationshipNotFound) {
			return service, nil, nil
		}
		return service, relationship, err
	}
	var name []string
	if err := db.WithContext(ctx).Table("digital_humans").Where("id = ?", digitalHumanID).Pluck("name", &name).Error; err != nil {
		return nil, nil, fmt.Errorf("load DigitalHuman name: %w", err)
	}
	// The subject is ensured with the issuer it already has, so the notes
	// share one relationship with the rest of the person's memory.
	request := relationshipmemory.EnsureRelationshipRequest{
		OrganizationID: instance.OrganizationID, WorkspaceID: instance.WorkspaceID,
		Subject:      usersubject.Ref{Kind: usersubject.Kind(instance.SubjectKind), ID: instance.SubjectID},
		ConsumerType: relationshipmemory.ConsumerDigitalHuman, ConsumerID: digitalHumanID,
	}
	if request.Subject.Kind == usersubject.APIEndUser {
		// The subject keeps the issuer it was created with: the
		// DigitalHuman's API or the Runtime Workspace serving it.
		var subject struct {
			IssuerType string
			IssuerID   *uuid.UUID
		}
		if err := db.WithContext(ctx).Table("user_subjects").Select("issuer_type, issuer_id").Where("id = ?", instance.SubjectID).Take(&subject).Error; err != nil {
			return nil, nil, fmt.Errorf("load end-user subject: %w", err)
		}
		request.IssuerType, request.IssuerID = subject.IssuerType, subject.IssuerID
	}
	if len(name) == 1 {
		request.ConsumerNameSnapshot = name[0]
	}
	relationship, err := service.EnsureRelationship(ctx, request)
	return service, relationship, err
}

// instanceNotes reads the person's notes as instance-scope memories.
func (r *GormRepository) instanceNotes(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, instanceID string) ([]*DigitalHumanMemory, error) {
	id, err := uuid.Parse(instanceID)
	if err != nil {
		return nil, nil
	}
	service, relationship, err := noteRelationship(ctx, r.db, scope, digitalHumanID, id, false)
	if err != nil || relationship == nil {
		if errors.Is(err, ErrMemoryPolicyRejected) {
			err = nil
		}
		return nil, err
	}
	items, err := service.Notes(ctx, relationship.ID)
	if err != nil {
		return nil, err
	}
	if len(items) > noteReadLimit {
		items = items[:noteReadLimit]
	}
	notes := make([]*DigitalHumanMemory, 0, len(items))
	for _, item := range items {
		notes = append(notes, noteMemory(item, scope, digitalHumanID, instanceID))
	}
	return notes, nil
}

func noteMemory(item relationshipmemory.UserMemoryItem, scope Scope, digitalHumanID uuid.UUID, instanceID string) *DigitalHumanMemory {
	return &DigitalHumanMemory{
		ID: item.ID, OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: digitalHumanID,
		MemoryKey: relationshipmemory.NoteName(item.MemoryKey), MemoryType: memoryruntime.MemoryType(item.Category), NoteInstanceID: instanceID,
		Content: item.ContentPayload, ContentDigest: memoryContentDigest(item.ContentPayload), Visibility: MemoryVisibilityInternal,
		Origin: MemoryOriginEndUser, Status: MemoryStatusActive, Confidence: 1, Revision: item.Revision,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

// applyInstanceNote writes an instance-scope proposal to the person's notes.
// A new note beyond the instance capacity is rejected, as instance memory
// was.
func (s *Service) applyInstanceNote(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request MemoryProposalRequest) (*MemoryMutationResult, error) {
	instanceID, err := uuid.Parse(request.noteInstanceID)
	if err != nil || request.SupersedesID != nil || !relationshipmemory.ValidNoteName(request.Key) {
		return nil, ErrMemoryPolicyRejected
	}
	service, relationship, err := noteRelationship(ctx, s.repo.db, scope, digitalHumanID, instanceID, true)
	if err != nil {
		return nil, err
	}
	key := relationshipmemory.NoteKey(request.Key)
	sourceKind := request.noteSourceKind
	if sourceKind == "" {
		sourceKind = relationshipmemory.SourceExplicit
	}
	actorType := "model"
	if request.SourceContext.SourceTrust == "" && request.SourceContext.PromotedByAuthorizedActor {
		actorType = "operator"
	}
	metadata := relationshipmemory.MutationMetadata{
		ActorType: actorType, Source: "digital_human", SourceKind: sourceKind, OperationID: request.OperationID,
		EvidenceFromUserInput: true, Reason: "the DigitalHuman's note on this person",
	}
	result := &MemoryMutationResult{Event: &MemoryEvent{OperationID: request.OperationID}}
	if request.Action == memoryruntime.MutationActionClear {
		if _, err := service.DeleteItem(ctx, relationship.ID, key, request.ExpectedRevision, metadata); err != nil {
			return nil, noteError(err)
		}
		result.Memory = &DigitalHumanMemory{MemoryKey: request.Key, NoteInstanceID: request.noteInstanceID, Status: MemoryStatusInvalidated}
		return result, nil
	}
	if limit := instanceMemoryMaxEntries(); limit > 0 && request.ExpectedRevision == 0 {
		notes, err := service.Notes(ctx, relationship.ID)
		if err != nil {
			return nil, err
		}
		if int64(len(notes)) >= limit {
			return nil, errInstanceMemoryFull
		}
	}
	item, err := service.PutItem(ctx, relationship.ID, relationshipmemory.PutItemRequest{
		Key: key, DisplayName: request.Key, Category: strings.ToLower(string(request.MemoryType)), Content: request.Content,
		ExpectedRevision: request.ExpectedRevision, Metadata: metadata,
		Contract: relationshipmemory.ConsumerMemoryContract{Enabled: true, DefinitionMode: "dynamic"},
	})
	if err != nil {
		return nil, noteError(err)
	}
	item.ContentPayload = request.Content
	result.Memory = noteMemory(*item, scope, digitalHumanID, request.noteInstanceID)
	return result, nil
}

// retrieve recalls the DigitalHuman's memory for a query and, for an end
// user's run, adds the notes on them that share the query's words. Notes
// live outside the vector projection (ADR 0015).
func (p *DefaultMemoryRuntimeProvider) retrieve(ctx context.Context, request DigitalHumanMemoryRetrievalRequest) (*DigitalHumanMemoryRetrievalResult, error) {
	result, err := p.retriever.Retrieve(ctx, request)
	if err != nil || result == nil || request.Run == nil {
		return result, err
	}
	readScope := memoryReadScopeForRun(request.Run, request.Event)
	if readScope.instanceID == "" {
		return result, nil
	}
	notes, err := p.service.repo.instanceNotes(ctx, runScope(request.Run), request.Run.DigitalHumanID, readScope.instanceID)
	if err != nil {
		logger.WarnContext(ctx, "digital human notes could not be read for recall", "digital_human_id", request.Run.DigitalHumanID.String(), err)
		return result, nil
	}
	seen := make(map[uuid.UUID]bool, len(result.Items))
	for _, item := range result.Items {
		seen[item.ID] = true
	}
	terms := relationshipmemory.SearchTerms(request.Query)
	for _, note := range notes {
		if seen[note.ID] || !sharesTerm(terms, note.MemoryKey+" "+note.Content) {
			continue
		}
		appendRecalledMemory(result, note, 0, "note")
	}
	return result, nil
}

func sharesTerm(terms []string, text string) bool {
	present := map[string]bool{}
	for _, term := range relationshipmemory.SearchTerms(text) {
		present[term] = true
	}
	for _, term := range terms {
		if present[term] {
			return true
		}
	}
	return false
}

// DeleteNote removes one of a DigitalHuman's notes on a person, for an
// operator managing the end user.
func (s *Service) DeleteNote(ctx context.Context, scope Scope, digitalHumanID, subjectID uuid.UUID, key string, expectedRevision int64) error {
	service, relationship, err := s.noteRelationshipFor(ctx, scope, digitalHumanID, subjectID)
	if err != nil {
		return applicationError(err, "digital_human.memory.note.delete")
	}
	actorID := scope.ActorID
	_, err = service.DeleteItem(ctx, relationship.ID, relationshipmemory.NoteKey(key), expectedRevision,
		relationshipmemory.MutationMetadata{ActorType: "operator", ActorID: &actorID, Source: "runtime_workspace", SourceKind: relationshipmemory.SourceManager})
	if err != nil {
		return applicationError(noteError(err), "digital_human.memory.note.delete")
	}
	return nil
}

// PromoteNote makes a note on a person the DigitalHuman's own memory,
// internal until an operator shows it to everyone (ADR 0015), and removes
// the note. The promotion is recorded on the end user's instance.
func (s *Service) PromoteNote(ctx context.Context, scope Scope, digitalHumanID, subjectID uuid.UUID, key string, expectedRevision int64, operationID uuid.UUID) (*MemoryMutationResult, error) {
	const operation = "digital_human.memory.note.promote"
	if operationID == uuid.Nil || !relationshipmemory.ValidNoteName(key) {
		return nil, applicationError(ErrInvalidRequest, operation)
	}
	service, relationship, err := s.noteRelationshipFor(ctx, scope, digitalHumanID, subjectID)
	if err != nil {
		return nil, applicationError(err, operation)
	}
	note, err := service.Item(ctx, relationship.ID, relationshipmemory.NoteKey(key))
	if err != nil {
		return nil, applicationError(err, operation)
	}
	if note == nil || note.Revision != expectedRevision {
		return nil, applicationError(ErrMemoryConflict, operation)
	}
	state, err := s.memoryState(ctx, scope, digitalHumanID)
	if err != nil {
		return nil, applicationError(err, operation)
	}
	result, err := s.applyMemoryProposal(ctx, scope, digitalHumanID, MemoryProposalRequest{
		OperationID: operationID, Key: key, Action: memoryruntime.MutationActionUpsert,
		MemoryType: memoryruntime.MemoryType(note.Category), Content: note.ContentPayload,
		ConfigRevision: state.ConfigRevision, MemoryEpoch: state.MemoryEpoch, Visibility: MemoryVisibilityInternal,
		// The promoted entry keeps where it came from: an end user's run.
		origin:     MemoryOriginEndUser,
		Evidence:   []memoryruntime.MemoryEvidence{{Type: memoryruntime.EvidenceTypeManual, ID: "operator-promote-note:" + operationID.String()}},
		Confidence: 1, SourceContext: MemorySourceContext{PromotedByAuthorizedActor: true}, AdminApproved: true, OccurredAt: s.now(),
	}, false)
	if err != nil {
		return nil, err
	}
	if !result.Replayed {
		actorID := scope.ActorID
		if _, err := service.DeleteItem(ctx, relationship.ID, note.MemoryKey, note.Revision,
			relationshipmemory.MutationMetadata{ActorType: "operator", ActorID: &actorID, Source: "runtime_workspace", SourceKind: relationshipmemory.SourceManager, Reason: "promoted into the DigitalHuman's memory"}); err != nil {
			return nil, applicationError(noteError(err), operation)
		}
		var instances []EndUserInstance
		if err := s.repo.db.WithContext(ctx).Where("digital_human_id = ? AND subject_id = ?", digitalHumanID, subjectID).Limit(1).Find(&instances).Error; err == nil && len(instances) == 1 {
			details := map[string]any{"note_key": key}
			if result.Memory != nil {
				details["promoted_memory_id"] = result.Memory.ID.String()
			}
			if auditErr := recordInstanceEvent(ctx, s.repo.db, instances[0], instanceAuditMemoryPromoted, accessAuditSourceConsole, &actorID, details); auditErr != nil {
				logger.WarnContext(ctx, "end-user note promotion audit failed", "digital_human_id", digitalHumanID.String(), auditErr)
			}
		}
	}
	return result, nil
}

// noteRelationshipFor finds the relationship holding a DigitalHuman's notes
// on one person.
func (s *Service) noteRelationshipFor(ctx context.Context, scope Scope, digitalHumanID, subjectID uuid.UUID) (*relationshipmemory.Service, *relationshipmemory.UserMemoryRelationship, error) {
	service := relationshipmemory.NewService(s.repo.db)
	relationship, err := service.FindRelationship(ctx, scope.WorkspaceID, subjectID, relationshipmemory.ConsumerDigitalHuman, digitalHumanID)
	if errors.Is(err, relationshipmemory.ErrRelationshipNotFound) {
		return nil, nil, ErrMemoryNotFound
	}
	return service, relationship, err
}

// noteError maps relationship memory errors to DigitalHuman memory errors.
func noteError(err error) error {
	switch {
	case errors.Is(err, relationshipmemory.ErrRevisionConflict):
		return ErrMemoryConflict
	case errors.Is(err, relationshipmemory.ErrPolicyRejected), errors.Is(err, relationshipmemory.ErrAccessDenied), errors.Is(err, relationshipmemory.ErrCapacityExceeded):
		return ErrMemoryPolicyRejected
	}
	return err
}
