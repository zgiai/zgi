import { getOllamaModels } from '@/server/chat.server'
import type { ModelConfig } from '@/types/chat'
import { produce } from 'immer'

export const getLoclOllamaModels = async ({ set }) => {
  const res = await getOllamaModels()
  const newOllamaModels: ModelConfig[] = []
  res?.forEach((item) => {
    newOllamaModels.push({
      ...item,
      id: item.name,
      contextSize: item.details?.parameter_size || '',
      type: 'ollama',
    })
  })
  set(
    produce(({ providers }: any) => {
      if (providers.ollama) {
        providers.ollama.models = newOllamaModels
      }
    }),
  )
}
