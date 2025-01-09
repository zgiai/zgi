import { OLLAMA_DEFAULT_SERVER_API } from '@/constants'
import { getAPIProxyAddress, getFetchApiKey } from '@/lib/utils'
import { useAppSettingsStore } from '@/store/appSettingsStore'
import type { ModelConfig } from '@/types/chat'
import ollama, { Ollama } from 'ollama/dist/browser'

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
      temperature: options?.temperature || 1,
      max_tokens: 4096,
    }),
  })

  if (!response.ok) throw new Error(`HTTP error! status: ${response.status}`)
  const reader = response.body?.getReader()
  if (!reader) throw new Error('No reader available')
  return reader
}

const getOllamaObj = () => {
  const ollamaConfig = useAppSettingsStore.getState().providers?.ollama
  const fetchUrl = ollamaConfig?.apiEndpoint || OLLAMA_DEFAULT_SERVER_API
  let resOllama: Ollama
  if (fetchUrl === OLLAMA_DEFAULT_SERVER_API) {
    resOllama = ollama
  } else {
    const _ollama = new Ollama({ host: fetchUrl })
    resOllama = _ollama
  }

  return resOllama
}

export const localStreamChatCompletions = async (
  data: Pick<StreamChatCompletionsParams, 'messages' | 'model'>,
) => {
  const _ollama = getOllamaObj()
  const response = await _ollama.chat({
    model: data?.model as any,
    messages: data?.messages as any,
    stream: true,
  })
  return response
}

export const getOllamaModels = async () => {
  try {
    const _ollama = getOllamaObj()
    const response = await _ollama?.list?.()
    const newOllamaModels: ModelConfig[] = []
    response?.models?.forEach((item) => {
      newOllamaModels.push({
        ...item,
        id: item.name,
        contextSize: item.details?.parameter_size || '',
        type: 'ollama',
      })
    })
    return newOllamaModels || []
  } catch (error) {
    console.error(error)
    return []
  }
}
