package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	maxUserInputMessageRunes           = 2000
	maxUserInputQuestionRunes          = 1000
	maxUserInputQuestions              = 5
	maxUserInputOptions                = 5
	maxUserInputOptionLabelRunes       = 80
	maxUserInputOptionDescriptionRunes = 200
)

func normalizeUserInputRequestMessage(args map[string]interface{}) string {
	return trimUserInputRunes(userInputString(args["message"]), maxUserInputMessageRunes)
}

func normalizeUserInputRequestArgs(args map[string]interface{}) ([]map[string]interface{}, error) {
	raw, ok := args["questions"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("%w: questions must be an array", ErrInvalidInput)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("%w: at least one question is required", ErrInvalidInput)
	}
	if len(raw) > maxUserInputQuestions {
		return nil, fmt.Errorf("%w: questions must contain at most %d items", ErrInvalidInput, maxUserInputQuestions)
	}
	questions := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		typed, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%w: each question must be an object", ErrInvalidInput)
		}
		question := trimUserInputRunes(userInputString(typed["question"]), maxUserInputQuestionRunes)
		if question == "" {
			return nil, fmt.Errorf("%w: question is required", ErrInvalidInput)
		}
		normalized := map[string]interface{}{"question": question}
		if id := trimUserInputRunes(userInputString(typed["id"]), 80); id != "" {
			normalized["id"] = id
		}
		options, err := normalizeUserInputOptions(typed["options"])
		if err != nil {
			return nil, err
		}
		if len(options) > 0 {
			normalized["options"] = options
		}
		questions = append(questions, normalized)
	}
	return questions, nil
}

func normalizeUserInputOptions(value interface{}) ([]map[string]interface{}, error) {
	if value == nil {
		return nil, nil
	}
	raw, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%w: options must be an array", ErrInvalidInput)
	}
	if len(raw) > maxUserInputOptions {
		return nil, fmt.Errorf("%w: options must contain at most %d items", ErrInvalidInput, maxUserInputOptions)
	}
	options := make([]map[string]interface{}, 0, len(raw))
	seen := map[string]struct{}{}
	for _, item := range raw {
		typed, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%w: each option must be an object", ErrInvalidInput)
		}
		label := trimUserInputRunes(userInputString(typed["label"]), maxUserInputOptionLabelRunes)
		if label == "" {
			return nil, fmt.Errorf("%w: option label is required", ErrInvalidInput)
		}
		if _, exists := seen[label]; exists {
			continue
		}
		seen[label] = struct{}{}
		option := map[string]interface{}{"label": label}
		if description := trimUserInputRunes(userInputString(typed["description"]), maxUserInputOptionDescriptionRunes); description != "" {
			option["description"] = description
		}
		options = append(options, option)
	}
	return options, nil
}

func userInputString(value interface{}) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func trimUserInputRunes(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return value
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return strings.TrimSpace(string(runes[:maxRunes]))
}

func userInputQuestionSummaries(questions []map[string]interface{}) []interface{} {
	summaries := make([]interface{}, 0, len(questions))
	for _, question := range questions {
		item := map[string]interface{}{
			"question": userInputString(question["question"]),
		}
		if id := userInputString(question["id"]); id != "" {
			item["id"] = id
		}
		if options, ok := question["options"].([]map[string]interface{}); ok && len(options) > 0 {
			labels := make([]interface{}, 0, len(options))
			for _, option := range options {
				if label := userInputString(option["label"]); label != "" {
					labels = append(labels, label)
				}
			}
			item["options"] = labels
		}
		summaries = append(summaries, item)
	}
	return summaries
}

func userInputRequestPayload(prepared *PreparedChat, requestID string, questions []map[string]interface{}) map[string]interface{} {
	payload := map[string]interface{}{
		"conversation_id": prepared.Conversation.ID.String(),
		"message_id":      prepared.Message.ID.String(),
		"request_id":      strings.TrimSpace(requestID),
		"questions":       questions,
		"created_at":      time.Now().Unix(),
	}
	return payload
}

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
