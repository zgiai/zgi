import { OLLAMA_DEFAULT_SERVER_API, defaultProviders } from '@/constants'
import { STORAGE_ADAPTER_KEYS } from '@/constants/storageAdapterKey'
import { getStorageAdapter } from '@/lib/storageAdapter'
import { createSubsStore } from '@/lib/store_utils'
import type { ModelConfig } from '@/types/chat'
import { getLoclOllamaModels } from './ollama'
import subscribeInit from './subscribe'
import type { AppSettingsStore, ModelProvider } from './types'

export const useAppSettingsStore = createSubsStore<AppSettingsStore>((set, get) => {
  const storageAdapter = getStorageAdapter({ key: STORAGE_ADAPTER_KEYS.app_settings.key })

  return {
    isOpenModal: false,
    activeSection: 'language-models',
    expandedCards: ['zgi', 'ollama'],
    providers: defaultProviders,
    selectedModelIds: {},
    checkResults: {},
    allProvidersSelectedModels: {},

    init: async () => {
      await get().loadSettings()
      get().generateModelsOptions()
    },

    setOpenModal: (flag: boolean) => set({ isOpenModal: flag }),

    setActiveSection: (section: string) => set({ activeSection: section }),

    toggleCard: (cardId: string) =>
      set((state) => ({
        expandedCards: state.expandedCards.includes(cardId)
          ? state.expandedCards.filter((id) => id !== cardId)
          : [...state.expandedCards, cardId],
      })),

    toggleProvider: (providerId: string) =>
      set((state) => {
        const provider = state.providers[providerId]
        const newEnabled = !provider.enabled

        // If enabling this provider and it's the first one, make it default
        const shouldBeDefault = newEnabled && !Object.values(state.providers).some((p) => p.enabled)

        return {
          providers: {
            ...state.providers,
            [providerId]: {
              ...provider,
              enabled: newEnabled,
              isDefault: shouldBeDefault || provider.isDefault,
            },
          },
        }
      }),

    updateProvider: (providerId: string, updates: Partial<ModelProvider>) => {
      set((state) => ({
        providers: {
          ...state.providers,
          [providerId]: {
            ...state.providers[providerId],
            ...updates,
          },
        },
      }))
    },
    loadSettings: async () => {
      try {
        const settings = await storageAdapter.load()
        if (settings) {
          console.log('配置处理', { settings, defaultProviders })
          // Merge stored settings with defaults, preserving enabled states
          const mergedProviders = Object.entries(defaultProviders).reduce(
            (acc, [id, defaultProvider]) => ({
              ...acc,
              [id]: {
                ...defaultProvider,
                ...settings.languageModel?.providers?.[id],
                models: defaultProvider.models,
              },
            }),
            {},
          )

          const selectedModelIds = settings.languageModel?.selectedModelIds || []
          set({ providers: mergedProviders, selectedModelIds })
          getLoclOllamaModels({ set })
        }
      } catch (error) {
        console.error('Failed to load settings:', error)
      }
    },
    saveSettings: async () => {
      try {
        const { providers, selectedModelIds } = get()
        await storageAdapter.save({
          languageModel: {
            providers,
            selectedModelIds,
          },
        })
      } catch (error) {
        console.error('Failed to save settings:', error)
      }
    },
    addCustomModel: (providerId: string, model: ModelConfig) => {
      set((state) => ({
        providers: {
          ...state.providers,
          [providerId]: {
            ...state.providers[providerId],
            customModels: [...state.providers[providerId].customModels, model],
          },
        },
      }))
    },
    removeCustomModel: (providerId: string, modelId: string) => {
      get().removeSelectModelList(providerId, [modelId])
      set((state) => ({
        providers: {
          ...state.providers,
          [providerId]: {
            ...state.providers[providerId],
            customModels: state.providers[providerId].customModels.filter(
              (model) => model.id !== modelId,
            ),
          },
        },
      }))
    },
    updateSelectModelList: (providerId: string, modelIds: string[]) => {
      set((state) => {
        const selectedModelIds = { ...state.selectedModelIds }
        if (!selectedModelIds[providerId]) {
          selectedModelIds[providerId] = []
        }
        selectedModelIds[providerId] = modelIds
        return { selectedModelIds }
      })
    },
    removeSelectModelList: (providerId: string, modelIds: string[]) => {
      set((state) => {
        const selectedModelIds = { ...state.selectedModelIds }
        if (!selectedModelIds[providerId]) {
          selectedModelIds[providerId] = []
        }
        selectedModelIds[providerId] = selectedModelIds[providerId].filter(
          (id) => !modelIds.includes(id),
        )
        return { selectedModelIds }
      })
    },
    checkProvider: async (providerId: string) => {
      const provider = get().providers[providerId]
      if (!provider.apiKey && providerId !== 'ollama') {
        set((state) => ({
          checkResults: {
            ...state.checkResults,
            [providerId]: { error: 'API Key or Endpoint is missing.' },
          },
        }))
        return
      }
      let apiEndpoint = provider.apiEndpoint || ''
      if (providerId === 'ollama' && !apiEndpoint) {
        apiEndpoint = OLLAMA_DEFAULT_SERVER_API
      }
      try {
        const response = await fetch(apiEndpoint, {
          method: 'GET',
          headers: {
            Authorization: `Bearer ${provider.apiKey}`,
          },
        })

        if (!response.ok) {
          const errorDetails = await response.text()
          set((state) => ({
            checkResults: {
              ...state.checkResults,
              [providerId]: { error: `Error: ${response.status} - ${errorDetails}` },
            },
          }))
        } else {
          set((state) => ({
            checkResults: {
              ...state.checkResults,
              [providerId]: { error: null },
            },
          }))
        }
      } catch (error) {
        set((state) => ({
          checkResults: {
            ...state.checkResults,
            [providerId]: { error: `Network error: ${error.message}` },
          },
        }))
      }
    },
    generateModelsOptions: () => {
      const { providers, selectedModelIds } = get()
      const allProvidersSelectedModels: Record<string, ModelConfig[]> = {}
      console.log(providers, 'providers')
      Object.entries(providers).forEach(([key, value]) => {
        if (!allProvidersSelectedModels[key]) {
          allProvidersSelectedModels[key] = []
        }
        const curSelectedModelIds = selectedModelIds[key] || []
        const filteredModels =
          value.models?.filter((item) => curSelectedModelIds.includes(item.id)) || []
        const filteredCustomModels =
          value.customModels?.filter((item) => curSelectedModelIds.includes(item.id)) || []
        if (value.enabled) {
          allProvidersSelectedModels[key].push(...filteredModels, ...filteredCustomModels)
        }
      })
      set({
        allProvidersSelectedModels,
      })
    },
  }
})

subscribeInit()
