// 定义消息类型
export type Role = 'system' | 'user' | 'assistant'

export interface ChatMessage {
  /** 角色 */
  role: Role
  /** 消息id */
  id?: string
  /** 消息内容 */
  content: string
  /** 消息时间戳 */
  timestamp?: string
  /** 文件类型 */
  fileType?: string
  /** 文件名称 */
  fileName?: string
}

export interface ChatCompletionRequest {
  model: string
  messages: ChatMessage[]
  stream: boolean
  temperature: number
  top_p: number
  n: number
  max_tokens: number
}

export interface ChatCompletionResponse {
  message: ChatMessage
  id: string
  created: number
  model: string
  usage: {
    prompt_tokens: number
    completion_tokens: number
    total_tokens: number
  }
}

export interface ChatHistory {
  id: string
  title: string
  messages: ChatMessage[]
  createdAt: string
  updatedAt?: number
  model?: string
  favorite?: boolean
}
