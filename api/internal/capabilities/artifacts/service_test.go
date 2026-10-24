package artifacts

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type memoryBlobs map[string][]byte

func (m memoryBlobs) Store(_ context.Context, _ uuid.UUID, _ string, _ string, data []byte) (string, error) {
	id := uuid.NewString()
	m[id] = append([]byte(nil), data...)
	return id, nil
}

func (m memoryBlobs) Read(_ context.Context, _ uuid.UUID, id string) ([]byte, error) {
	data, ok := m[id]
	if !ok {
		return nil, errors.New("missing blob")
	}
	return data, nil
}

func newTestService(t *testing.T) (*Service, memoryBlobs) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&Artifact{}, &Version{}, &Share{}); err != nil {
		t.Fatal(err)
	}
	blobs := memoryBlobs{}
	return NewService(db, blobs), blobs
}

func TestRepeatedPublishesBecomeVersions(t *testing.T) {
	service, blobs := newTestService(t)
	ctx := t.Context()
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ApplicationType: "digital_human", ApplicationID: uuid.New()}
	owner := Owner{Kind: OwnerAccount, ID: uuid.New()}
	task := uuid.New()
	publish := func(text string) *Published {
		t.Helper()
		published, err := service.Publish(ctx, PublishRequest{Scope: scope, Owner: owner, Visibility: VisibilityOwner, Kind: KindDocument,
			Title: "Weekly report", Text: text, Source: SourceRun, SourceTaskID: &task})
		if err != nil {
			t.Fatal(err)
		}
		return published
	}
	first := publish("# Week 1")
	second := publish("# Week 2")
	same := publish("# Week 2")
	if first.Artifact.ID != second.Artifact.ID || second.Version.Number != 2 || same.Created || same.Version.ID != second.Version.ID {
		t.Fatalf("versions: first %v, second %v (#%d), repeat created=%v", first.Artifact.ID, second.Artifact.ID, second.Version.Number, same.Created)
	}
	reader := Reader{Scope: scope, Principal: owner}
	_, version, content, err := service.Content(ctx, reader, first.Artifact.ID, 1)
	if err != nil || version.Number != 1 || string(content) != "# Week 1" {
		t.Fatalf("version 1 = %v, %q, %v", version, content, err)
	}

	file, err := service.Publish(ctx, PublishRequest{Scope: scope, Owner: owner, Visibility: VisibilityOwner, Kind: KindImage, Title: "Chart",
		File: &Blob{Name: "chart.png", MimeType: "image/png", Data: []byte("png")}})
	if err != nil || file.Version.FileID == nil || len(blobs) != 1 {
		t.Fatalf("image = %#v, %v", file, err)
	}
	if _, _, data, err := service.Content(ctx, reader, file.Artifact.ID, 0); err != nil || string(data) != "png" {
		t.Fatalf("image content = %q, %v", data, err)
	}
	for name, req := range map[string]PublishRequest{
		"image that is not an image": {Kind: KindImage, Title: "x", File: &Blob{Name: "a.txt", MimeType: "text/plain", Data: []byte("a")}},
		"empty document":             {Kind: KindDocument, Title: "x"},
		"end user's team artifact":   {Kind: KindDocument, Title: "x", Text: "y", Owner: Owner{Kind: OwnerAPIEndUser, ID: uuid.New()}, Visibility: VisibilityTeam},
		"another owner's artifact":   {Kind: KindDocument, Title: "x", Text: "y", ArtifactID: &first.Artifact.ID, Owner: Owner{Kind: OwnerAccount, ID: uuid.New()}},
	} {
		req.Scope = scope
		if req.Owner.ID == uuid.Nil {
			req.Owner = owner
		}
		if req.Visibility == "" {
			req.Visibility = VisibilityOwner
		}
		if _, err := service.Publish(ctx, req); err == nil {
			t.Fatalf("%s was published", name)
		}
	}
}

func TestReadersSeeOnlyWhatTheirRoleAllows(t *testing.T) {
	service, _ := newTestService(t)
	ctx := t.Context()
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ApplicationType: "digital_human", ApplicationID: uuid.New()}
	member, colleague := Owner{Kind: OwnerAccount, ID: uuid.New()}, Owner{Kind: OwnerAccount, ID: uuid.New()}
	alice, bob := Owner{Kind: OwnerAPIEndUser, ID: uuid.New()}, Owner{Kind: OwnerAPIEndUser, ID: uuid.New()}
	publish := func(owner Owner, visibility Visibility, title string) {
		t.Helper()
		if _, err := service.Publish(ctx, PublishRequest{Scope: scope, Owner: owner, Visibility: visibility, Kind: KindDocument, Title: title, Text: title}); err != nil {
			t.Fatal(err)
		}
	}
	publish(member, VisibilityOwner, "member private")
	publish(colleague, VisibilityTeam, "team")
	publish(alice, VisibilityOwner, "alice")
	publish(bob, VisibilityOwner, "bob")
	publish(Owner{Kind: OwnerApplication, ID: scope.ApplicationID}, VisibilityApplication, "public")
	// Another application's artifact is never visible.
	other := scope
	other.ApplicationID = uuid.New()
	if _, err := service.Publish(ctx, PublishRequest{Scope: other, Owner: member, Visibility: VisibilityTeam, Kind: KindDocument, Title: "elsewhere", Text: "x"}); err != nil {
		t.Fatal(err)
	}

	titles := func(reader Reader) map[string]bool {
		t.Helper()
		result, err := service.List(ctx, reader, ListQuery{})
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, item := range result.Items {
			seen[item.Title] = true
		}
		return seen
	}
	expect := func(label string, got map[string]bool, want ...string) {
		t.Helper()
		if len(got) != len(want) {
			t.Fatalf("%s sees %v, want %v", label, got, want)
		}
		for _, title := range want {
			if !got[title] {
				t.Fatalf("%s sees %v, want %v", label, got, want)
			}
		}
	}
	expect("alice", titles(Reader{Scope: scope, Principal: alice}), "alice", "public")
	expect("member", titles(Reader{Scope: scope, Principal: member, Member: true}), "member private", "team", "public")
	expect("operator", titles(Reader{Scope: scope, Principal: colleague, Member: true, Operator: true}), "member private", "team", "alice", "bob", "public")
	expect("operator in privacy mode", titles(Reader{Scope: scope, Principal: colleague, Member: true, Operator: true, Privacy: true}), "member private", "team", "public")
}
