'use client';

import React, { useCallback } from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
} from '@/components/ui/dialog';
import { Input, PasswordInput } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { Button } from '@/components/ui/button';
import {
  useCreateChannel,
  useDiscoverDraftChannelModels,
  useTestDraftChannelModel,
  useUpdateChannel,
} from '@/hooks';
import type {
  CreateChannelRequest,
  ChannelDetail,
  ChannelModelTestResult,
  DiscoveredChannelModel,
  UpdateChannelRequest,
} from '@/services/types/channel';
import type { ModelItem, ModelUseCase } from '@/services/types/model';
import { useT } from '@/i18n';
import ModelMultiSelector from '@/components/common/model-multi-selector/model-multi-selector';
import {
  AlertCircle,
  ArrowLeft,
  CheckCircle2,
  ChevronDown,
  Loader2,
  RefreshCw,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { toast } from 'sonner';
import ChannelProviderSelector, {
  getMappedProvider,
  getChannelProviderOption,
} from '@/components/channel/channel-provider-selector';
import { ProviderIcon } from '@/components/common/provider-icon';
import {
  CHANNEL_INITIAL_CREDIT_MAX,
  CHANNEL_POINTS_PER_USD,
  formatChannelCreditPoints,
  usdToChannelPoints,
} from '@/utils/ai-credits';

const DEFAULT_INITIAL_FUNDS_USD = '100.00';

type DraftTestFailureKind =
  | 'auth'
  | 'baseUrl'
  | 'model'
  | 'rateLimit'
  | 'quota'
  | 'protocol'
  | 'unknown';

const DRAFT_TEST_MESSAGE_TRANSLATION_KEYS = {
  'model is returned by upstream model list; real image generation was not run':
    'dialog.testConnection.messages.imageModelFound',
  'validated local model metadata; upstream model listing is unsupported; real image generation was not run':
    'dialog.testConnection.messages.imageModelMetadataOnly',
  'model is not returned by upstream model list; real image generation was not run':
    'dialog.testConnection.messages.imageModelMissing',
  'Private channel API key is invalid or expired. Update the API key and try again.':
    'dialog.testConnection.messages.apiKeyInvalid',
  'model not found or endpoint not available': 'dialog.testConnection.messages.modelNotFound',
  'rate limit exceeded': 'dialog.testConnection.messages.rateLimited',
  'request timeout': 'dialog.testConnection.messages.timeout',
} as const;

type ChannelSetupCategory = 'common' | 'aggregator' | 'advanced';
type ChannelSetupKind = 'direct' | 'aggregator' | 'compatible' | 'local' | 'custom';

interface ChannelSetupOption {
  id: string;
  category: ChannelSetupCategory;
  kind: ChannelSetupKind;
  labelKey: string;
  descriptionKey: string;
  capabilityKey: string;
  channelProvider: string;
  providerFilter?: string;
  icon?: string;
  defaultApiBaseUrl?: string;
  lockProtocol?: boolean;
}

const CHANNEL_SETUP_CATEGORIES: ChannelSetupCategory[] = ['common', 'aggregator', 'advanced'];

const CHANNEL_SETUP_OPTIONS: ChannelSetupOption[] = [
  {
    id: 'openai',
    category: 'common',
    kind: 'direct',
    labelKey: 'dialog.setup.providers.openai.label',
    descriptionKey: 'dialog.setup.providers.openai.description',
    capabilityKey: 'dialog.setup.providers.openai.capabilities',
    channelProvider: 'openai',
    providerFilter: 'openai',
    icon: 'openai',
    lockProtocol: true,
  },
  {
    id: 'qwen',
    category: 'common',
    kind: 'direct',
    labelKey: 'dialog.setup.providers.qwen.label',
    descriptionKey: 'dialog.setup.providers.qwen.description',
    capabilityKey: 'dialog.setup.providers.qwen.capabilities',
    channelProvider: 'qwen',
    providerFilter: 'qwen',
    icon: 'qwen',
    lockProtocol: true,
  },
  {
    id: 'deepseek',
    category: 'common',
    kind: 'direct',
    labelKey: 'dialog.setup.providers.deepseek.label',
    descriptionKey: 'dialog.setup.providers.deepseek.description',
    capabilityKey: 'dialog.setup.providers.deepseek.capabilities',
    channelProvider: 'deepseek',
    providerFilter: 'deepseek',
    icon: 'deepseek',
    lockProtocol: true,
  },
  {
    id: 'anthropic',
    category: 'common',
    kind: 'direct',
    labelKey: 'dialog.setup.providers.anthropic.label',
    descriptionKey: 'dialog.setup.providers.anthropic.description',
    capabilityKey: 'dialog.setup.providers.anthropic.capabilities',
    channelProvider: 'anthropic',
    providerFilter: 'anthropic',
    icon: 'anthropic',
    lockProtocol: true,
  },
  {
    id: 'google',
    category: 'common',
    kind: 'direct',
    labelKey: 'dialog.setup.providers.google.label',
    descriptionKey: 'dialog.setup.providers.google.description',
    capabilityKey: 'dialog.setup.providers.google.capabilities',
    channelProvider: 'google',
    providerFilter: 'google',
    icon: 'google',
    lockProtocol: true,
  },
  {
    id: 'moonshot',
    category: 'common',
    kind: 'direct',
    labelKey: 'dialog.setup.providers.moonshot.label',
    descriptionKey: 'dialog.setup.providers.moonshot.description',
    capabilityKey: 'dialog.setup.providers.moonshot.capabilities',
    channelProvider: 'moonshotai-cn',
    providerFilter: 'moonshot',
    icon: 'moonshot',
    lockProtocol: true,
  },
  {
    id: 'mistral',
    category: 'common',
    kind: 'direct',
    labelKey: 'dialog.setup.providers.mistral.label',
    descriptionKey: 'dialog.setup.providers.mistral.description',
    capabilityKey: 'dialog.setup.providers.mistral.capabilities',
    channelProvider: 'mistral',
    providerFilter: 'mistral',
    icon: 'mistral',
    lockProtocol: true,
  },
  {
    id: 'openrouter',
    category: 'aggregator',
    kind: 'aggregator',
    labelKey: 'dialog.setup.providers.openrouter.label',
    descriptionKey: 'dialog.setup.providers.openrouter.description',
    capabilityKey: 'dialog.setup.providers.openrouter.capabilities',
    channelProvider: 'openrouter',
    providerFilter: 'openrouter',
    icon: 'openrouter',
    lockProtocol: true,
  },
  {
    id: 'openai-compatible',
    category: 'advanced',
    kind: 'compatible',
    labelKey: 'dialog.setup.providers.openaiCompatible.label',
    descriptionKey: 'dialog.setup.providers.openaiCompatible.description',
    capabilityKey: 'dialog.setup.providers.openaiCompatible.capabilities',
    channelProvider: 'openai-compatible',
    providerFilter: 'all',
    icon: 'openai',
    defaultApiBaseUrl: '',
    lockProtocol: true,
  },
  {
    id: 'ollama',
    category: 'advanced',
    kind: 'local',
    labelKey: 'dialog.setup.providers.ollama.label',
    descriptionKey: 'dialog.setup.providers.ollama.description',
    capabilityKey: 'dialog.setup.providers.ollama.capabilities',
    channelProvider: 'ollama',
    providerFilter: 'ollama',
    icon: 'ollama',
    lockProtocol: true,
  },
  {
    id: 'custom',
    category: 'advanced',
    kind: 'custom',
    labelKey: 'dialog.setup.providers.custom.label',
    descriptionKey: 'dialog.setup.providers.custom.description',
    capabilityKey: 'dialog.setup.providers.custom.capabilities',
    channelProvider: 'openai-compatible',
    providerFilter: 'all',
    icon: 'openai',
    defaultApiBaseUrl: '',
    lockProtocol: false,
  },
];

function getSetupOption(id?: string): ChannelSetupOption | undefined {
  if (!id) return undefined;
  const normalized = id.trim().toLowerCase();
  return CHANNEL_SETUP_OPTIONS.find(
    option =>
      option.id === normalized ||
      option.channelProvider.toLowerCase() === normalized ||
      option.providerFilter?.toLowerCase() === normalized
  );
}

function getSetupKindClassName(kind: ChannelSetupKind): string {
  switch (kind) {
    case 'direct':
      return 'border-emerald-200 bg-emerald-50 text-emerald-700';
    case 'aggregator':
      return 'border-blue-200 bg-blue-50 text-blue-700';
    case 'compatible':
      return 'border-amber-200 bg-amber-50 text-amber-700';
    case 'local':
      return 'border-slate-200 bg-slate-50 text-slate-700';
    case 'custom':
      return 'border-purple-200 bg-purple-50 text-purple-700';
    default:
      return 'border-muted bg-muted text-muted-foreground';
  }
}

/**
 * Props for ChannelDialog component.
 * Only supports fields that exist in current channel API.
 */
export interface ChannelDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: 'create' | 'edit';
  initial?: ChannelDetail | null;
  defaultChannelProvider?: string;
  lockChannelProvider?: boolean;
  providerOptions?: string[];
}

/** Utility: number or undefined */
function toNumberOrUndefined(value: string): number | undefined {
  const v = value.trim();
  if (!v) return undefined;
  const n = Number(v);
  return Number.isFinite(n) ? n : undefined;
}

/** Utility: sanitize numeric input to allow only non-negative integers */
function sanitizeNonNegativeInt(value: string, max: number): string {
  const digits = value.replace(/\D/g, '').replace(/^0+(?=\d)/, '');
  if (!digits) return '';

  const numeric = Number(digits);
  if (!Number.isFinite(numeric)) return String(max);

  return String(Math.min(numeric, max));
}

function sanitizeUsdInput(value: string, maxUsd: number): string {
  const normalized = value.replace(/[^\d.]/g, '');
  const [integer = '', ...decimalParts] = normalized.split('.');
  const decimal = decimalParts.join('').slice(0, 2);
  const next = decimalParts.length > 0 ? `${integer}.${decimal}` : integer;
  if (!next) return '';

  const numeric = Number(next);
  if (!Number.isFinite(numeric)) return '';
  if (numeric > maxUsd) return maxUsd.toFixed(2);

  return next;
}

function hasNativeQwenModel(models: ModelItem[]): boolean {
  return models.some(model => {
    if (model.provider !== 'qwen') return false;
    const useCases = model.use_cases ?? [];
    return (
      useCases.includes('image-gen') ||
      useCases.includes('rerank') ||
      useCases.includes('vision') ||
      useCases.includes('realtime-audio') ||
      /(?:rerank|image|vl|vision|omni|tongyi)/i.test(model.model)
    );
  });
}

function getCompatibilityWarningKey(
  channelProvider: string,
  selectedModels: ModelItem[]
): 'dialog.errors.qwenOpenAICompatibleMismatch' | 'dialog.errors.dashScopeProviderMismatch' | '' {
  if (channelProvider.trim().toLowerCase() !== 'openai-compatible') {
    return '';
  }

  if (hasNativeQwenModel(selectedModels)) {
    return 'dialog.errors.qwenOpenAICompatibleMismatch';
  }

  return '';
}

function capabilitiesToUseCases(capabilities: string[] | undefined): ModelUseCase[] {
  const values = new Set((capabilities ?? []).map(item => item.toLowerCase()));
  const useCases = new Set<ModelUseCase>();

  if (values.has('embedding') || values.has('embeddings') || values.has('embed')) {
    useCases.add('embedding');
  }
  if (values.has('rerank')) {
    useCases.add('rerank');
  }
  if (values.has('image') || values.has('image-gen') || values.has('image_generation')) {
    useCases.add('image-gen');
  }
  if (values.has('vision')) {
    useCases.add('vision');
  }
  if (values.has('reasoning')) {
    useCases.add('reasoning');
  }
  if (useCases.size === 0 || values.has('chat') || values.has('text')) {
    useCases.add('text-chat');
  }

  return Array.from(useCases);
}

function discoveredModelToItem(model: DiscoveredChannelModel, provider: string): ModelItem {
  const useCases = capabilitiesToUseCases(model.capabilities);
  return {
    id: `discovered-${provider}-${model.id}`,
    provider,
    model: model.id || model.name,
    model_name: model.display_name || model.name || model.id,
    family: provider,
    family_name: provider,
    status: 'active',
    tagline: '',
    is_flagship: false,
    is_recommended: false,
    is_featured: false,
    is_new: false,
    access_type: 'open',
    currency: 'USD',
    input_price: 0,
    output_price: 0,
    context_window: model.context_length ?? 0,
    max_output_tokens: 0,
    endpoints: {},
    features: {},
    tools: {},
    use_cases: useCases,
    input_modalities: [],
    output_modalities: [],
    is_enabled: true,
    is_available: true,
    is_configured: false,
    callable: true,
    tier: 'custom',
    created_at: model.created ?? Math.floor(Date.now() / 1000),
    updated_at: Math.floor(Date.now() / 1000),
  };
}

function classifyDraftTestFailure(message: string): DraftTestFailureKind {
  const normalized = message.toLowerCase();

  if (
    normalized.includes('api key') ||
    normalized.includes('apikey') ||
    normalized.includes('unauthorized') ||
    normalized.includes('forbidden') ||
    normalized.includes('invalid key') ||
    normalized.includes('authentication') ||
    normalized.includes('auth')
  ) {
    return 'auth';
  }

  if (
    normalized.includes('base_url') ||
    normalized.includes('base url') ||
    normalized.includes('connection refused') ||
    normalized.includes('no such host') ||
    normalized.includes('timeout') ||
    normalized.includes('deadline exceeded') ||
    normalized.includes('unsupported protocol scheme')
  ) {
    return 'baseUrl';
  }

  if (
    normalized.includes('model') &&
    (normalized.includes('not found') ||
      normalized.includes('not exist') ||
      normalized.includes('not available') ||
      normalized.includes('not registered') ||
      normalized.includes('unknown'))
  ) {
    return 'model';
  }

  if (normalized.includes('rate limit') || normalized.includes('too many requests')) {
    return 'rateLimit';
  }

  if (
    normalized.includes('quota') ||
    normalized.includes('balance') ||
    normalized.includes('insufficient') ||
    normalized.includes('billing')
  ) {
    return 'quota';
  }

  if (
    normalized.includes('protocol') ||
    normalized.includes('not supported') ||
    normalized.includes('unsupported') ||
    normalized.includes('method not allowed')
  ) {
    return 'protocol';
  }

  return 'unknown';
}

interface ChannelFormProps {
  mode: 'create' | 'edit';
  initial?: ChannelDetail | null;
  defaultChannelProvider?: string;
  lockChannelProvider?: boolean;
  onOpenChange: (open: boolean) => void;
}

function ChannelForm({
  mode,
  initial,
  defaultChannelProvider,
  lockChannelProvider = false,
  onOpenChange,
}: ChannelFormProps) {
  const t = useT('channels');
  const rawInitialChannelProvider =
    initial?.channel_provider ?? initial?.provider ?? defaultChannelProvider ?? 'openai-compatible';
  const initialSetupOption =
    mode === 'create' && defaultChannelProvider ? getSetupOption(defaultChannelProvider) : undefined;
  const initialChannelProvider =
    initialSetupOption?.channelProvider ??
    getChannelProviderOption(rawInitialChannelProvider)?.value ?? rawInitialChannelProvider;
  const initialProviderOption = getChannelProviderOption(initialChannelProvider);
  const initialFundsMaxLabel = CHANNEL_INITIAL_CREDIT_MAX.toLocaleString();
  const maxInitialFundsUsd = CHANNEL_INITIAL_CREDIT_MAX / CHANNEL_POINTS_PER_USD;

  // Local form state
  const [name, setName] = React.useState<string>(initial?.name ?? '');
  const [channelProvider, setChannelProvider] = React.useState<string>(initialChannelProvider);
  const [apiKey, setApiKey] = React.useState<string>('');
  const [apiBaseUrl, setApiBaseUrl] = React.useState<string>(
    initial?.api_base_url ?? initialProviderOption?.defaultApiBaseUrl ?? ''
  );
  const [initialFundsUsd, setInitialFundsUsd] = React.useState<string>(
    mode === 'create' ? DEFAULT_INITIAL_FUNDS_USD : ''
  );
  // Edit mode: toggle to update API key
  const [updateApiKey, setUpdateApiKey] = React.useState<boolean>(false);
  const [modelsSelected, setModelsSelected] = React.useState<string[]>(
    Array.isArray(initial?.models) ? (initial?.models as string[]) : []
  );
  const [selectedModelItems, setSelectedModelItems] = React.useState<ModelItem[]>([]);
  const [priority, setPriority] = React.useState<string>(
    initial?.priority != null ? String(initial.priority) : ''
  );
  const [weight, setWeight] = React.useState<string>(
    initial?.weight != null ? String(initial.weight) : ''
  );
  const [isEnabled, setIsEnabled] = React.useState<boolean>(initial?.is_enabled ?? true);
  const [advancedOpen, setAdvancedOpen] = React.useState<boolean>(false);
  const [draftTestResult, setDraftTestResult] = React.useState<ChannelModelTestResult | null>(
    null
  );
  const [draftTestIntent, setDraftTestIntent] = React.useState<'default' | null>(null);
  const draftTestRequestIdRef = React.useRef(0);
  const [discoveredModels, setDiscoveredModels] = React.useState<ModelItem[]>([]);
  const [discoverResult, setDiscoverResult] = React.useState<{
    total: number;
    provider: string;
  } | null>(null);
  const [setupStep, setSetupStep] = React.useState<'provider' | 'config'>(() =>
    mode === 'create' && !initialSetupOption && !lockChannelProvider ? 'provider' : 'config'
  );
  const [setupOption, setSetupOption] = React.useState<ChannelSetupOption | undefined>(
    initialSetupOption
  );
  const parsedInitialFundsUsd = initialFundsUsd.trim() ? Number(initialFundsUsd) : undefined;
  const initialFundsPoints = usdToChannelPoints(parsedInitialFundsUsd);
  const initialFundsPreview =
    initialFundsPoints === undefined ? 0 : Math.min(initialFundsPoints, CHANNEL_INITIAL_CREDIT_MAX);
  const hasValidInitialFunds =
    mode !== 'create' ||
    (initialFundsPoints !== undefined &&
      initialFundsPoints > 0 &&
      initialFundsPoints <= CHANNEL_INITIAL_CREDIT_MAX);
  const apiKeyRequired = mode === 'create' && channelProvider !== 'ollama';
  const compatibilityWarningKey = getCompatibilityWarningKey(channelProvider, selectedModelItems);

  const selectedProviderOption = React.useMemo(
    () => getChannelProviderOption(channelProvider),
    [channelProvider]
  );
  const mappedProvider = React.useMemo(
    () => selectedProviderOption?.provider || getMappedProvider(channelProvider),
    [channelProvider, selectedProviderOption]
  );
  const effectiveProviderFilter =
    mode === 'create' && setupOption?.providerFilter
      ? setupOption.providerFilter
      : lockChannelProvider
        ? mappedProvider
        : undefined;

  // Stable callbacks to prevent child component re-renders when parent re-renders

  const handleModelsChange = useCallback((models: string[]) => {
    setModelsSelected(models);
  }, []);

  const selectSetupOption = React.useCallback((option: ChannelSetupOption) => {
    const providerOption = getChannelProviderOption(option.channelProvider);
    setSetupOption(option);
    setChannelProvider(option.channelProvider);
    setApiBaseUrl(option.defaultApiBaseUrl ?? providerOption?.defaultApiBaseUrl ?? '');
    setModelsSelected([]);
    setSelectedModelItems([]);
    setDiscoveredModels([]);
    setDiscoverResult(null);
    setDraftTestResult(null);
    setSetupStep('config');
  }, []);

  const { createChannel, isCreating } = useCreateChannel();
  const { updateChannel, isUpdating } = useUpdateChannel();
  const { testDraftChannelModel, isTestingDraft } = useTestDraftChannelModel();
  const { discoverDraftChannelModels, isDiscoveringDraftModels } =
    useDiscoverDraftChannelModels();

  const handleNumericInput = (
    value: string,
    setter: React.Dispatch<React.SetStateAction<string>>,
    max: number
  ) => {
    setter(sanitizeNonNegativeInt(value, max));
  };

  const disabled = isCreating || isUpdating || isTestingDraft || isDiscoveringDraftModels;

  const representativeModel = modelsSelected[0];
  const canTestConnection =
    mode === 'create' &&
    Boolean(channelProvider.trim()) &&
    Boolean(apiBaseUrl.trim()) &&
    Boolean(representativeModel) &&
    (!apiKeyRequired || Boolean(apiKey.trim()));
  const canDiscoverModels =
    mode === 'create' &&
    Boolean(channelProvider.trim()) &&
    Boolean(apiBaseUrl.trim()) &&
    (!apiKeyRequired || Boolean(apiKey.trim()));
  const connectionTestHint =
    mode !== 'create'
      ? null
      : !apiBaseUrl.trim()
        ? t('dialog.testConnection.apiBaseUrlHint')
        : apiKeyRequired && !apiKey.trim()
          ? t('dialog.testConnection.apiKeyHint')
          : !representativeModel
            ? t('dialog.testConnection.selectModelHint')
            : null;
  const draftTestFailureKind =
    draftTestResult && !draftTestResult.success
      ? classifyDraftTestFailure(draftTestResult.message)
      : null;
  const draftTestMessageKey = draftTestResult?.message
    ? DRAFT_TEST_MESSAGE_TRANSLATION_KEYS[
        draftTestResult.message as keyof typeof DRAFT_TEST_MESSAGE_TRANSLATION_KEYS
      ]
    : undefined;
  const draftTestDisplayMessage = draftTestResult
    ? draftTestMessageKey
      ? t(draftTestMessageKey as never)
      : draftTestResult.message ||
        (draftTestResult.success
          ? t('dialog.testConnection.messages.successFallback')
          : t('dialog.testConnection.messages.failedFallback'))
    : '';
  const testConnectionDescription =
    modelsSelected.length > 1
      ? t('dialog.testConnection.descriptionWithModelCount', {
          count: modelsSelected.length,
        })
      : representativeModel
        ? t('dialog.testConnection.descriptionWithModel', {
            model: representativeModel,
          })
        : t('dialog.testConnection.description');
  const createReadiness =
    mode !== 'create'
      ? null
      : draftTestResult?.success
        ? 'verified'
        : draftTestResult && !draftTestResult.success
          ? 'failed'
          : representativeModel
            ? 'untested'
            : 'missingModel';

  React.useEffect(() => {
    draftTestRequestIdRef.current += 1;
    setDraftTestResult(null);
  }, [apiKey, apiBaseUrl, channelProvider, representativeModel]);

  React.useEffect(() => {
    setDiscoveredModels([]);
    setDiscoverResult(null);
  }, [apiKey, apiBaseUrl, channelProvider]);

  const onTestConnection = async (): Promise<void> => {
    if (!canTestConnection || !representativeModel) return;
    const requestId = draftTestRequestIdRef.current + 1;
    draftTestRequestIdRef.current = requestId;
    setDraftTestResult(null);
    setDraftTestIntent('default');

    const testedProvider = channelProvider.trim();
    const testedApiKey = apiKey.trim();
    const testedApiBaseUrl = apiBaseUrl.trim();
    const testedModel = representativeModel;

    try {
      const result = await testDraftChannelModel({
        channel_provider: testedProvider,
        api_key: testedApiKey,
        api_base_url: testedApiBaseUrl,
        model: testedModel,
      });

      const inputsStillMatch =
        channelProvider.trim() === testedProvider &&
        apiKey.trim() === testedApiKey &&
        apiBaseUrl.trim() === testedApiBaseUrl &&
        modelsSelected[0] === testedModel;

      if (draftTestRequestIdRef.current === requestId && inputsStillMatch) {
        setDraftTestResult(result);
      }
    } catch {
      if (draftTestRequestIdRef.current === requestId) {
        setDraftTestResult(null);
      }
    } finally {
      if (draftTestRequestIdRef.current === requestId) {
        setDraftTestIntent(null);
      }
    }
  };

  const onDiscoverModels = async (): Promise<void> => {
    if (!canDiscoverModels) return;
    setDiscoveredModels([]);
    setDiscoverResult(null);

    const fallbackProvider =
      (effectiveProviderFilter && effectiveProviderFilter !== 'all'
        ? effectiveProviderFilter
        : undefined) ||
      getMappedProvider(channelProvider) ||
      channelProvider.trim();

    try {
      const result = await discoverDraftChannelModels({
        channel_provider: channelProvider.trim(),
        api_key: apiKey.trim(),
        api_base_url: apiBaseUrl.trim(),
      });
      const nextModels = result.models.map(model =>
        discoveredModelToItem(model, model.provider || fallbackProvider)
      );
      setDiscoveredModels(nextModels);
      setDiscoverResult({ total: nextModels.length, provider: fallbackProvider });
    } catch {
      setDiscoveredModels([]);
      setDiscoverResult(null);
    }
  };
  const onSubmit = async (): Promise<void> => {
    if (compatibilityWarningKey) {
      toast.error(t(compatibilityWarningKey as never));
      return;
    }

    const normalizedApiBaseUrl = apiBaseUrl.trim();

    // Build payloads strictly from allowed fields
    const common = {
      channel_provider: channelProvider.trim(),
      api_base_url: normalizedApiBaseUrl || undefined,
      priority: toNumberOrUndefined(priority),
      weight: toNumberOrUndefined(weight),
      // Advanced fields for creation or legacy
    } as const;

    if (mode === 'create') {
      const payload: CreateChannelRequest = {
        name: name.trim(),
        api_key: apiKey.trim(),
        ...common,
        models: modelsSelected.length ? modelsSelected : undefined,
        initial_funds: initialFundsPoints,
      };
      await createChannel(payload);
      onOpenChange(false);
      return;
    }

    // edit mode requires id
    if (!initial) return;

    // Build partial update - only include changed fields
    const update: UpdateChannelRequest = {};

    // Compare and add only changed fields
    if (name.trim() !== (initial.name ?? '')) {
      update.name = name.trim() || undefined;
    }
    if (channelProvider !== initialChannelProvider) {
      update.channel_provider = channelProvider || undefined;
    }
    if (normalizedApiBaseUrl !== (initial.api_base_url ?? '')) {
      update.api_base_url = normalizedApiBaseUrl || undefined;
    }
    if (updateApiKey && apiKey.trim()) {
      update.api_key = apiKey.trim();
    }
    if (JSON.stringify(modelsSelected) !== JSON.stringify(initial.models ?? [])) {
      update.models = modelsSelected;
    }
    if (isEnabled !== (initial.is_enabled ?? true)) {
      update.is_enabled = isEnabled;
    }

    const newWeight = toNumberOrUndefined(weight);
    if (newWeight !== initial.weight) {
      update.weight = newWeight;
    }

    // Only submit if there are changes
    if (Object.keys(update).length === 0) {
      onOpenChange(false);
      return;
    }

    await updateChannel(initial.id, update);
    onOpenChange(false);
  };

  const apiKeyPlaceholder =
    selectedProviderOption?.apiKeyPlaceholder || t('dialog.placeholders.apiKey');
  const apiBaseUrlPlaceholder =
    selectedProviderOption?.defaultApiBaseUrl ||
    selectedProviderOption?.apiBaseUrlPlaceholder ||
    t('dialog.placeholders.apiBaseUrl');

  if (mode === 'create' && setupStep === 'provider') {
    return (
      <div className="flex h-full flex-col overflow-hidden">
        <DialogHeader className="px-6 pt-6">
          <DialogTitle>{t('dialog.titleCreate')}</DialogTitle>
          <DialogDescription>{t('dialog.setup.description')}</DialogDescription>
        </DialogHeader>

        <DialogBody className="h-0 grow overflow-y-auto px-6">
          <div className="mb-4 flex items-center gap-2 text-sm text-muted-foreground">
            <span className="font-medium text-primary">{t('dialog.setup.steps.provider')}</span>
            <span>/</span>
            <span>{t('dialog.setup.steps.config')}</span>
          </div>

          <div className="space-y-5">
            {CHANNEL_SETUP_CATEGORIES.map(category => (
              <section key={category} className="space-y-2">
                <div>
                  <div className="text-sm font-semibold text-foreground">
                    {t(`dialog.setup.categories.${category}` as never)}
                  </div>
                  <div className="text-xs text-muted-foreground">
                    {t(`dialog.setup.categoryDescriptions.${category}` as never)}
                  </div>
                </div>
                <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
                  {CHANNEL_SETUP_OPTIONS.filter(option => option.category === category).map(
                    option => {
                      const providerOption = getChannelProviderOption(option.channelProvider);
                      return (
                        <button
                          key={option.id}
                          type="button"
                          className="min-h-[132px] rounded-md border bg-background p-4 text-left transition hover:border-primary/60 hover:bg-primary/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          onClick={() => selectSetupOption(option)}
                        >
                          <div className="flex items-start justify-between gap-3">
                            <div className="flex min-w-0 items-center gap-2">
                              {option.icon ? (
                                <ProviderIcon size={24} provider={option.icon} />
                              ) : null}
                              <div className="truncate text-sm font-semibold">
                                {t(option.labelKey as never)}
                              </div>
                            </div>
                            <span
                              className={cn(
                                'shrink-0 rounded-full border px-2 py-0.5 text-[11px] font-medium',
                                getSetupKindClassName(option.kind)
                              )}
                            >
                              {t(`dialog.setup.kinds.${option.kind}.label` as never)}
                            </span>
                          </div>
                          <div className="mt-3 line-clamp-2 text-xs leading-5 text-muted-foreground">
                            {t(option.descriptionKey as never)}
                          </div>
                          <div className="mt-3 space-y-1 text-xs">
                            <div className="text-muted-foreground">
                              {t('dialog.setup.fields.modelStrategy')}:{' '}
                              <span className="text-foreground">
                                {t(`dialog.setup.kinds.${option.kind}.strategy` as never)}
                              </span>
                            </div>
                            <div className="text-muted-foreground">
                              {t('dialog.setup.fields.capabilities')}:{' '}
                              <span className="text-foreground">
                                {t(option.capabilityKey as never)}
                              </span>
                            </div>
                            <div className="text-muted-foreground">
                              {t('dialog.setup.fields.protocol')}:{' '}
                              <span className="text-foreground">
                                {providerOption
                                  ? t(providerOption.labelKey as never)
                                  : option.channelProvider}
                              </span>
                            </div>
                          </div>
                        </button>
                      );
                    }
                  )}
                </div>
              </section>
            ))}
          </div>
        </DialogBody>

        <div className="flex shrink-0 items-center justify-end gap-2 border-t px-6 py-4">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('dialog.buttons.cancel')}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="h-full overflow-hidden flex flex-col gap-2">
      <DialogHeader className="px-6 pt-6">
        <DialogTitle>
          {mode === 'create' ? t('dialog.titleCreate') : t('dialog.titleEdit')}
        </DialogTitle>
        <DialogDescription>{t('description')}</DialogDescription>
      </DialogHeader>

      <DialogBody className="grid h-0 grow grid-cols-1 gap-4 overflow-y-auto xl:grid-cols-3 xl:overflow-hidden">
        <div className="min-w-0 space-y-3 xl:overflow-y-auto xl:pr-3">
          {mode === 'create' && setupOption && (
            <div className="rounded-md border bg-muted/30 p-3">
              <button
                type="button"
                className="mb-2 inline-flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
                onClick={() => setSetupStep('provider')}
                disabled={disabled}
              >
                <ArrowLeft className="h-3.5 w-3.5" />
                {t('dialog.setup.actions.changeProvider')}
              </button>
              <div className="flex items-center gap-2">
                {setupOption.icon ? <ProviderIcon size={24} provider={setupOption.icon} /> : null}
                <div>
                  <div className="flex flex-wrap items-center gap-2">
                    <div className="text-sm font-semibold">{t(setupOption.labelKey as never)}</div>
                    <span
                      className={cn(
                        'rounded-full border px-2 py-0.5 text-[11px] font-medium',
                        getSetupKindClassName(setupOption.kind)
                      )}
                    >
                      {t(`dialog.setup.kinds.${setupOption.kind}.label` as never)}
                    </span>
                  </div>
                  <div className="text-xs text-muted-foreground">
                    {t(setupOption.descriptionKey as never)}
                  </div>
                </div>
              </div>
              <div className="mt-3 rounded-md border bg-background px-3 py-2 text-xs leading-5">
                <div className="font-medium text-foreground">
                  {t(`dialog.setup.kinds.${setupOption.kind}.headline` as never)}
                </div>
                <div className="mt-1 text-muted-foreground">
                  {t(`dialog.setup.kinds.${setupOption.kind}.guidance` as never)}
                </div>
              </div>
            </div>
          )}
          <div className="text-sm font-semibold text-foreground">{t('dialog.basic')}</div>
          <div className="space-y-1">
            <div className="text-sm font-medium">
              {t('dialog.labels.name')}
              <span className="text-destructive ml-0.5">*</span>
            </div>
            <Input
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder={t('dialog.placeholders.name')}
            />
          </div>
          <div className="space-y-1">
            <div className="text-sm font-medium">
              {t('dialog.labels.provider')}
              <span className="text-destructive ml-0.5">*</span>
            </div>
            <ChannelProviderSelector
              value={channelProvider}
              onChange={option => {
                if (lockChannelProvider || setupOption?.lockProtocol) return;
                setChannelProvider(option.value);
                if (option.defaultApiBaseUrl) {
                  setApiBaseUrl(option.defaultApiBaseUrl);
                  return;
                }
                setApiBaseUrl('');
              }}
              disabled={disabled || lockChannelProvider || Boolean(setupOption?.lockProtocol)}
            />
            {(lockChannelProvider || setupOption?.lockProtocol) && (
              <div className="text-xs text-muted-foreground">
                {t('dialog.hints.providerLocked')}
              </div>
            )}
            {selectedProviderOption?.notesKey && (
              <div className="text-xs text-muted-foreground">
                {t(selectedProviderOption.notesKey as never)}
              </div>
            )}
          </div>
          <div className="space-y-1">
            <div className="text-sm font-medium">
              {t('dialog.labels.apiKey')}
              {apiKeyRequired && <span className="text-destructive ml-0.5">*</span>}
              {mode === 'edit' && updateApiKey && (
                <span className="text-destructive ml-0.5">*</span>
              )}
            </div>
            {mode === 'edit' ? (
              <div className="space-y-2">
                {updateApiKey ? (
                  <PasswordInput
                    value={apiKey}
                    onChange={e => setApiKey(e.target.value)}
                    placeholder={apiKeyPlaceholder}
                    autoComplete="new-password"
                  />
                ) : (
                  <div className="text-sm bg-muted px-3 py-2 rounded-md font-mono">
                    {initial?.api_key_masked || '••••••••••••••••'}
                  </div>
                )}
                <div className="flex items-center gap-2">
                  <Switch
                    checked={updateApiKey}
                    onCheckedChange={v => {
                      setUpdateApiKey(Boolean(v));
                      if (!v) {
                        setApiKey('');
                      }
                    }}
                  />
                  <span className="text-sm text-muted-foreground">
                    {t('dialog.labels.updateApiKey')}
                  </span>
                </div>
              </div>
            ) : (
              <PasswordInput
                value={apiKey}
                onChange={e => setApiKey(e.target.value)}
                placeholder={apiKeyPlaceholder}
                autoComplete="new-password"
              />
            )}
          </div>
          <div className="space-y-1">
            <div className="text-sm font-medium">
              {t('dialog.labels.apiBaseUrl')}
              <span className="text-destructive ml-0.5">*</span>
            </div>
            <Input
              value={apiBaseUrl}
              onChange={e => setApiBaseUrl(e.target.value)}
              placeholder={apiBaseUrlPlaceholder}
            />
          </div>
          {mode === 'create' && (
            <div className="space-y-2 rounded-md border bg-muted/30 p-3">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <div className="text-sm font-medium">{t('dialog.testConnection.title')}</div>
                  <div className="mt-1 text-xs leading-5 text-muted-foreground">
                    {testConnectionDescription}
                  </div>
                </div>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => void onTestConnection()}
                  disabled={!canTestConnection || disabled}
                  className="shrink-0"
                >
                  {isTestingDraft && draftTestIntent === 'default' && (
                    <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                  )}
                  {t('dialog.testConnection.button')}
                </Button>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={onDiscoverModels}
                  disabled={!canDiscoverModels || disabled}
                >
                  {isDiscoveringDraftModels ? (
                    <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <RefreshCw className="mr-1.5 h-3.5 w-3.5" />
                  )}
                  {t('dialog.discoverModels.button')}
                </Button>
                {discoverResult && (
                  <span className="text-xs text-muted-foreground">
                    {t('dialog.discoverModels.messages.success', {
                      count: discoverResult.total,
                    })}
                  </span>
                )}
              </div>
              {connectionTestHint && (
                <div className="flex items-start gap-2 text-xs text-muted-foreground">
                  <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  <span>{connectionTestHint}</span>
                </div>
              )}
              {draftTestResult && (
                <div
                  className={cn(
                    'flex items-start gap-2 rounded-md border px-3 py-2 text-xs',
                    draftTestResult.success
                      ? 'border-emerald-200 bg-emerald-50 text-emerald-700'
                      : 'border-destructive/20 bg-destructive/5 text-destructive'
                  )}
                >
                  {draftTestResult.success ? (
                    <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  ) : (
                    <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  )}
                  <div className="min-w-0">
                    <div className="font-medium">
                      {draftTestResult.success
                        ? t('dialog.testConnection.messages.success')
                        : t('dialog.testConnection.messages.failed')}
                    </div>
                    <div className="mt-0.5 break-words">{draftTestDisplayMessage}</div>
                    <div className="mt-1 font-medium">
                      {draftTestResult.success
                        ? t('dialog.testConnection.nextSteps.success')
                        : t(
                            `dialog.testConnection.nextSteps.failures.${draftTestFailureKind ?? 'unknown'}` as never
                          )}
                    </div>
                    {draftTestResult.response_time_ms > 0 && (
                      <div className="mt-0.5">
                        {t('dialog.testConnection.latency', {
                          ms: draftTestResult.response_time_ms,
                        })}
                      </div>
                    )}
                  </div>
                </div>
              )}
            </div>
          )}
          {mode === 'create' && (
            <div className="space-y-2 rounded-md border border-primary/30 bg-primary/5 p-3">
              <div className="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
                <div className="text-sm font-medium">
                  {t('dialog.labels.initialFunds')}
                  <span className="text-destructive ml-0.5">*</span>
                </div>
                <div className="text-xs text-muted-foreground">{t('credit.rate')}</div>
              </div>
              <p className="text-xs text-muted-foreground">
                {t('dialog.hints.initialFundsDefault')}
              </p>
              <div className="relative">
                <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
                  $
                </span>
                <Input
                  className="pl-8 pr-16"
                  inputMode="decimal"
                  value={initialFundsUsd}
                  onChange={e =>
                    setInitialFundsUsd(sanitizeUsdInput(e.target.value, maxInitialFundsUsd))
                  }
                  placeholder={t('dialog.placeholders.initialFunds')}
                />
                <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
                  USD
                </span>
              </div>
              <div className="flex flex-wrap gap-2">
                {[50, 100, 500].map(amount => (
                  <Button
                    key={amount}
                    type="button"
                    variant="outline"
                    size="sm"
                    className="h-7 rounded-sm px-2 text-xs"
                    onClick={() => setInitialFundsUsd(amount.toFixed(2))}
                  >
                    {t('credit.quickAmount', { amount })}
                  </Button>
                ))}
              </div>
              <p className="text-xs text-muted-foreground">
                {t('credit.usdToPoints', {
                  usd: initialFundsUsd ? `$${Number(initialFundsUsd || 0).toFixed(2)}` : '$0.00',
                  points: formatChannelCreditPoints(initialFundsPreview),
                })}
              </p>
              <p className="text-xs text-muted-foreground">
                {t('dialog.hints.initialFundsMax', { max: initialFundsMaxLabel })}
              </p>
            </div>
          )}
          <div
            className="flex items-center justify-between cursor-pointer py-2 mt-2"
            onClick={() => setAdvancedOpen(v => !v)}
          >
            <div className="text-sm font-semibold text-foreground">{t('dialog.advanced')}</div>
            <ChevronDown
              className={`h-4 w-4 transition-transform ${advancedOpen ? '' : 'rotate-90'}`}
            />
          </div>
          <div
            className={cn(
              'space-y-2 transition-all duration-300 overflow-hidden',
              advancedOpen ? 'h-auto opacity-100 pb-3' : 'h-0 opacity-0'
            )}
          >
            <div id="advanced-section" className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div className="space-y-1">
                <div className="text-sm font-medium">{t('dialog.labels.priority')}</div>
                <Input
                  inputMode="numeric"
                  value={priority}
                  onChange={e => handleNumericInput(e.target.value, setPriority, 9999)}
                  placeholder={t('dialog.placeholders.priority')}
                  min={0}
                  max={9999}
                />
                <p className="text-xs text-muted-foreground">{t('dialog.hints.priority')}</p>
              </div>
              <div className="space-y-1">
                <div className="text-sm font-medium">{t('dialog.labels.weight')}</div>
                <Input
                  inputMode="numeric"
                  value={weight}
                  onChange={e => handleNumericInput(e.target.value, setWeight, 9999)}
                  placeholder={t('dialog.placeholders.weight')}
                  min={0}
                  max={9999}
                />
                <p className="text-xs text-muted-foreground">{t('dialog.hints.weight')}</p>
              </div>
              <div className="flex items-center gap-3 sm:col-span-2">
                <div className="text-sm font-medium">{t('dialog.labels.enabled')}</div>
                <Switch checked={isEnabled} onCheckedChange={v => setIsEnabled(Boolean(v))} />
              </div>
            </div>
          </div>
        </div>
        <div className="min-w-0 xl:col-span-2 xl:overflow-y-auto xl:pl-3">
          {compatibilityWarningKey && (
            <div className="mb-3 rounded-md border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive">
              {t(compatibilityWarningKey as never)}
            </div>
          )}
          <ModelMultiSelector
            value={modelsSelected}
            onChange={handleModelsChange}
            onSelectionMetaChange={setSelectedModelItems}
            placeholder={t('dialog.placeholders.modelsCsv')}
            className="min-h-[360px] max-h-[calc(92vh-12rem)] overflow-hidden xl:h-full"
            columns={2}
            preferredProvider={effectiveProviderFilter || mappedProvider}
            autoCollapseOthers={(effectiveProviderFilter || mappedProvider) !== 'all'}
            providerFilter={effectiveProviderFilter}
            supplementalModels={discoveredModels}
            selectionPolicy={mode === 'create' ? 'catalog' : 'available'}
          />
        </div>
      </DialogBody>

      <div className="pb-6 px-6 pt-4 border-t shrink-0 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        {mode === 'create' && createReadiness && (
          <div
            className={cn(
              'flex min-w-0 items-start gap-2 text-xs',
              createReadiness === 'verified'
                ? 'text-emerald-700'
                : createReadiness === 'failed'
                  ? 'text-destructive'
                  : 'text-muted-foreground'
            )}
          >
            {createReadiness === 'verified' ? (
              <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            ) : (
              <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            )}
            <span className="min-w-0">
              {t(`dialog.testConnection.readiness.${createReadiness}` as never)}
            </span>
          </div>
        )}
        <div className="flex shrink-0 items-center justify-end gap-2">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={disabled}>
            {t('dialog.buttons.cancel')}
          </Button>
          <Button
            onClick={onSubmit}
            disabled={
              disabled ||
              !name.trim() ||
              !channelProvider.trim() ||
              !apiBaseUrl.trim() ||
              !hasValidInitialFunds ||
              Boolean(compatibilityWarningKey) ||
              (mode === 'create' && apiKeyRequired && !apiKey.trim()) ||
              (mode === 'edit' && updateApiKey && !apiKey.trim())
            }
          >
            {mode === 'create' ? t('dialog.buttons.create') : t('dialog.buttons.save')}
          </Button>
        </div>
      </div>
    </div>
  );
}

export default function ChannelDialog({
  open,
  onOpenChange,
  mode,
  initial,
  defaultChannelProvider,
  lockChannelProvider,
  providerOptions: _providerOptions,
}: ChannelDialogProps): JSX.Element {
  const normalizedInitial = React.useMemo(() => {
    if (!initial) return initial;
    const rawChannelProvider = initial?.channel_provider ?? initial?.provider;
    if (!rawChannelProvider) return initial;
    const option = getChannelProviderOption(rawChannelProvider);
    if (!option) return initial;
    return {
      ...initial,
      provider: option.value,
      channel_provider: option.value,
    };
  }, [initial]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-[96vw] max-w-[96vw] h-[92vh] p-0">
        {open && (
          <ChannelForm
            mode={mode}
            initial={normalizedInitial}
            defaultChannelProvider={defaultChannelProvider}
            lockChannelProvider={lockChannelProvider}
            onOpenChange={onOpenChange}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}
