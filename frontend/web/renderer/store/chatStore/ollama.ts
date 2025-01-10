import { getOllamaModels } from '@/server/chat.server'

export const getLoclOllamaModels = async ({ set }) => {
  const res = await getOllamaModels()
  const newOllamaModels: any = []
  res?.forEach((item) => {
    newOllamaModels.push({
      ...item,
      usage: item.details?.parameter_size || '',
      type: 'local',
    })
  })
  set({
    ollamaModels: newOllamaModels,
  })
}
