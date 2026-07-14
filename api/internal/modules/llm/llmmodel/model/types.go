package model

import (
	"github.com/zgiai/zgi/api/internal/modules/llm/shared/types"
)

// Type aliases for backward compatibility - all JSON types now use shared/types
type JSONArray = types.JSONArray
type JSONObject = types.JSONObject
type JSONStringArray = types.JSONStringArray
type JSONMap = types.JSONMap
type StringArray = types.StringArray

// ============================================================================
// Common Types and Constants
// ============================================================================

const (
	ModelStatusActive     = "active"
	ModelStatusDeprecated = "deprecated"
)

// ModelType defines the type of model
type ModelType string

const (
	ModelTypeLLM       ModelType = "llm"
	ModelTypeEmbedding ModelType = "text-embedding"
	ModelTypeImage     ModelType = "image"
	ModelTypeAudio     ModelType = "audio"
	ModelTypeVideo     ModelType = "video"
	ModelTypeRerank    ModelType = "rerank"
)

// AccessScope defines who can access a resource
type AccessScope string

const (
	AccessScopeAll   AccessScope = "all"
	AccessScopeGroup AccessScope = "group"
	AccessScopeUser  AccessScope = "user"
)

// UseCase defines the usage scenario of a model
type UseCase string

const (
	UseCaseTextChat      UseCase = "text-chat"        // Text chat
	UseCaseVision        UseCase = "vision"           // Image understanding
	UseCaseImageGen      UseCase = "image-gen"        // Image generation
	UseCaseEmbedding     UseCase = "embedding"        // Text embeddings
	UseCaseRerank        UseCase = "rerank"           // Search reranking
	UseCaseSpeechToText  UseCase = "speech-to-text"   // Speech recognition
	UseCaseTextToSpeech  UseCase = "text-to-speech"   // Speech synthesis
	UseCaseRealtimeAudio UseCase = "realtime-audio"   // Real-time audio
	UseCaseVideoGen      UseCase = "video-gen"        // Video generation
	UseCaseModeration    UseCase = "moderation"       // Content moderation
	UseCaseReasoning     UseCase = "reasoning"        // Deep reasoning
	UseCaseFuncCalling   UseCase = "function-calling" // Function calling
	UseCaseAgent         UseCase = "agent"            // Tool-using agents
)

// ValidUseCases returns all valid use case values
func ValidUseCases() []UseCase {
	return []UseCase{
		UseCaseTextChat, UseCaseVision, UseCaseImageGen, UseCaseEmbedding,
		UseCaseRerank, UseCaseSpeechToText, UseCaseTextToSpeech, UseCaseRealtimeAudio,
		UseCaseVideoGen, UseCaseModeration, UseCaseReasoning, UseCaseFuncCalling,
		UseCaseAgent,
	}
}
