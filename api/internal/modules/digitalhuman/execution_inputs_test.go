package digitalhuman

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestExecutionUploadsRecheckDigitalHumanBindingAndExplicitSubject(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&ExternalFileBinding{}); err != nil {
		t.Fatal(err)
	}
	service := NewAccessService(db)
	for _, surface := range []string{SurfaceAPI, SurfaceWebApp} {
		t.Run(surface, func(t *testing.T) {
			user, app, fileID := uuid.New(), uuid.New(), uuid.New()
			scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: user, UserSubjectID: &user, Surface: surface}
			role := dto.CreatedByRoleAccount
			scope.UserSubjectKind = "account"
			if surface == SurfaceAPI {
				role, scope.UserSubjectKind = dto.CreatedByRoleEndUser, "api_end_user"
			}
			if err := service.BindExternalFile(t.Context(), scope, app, fileID, surface); err != nil {
				t.Fatal(err)
			}
			file := &dto.UploadFile{ID: fileID.String(), IsTemporary: true, CreatedBy: user.String(), CreatedByRole: role}
			if err := service.AuthorizeExecutionUpload(t.Context(), scope, app, file); err != nil {
				t.Fatal(err)
			}
			for _, change := range []func(*Scope){
				func(s *Scope) { s.OrganizationID = uuid.New() },
				func(s *Scope) { s.WorkspaceID = uuid.New() },
				func(s *Scope) { other := uuid.New(); s.UserSubjectID, s.ActorID = &other, other },
				func(s *Scope) { s.UserSubjectID = nil },
				func(s *Scope) { s.ActorID = uuid.New() },
				func(s *Scope) { s.Surface = SurfaceTask },
			} {
				other := scope
				change(&other)
				if err := service.AuthorizeExecutionUpload(t.Context(), other, app, file); err == nil {
					t.Fatalf("cross-scope upload=%+v", other)
				}
			}
			if err := service.AuthorizeExecutionUpload(t.Context(), scope, uuid.New(), file); err == nil {
				t.Fatal("cross-application upload accepted")
			}
			if err := db.Delete(&ExternalFileBinding{}, "file_id = ?", fileID).Error; err != nil {
				t.Fatal(err)
			}
			if err := service.AuthorizeExecutionUpload(t.Context(), scope, app, file); err == nil {
				t.Fatal("revoked binding still readable")
			}
		})
	}
}
