package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
)

type runtimeExternalFileAuthorizerStub struct {
	allowed bool
}

func (s runtimeExternalFileAuthorizerStub) CanReadRuntimeExternalFile(context.Context, uuid.UUID, uuid.UUID, string) (bool, error) {
	return s.allowed, nil
}

func TestEnsureAttachmentReadableUsesExternalUserForTemporaryFiles(t *testing.T) {
	accountID := uuid.New()
	externalUserID := uuid.New()
	svc := &service{}
	scope := Scope{AccountID: accountID, ExternalUserID: &externalUserID}

	if err := svc.ensureAttachmentReadable(context.Background(), scope, &dto.UploadFile{
		IsTemporary: true,
		CreatedBy:   externalUserID.String(),
	}); err != nil {
		t.Fatalf("external user's file rejected: %v", err)
	}

	err := svc.ensureAttachmentReadable(context.Background(), scope, &dto.UploadFile{
		IsTemporary: true,
		CreatedBy:   accountID.String(),
	})
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("API-key-owned file error = %v, want ErrPermissionDenied", err)
	}
}

func TestEnsureAttachmentReadableUsesRuntimeFileBindingForPermanentFiles(t *testing.T) {
	runtimeWorkspaceID := uuid.New()
	subjectID := uuid.New()
	fileID := uuid.NewString()
	svc := &service{}
	scope := Scope{
		RuntimeWorkspaceID:    &runtimeWorkspaceID,
		RuntimeSubjectID:      &subjectID,
		RuntimeFileAuthorizer: runtimeExternalFileAuthorizerStub{allowed: true},
	}

	if err := svc.ensureAttachmentReadable(context.Background(), scope, &dto.UploadFile{ID: fileID, IsTemporary: false}); err != nil {
		t.Fatalf("runtime-bound permanent file rejected: %v", err)
	}

	scope.RuntimeFileAuthorizer = runtimeExternalFileAuthorizerStub{allowed: false}
	err := svc.ensureAttachmentReadable(context.Background(), scope, &dto.UploadFile{ID: fileID, IsTemporary: false})
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("foreign runtime file error = %v, want ErrPermissionDenied", err)
	}
}

func TestEnsureAttachmentReadableFailsClosedWithoutRuntimeFileAuthorizer(t *testing.T) {
	runtimeWorkspaceID := uuid.New()
	subjectID := uuid.New()
	svc := &service{}
	err := svc.ensureAttachmentReadable(context.Background(), Scope{
		RuntimeWorkspaceID: &runtimeWorkspaceID,
		RuntimeSubjectID:   &subjectID,
	}, &dto.UploadFile{ID: uuid.NewString()})
	if !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("missing runtime file authorizer error = %v, want ErrPermissionDenied", err)
	}
}
