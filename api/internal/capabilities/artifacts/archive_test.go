package artifacts

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestConversationFilesVersionAndSameContentIsKeptOnce(t *testing.T) {
	service, _ := newTestService(t)
	ctx := t.Context()
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ApplicationType: "digital_human", ApplicationID: uuid.New()}
	owner := Owner{Kind: OwnerAccount, ID: uuid.New()}
	conversation := uuid.New()
	publish := func(title string, data string, conversationID *uuid.UUID) *Published {
		t.Helper()
		published, err := service.Publish(ctx, PublishRequest{Scope: scope, Owner: owner, Visibility: VisibilityOwner, Kind: KindFile, Title: title,
			File: &Blob{Name: title, MimeType: "application/pdf", Data: []byte(data)}, Source: SourceRun, SourceConversationID: conversationID})
		if err != nil {
			t.Fatal(err)
		}
		return published
	}
	first := publish("report.pdf", "v1", &conversation)
	second := publish("report.pdf", "v2", &conversation)
	if first.Artifact.ID != second.Artifact.ID || second.Version.Number != 2 {
		t.Fatalf("a conversation's file of the same name must version: %+v %+v", first.Artifact, second.Artifact)
	}
	other := publish("report.pdf", "v1", nil)
	if !other.Created || other.Artifact.ID == first.Artifact.ID {
		t.Fatalf("content that is not current anywhere must be kept as new: %+v", other)
	}
	renamed := publish("Quarterly report", "v2", nil)
	if renamed.Created || renamed.Artifact.ID != first.Artifact.ID {
		t.Fatalf("content the owner already keeps must not be kept twice: %+v", renamed)
	}
	stranger, err := service.Publish(ctx, PublishRequest{Scope: scope, Owner: Owner{Kind: OwnerAccount, ID: uuid.New()}, Visibility: VisibilityOwner, Kind: KindFile,
		Title: "report.pdf", File: &Blob{Name: "report.pdf", MimeType: "application/pdf", Data: []byte("v2")}, SourceConversationID: &conversation})
	if err != nil || !stranger.Created || stranger.Artifact.ID == first.Artifact.ID {
		t.Fatalf("another owner's publish must never join this owner's artifact: %+v %v", stranger, err)
	}
}

func TestArchivedArtifactsLeaveTheListAndComeBackWhenPublished(t *testing.T) {
	service, _ := newTestService(t)
	ctx := t.Context()
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ApplicationType: "digital_human", ApplicationID: uuid.New()}
	owner := Owner{Kind: OwnerAccount, ID: uuid.New()}
	task := uuid.New()
	publish := func(text string) *Published {
		t.Helper()
		published, err := service.Publish(ctx, PublishRequest{Scope: scope, Owner: owner, Visibility: VisibilityTeam, Kind: KindDocument,
			Title: "Plan", Text: text, SourceTaskID: &task})
		if err != nil {
			t.Fatal(err)
		}
		return published
	}
	first := publish("# One")
	reader := Reader{Scope: scope, Principal: owner, Member: true}
	colleague := Reader{Scope: scope, Principal: Owner{Kind: OwnerAccount, ID: uuid.New()}, Member: true}
	if _, err := service.SetArchived(ctx, colleague, first.Artifact.ID, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a member must not archive a colleague's artifact: %v", err)
	}
	archived, err := service.SetArchived(ctx, reader, first.Artifact.ID, true)
	if err != nil || archived.ArchivedAt == nil {
		t.Fatalf("archive: %+v %v", archived, err)
	}
	count := func(filter ArchiveFilter) int64 {
		t.Helper()
		result, err := service.List(ctx, reader, ListQuery{Archive: filter})
		if err != nil {
			t.Fatal(err)
		}
		return result.Total
	}
	if count(ArchiveActive) != 0 || count(ArchiveOnly) != 1 || count(ArchiveAll) != 1 {
		t.Fatalf("archived artifacts must leave only the default listing")
	}
	if _, _, data, err := service.Content(ctx, reader, first.Artifact.ID, 0); err != nil || string(data) != "# One" {
		t.Fatalf("an archived artifact stays readable: %q %v", data, err)
	}
	if again := publish("# One"); again.Created || again.Artifact.ArchivedAt != nil {
		t.Fatalf("publishing the same content again must restore it: %+v", again.Artifact)
	}
	if _, err := service.SetArchived(ctx, reader, first.Artifact.ID, true); err != nil {
		t.Fatal(err)
	}
	if next := publish("# Two"); !next.Created || next.Version.Number != 2 || count(ArchiveActive) != 1 {
		t.Fatalf("a new version must restore an archived artifact: %+v", next.Artifact)
	}
	operator := Reader{Scope: scope, Principal: Owner{Kind: OwnerAccount, ID: uuid.New()}, Member: true, Operator: true}
	if _, err := service.SetArchived(ctx, operator, first.Artifact.ID, true); err != nil {
		t.Fatalf("an operator may archive: %v", err)
	}
	restored, err := service.SetArchived(ctx, reader, first.Artifact.ID, false)
	if err != nil || restored.ArchivedAt != nil {
		t.Fatalf("restore: %+v %v", restored, err)
	}
}

func TestArchivingKeepsTheUpdateTime(t *testing.T) {
	service, _ := newTestService(t)
	ctx := t.Context()
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ApplicationType: "digital_human", ApplicationID: uuid.New()}
	owner := Owner{Kind: OwnerAccount, ID: uuid.New()}
	published, err := service.Publish(ctx, PublishRequest{Scope: scope, Owner: owner, Visibility: VisibilityOwner, Kind: KindDocument, Title: "Notes", Text: "# Notes"})
	if err != nil {
		t.Fatal(err)
	}
	reader := Reader{Scope: scope, Principal: owner, Member: true}
	if _, err := service.SetArchived(ctx, reader, published.Artifact.ID, true); err != nil {
		t.Fatal(err)
	}
	got, err := service.Get(ctx, reader, published.Artifact.ID)
	if err != nil || !got.UpdatedAt.Equal(published.Artifact.UpdatedAt) || got.ArchivedAt == nil {
		t.Fatalf("archiving changed the update time: %v -> %v (%v)", published.Artifact.UpdatedAt, got.UpdatedAt, err)
	}
}
