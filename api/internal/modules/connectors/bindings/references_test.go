package bindings

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestReferenceDirectMutationIsChatOnly(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	authority := &recordingTargetAuthority{}
	service := NewReferenceService(repository.db, authority)
	for _, kind := range []core.TargetType{core.TargetTypeAgent, core.TargetTypeWorkflow} {
		_, err := service.ResolveChat(context.Background(), uuid.New(), uuid.New(), TargetIdentity{Type: kind, ID: uuid.NewString(), Scope: core.ScopeDraft}, ReferenceCommand{ReferenceID: uuid.New(), ExpectedRevision: 1, Action: "detach"})
		if !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
			t.Fatalf("non-Chat direct repair was accepted: %v", err)
		}
	}
	if len(authority.calls) != 0 {
		t.Fatal("direct mutation attempted host access")
	}
	assertMock(t, mock)
}

func TestPublishedReferenceCannotBeChangedInsideHostTransaction(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	_, err := ResolveReferenceInTransaction(context.Background(), repository.db, uuid.New(), uuid.New(), TargetIdentity{Type: core.TargetTypeAgent, ID: uuid.NewString(), Scope: core.ScopePublished, VersionID: uuid.NewString()}, ReferenceCommand{ReferenceID: uuid.New(), ExpectedRevision: 1, Action: "detach"})
	if !core.IsErrorCode(err, core.ErrorCodeInvalidInput) {
		t.Fatalf("published repair accepted: %v", err)
	}
	assertMock(t, mock)
}

func TestReferenceRevisionConflictRollsBackWithoutChangingBinding(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	org, owner, host, ref := uuid.New(), uuid.New(), uuid.NewString(), uuid.New()
	authority := &recordingTargetAuthority{}
	service := NewReferenceService(repository.db, authority)
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT \* FROM "connector_unavailable_references" WHERE \(organization_id = .*owner_account_id = .*target_type = .*target_id = .*target_scope = .*target_version_id = .*\).*FOR UPDATE`).WithArgs(org, owner, core.TargetTypeChat, host, core.ScopeLive, "", ref, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "owner_account_id", "target_type", "target_id", "target_scope", "target_version_id", "revision", "status"}).AddRow(ref, org, owner, "chat", host, "live", "", 3, "needs_replacement"))
	mock.ExpectRollback()
	_, err := service.ResolveChat(context.Background(), org, owner, TargetIdentity{Type: core.TargetTypeChat, ID: host, Scope: core.ScopeLive}, ReferenceCommand{ReferenceID: ref, ExpectedRevision: 2, Action: "detach"})
	if !core.IsErrorCode(err, core.ErrorCodeConflict) {
		t.Fatalf("stale revision: %v", err)
	}
	assertMock(t, mock)
}
