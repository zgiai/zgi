import { OLLAMA_DEFAULT_SERVER_API } from '@/constants'
import { useAppSettingsStore } from '@/store/appSettingsStore'
import type { ModelConfig, StreamChatCompletionsParams } from '@/types/chat'
import ollama, { Ollama } from 'ollama/dist/browser'

/** Get the ollama instance object */
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

/** Send chat to the local ollama service */
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

/** Get the list of all models from the local ollama */
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
