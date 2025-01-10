import { http } from '@/lib/http'
import { message } from '@/lib/tips_utils'
import { getAPIProxyAddress, getFetchApiKey } from '@/lib/utils'
import type { FetchChatMessage, StreamChatCompletionsParams } from '@/types/chat'
import { toast } from 'react-toastify'
/**
 * Send messages and get real-time response stream
 * @param params Request parameters including messages and configuration options
 * @returns Returns a readable stream
 */
export const streamChatCompletions = async (params: StreamChatCompletionsParams) => {
  const { messages, ...options } = params
  const baseUrl = getAPIProxyAddress()
  const fetchUrl = `${baseUrl}/v1/chat/completions`
  const apiKey = getFetchApiKey()
  const response = await fetch(fetchUrl, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${apiKey}`,
    },
    body: JSON.stringify({
      ...options,
      model: options?.model,
      messages,
      stream: true,
      temperature: options?.temperature || 0.7,
      max_tokens: 4096,
    }),
  })
  if (!response.ok) {
    message.error(`HTTP error! status: ${response.status}`)
    throw new Error(`HTTP error! status: ${response.status}`)
  }
  const reader = response.body?.getReader()
  if (!reader) {
    message.error('No reader available')
    throw new Error('No reader available')
  }
  return reader
}

/** Add a hidden record */
export const addChatMessages = (data: {
  session_id?: string
  messages: FetchChatMessage
}) => {
  return http.post('/v1/chat/add_chat_messages', {
    data,
  })
}

/** Get a single history record */
export const getChatHistory = (session_id: string) => {
  return http.get('/v1/chat/chat_history', {
    params: {
      session_id,
    },
  })
}
