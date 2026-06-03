package service

import (
	"context"

	"github.com/zgiai/zgi/api/pkg/logger"
)

func (s *service) persistUserInputRequestBestEffort(ctx context.Context, prepared *PreparedChat, payload map[string]interface{}) {
	if prepared == nil || prepared.Message == nil || len(payload) == 0 {
		return
	}
	metadata := mergeUserInputRequestMetadata(prepared.Message.Metadata, payload)
	prepared.Message.Metadata = metadata
	if s == nil || s.repos == nil || s.repos.Message == nil {
		return
	}
	if err := s.repos.Message.UpdateMetadata(ctx, prepared.Message.ID, metadata); err != nil {
		logger.WarnContext(ctx, "failed to persist aichat user input request metadata", "message_id", prepared.Message.ID.String(), err)
	}
}

func mergeUserInputRequestMetadata(source map[string]interface{}, payload map[string]interface{}) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	request := map[string]interface{}{
		"request_id": payload["request_id"],
		"questions":  payload["questions"],
		"created_at": payload["created_at"],
	}
	metadata["user_input_request"] = request
	return metadata
}
