import { API_KEY } from '@/constants'
import { API_CONFIG } from '@/lib/http'
import ollama from 'ollama/dist/browser'

/** Send messages and get real-time response stream */
interface StreamChatCompletionsParams {
  messages: Record<string, any>[]
  model?: string
  temperature?: number
  presence_penalty?: number
  stream?: boolean
}

/**
 * Send messages and get real-time response stream
 * @param params Request parameters including messages and configuration options
 * @returns Returns a readable stream
 */
export const streamChatCompletions = async (params: StreamChatCompletionsParams) => {
  const { messages, ...options } = params
  const fetchUrl = `${API_CONFIG.COMMON}/v1/chat/completions`
  const response = await fetch(fetchUrl, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: API_KEY,
    },
    body: JSON.stringify({
      ...options,
      model: options?.model || 'gpt-4o',
      messages,
      stream: true,
      temperature: options?.temperature || 1,
      max_tokens: 4096,
    }),
  })

  if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`)
  const reader = response.body?.getReader()
  if (!reader) throw new Error('No reader available')
  return reader
}

export const localStreamChatCompletions = async (
  data: Pick<StreamChatCompletionsParams, 'messages' | 'model'>,
) => {
  const response = await ollama.chat({
    model: data?.model as any,
    messages: data?.messages as any,
    stream: true,
  })
  return response
}

export const getOllamaModels = async () => {
  try {
    const response = await ollama?.list?.()
    return response?.models || []
  } catch (error) {
    console.error(error)
    return []
  }
}
