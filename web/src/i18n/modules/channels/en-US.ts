const messages = {
  title: 'Model Channels',
  description: 'Configure model providers, routing, available models, and call quota.',
  listTitle: 'Channel List',
  searchPlaceholder: 'Search by name',
  overview: {
    total: 'Total Channels',
    enabled: 'Enabled',
    models: 'Supported Models',
    quota: 'Remaining Quota',
  },
  credit: {
    points: 'pts',
    rate: 'Conversion: $1.00 = 10,000 pts',
    approxUsd: 'about {amount}',
    usdToPoints: '{usd} = {points} pts',
    quickAmount: '${amount}',
  },
  filters: {
    allStatus: 'All Status',
    enabled: 'Enabled',
    disabled: 'Disabled',
    allProviders: 'All Providers',
  },
  status: {
    enabled: 'Enabled',
    disabled: 'Disabled',
  },
  table: {
    name: 'Channel',
    provider: 'Protocol',
    status: 'Status',
    latency: 'Latency',
    balance: 'Balance',
    quota: 'Remaining Quota',
    priority: 'Priority',
    weight: 'Weight',
    group: 'Group',
    models: 'Models',
    updatedAt: 'Updated At',
    enabled: 'Enabled',
    actions: 'Actions',
  },
  labels: {
    official: 'Official',
  },
  actions: {
    add: 'Add Channel',
    edit: 'Edit',
    delete: 'Delete',
    batch: 'Batch Actions',
    testConnectivity: 'Model Test',
    testConnectivityShort: 'Test',
    confirmDeleteTitle: 'Delete Channel',
    confirmDeleteDesc:
      'Are you sure you want to delete this channel? This action cannot be undone.',
    cancel: 'Cancel',
    confirm: 'Confirm',
  },
  modelsDialog: {
    title: 'Supported Models',
    description: 'This channel currently supports the following models',
    empty: 'No models configured for this channel',
    close: 'Close',
  },
  groups: {
    official: 'Official Channels',
    user: 'User Channels',
  },
  groupsTips: {
    official:
      'Use platform AI credits to call official channel models with official load, reliable and stable.',
    user: 'Add custom channels to expand model coverage, or use self-hosted channels to reduce model call costs.',
  },
  connectivityTest: {
    title: 'Model Test',
    description:
      'Test real calls for configured channel models. Image generation models are skipped here and should be verified in the image workspace.',
    stream: 'Streaming test',
    testing: 'Testing...',
    completed: 'Test Completed',
    summary: '{total} total, {success} success, {failure} failed, {skipped} skipped',
    imageSkippedHint:
      'Image generation models require a real image generation test. Use the image workspace to verify them.',
    pricingNotConfiguredHint:
      'Model price is not configured. Configure model pricing or a pricing policy first.',
    columns: {
      model: 'Model',
      status: 'Status',
      latency: 'Latency',
    },
    status: {
      success: 'Success',
      failure: 'Failure',
      connectionFailed: 'Connection Failed',
      connectionTimeout: 'Connection Timeout',
      notTested: 'Not Tested',
      skipped: 'Not Tested',
      pricingNotConfigured: 'Missing Price',
    },
    buttons: {
      testAll: 'Test All',
      testSelected: 'Test Selected',
      abort: 'Abort',
      remove: 'Remove',
      removeFailed: 'Remove failed models ({count})',
      testImage: 'Test in Image',
      setPrice: 'Set Price',
    },
    toast: {
      start: 'Model test started',
      error: 'Model test failed',
      abort: 'Test aborted',
      removeAllBlocked:
        'A channel must keep at least one model. Edit or delete the channel instead.',
    },
  },
  dialog: {
    titleCreate: 'Create Channel',
    titleEdit: 'Edit Channel',
    officialChannelHint:
      'This is an official channel. You can only modify priority and weight settings.',
    setup: {
      description:
        'Choose the service first. The system will narrow the protocol, default API URL, and model scope.',
      steps: {
        provider: 'Choose Provider',
        config: 'Connection',
      },
      categories: {
        common: 'Common Providers',
        aggregator: 'Aggregators',
        advanced: 'Advanced Setup',
      },
      categoryDescriptions: {
        common: 'Use official model services with preset protocol and model scope.',
        aggregator:
          'Use platforms like OpenRouter that aggregate many providers. This first version filters by platform scope.',
        advanced:
          'Use self-hosted, proxy, or local model services when you know the exact API protocol.',
      },
      fields: {
        capabilities: 'Capabilities',
        protocol: 'Protocol',
        modelStrategy: 'Model source',
      },
      actions: {
        changeProvider: 'Back to provider selection',
      },
      kinds: {
        direct: {
          label: 'Direct',
          strategy: 'Use provider-scoped models from the local model catalog',
          headline: 'Connect by official provider',
          guidance:
            'The protocol and default URL are locked to the selected provider, and the model picker stays scoped to that provider.',
        },
        aggregator: {
          label: 'Aggregator',
          strategy: 'Use aggregator-scoped models from the local model catalog',
          headline: 'Connect by aggregator platform',
          guidance:
            'Aggregator catalogs are large and change often. Selectable models come from the local model catalog, and creation saves a valid channel configuration.',
        },
        compatible: {
          label: 'Compatible',
          strategy: 'Use local catalog models or administrator-confirmed models',
          headline: 'Connect by compatible API',
          guidance:
            'Use this for proxies, custom gateways, or third-party APIs that truly support OpenAI-compatible calls for locally registered models.',
        },
        local: {
          label: 'Local',
          strategy: 'Discover models from the local runtime',
          headline: 'Connect by local runtime',
          guidance:
            'Use this for local model services such as Ollama. API keys are usually optional, but the server must be able to reach the URL.',
        },
        custom: {
          label: 'Advanced',
          strategy: 'Admin confirms protocol and model names manually',
          headline: 'Connect by advanced setup',
          guidance:
            'Keeps protocol control available for admins who already know the provider protocol, URL, and model naming rules.',
        },
      },
      providers: {
        openai: {
          label: 'OpenAI',
          description:
            'Connect OpenAI official models for standard text, vision, image, and embedding use cases.',
          capabilities: 'Text, vision, image, embeddings',
        },
        qwen: {
          label: 'Alibaba Cloud Qwen',
          description:
            'Connect native DashScope capabilities for Qwen text, vision, image, rerank, and multimodal models.',
          capabilities: 'Text, vision, image, rerank',
        },
        deepseek: {
          label: 'DeepSeek',
          description:
            'Connect DeepSeek official models for chat, coding, and reasoning use cases.',
          capabilities: 'Text, coding, reasoning',
        },
        anthropic: {
          label: 'Anthropic Claude',
          description:
            'Connect Claude official models for long-context, reasoning, and enterprise Q&A.',
          capabilities: 'Text, vision, long context',
        },
        google: {
          label: 'Google Gemini',
          description:
            'Connect Gemini models. Text and Vertex image modes can require different key rules.',
          capabilities: 'Text, vision, multimodal',
        },
        moonshot: {
          label: 'Moonshot Kimi',
          description: 'Connect Moonshot/Kimi models for Chinese chat and long-context tasks.',
          capabilities: 'Text, long context',
        },
        mistral: {
          label: 'Mistral AI',
          description:
            'Connect Mistral official models for multilingual text, coding, and reasoning use cases.',
          capabilities: 'Text, coding, reasoning',
        },
        openrouter: {
          label: 'OpenRouter',
          description:
            'Connect a multi-provider aggregator. Because the model list is large, this version filters by OpenRouter scope.',
          capabilities: 'Aggregated models, text, vision',
        },
        openaiCompatible: {
          label: 'OpenAI-Compatible Service',
          description:
            'Connect self-hosted or third-party OpenAI-compatible APIs for standard-compatible models.',
          capabilities: 'Compatible API, manual setup',
        },
        custom: {
          label: 'Custom Advanced Setup',
          description:
            'Keep full protocol control for admins who already know the provider API and model names.',
          capabilities: 'Advanced config, full selection',
        },
        ollama: {
          label: 'Ollama',
          description:
            'Connect a local Ollama service for local development and private model trials.',
          capabilities: 'Local models, text',
        },
      },
    },
    basic: 'Connection',
    availableModels: 'Available Models',
    advanced: 'Advanced Settings',
    labels: {
      name: 'Name',
      provider: 'Provider / Protocol',
      apiKey: 'API Key',
      updateApiKey: 'Update API Key',
      apiBaseUrl: 'API Base URL',
      initialFunds: 'Initial Quota',
      modelsCsv: 'Models',
      priority: 'Priority',
      weight: 'Weight',
      tagsCsv: 'Tags (comma separated)',
      autoBan: 'Automatically disable when overused',
      enabled: 'Enabled',
      modelMaps: 'Model Maps (JSON)',
      paramOverride: 'Param Override (JSON)',
      headerOverride: 'Header Override (JSON)',
      statusCodeMaps: 'Status Code Maps (JSON)',
    },
    placeholders: {
      name: 'Channel name',
      provider: 'Select a model call protocol',
      apiKey: 'sk-...',
      apiBaseUrl: 'https://api.example.com/v1',
      initialFunds: '0.00',
      modelsCsv: 'No model selected',
      priority: '100',
      weight: '100',
      tagsCsv: 'prod, test',
    },
    hints: {
      initialFundsMax: 'Maximum quota: {max} pts',
      initialFundsRate: 'Enter USD for clarity; the system converts it into points.',
      initialFundsDefault: 'Internal company channels default to $100. Adjust it for the use case.',
      providerLocked:
        'The adapter is locked to the current provider. Create a channel from Channels to choose another protocol.',
      priority: 'Lower numbers are routed first.',
      weight: 'Used for traffic split within the same priority.',
    },
    testConnection: {
      title: 'Test Connection',
      description:
        'Model selection uses the local model catalog. Testing verifies one representative model. Image generation models must be verified by generating an image in the image workspace.',
      descriptionWithModel:
        'Testing verifies {model}. Text, embedding, and rerank models make one small request. Image generation models must be verified by generating an image in the image workspace.',
      descriptionWithModelCount:
        'Selected {count} models. Testing verifies the first representative model; other models are saved from local model metadata.',
      button: 'Test',
      apiBaseUrlHint: 'Enter the API base URL before testing.',
      apiKeyHint: 'Enter the API key before testing.',
      selectModelHint: 'Select at least one representative model on the right before testing.',
      latency: 'Latency: {ms} ms',
      messages: {
        success: 'Connection test passed',
        failed: 'Connection test failed',
        successFallback: 'The model responded successfully.',
        failedFallback: 'Check that the provider, API base URL, API key, and model match.',
        requestFailed: 'Connection test request failed',
        imageModelMetadataOnly:
          'Image generation models are not included in this test. Generate an image in the image workspace to verify them.',
        apiKeyInvalid: 'The API key is invalid or expired. Update it and try again.',
        modelNotFound: 'The model was not found, or this provider endpoint does not support it.',
        rateLimited: 'The provider returned a rate limit. Retry later.',
        timeout: 'The request timed out. Retry later or check the provider connection.',
      },
      nextSteps: {
        success: 'This configuration is ready to create a channel.',
        failures: {
          auth: 'Confirm the API key belongs to this provider and can call the selected model.',
          baseUrl:
            'Confirm the API base URL is reachable and includes the correct version prefix, such as /v1.',
          model:
            'Confirm the selected model is available on this provider account, or check provider-available models and choose again.',
          rateLimit: 'The provider returned a rate limit. Retry later or check account limits.',
          quota: 'Confirm the provider account balance, plan, or billing status is active.',
          protocol: 'Confirm the selected adapter protocol is compatible with the provider API.',
          unknown:
            'Use the error above to check the key, base URL, protocol, and model configuration.',
        },
      },
      readiness: {
        verified: 'Connection verified. You can create the channel.',
        failed: 'Connection failed. Fix the key, base URL, protocol, or model before creating.',
        untested:
          'Test one representative model first. You can create the channel after it passes.',
        missingModel: 'Select at least one representative model before testing.',
      },
    },
    discoverModels: {
      button: 'Check Model List',
      messages: {
        success: 'Provider model list returned {count} models',
        supportedOnly:
          'The provider model list is informational. Selectable models come from the local model catalog.',
        unsupported:
          'This service does not support model listing. Select registered models from the local model catalog.',
        requestFailed:
          'Failed to check the model list. Non-key issues do not block saving the channel.',
      },
    },
    protocolOptions: {
      openaiCompatible: 'OpenAI Compatible',
      ollama: 'Ollama',
      openai: 'OpenAI',
      glm: 'GLM (Zhipu)',
      minimax: 'MiniMax',
      deepseek: 'DeepSeek',
      mistral: 'Mistral AI',
      cohere: 'Cohere',
      openrouter: 'OpenRouter',
      anthropic: 'Anthropic',
      qwen: 'Alibaba Cloud (Qwen)',
      moonshotaiCn: 'Moonshot AI (Kimi)',
      doubao: 'Volcengine (Doubao)',
      google: 'Google (Gemini)',
    },
    protocolGroups: {
      aggregator: 'Aggregators',
      compatible: 'Compatible protocols',
      vertical: 'Vertical providers',
    },
    protocolNotes: {
      google:
        'Gemini text mode and Vertex image mode require different API key and API base URL settings.',
    },
    errors: {
      qwenOpenAICompatibleMismatch:
        'OpenAI Compatible is selected, but the selected models include native Qwen capabilities such as rerank, image, vision, or multimodal models. Use Alibaba Cloud (Qwen), or select only OpenAI-compatible models.',
      dashScopeProviderMismatch:
        'Use Alibaba Cloud (Qwen) for DashScope rerank, image, and vision models. OpenAI Compatible is only for compatible-mode models.',
    },
    buttons: {
      cancel: 'Cancel',
      create: 'Create',
      save: 'Save',
    },
  },
  empty: {
    title: 'No Channels',
    description: 'Add your model call channels to manage routing and run model tests.',
  },
  messages: {
    updateSuccess: 'Channel updated',
    updateFailed: 'Failed to update channel',
    deleteSuccess: 'Channel deleted',
    deleteFailed: 'Failed to delete channel',
    createSuccess: 'Channel created',
    createFailed: 'Failed to create channel',
    loadFailed: 'Failed to load channel',
  },
  walletAdjust: {
    title: 'Adjust Quota',
    description: 'Adjust the call quota available to this channel',
    currentBalance: 'Current Quota',
    targetBalance: 'Target Quota',
    adjustAmount: 'Adjustment',
    note: 'Note',
    notePlaceholder: 'Optional, record the reason for adjustment',
    increase: 'Increase',
    decrease: 'Decrease',
    noChange: 'No Change',
    maxLimit: 'Maximum quota: {max} pts',
    rateHint: '10,000 pts is approximately $1.00 of call quota',
    validation: {
      required: 'Please enter target quota',
      min: 'Quota cannot be negative',
      max: 'Quota cannot exceed {max}',
      noChange: 'Target quota is the same as current quota',
    },
    success: 'Quota adjusted successfully',
    error: 'Failed to adjust quota',
    permissionDenied: 'Only admin can adjust quota',
    buttons: {
      cancel: 'Cancel',
      confirm: 'Confirm',
    },
  },
};

export default messages;
export type ChannelsMessages = typeof messages;
