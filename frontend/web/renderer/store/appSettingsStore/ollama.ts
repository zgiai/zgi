import { getOllamaModels } from '@/server/chat.server'
import { produce } from 'immer'
import type { ModelConfig } from './types'

export const getLoclOllamaModels = async ({ set }) => {
  const res = await getOllamaModels()
  const newOllamaModels: ModelConfig[] = []
  res?.forEach((item) => {
    newOllamaModels.push({
      ...item,
      id: item.name,
      contextSize: item.details?.parameter_size || '',
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
