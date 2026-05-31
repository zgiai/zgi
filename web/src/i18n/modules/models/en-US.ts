const messages = {
  modelParameters: 'Model Parameters',
  unsavedChanges: {
    title: 'Unsaved parameter changes',
    description: 'There are unsaved parameter changes. Save them?',
    discard: 'Discard',
    save: 'Save',
  },
  configParameters: {
    presets: {
      title: 'Preset',
      description:
        'Apply a schema-aware preset for supported chat parameters. Unsupported items are skipped automatically.',
      placeholder: 'Choose preset',
      defaultLabel: 'Balanced',
      items: {
        precise: {
          label: 'Precise',
          description:
            'Lower randomness for translation, extraction, rewriting, and other high-consistency tasks.',
        },
        balanced: {
          label: 'Balanced',
          description:
            'A professional default for everyday assistant use, balancing stability, fluency, and naturalness.',
        },
        creative: {
          label: 'Creative',
          description:
            'Higher diversity for brainstorming, marketing copy, storytelling, and expressive writing.',
        },
      },
    },
    states: {
      empty: {
        badge: 'Using defaults',
        title: 'This model uses default parameters',
        description:
          'This model does not expose configurable runtime parameters here, so requests will use the provider defaults.',
        hint: 'Switch to a model with advanced controls if you need parameter tuning.',
      },
      loadFailed: {
        badge: 'Load failed',
        title: 'Failed to load parameter definitions',
        description:
          'We could not load this model parameter schema. Try reopening the dialog or reselecting the model.',
        hint: 'If the problem continues, the provider may not be returning a valid parameter schema.',
      },
      notFound: {
        badge: 'Schema unavailable',
        title: 'Parameter definition unavailable',
        description:
          'The current model did not provide a parameter schema, so runtime parameters cannot be configured here.',
        hint: 'You can continue using the model, but parameter tuning is not available for this selection.',
      },
    },
    templates: {
      temperature: {
        label: 'Temperature',
        help: 'Controls output randomness. Lower values are more deterministic.',
      },
      top_p: {
        label: 'Top P',
        help: 'Limits token sampling to the smallest probability mass within the threshold.',
      },
      presence_penalty: {
        label: 'Presence Penalty',
        help: 'Encourages the model to introduce new topics instead of repeating prior content.',
      },
      frequency_penalty: {
        label: 'Frequency Penalty',
        help: 'Reduces repeated tokens by penalizing frequent token reuse.',
      },
      logit_bias: {
        label: 'Logit Bias',
        help: 'Adjusts the probability of specific tokens before sampling.',
      },
      seed: {
        label: 'Seed',
        help: 'Uses a fixed random seed to improve repeatability across runs.',
      },
      stop: {
        label: 'Stop',
        help: 'Stops generation when the configured stop sequence is encountered.',
      },
      max_tokens: {
        label: 'Max Tokens',
        help: 'Sets the upper bound for generated output tokens.',
      },
    },
  },
  selector: {
    placeholder: 'Select model',
    searchPlaceholder: 'Search models...',
    refreshSuccess: 'Refreshed successfully',
    noResults: 'No models found matching',
    noModelsAvailable: 'No models available',
    expandAll: 'Expand All',
    collapseAll: 'Collapse All',
    modelCount: {
      single: 'model',
      multiple: 'models',
    },
    types: {
      llm: 'LLM Model',
      'text-embedding': 'Embedding Model',
      rerank: 'Rerank Model',
      moderation: 'Moderation Model',
      speech2text: 'Speech to Text Model',
      tts: 'Text to Speech Model',
    },
    usecases: {
      'text-chat': 'Chat Model',
      vision: 'Vision Model',
      'image-gen': 'Image Gen Model',
      embedding: 'Embedding Model',
      rerank: 'Rerank Model',
      'speech-to-text': 'STT Model',
      'text-to-speech': 'TTS Model',
      'realtime-audio': 'Realtime Audio',
      'video-gen': 'Video Gen Model',
      moderation: 'Moderation Model',
      reasoning: 'Reasoning Model',
      'function-calling': 'Function Calling Model',
    },
    empty: {
      noModelsTitle: 'Model setup required',
      noResults: 'No models found matching',
      noModels: 'No {type} available',
      contactAdmin: 'Contact an admin to enable models for this workspace.',
      configureDescription: 'Configure at least one available {type} before using this workflow or agent.',
      configure: 'Configure',
      clearSearch: 'Clear search',
      refresh: 'Refresh models',
    },
    tooltip: {
      modelId: 'Model ID:',
      context: 'Context',
      useCases: 'Use Cases',
      features: 'Features',
      description: 'Description:',
      unknown: 'unknown',
      // Note: Feature labels now use aiProviders.models.features translations
    },
  },
  messages: {
    loadFailed: 'Failed to load models',
    createSuccess: 'Custom model created successfully',
    createFailed: 'Failed to create custom model',
    updateSuccess: 'Custom model updated successfully',
    updateFailed: 'Failed to update custom model',
    deleteSuccess: 'Custom model deleted successfully',
    deleteFailed: 'Failed to delete custom model',
  },
};

export default messages;
export type ModelsMessages = typeof messages;
