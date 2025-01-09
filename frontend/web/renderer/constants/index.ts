import type { ModelProvider } from '@/store/appSettingsStore/types'

// export const API_KEY = 'zgi_ZQXZ-S_fG6erk2JFyuJ1nr69Lj40k3yhx_RBpFjhEjg'
export const OLLAMA_DEFAULT_SERVER_API = 'http://127.0.0.1:11434'

export const SELECT_VALUE_DECOLLATOR = '&&'

/**
 * Default provider configurations
 * Initial state for each provider when no stored settings exist
 */
export const defaultProviders: Record<string, ModelProvider> = {
  zgi: {
    id: 'zgi',
    name: 'zgi',
    enabled: true,
    apiKey: undefined,
    apiEndpoint: undefined,
    models: [
      { id: 'deepseek-chat', name: 'deepseek-chat', contextSize: '200K' },
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
    apiEndpoint: undefined,
    models: [],
    customModels: [],
    useStreamMode: false,
    isDefault: false,
  },
}
