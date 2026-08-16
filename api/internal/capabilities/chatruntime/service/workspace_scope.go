package service

import (
	"fmt"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
)

// ensureConversationWorkspaceScope prevents an existing conversation from
// changing its workspace authorization boundary when an account switches its
// current workspace. Regeneration and every continuation path load the
// conversation through one of the guarded helpers before external tools run.
func ensureConversationWorkspaceScope(scope Scope, conversation *runtimemodel.Conversation) error {
	if conversation == nil {
		return fmt.Errorf("%w: conversation is required", ErrNotFound)
	}
	if sameOptionalWorkspaceID(scope.WorkspaceID, conversation.WorkspaceID) {
		return nil
	}
	return fmt.Errorf("%w: conversation workspace does not match the active workspace", ErrPermissionDenied)
}

func sameOptionalWorkspaceID(left, right *uuid.UUID) bool {
	leftPresent := left != nil && *left != uuid.Nil
	rightPresent := right != nil && *right != uuid.Nil
	if leftPresent != rightPresent {
		return false
	}
	if !leftPresent {
		return true
	}
	return *left == *right
}
