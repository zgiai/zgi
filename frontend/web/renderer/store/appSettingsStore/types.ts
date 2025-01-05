/**
 * Model provider configuration interface
 * Represents the configuration for a language model provider (e.g., OpenAI, Ollama)
 */
export interface ModelProvider {
  /** Unique identifier for the provider */
  id: string

  /** Display name of the provider */
  name: string

  /** Whether the provider is enabled */
  enabled: boolean

  /** API key for authentication (if required) */
  apiKey?: string

  /** API endpoint URL */
  apiEndpoint?: string

  /** List of available/selected models for this provider */
  models: ModelConfig[]

  /** Whether to use client-side streaming mode (specific to Ollama) */
  useStreamMode?: boolean

  /** Whether this provider is the default one */
  isDefault: boolean

  /** Custom models added by user */
  customModels: ModelConfig[]
}

/**
 * Settings section identifiers
 * Defines the available sections in the settings panel
 */
export type SettingsSection =
  | 'general'
  | 'language-models'
  | 'voice-services'
  | 'default-assistant'
  | 'about'

/**
 * App Settings Store interface
 * Defines the state and actions for managing application settings
 */
export interface AppSettingsStore {
  /** Modal visibility state */
  isOpenModal: boolean

  /** Currently active settings section */
  activeSection: SettingsSection

  /** List of expanded provider cards */
  expandedCards: string[]

  /** Map of provider configurations */
  providers: Record<string, ModelProvider>

  /**
   * Modal Actions
   */
  /** Set modal visibility */
  setOpenModal: (flag: boolean) => void

  /** Set active settings section */
  setActiveSection: (section: SettingsSection) => void

  /** Toggle provider card expansion state */
  toggleCard: (cardId: string) => void

  /**
   * Provider Actions
   */
  /** Toggle provider enabled state */
  toggleProvider: (providerId: string) => void

  /** Update provider configuration */
  updateProvider: (providerId: string, updates: Partial<ModelProvider>) => void

  /** Set available models for a provider */
  setProviderModels: (providerId: string, models: string[]) => void

  /** Toggle selection state of a specific model */
  toggleProviderModel: (providerId: string, model: string) => void

  /**
   * Storage Actions
   */
  /** Load settings from storage */
  loadSettings: () => Promise<void>

  /** Save current settings to storage */
  saveSettings: () => Promise<void>

  /** Set a provider as the default provider */
  setDefaultProvider: (providerId: string) => void

  /** Get all enabled providers */
  getEnabledProviders: () => ModelProvider[]

  /** Get the default provider */
  getDefaultProvider: () => ModelProvider | undefined

  /** Add a custom model to provider */
  addCustomModel: (providerId: string, modelName: string) => void

  /** Remove a custom model from provider */
  removeCustomModel: (providerId: string, modelId: string) => void

  /** Toggle model selection state */
  toggleModel: (providerId: string, modelId: string) => void
}

/**
 * Provider-specific configuration interfaces
 */
export interface OpenAIConfig extends ModelProvider {
  id: 'openai'
  apiKey: string
  apiEndpoint: string
}

export interface OllamaConfig extends ModelProvider {
  id: 'ollama'
  apiEndpoint: string
  useStreamMode: boolean
}

/**
 * Storage data structure
 * Defines the shape of data stored in persistent storage
 */
export interface StorageData {
  providers: Record<string, ModelProvider>
}

/**
 * Model configuration interface
 */
export interface ModelConfig {
  /** Model identifier */
  id: string
  /** Display name */
  name: string
  /** Model context size */
  contextSize?: string
  /** Whether the model is selected */
  selected: boolean
}
