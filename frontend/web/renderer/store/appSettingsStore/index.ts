import { getStorageAdapter } from '@/lib/storageAdapter'
import { create } from 'zustand'
import type { AppSettingsStore, ModelConfig, ModelProvider } from './types'

const STORAGE_KEY = 'app_settings'

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
    apiEndpoint: 'https://api.openai.com/v1',
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
    enabled: false,
    apiEndpoint: 'http://127.0.0.1:11434',
    models: [],
    customModels: [],
    useStreamMode: false,
    isDefault: false,
  },
  // Add more providers here as needed
}

export const useAppSettingsStore = create<AppSettingsStore>()((set, get) => {
  const storageAdapter = getStorageAdapter({ key: STORAGE_KEY })

  return {
    isOpenModal: false,
    activeSection: 'language-models',
    expandedCards: [],
    providers: defaultProviders,
    selectedModels: {},

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

    /**
     * Set a provider as the default
     * Only one provider can be default at a time
     */
    setDefaultProvider: (providerId: string) =>
      set((state) => {
        // Only enabled providers can be set as default
        if (!state.providers[providerId].enabled) {
          return state
        }

        return {
          providers: Object.entries(state.providers).reduce(
            (acc, [id, provider]) => ({
              ...acc,
              [id]: {
                ...provider,
                isDefault: id === providerId,
              },
            }),
            {},
          ),
        }
      }),

    /**
     * Update provider configuration
     */
    updateProvider: (providerId: string, updates: Partial<ModelProvider>) =>
      set((state) => ({
        providers: {
          ...state.providers,
          [providerId]: {
            ...state.providers[providerId],
            ...updates,
          },
        },
      })),

    /**
     * Set available models for a provider
     */
    setProviderModels: (providerId: string, models: ModelConfig[]) =>
      set((state) => ({
        providers: {
          ...state.providers,
          [providerId]: {
            ...state.providers[providerId],
            models,
          },
        },
      })),

    /**
     * Toggle selection state of a specific model
     */
    toggleProviderModel: (providerId: string, model: ModelConfig) =>
      set((state) => ({
        providers: {
          ...state.providers,
          [providerId]: {
            ...state.providers[providerId],
            models: state.providers[providerId].models.includes(model)
              ? state.providers[providerId].models.filter((m) => m !== model)
              : [...state.providers[providerId].models, model],
          },
        },
      })),

    /**
     * Load settings from storage
     */
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
                ...settings.providers[id],
              },
            }),
            {},
          )
          set({ providers: mergedProviders })
        }
      } catch (error) {
        console.error('Failed to load settings:', error)
      }
    },

    /**
     * Save current settings to storage
     */
    saveSettings: async () => {
      try {
        const { providers } = get()
        await storageAdapter.save({ providers })
      } catch (error) {
        console.error('Failed to save settings:', error)
      }
    },

    /**
     * Get all enabled providers
     */
    getEnabledProviders: () => {
      const { providers } = get()
      return Object.values(providers).filter((provider) => provider.enabled)
    },

    /**
     * Get the default provider
     */
    getDefaultProvider: () => {
      const { providers } = get()
      return Object.values(providers).find((provider) => provider.isDefault)
    },

    /**
     * Add a custom model to provider
     */
    addCustomModel: (providerId: string, model: ModelConfig) =>
      set((state) => ({
        providers: {
          ...state.providers,
          [providerId]: {
            ...state.providers[providerId],
            customModels: [...state.providers[providerId].customModels, model],
          },
        },
      })),

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
        const selectedModels = { ...state.selectedModels }
        if (!selectedModels[providerId]) {
          selectedModels[providerId] = []
        }

        selectedModels[providerId] = modelIds

        get().saveSettings()

        return { selectedModels }
      })
    },

    removeSelectModelList: (providerId: string, modelIds: string[]) => {
      set((state) => {
        const selectedModels = { ...state.selectedModels }
        if (!selectedModels[providerId]) {
          selectedModels[providerId] = []
        }

        selectedModels[providerId] = selectedModels[providerId].filter(
          (id) => !modelIds.includes(id),
        )

        get().saveSettings()
        return { selectedModels }
      })
    },
  }
})
