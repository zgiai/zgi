package artifacts

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAShareOpensOneVersionUntilItExpiresOrIsRevoked(t *testing.T) {
	service, _ := newTestService(t)
	ctx := t.Context()
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ApplicationType: "digital_human", ApplicationID: uuid.New()}
	owner := Owner{Kind: OwnerAccount, ID: uuid.New()}
	task := uuid.New()
	publish := func(text string) *Published {
		t.Helper()
		published, err := service.Publish(ctx, PublishRequest{Scope: scope, Owner: owner, Visibility: VisibilityOwner, Kind: KindDocument,
			Title: "Plan", Text: text, Source: SourceRun, SourceTaskID: &task})
		if err != nil {
			t.Fatal(err)
		}
		return published
	}
	first := publish("# v1")
	id := first.Artifact.ID
	ownerReader := Reader{Scope: scope, Principal: owner, Member: true}
	actor := Actor{Kind: string(OwnerAccount), ID: owner.ID.String()}

	share, err := service.CreateShare(ctx, ownerReader, id, 0, "", actor)
	if err != nil || share.Token == "" || share.ExpiresAt == nil || share.VersionNumber != 1 {
		t.Fatalf("CreateShare() = %#v, %v", share, err)
	}
	if left := time.Until(*share.ExpiresAt); left < 6*24*time.Hour || left > 7*24*time.Hour+time.Minute {
		t.Fatalf("default lifetime left %s, want seven days", left)
	}
	publish("# v2")
	opened, err := service.OpenShare(ctx, share.Token, true)
	if err != nil || opened.Version.Number != 1 {
		t.Fatalf("OpenShare() = %#v, %v; want the shared version, not the newer one", opened, err)
	}
	data, err := service.SharedContent(ctx, opened)
	if err != nil || string(data) != "# v1" {
		t.Fatalf("SharedContent() = %q, %v", data, err)
	}
	shares, err := service.Shares(ctx, ownerReader, id)
	if err != nil || len(shares) != 1 || shares[0].AccessCount != 1 || shares[0].LastAccessedAt == nil {
		t.Fatalf("Shares() = %#v, %v", shares, err)
	}

	// Another member cannot share an owner-only artifact, even as an operator.
	operator := Reader{Scope: scope, Principal: Owner{Kind: OwnerAccount, ID: uuid.New()}, Member: true, Operator: true}
	if _, err := service.CreateShare(ctx, operator, id, 0, "7d", actor); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an operator shared another's private artifact: %v", err)
	}
	if _, err := service.CreateShare(ctx, ownerReader, id, 0, "forever", actor); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("an unknown lifetime = %v", err)
	}
	never, err := service.CreateShare(ctx, ownerReader, id, 2, "never", actor)
	if err != nil || never.ExpiresAt != nil {
		t.Fatalf("a share that never expires = %#v, %v", never, err)
	}

	if err := service.RevokeShare(ctx, ownerReader, id, share.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenShare(ctx, share.Token, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a revoked share opened: %v", err)
	}
	if _, err := service.OpenShare(ctx, "not-a-token", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an unknown token opened: %v", err)
	}
	if _, err := service.SetArchived(ctx, ownerReader, id, true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenShare(ctx, never.Token, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a share of an archived artifact opened: %v", err)
	}
	service.now = func() time.Time { return time.Now().UTC().Add(8 * 24 * time.Hour) }
	if _, err := service.SetArchived(ctx, ownerReader, id, false); err != nil {
		t.Fatal(err)
	}
	expiring, err := service.CreateShare(ctx, ownerReader, id, 0, "1d", actor)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Now().UTC().Add(10 * 24 * time.Hour) }
	if _, err := service.OpenShare(ctx, expiring.Token, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("an expired share opened: %v", err)
	}
}

func TestStoppingSharingClosesEveryLinkOut(t *testing.T) {
	service, _ := newTestService(t)
	ctx := t.Context()
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ApplicationType: "digital_human", ApplicationID: uuid.New()}
	owner := Owner{Kind: OwnerAccount, ID: uuid.New()}
	published, err := service.Publish(ctx, PublishRequest{Scope: scope, Owner: owner, Visibility: VisibilityTeam, Kind: KindFile, Title: "deck.pptx",
		File:   &Blob{Name: "deck.pptx", MimeType: "application/vnd.openxmlformats-officedocument.presentationml.presentation", Data: []byte("pptx")},
		Source: SourceRun, SourceFileID: "run-file-1"})
	if err != nil {
		t.Fatal(err)
	}
	id := published.Artifact.ID
	ownerReader := Reader{Scope: scope, Principal: owner, Member: true}
	member := Reader{Scope: scope, Principal: Owner{Kind: OwnerAccount, ID: uuid.New()}, Member: true}
	actor := Actor{Kind: string(OwnerAccount), ID: owner.ID.String()}
	share, err := service.CreateShare(ctx, member, id, 0, "7d", actor)
	if err != nil {
		t.Fatalf("a member could not share a team artifact: %v", err)
	}
	if withdrawn, err := service.SourceFileWithdrawn(ctx, "run-file-1"); err != nil || withdrawn {
		t.Fatalf("a shared run file is withdrawn: %v, %v", withdrawn, err)
	}

	if _, err := service.SetSharing(ctx, member, id, false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenShare(ctx, share.Token, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a link opened after sharing stopped: %v", err)
	}
	if withdrawn, _ := service.SourceFileWithdrawn(ctx, "run-file-1"); !withdrawn {
		t.Fatal("a channel link to the run file still works after sharing stopped")
	}
	if _, err := service.CreateShare(ctx, ownerReader, id, 0, "7d", actor); !errors.Is(err, ErrSharingDisabled) {
		t.Fatalf("a link was made while sharing is stopped: %v", err)
	}

	if _, err := service.SetSharing(ctx, ownerReader, id, true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenShare(ctx, share.Token, false); err != nil {
		t.Fatalf("a link did not open after sharing was allowed again: %v", err)
	}
	if _, err := service.SetArchived(ctx, ownerReader, id, true); err != nil {
		t.Fatal(err)
	}
	if withdrawn, _ := service.SourceFileWithdrawn(ctx, "run-file-1"); !withdrawn {
		t.Fatal("a channel link to the run file still works after archiving")
	}
	if withdrawn, _ := service.SourceFileWithdrawn(ctx, "another-file"); withdrawn {
		t.Fatal("an unrelated file is withdrawn")
	}
}
