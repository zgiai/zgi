package digitalhuman

import (
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestValidateExternalFilesEnforcesConsumerSubjectAndSurface(t *testing.T) {
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	service := NewAccessService(db)
	subjectID := uuid.New()
	scope := Scope{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), UserSubjectKind: "account",
		UserSubjectID: &subjectID, Surface: SurfaceWebApp,
	}
	files := []string{uuid.NewString(), uuid.NewString()}
	query := `SELECT count\(\*\) FROM "digital_human_external_files".*file_id IN.*digital_human_id.*organization_id.*workspace_id.*user_subject_kind.*user_subject_id.*surface`
	mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	if err := service.ValidateExternalFiles(t.Context(), scope, uuid.New(), files, SurfaceWebApp); err != nil {
		t.Fatalf("ValidateExternalFiles(valid) error = %v", err)
	}

	mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	if err := service.ValidateExternalFiles(t.Context(), scope, uuid.New(), files, SurfaceWebApp); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ValidateExternalFiles(other consumer) error = %v, want ErrNotFound", err)
	}

	if err := service.ValidateExternalFiles(t.Context(), scope, uuid.New(), files, SurfaceAPI); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ValidateExternalFiles(other surface) error = %v, want ErrNotFound", err)
	}
	if err := service.ValidateExternalFiles(t.Context(), scope, uuid.New(), []string{files[0], files[0]}, SurfaceWebApp); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("ValidateExternalFiles(duplicate) error = %v, want ErrInvalidRequest", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBindExternalFileRequiresUserSubjectAndMatchingSurface(t *testing.T) {
	service := &AccessService{}
	scope := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), Surface: SurfaceWebApp}
	if err := service.BindExternalFile(t.Context(), scope, uuid.New(), uuid.New(), SurfaceWebApp); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("BindExternalFile(no subject) error = %v, want ErrInvalidRequest", err)
	}
	subjectID := uuid.New()
	scope.UserSubjectID = &subjectID
	scope.UserSubjectKind = "account"
	if err := service.BindExternalFile(t.Context(), scope, uuid.New(), uuid.New(), SurfaceAPI); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("BindExternalFile(surface mismatch) error = %v, want ErrInvalidRequest", err)
	}
}

func TestValidateExternalFilesAsksAnonymousWebAppVisitorToSignIn(t *testing.T) {
	service := &AccessService{}
	files := []string{uuid.NewString()}
	webApp := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), Surface: SurfaceWebApp}
	if err := service.ValidateExternalFiles(t.Context(), webApp, uuid.New(), files, SurfaceWebApp); !errors.Is(err, ErrWebAppFileRequiresLogin) {
		t.Fatalf("ValidateExternalFiles(anonymous webapp) error = %v, want ErrWebAppFileRequiresLogin", err)
	}
	if err := service.ValidateExternalFiles(t.Context(), webApp, uuid.New(), nil, SurfaceWebApp); err != nil {
		t.Fatalf("ValidateExternalFiles(anonymous webapp, no files) error = %v", err)
	}
	api := Scope{OrganizationID: uuid.New(), WorkspaceID: uuid.New(), Surface: SurfaceAPI}
	if err := service.ValidateExternalFiles(t.Context(), api, uuid.New(), files, SurfaceAPI); !errors.Is(err, ErrNotFound) {
		t.Fatalf("ValidateExternalFiles(api without subject) error = %v, want ErrNotFound", err)
	}
}
