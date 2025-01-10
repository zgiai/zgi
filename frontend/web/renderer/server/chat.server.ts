import { OLLAMA_DEFAULT_SERVER_API } from '@/constants'
import { getAPIProxyAddress, getFetchApiKey } from '@/lib/utils'
import { useAppSettingsStore } from '@/store/appSettingsStore'
import type { ModelConfig, StreamChatCompletionsParams } from '@/types/chat'
import ollama, { Ollama } from 'ollama/dist/browser'

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

  if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`)
  const reader = response.body?.getReader()
  if (!reader) throw new Error('No reader available')
  return reader
}
