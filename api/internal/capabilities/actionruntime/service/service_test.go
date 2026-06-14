package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	actiondto "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/dto"
	actionmodel "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/model"
	"github.com/zgiai/zgi/api/internal/dto"
	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
)

func TestPlanActionRequiresConfirmationForElevatedRisk(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, NewRegistry([]CapabilityManifest{{
		ID:        "test.low",
		Name:      "Low risk test",
		Runtime:   RuntimeInternal,
		AuthMode:  AuthModeActorContext,
		RiskLevel: actionmodel.RiskLevelLow,
	}}))
	scope := fakeScope()

	view, err := svc.PlanAction(context.Background(), scope, actiondto.ActionPlanRequest{
		CapabilityID: "test.low",
		RiskLevel:    actionmodel.RiskLevelMedium,
	})
	if err != nil {
		t.Fatalf("PlanAction: %v", err)
	}
	if view.Run.Status != actionmodel.ActionRunStatusNeedsConfirmation {
		t.Fatalf("status = %q, want %q", view.Run.Status, actionmodel.ActionRunStatusNeedsConfirmation)
	}
	if !view.Run.RequiresConfirmation {
		t.Fatal("RequiresConfirmation = false, want true")
	}
	if view.Run.RiskLevel != actionmodel.RiskLevelMedium {
		t.Fatalf("risk = %q, want medium", view.Run.RiskLevel)
	}
	if got := view.Run.Ledger["version"]; got != ledgerVersion {
		t.Fatalf("ledger version = %#v, want %q", got, ledgerVersion)
	}
}

func TestExecuteActionRequiresConfirmation(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, NewDefaultRegistry())
	scope := fakeScope()
	view, err := svc.PlanAction(context.Background(), scope, actiondto.ActionPlanRequest{CapabilityID: "agent.publish"})
	if err != nil {
		t.Fatalf("PlanAction: %v", err)
	}

	_, err = svc.ExecuteAction(context.Background(), scope, view.Run.ID, actiondto.ExecuteActionRequest{})
	if !errors.Is(err, ErrConfirmationRequired) {
		t.Fatalf("ExecuteAction error = %v, want ErrConfirmationRequired", err)
	}
}

func TestConfirmThenExecuteBlocksUntilAdapterConnected(t *testing.T) {
	repo := newFakeRepository()
	svc := NewService(repo, NewDefaultRegistry())
	scope := fakeScope()
	view, err := svc.PlanAction(context.Background(), scope, actiondto.ActionPlanRequest{CapabilityID: "agent.publish"})
	if err != nil {
		t.Fatalf("PlanAction: %v", err)
	}
	if _, err := svc.ConfirmAction(context.Background(), scope, view.Run.ID, actiondto.ConfirmActionRequest{Confirmed: true}); err != nil {
		t.Fatalf("ConfirmAction: %v", err)
	}

	executed, err := svc.ExecuteAction(context.Background(), scope, view.Run.ID, actiondto.ExecuteActionRequest{})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if executed.Run.Status != actionmodel.ActionRunStatusBlocked {
		t.Fatalf("status = %q, want blocked", executed.Run.Status)
	}
	if executed.Run.Error == nil || *executed.Run.Error != adapterPendingMessage {
		t.Fatalf("error = %#v, want adapter pending message", executed.Run.Error)
	}
	if len(executed.Steps) != 1 || executed.Steps[0].Status != actionmodel.ActionStepStatusBlocked {
		t.Fatalf("step status = %#v, want one blocked step", executed.Steps)
	}
}

func TestExecuteFileReadCompletesWithPreview(t *testing.T) {
	repo := newFakeRepository()
	scope := fakeScope()
	workspaceID := uuid.NewString()
	fileService := &fakeFileReadService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: scope.OrganizationID.String(),
				WorkspaceID:    &workspaceID,
				Name:           "notes.txt",
				Size:           11,
				Extension:      "txt",
				MimeType:       "text/plain",
				CreatedBy:      scope.AccountID.String(),
			},
		},
	}
	extractor := &fakeFileReadExtractor{
		contents: map[string]*workflowfile.FileContent{
			"file-1": {
				FileID:    "file-1",
				Content:   "hello world",
				FromCache: true,
			},
		},
	}
	svc := NewService(
		repo,
		NewDefaultRegistry(),
		WithExecutor("file.read", NewFileReadExecutor(fileService, extractor, fakeWorkspacePermissionService{allowed: true})),
	)

	view, err := svc.PlanAction(context.Background(), scope, actiondto.ActionPlanRequest{
		CapabilityID: "file.read",
		Arguments: map[string]interface{}{
			"file_id":   "file-1",
			"max_chars": 5,
		},
	})
	if err != nil {
		t.Fatalf("PlanAction: %v", err)
	}

	executed, err := svc.ExecuteAction(context.Background(), scope, view.Run.ID, actiondto.ExecuteActionRequest{})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if executed.Run.Status != actionmodel.ActionRunStatusCompleted {
		t.Fatalf("status = %q, want completed", executed.Run.Status)
	}
	if len(executed.Steps) != 1 || executed.Steps[0].Status != actionmodel.ActionStepStatusDone {
		t.Fatalf("step status = %#v, want one done step", executed.Steps)
	}
	files, ok := executed.Steps[0].Output["files"].([]map[string]interface{})
	if !ok || len(files) != 1 {
		t.Fatalf("output files = %#v, want one file output", executed.Steps[0].Output["files"])
	}
	if got := files[0]["content_preview"]; got != "hello" {
		t.Fatalf("content_preview = %#v, want hello", got)
	}
	if got := files[0]["content_truncated"]; got != true {
		t.Fatalf("content_truncated = %#v, want true", got)
	}
	if got := files[0]["from_cache"]; got != true {
		t.Fatalf("from_cache = %#v, want true", got)
	}
}

func TestExecuteFileReadFailureIsRecordedOnRun(t *testing.T) {
	repo := newFakeRepository()
	scope := fakeScope()
	fileService := &fakeFileReadService{
		files: map[string]*dto.UploadFile{
			"file-1": {
				ID:             "file-1",
				OrganizationID: uuid.NewString(),
				Name:           "private.txt",
				Size:           7,
				Extension:      "txt",
				MimeType:       "text/plain",
				CreatedBy:      uuid.NewString(),
			},
		},
	}
	svc := NewService(
		repo,
		NewDefaultRegistry(),
		WithExecutor("file.read", NewFileReadExecutor(fileService, nil, nil)),
	)
	view, err := svc.PlanAction(context.Background(), scope, actiondto.ActionPlanRequest{
		CapabilityID: "file.read",
		Arguments: map[string]interface{}{
			"file_id": "file-1",
		},
	})
	if err != nil {
		t.Fatalf("PlanAction: %v", err)
	}

	executed, err := svc.ExecuteAction(context.Background(), scope, view.Run.ID, actiondto.ExecuteActionRequest{})
	if err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if executed.Run.Status != actionmodel.ActionRunStatusFailed {
		t.Fatalf("status = %q, want failed", executed.Run.Status)
	}
	if executed.Run.Error == nil || !strings.Contains(*executed.Run.Error, ErrPermissionDenied.Error()) {
		t.Fatalf("error = %#v, want permission denied", executed.Run.Error)
	}
	if len(executed.Steps) != 1 || executed.Steps[0].Status != actionmodel.ActionStepStatusFailed {
		t.Fatalf("step status = %#v, want one failed step", executed.Steps)
	}
}

type fakeRepository struct {
	member bool
	runs   map[uuid.UUID]*actionmodel.ActionRun
	steps  map[uuid.UUID][]*actionmodel.ActionStep
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		member: true,
		runs:   map[uuid.UUID]*actionmodel.ActionRun{},
		steps:  map[uuid.UUID][]*actionmodel.ActionStep{},
	}
}

func (r *fakeRepository) CreateRunWithSteps(_ context.Context, run *actionmodel.ActionRun, steps []*actionmodel.ActionStep) error {
	now := time.Now()
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = now
	}
	run.UpdatedAt = now
	r.runs[run.ID] = run
	for index, step := range steps {
		if step.ID == uuid.Nil {
			step.ID = uuid.New()
		}
		step.RunID = run.ID
		step.StepIndex = index
		if step.CreatedAt.IsZero() {
			step.CreatedAt = now
		}
		step.UpdatedAt = now
		r.steps[run.ID] = append(r.steps[run.ID], step)
	}
	return nil
}

func (r *fakeRepository) GetRunScoped(_ context.Context, id, organizationID, accountID uuid.UUID) (*actionmodel.ActionRun, []*actionmodel.ActionStep, error) {
	run := r.runs[id]
	if run == nil || run.OrganizationID != organizationID || run.AccountID != accountID {
		return nil, nil, gorm.ErrRecordNotFound
	}
	return run, r.steps[id], nil
}

func (r *fakeRepository) GetRunByIdempotencyKey(_ context.Context, organizationID, accountID uuid.UUID, key string) (*actionmodel.ActionRun, []*actionmodel.ActionStep, error) {
	for _, run := range r.runs {
		if run.OrganizationID == organizationID && run.AccountID == accountID && run.IdempotencyKey != nil && *run.IdempotencyKey == key {
			return run, r.steps[run.ID], nil
		}
	}
	return nil, nil, gorm.ErrRecordNotFound
}

func (r *fakeRepository) UpdateRunFieldsScoped(_ context.Context, id, organizationID, accountID uuid.UUID, updates map[string]interface{}) error {
	run := r.runs[id]
	if run == nil || run.OrganizationID != organizationID || run.AccountID != accountID {
		return gorm.ErrRecordNotFound
	}
	for key, value := range updates {
		switch key {
		case "status":
			run.Status = value.(string)
		case "confirmed_by":
			confirmedBy := value.(uuid.UUID)
			run.ConfirmedBy = &confirmedBy
		case "confirmed_at":
			confirmedAt := value.(time.Time)
			run.ConfirmedAt = &confirmedAt
		case "canceled_at":
			canceledAt := value.(time.Time)
			run.CanceledAt = &canceledAt
		case "error":
			if value == nil {
				run.Error = nil
				continue
			}
			errText := value.(string)
			run.Error = &errText
		case "ledger":
			run.Ledger = value.(map[string]interface{})
		case "metadata":
			run.Metadata = value.(map[string]interface{})
		}
	}
	run.UpdatedAt = time.Now()
	return nil
}

func (r *fakeRepository) UpdateStepFields(_ context.Context, runID, stepID uuid.UUID, updates map[string]interface{}) error {
	for _, step := range r.steps[runID] {
		if step.ID != stepID {
			continue
		}
		for key, value := range updates {
			switch key {
			case "status":
				step.Status = value.(string)
			case "error":
				if value == nil {
					step.Error = nil
					continue
				}
				errText := value.(string)
				step.Error = &errText
			case "started_at":
				startedAt := value.(time.Time)
				step.StartedAt = &startedAt
			case "completed_at":
				completedAt := value.(time.Time)
				step.CompletedAt = &completedAt
			case "output":
				step.Output = value.(map[string]interface{})
			}
		}
		step.UpdatedAt = time.Now()
		return nil
	}
	return gorm.ErrRecordNotFound
}

func (r *fakeRepository) IsOrganizationMember(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return r.member, nil
}

func fakeScope() Scope {
	return Scope{
		OrganizationID: uuid.New(),
		AccountID:      uuid.New(),
	}
}

type fakeFileReadService struct {
	files map[string]*dto.UploadFile
}

func (s *fakeFileReadService) GetFileByID(_ context.Context, fileID string) (*dto.UploadFile, error) {
	file := s.files[fileID]
	if file == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return file, nil
}

func (s *fakeFileReadService) GetFileURL(_ context.Context, fileID string) (string, error) {
	if s.files[fileID] == nil {
		return "", gorm.ErrRecordNotFound
	}
	return "https://files.example/" + fileID, nil
}

type fakeFileReadExtractor struct {
	contents map[string]*workflowfile.FileContent
}

func (e *fakeFileReadExtractor) ExtractMultipleFiles(_ context.Context, fileIDs []string, _ string) ([]*workflowfile.FileContent, error) {
	out := make([]*workflowfile.FileContent, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		out = append(out, e.contents[fileID])
	}
	return out, nil
}

type fakeWorkspacePermissionService struct {
	allowed bool
}

func (s fakeWorkspacePermissionService) CheckWorkspacePermission(context.Context, string, string, string, workspacemodel.WorkspacePermissionCode) (bool, error) {
	return s.allowed, nil
}
