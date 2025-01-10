// Define message types
export type Role = 'system' | 'user' | 'assistant'

export interface ChatMessage {
  skipAIResponse?: boolean
  /** Role */
  role: Role
  /** Message ID */
  id?: string
  /** Message content */
  content: string
  /** Message timestamp */
  timestamp?: string
  /** File type */
  fileType?: string
  /** File name */
  fileName?: string
}

/** Request parameters for chat completion API */
export interface ChatCompletionRequest {
  /** Model identifier */
  model: string
  /** Array of chat messages */
  messages: ChatMessage[]
  /** Enable streaming response */
  stream: boolean
  /** Sampling temperature */
  temperature: number
  /** Top p sampling */
  top_p: number
  /** Number of completions */
  n: number
  /** Maximum tokens to generate */
  max_tokens: number
}

/** Response structure from chat completion API */
export interface ChatCompletionResponse {
  /** Generated message */
  message: ChatMessage
  /** Response ID */
  id: string
  /** Creation timestamp */
  created: number
  /** Model used */
  model: string
  /** Token usage statistics */
  usage: {
    /** Number of tokens in prompt */
    prompt_tokens: number
    /** Number of tokens in completion */
    completion_tokens: number
    /** Total tokens used */
    total_tokens: number
  }
}

/** Chat history entry structure */
export interface ChatHistory {
  /** Chat history ID */
  id: string
  /** Chat title */
  title: string
  /** Array of chat messages */
  messages: ChatMessage[]
  /** Creation timestamp */
  createdAt: string
  /** Last update timestamp */
  updatedAt?: number
  /** Model used in chat */
  model?: string
  /** Favorite status */
  favorite?: boolean
}

export enum StreamChatMode {
  commonChat = 'commonChat',
  ollama = 'ollama',
}

/**
 * Model configuration interface
 */
export interface ModelConfig {
  /** Model identifier */
  id: string
  /** Display name */
  name: string
  /** Model context size */
  contextSize?: string
  isCustom?: boolean
  type?: string
}

/** Send messages and get real-time response stream */
export interface StreamChatCompletionsParams {
  messages: FetchChatMessage
  model?: string
  temperature?: number
  presence_penalty?: number
  stream?: boolean
}

export type FetchChatMessage = FetchChatMessageItem[]

export interface FetchChatMessageItem {
  role: Role
  content: string | ChatMessageItemContentArrItem[]
}

interface ChatMessageItemContentArrItem {
  type: string
  image_url?: {
    url: string
  }
}
