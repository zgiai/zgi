import { OLLAMA_DEFAULT_SERVER_API } from '@/constants'
import { STORAGE_ADAPTER_KEYS } from '@/constants/storageAdapterKey'
import { getStorageAdapter } from '@/lib/storageAdapter'
import { createSubsStore } from '@/lib/store_utils'
import { getLoclOllamaModels } from './ollama'
import subscribeInit from './subscribe'
import type { AppSettingsStore, ModelConfig, ModelProvider } from './types'

/**
 * Default provider configurations
 * Initial state for each provider when no stored settings exist
 */
const defaultProviders: Record<string, ModelProvider> = {
  zgi: {
    id: 'zgi',
    name: 'zgi',
    enabled: true,
    apiKey: '',
    apiEndpoint: '',
    models: [
      { id: 'gpt-4', name: 'GPT-4', contextSize: '8K' },
      { id: 'gpt-3.5-turbo', name: 'GPT-3.5 Turbo', contextSize: '4K' },
    ],
    customModels: [],
    isDefault: false,
  },
  ollama: {
    id: 'ollama',
    name: 'Ollama',
    enabled: true,
    apiEndpoint: '',
    models: [],
    customModels: [],
    useStreamMode: false,
    isDefault: false,
  },
}

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
    },

    setOpenModal: (flag: boolean) => set({ isOpenModal: flag }),

    setActiveSection: (section: string) => set({ activeSection: section }),

    toggleCard: (cardId: string) =>
      set((state) => ({
        expandedCards: state.expandedCards.includes(cardId)
          ? state.expandedCards.filter((id) => id !== cardId)
          : [...state.expandedCards, cardId],
      })),

    /**
     * Toggle provider enabled state
     * Multiple providers can be enabled simultaneously
     */
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
    setProviderModels: (providerId, models) => {
      set((state) => ({
        providers: {
          ...state.providers,
          [providerId]: {
            ...state.providers[providerId],
            models,
          },
        },
      }))
    },
    loadSettings: async () => {
      try {
        const settings = await storageAdapter.load()
        if (settings) {
          // Merge stored settings with defaults, preserving enabled states
          const mergedProviders = Object.entries(defaultProviders).reduce(
            (acc, [id, defaultProvider]) => ({
              ...acc,
              [id]: {
                ...defaultProvider,
                ...settings.languageModel?.providers?.[id],
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
