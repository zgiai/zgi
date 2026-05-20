package chat

import (
	"github.com/zgiai/zgi/api/internal/modules/shared/model"
)

type Message = model.Message
type Conversation = model.Conversation
type JSONMap = model.JSONMap
type JSONArray = model.JSONArray
type MessageStatus = model.MessageStatus
type MessageFrom = model.MessageFrom
type MessageFeedback = model.MessageFeedback
type MessageAnnotation = model.MessageAnnotation
type MessageFile = model.MessageFile
type AgentThought = model.AgentThought

const (
	MessageStatusNormal    = model.MessageStatusNormal
	MessageStatusStopped   = model.MessageStatusStopped
	MessageStatusError     = model.MessageStatusError
	MessageStatusCompleted = model.MessageStatusCompleted
	MessageFromUser        = model.MessageFromUser
	MessageFromHuman       = model.MessageFromHuman
	MessageFromAssistant   = model.MessageFromAssistant
)

type MessageDetailResponse struct {
}
