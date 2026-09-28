package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
)

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
