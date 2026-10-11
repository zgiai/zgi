package agentexecution

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestUploadOwnershipIsCompleteAndCannotBeAdopted(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&UploadBinding{}); err != nil {
		t.Fatal(err)
	}
	owner := UploadOwner{OrganizationID: uuid.NewString(), WorkspaceID: uuid.NewString(), ApplicationType: "agent",
		ApplicationID: uuid.NewString(), Entry: "webapp", SubjectKind: "account", SubjectID: uuid.NewString()}
	fileID := uuid.NewString()
	if err := BindUpload(t.Context(), db, fileID, owner); err != nil {
		t.Fatal(err)
	}
	if err := AuthorizeUpload(t.Context(), db, fileID, owner); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*UploadOwner){
		func(o *UploadOwner) { o.OrganizationID = uuid.NewString() },
		func(o *UploadOwner) { o.WorkspaceID = uuid.NewString() },
		func(o *UploadOwner) { o.ApplicationID = uuid.NewString() },
		func(o *UploadOwner) { o.ApplicationType = "digital_human" },
		func(o *UploadOwner) { o.SubjectID = uuid.NewString() },
		func(o *UploadOwner) { o.Entry, o.SubjectKind = "external-api", "api_end_user" },
		func(o *UploadOwner) { o.SubjectKind = "api_key" },
		func(o *UploadOwner) { o.Entry = "console" },
	} {
		other := owner
		change(&other)
		if err := AuthorizeUpload(t.Context(), db, fileID, other); err == nil {
			t.Fatalf("cross-scope upload accepted: %+v", other)
		}
		if err := BindUpload(t.Context(), db, fileID, other); err == nil {
			t.Fatalf("upload rebound: %+v", other)
		}
	}
	if err := AuthorizeUpload(t.Context(), db, uuid.NewString(), owner); err == nil {
		t.Fatal("historical unbound file was adopted")
	}
	if err := AuthorizeUpload(t.Context(), nil, fileID, owner); err == nil {
		t.Fatal("missing ownership store accepted")
	}
	if err := AuthorizeUpload(t.Context(), db, fileID, owner); err != nil {
		t.Fatalf("owner changed: %v", err)
	}
}
