'use client';

import React, { useEffect, useMemo, useRef } from 'react';
import { useFieldArray, useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import * as z from 'zod';
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Checkbox } from '@/components/ui/checkbox';
import { Badge } from '@/components/ui/badge';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Alert, AlertDescription } from '@/components/ui/alert';
import {
  X,
  Box,
  CreditCard,
  Layers,
  Activity,
  Zap,
  Terminal,
  Settings,
  List,
  Info,
  Plus,
  Trash2,
} from 'lucide-react';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import { useModelParameterRules } from '@/hooks/model/use-model-parameter-rules';
import type {
  CreateCustomModelRequest,
  ModelConfigParameter,
  ModelItem,
  ModelUseCase,
  ModelParameters,
  ParameterRuleItem,
  ParameterValueType,
  UpdateCustomModelRequest,
} from '@/services/types/model';
import { USE_CASE_BADGE_COLORS, USE_CASE_ORDER } from '@/config/model-colors';
import { resolveModelConfigParameterCopy } from '@/utils/model-config-parameter-i18n';
import { CUSTOM_MODEL_ID_PATTERN } from '@/utils/model-id';
import {
  type BillingDisplaySettings,
  billingDisplayInputToUSD,
  billingDisplayInputValueFromUSD,
  getBillingCurrencySymbol,
  getBillingDisplaySettings,
} from '@/utils/billing-display';
import { useOrganizationStore } from '@/store/organization-store';
import {
  buildCustomModelPricing,
  getRealtimeAudioBillingMode,
  readCustomModelMeteredPrice,
  getImageBillingMode,
  readCustomModelPricePerImage,
  readCustomModelVideoRates,
  readImageTokenPrice,
  readRealtimeAudioPrice,
} from '@/utils/build-custom-model-pricing';
import { hasCacheBillingCapability } from '@/utils/model-price';
import {
  MISSING_PRICING_FIELD_LABELS,
  missingModelPricing,
} from '@/utils/model-pricing-requirements';

const useCases: ModelUseCase[] = USE_CASE_ORDER;
const EXCLUSIVE_USE_CASES: string[] = ['text-chat', 'embedding', 'rerank'];
const CONFIG_PARAMETER_TYPES = [
  'int',
  'float',
  'string',
  'boolean',
  'text',
] as const satisfies readonly ParameterValueType[];

const endpointSchema = z.object({
  chat_completions: z.boolean().default(false),
  responses: z.boolean().default(false),
  realtime: z.boolean().default(false),
  assistants: z.boolean().default(false),
  batch: z.boolean().default(false),
  embeddings: z.boolean().default(false),
  fine_tuning: z.boolean().default(false),
  image_generation: z.boolean().default(false),
  vision: z.boolean().default(false),
  speech_generation: z.boolean().default(false),
  transcription: z.boolean().default(false),
  translation: z.boolean().default(false),
  moderation: z.boolean().default(false),
  videos: z.boolean().default(false),
  image_edit: z.boolean().default(false),
});

const featureSchema = z.object({
  streaming: z.boolean().default(false),
  function_calling: z.boolean().default(false),
  structured_output: z.boolean().default(false),
  json_mode: z.boolean().default(false),
  distillation: z.boolean().default(false),
  reasoning: z.boolean().default(false),
  system_prompt: z.boolean().default(false),
});

const toolSchema = z.object({
  web_search: z.boolean().default(false),
  file_search: z.boolean().default(false),
  image_generation: z.boolean().default(false),
  code_interpreter: z.boolean().default(false),
  computer_use: z.boolean().default(false),
  mcp: z.boolean().default(false),
  parallel_tool_calls: z.boolean().default(false),
});

const parameterSchema = z.object({
  temperature: z.boolean().default(true),
  top_p: z.boolean().default(true),
  presence_penalty: z.boolean().default(false),
  frequency_penalty: z.boolean().default(false),
  logit_bias: z.boolean().default(false),
  seed: z.boolean().default(false),
  stop: z.boolean().default(true),
  max_stop_sequences: z.coerce.number().min(0).default(4),
});
const ENDPOINT_KEYS = Object.keys(endpointSchema.shape) as Array<
  keyof z.infer<typeof endpointSchema>
>;
const FEATURE_KEYS = Object.keys(featureSchema.shape) as Array<keyof z.infer<typeof featureSchema>>;
const TOOL_KEYS = Object.keys(toolSchema.shape) as Array<keyof z.infer<typeof toolSchema>>;
const MODALITY_OPTIONS = [
  'text',
  'image',
  'audio',
  'video',
  'document',
  'json',
  'embedding',
] as const;
const LEGACY_PARAMETER_KEYS = ['temperature', 'top_p', 'seed', 'stop'] as const;

interface ConfigParameterFormValue {
  name: string;
  template_key: string;
  type: ParameterValueType;
  required: boolean;
  default: string;
  min: string;
  max: string;
  precision: string;
}

function createEmptyConfigParameter(): ConfigParameterFormValue {
  return {
    name: '',
    template_key: '',
    type: 'float',
    required: false,
    default: '',
    min: '',
    max: '',
    precision: '',
  };
}

function stringifyConfigValue(value: unknown): string {
  if (value === null || value === undefined) {
    return '';
  }

  if (typeof value === 'string') {
    return value;
  }

  if (typeof value === 'number' || typeof value === 'boolean') {
    return String(value);
  }

  return '';
}

function toConfigParameterFormValue(parameter: ParameterRuleItem): ConfigParameterFormValue {
  return {
    name: parameter.name,
    template_key: parameter.template_key,
    type: parameter.type,
    required: parameter.required,
    default: stringifyConfigValue(parameter.default),
    min: stringifyConfigValue(parameter.min),
    max: stringifyConfigValue(parameter.max),
    precision:
      typeof parameter.precision === 'number' && Number.isFinite(parameter.precision)
        ? String(parameter.precision)
        : '',
  };
}

function isNumericParameterType(type: ParameterValueType): boolean {
  return type === 'int' || type === 'float';
}

function parseNumericConfigValue(value: string): number | undefined {
  const trimmed = value.trim();
  if (!trimmed) {
    return undefined;
  }

  const parsed = Number(trimmed);
  if (!Number.isFinite(parsed)) {
    return undefined;
  }

  return parsed;
}

/**
 * Converts a canonical USD price from the stored pricing JSON into the
 * organization's display-currency input value. Media prices are persisted in
 * USD, so the editor must apply the current rate here to stay symmetric with
 * billingDisplayInputToUSD on submit; otherwise a CNY organization would divide
 * the already-USD amount a second time.
 */
function displayPriceInputValue(usdValue: string, billingDisplay: BillingDisplaySettings): string {
  if (usdValue.trim() === '') return '';
  return billingDisplayInputValueFromUSD(usdValue, true, billingDisplay);
}

function normalizeTier(value?: string): 'standard' | 'premium' | 'enterprise' {
  if (value === 'premium' || value === 'enterprise' || value === 'standard') {
    return value;
  }

  return 'standard';
}

function buildConfigParameters(
  values: ConfigParameterFormValue[] | undefined
): ModelConfigParameter[] {
  return (values ?? []).map(value => {
    const parameter: ModelConfigParameter = {
      name: value.name.trim(),
      template_key: value.template_key.trim(),
      type: value.type,
      required: value.required,
    };

    if (value.type === 'boolean') {
      if (value.default === 'true') {
        parameter.default = true;
      } else if (value.default === 'false') {
        parameter.default = false;
      }

      return parameter;
    }

    if (isNumericParameterType(value.type)) {
      const defaultValue = parseNumericConfigValue(value.default);
      const minValue = parseNumericConfigValue(value.min);
      const maxValue = parseNumericConfigValue(value.max);
      const precisionValue = value.precision.trim() ? Number(value.precision.trim()) : undefined;

      if (defaultValue !== undefined) {
        parameter.default = value.type === 'int' ? Math.round(defaultValue) : defaultValue;
      }
      if (minValue !== undefined) {
        parameter.min = value.type === 'int' ? Math.round(minValue) : minValue;
      }
      if (maxValue !== undefined) {
        parameter.max = value.type === 'int' ? Math.round(maxValue) : maxValue;
      }
      if (precisionValue !== undefined && Number.isFinite(precisionValue)) {
        parameter.precision = Math.max(0, Math.round(precisionValue));
      }

      return parameter;
    }

    if (value.default !== '') {
      parameter.default = value.default;
    }

    return parameter;
  });
}

const createSchema = (t: (key: string, values?: Record<string, string | number>) => string) =>
  z
    .object({
      model: z
        .string()
        .trim()
        .min(1, t('aiProviders.models.validation.modelRequired'))
        .regex(CUSTOM_MODEL_ID_PATTERN, t('aiProviders.models.validation.modelPattern')),
      model_name: z.string().min(1, t('aiProviders.models.validation.modelNameRequired')),
      use_cases: z.array(z.string()).min(1, t('aiProviders.models.validation.useCaseRequired')),
      family: z.string().optional(),
      family_name: z.string().optional(),
      status: z.string().optional(),
      tagline: z.string().optional(),
      description: z.string().optional(),
      tier: z.enum(['standard', 'premium', 'enterprise']).default('standard'),

      context_window: z.coerce.number().min(0).optional(),
      max_output_tokens: z.coerce.number().min(0).optional(),
      input_price: z.string().optional(),
      output_price: z.string().optional(),
      cache_read_price: z.string().optional(),
      cache_write_price: z.string().optional(),
      cache_write_5m_price: z.string().optional(),
      cache_write_1h_price: z.string().optional(),
      price_per_image: z.string().optional(),
      image_billing_mode: z.enum(['per_image', 'token']).default('per_image'),
      image_token_text_price: z.string().optional(),
      image_token_image_price: z.string().optional(),
      image_token_output_price: z.string().optional(),
      video_rates: z
        .array(
          z.object({
            resolution: z.string().optional(),
            price_per_second: z.string().optional(),
          })
        )
        .default([{ resolution: '720p', price_per_second: '' }]),
      speech_to_text_price: z.string().optional(),
      text_to_speech_price: z.string().optional(),
      music_price_per_track: z.string().optional(),
      realtime_audio_billing_mode: z.enum(['text', 'audio']).default('text'),
      realtime_audio_input_price: z.string().optional(),
      realtime_audio_output_price: z.string().optional(),

      input_modalities: z.array(z.string()).min(1),
      output_modalities: z.array(z.string()).min(1),

      knowledge_cutoff: z.string().optional(),
      is_active: z.boolean().default(true),

      endpoints: endpointSchema,
      features: featureSchema,
      tools: toolSchema,
      parameters: parameterSchema,
      config_parameters: z.array(
        z
          .object({
            name: z
              .string()
              .trim()
              .min(1, t('aiProviders.models.validation.configParameterNameRequired')),
            template_key: z
              .string()
              .trim()
              .min(1, t('aiProviders.models.validation.configParameterTemplateKeyRequired')),
            type: z.enum(CONFIG_PARAMETER_TYPES),
            required: z.boolean().default(false),
            default: z.string().default(''),
            min: z.string().default(''),
            max: z.string().default(''),
            precision: z.string().default(''),
          })
          .superRefine((value, ctx) => {
            if (value.type === 'boolean') {
              if (!['', 'true', 'false'].includes(value.default)) {
                ctx.addIssue({
                  code: z.ZodIssueCode.custom,
                  message: t('aiProviders.models.validation.configParameterNumberInvalid'),
                  path: ['default'],
                });
              }
              return;
            }

            if (!isNumericParameterType(value.type)) {
              return;
            }

            const numericFields: Array<
              keyof Pick<ConfigParameterFormValue, 'default' | 'min' | 'max'>
            > = ['default', 'min', 'max'];

            numericFields.forEach(fieldName => {
              const fieldValue = value[fieldName].trim();
              if (!fieldValue) {
                return;
              }

              if (!Number.isFinite(Number(fieldValue))) {
                ctx.addIssue({
                  code: z.ZodIssueCode.custom,
                  message: t('aiProviders.models.validation.configParameterNumberInvalid'),
                  path: [fieldName],
                });
              }
            });

            if (value.precision.trim()) {
              const precisionValue = Number(value.precision.trim());
              if (!Number.isInteger(precisionValue) || precisionValue < 0) {
                ctx.addIssue({
                  code: z.ZodIssueCode.custom,
                  message: t('aiProviders.models.validation.configParameterPrecisionInvalid'),
                  path: ['precision'],
                });
              }
            }
          })
      ),
    })
    .refine(
      data => {
        const selectedExclusive = data.use_cases.filter(uc => EXCLUSIVE_USE_CASES.includes(uc));
        return selectedExclusive.length <= 1;
      },
      {
        message: t('aiProviders.models.errors.exclusiveUseCase'),
        path: ['use_cases'],
      }
    )
    .superRefine((data, ctx) => {
      // Required price gate: a selected capability must carry a usable price
      // before the model can be saved. Mirrors backend MissingPricingForUseCases.
      const missing = missingModelPricing({
        useCases: data.use_cases ?? [],
        realtimeAudioBillingMode: data.realtime_audio_billing_mode,
        inputPrice: data.input_price,
        outputPrice: data.output_price,
        pricePerImage: data.price_per_image,
        imageBillingMode: data.image_billing_mode,
        imageTokenTextPrice: data.image_token_text_price,
        imageTokenImagePrice: data.image_token_image_price,
        imageTokenOutputPrice: data.image_token_output_price,
        videoRates: data.video_rates,
        speechToTextPrice: data.speech_to_text_price,
        textToSpeechPrice: data.text_to_speech_price,
        musicPricePerTrack: data.music_price_per_track,
        realtimeAudioInputPrice: data.realtime_audio_input_price,
        realtimeAudioOutputPrice: data.realtime_audio_output_price,
      });
      for (const item of missing) {
        const field = MISSING_PRICING_FIELD_LABELS[item.field] ?? String(item.field);
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: [field],
          message: t('aiProviders.models.validation.requiredPrice', { useCase: item.useCase, field }),
        });
      }
    });

interface CustomModelDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  providerId: string;
  initialData?: ModelItem & { description?: string };
  onSubmit: (data: CreateCustomModelRequest | UpdateCustomModelRequest) => Promise<void>;
  isSubmitting?: boolean;
}

export function CustomModelDialog({
  open,
  onOpenChange,
  providerId,
  initialData,
  onSubmit,
  isSubmitting,
}: CustomModelDialogProps) {
  const t = useT();
  const currentOrganization = useOrganizationStore.use.currentOrganization();
  const billingDisplay = useMemo(
    () => getBillingDisplaySettings(currentOrganization),
    [currentOrganization]
  );
  const currencySymbol = getBillingCurrencySymbol(billingDisplay);
  const perMillionUnit = t(
    billingDisplay.currency === 'CNY'
      ? 'aiProviders.models.priceDialog.cnyPerMillion'
      : 'aiProviders.models.priceDialog.usdPerMillion'
  );
  const pointsPerUsd = currentOrganization?.points_per_usd ?? 1000;
  const formSchema = createSchema((key, values) => t(key as never, values));
  type CustomModelFormInput = z.input<typeof formSchema>;
  type CustomModelFormValues = z.output<typeof formSchema>;
  const appliedConfigParametersRef = useRef<string | null>(null);

  const form = useForm<CustomModelFormInput, undefined, CustomModelFormValues>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      model: '',
      model_name: '',
      use_cases: ['text-chat'],
      family: '',
      family_name: '',
      status: 'active',
      tagline: '',
      description: '',
      tier: 'standard',
      context_window: 128000,
      max_output_tokens: 4096,
      input_price: '',
      output_price: '',
      cache_read_price: '',
      cache_write_price: '',
      cache_write_5m_price: '',
      cache_write_1h_price: '',
      price_per_image: '',
      image_billing_mode: 'per_image',
      image_token_text_price: '',
      image_token_image_price: '',
      image_token_output_price: '',
      video_rates: [{ resolution: '720p', price_per_second: '' }],
      speech_to_text_price: '',
      text_to_speech_price: '',
      music_price_per_track: '',
      realtime_audio_billing_mode: 'text',
      realtime_audio_input_price: '',
      realtime_audio_output_price: '',
      input_modalities: ['text'],
      output_modalities: ['text'],
      knowledge_cutoff: '',
      is_active: true,
      endpoints: {
        chat_completions: true,
      },
      features: {
        streaming: true,
        system_prompt: true,
      },
      tools: {},
      parameters: {
        temperature: true,
        top_p: true,
        stop: true,
        max_stop_sequences: 4,
      },
      config_parameters: [],
    },
  });
  const {
    fields: configParameterFields,
    append: appendConfigParameter,
    remove: removeConfigParameter,
    replace: replaceConfigParameters,
  } = useFieldArray({
    control: form.control,
    name: 'config_parameters',
    keyName: 'fieldId',
  });
  const {
    fields: videoRateFields,
    append: appendVideoRate,
    remove: removeVideoRate,
  } = useFieldArray({
    control: form.control,
    name: 'video_rates',
  });
  const configParameterValues = form.watch('config_parameters');
  const currentSchemaProvider = initialData?.provider || providerId;
  const currentSchemaModel = initialData?.model;
  const {
    data: existingConfigParameters,
    isLoading: isLoadingConfigParameters,
    error: configParametersError,
    isNotFound: isConfigParametersNotFound,
    hasLoaded: hasLoadedConfigParameters,
  } = useModelParameterRules({
    provider: currentSchemaProvider,
    model: currentSchemaModel,
    enabled: open && Boolean(initialData?.id && currentSchemaModel),
  });

  // Auto-adjust modalities based on use cases
  const useCasesValue = form.watch('use_cases');
  const realtimeAudioBillingMode = form.watch('realtime_audio_billing_mode') ?? 'text';
  const hasRealtimeAudio = Boolean(useCasesValue?.includes('realtime-audio'));
  const realtimeAudioUsesTextTokens = hasRealtimeAudio && realtimeAudioBillingMode !== 'audio';
  const hasTokenPricing =
    Boolean(
      useCasesValue?.some(useCase =>
        ['text-chat', 'vision', 'reasoning', 'function-calling', 'agent'].includes(useCase)
      )
    ) || realtimeAudioUsesTextTokens;
  const isInputOnlyPricing =
    !hasTokenPricing &&
    Boolean(useCasesValue?.some(useCase => ['embedding', 'rerank'].includes(useCase)));
  const showInputPrice = hasTokenPricing || isInputOnlyPricing;
  const showOutputPrice = hasTokenPricing;
  const showRealtimeAudioFields = hasRealtimeAudio && realtimeAudioBillingMode === 'audio';
  const imageBillingMode = form.watch('image_billing_mode') ?? 'per_image';
  // Cache billing is opt-in: only a model that already advertises cache meters
  // may expose cache prices, matching the model-price override dialog.
  const showCachePricing = Boolean(
    initialData && hasCacheBillingCapability(initialData.pricing) && hasTokenPricing
  );

  useEffect(() => {
    if (!useCasesValue) return;

    const inputModalities = new Set(form.getValues('input_modalities') || []);
    const outputModalities = new Set(form.getValues('output_modalities') || []);

    const originalInput = new Set(inputModalities);
    const originalOutput = new Set(outputModalities);

    if (useCasesValue.includes('text-chat')) {
      inputModalities.add('text');
      outputModalities.add('text');
    }
    if (useCasesValue.includes('vision')) {
      inputModalities.add('image');
    }
    if (useCasesValue.includes('image-gen')) {
      outputModalities.add('image');
    }
    if (useCasesValue.includes('embedding') || useCasesValue.includes('rerank')) {
      inputModalities.add('text');
      outputModalities.add('embedding');
    }
    if (useCasesValue.includes('speech-to-text')) {
      inputModalities.add('audio');
      outputModalities.add('text');
    }
    if (useCasesValue.includes('text-to-speech')) {
      inputModalities.add('text');
      outputModalities.add('audio');
    }
    if (useCasesValue.includes('realtime-audio')) {
      inputModalities.add('audio');
      outputModalities.add('audio');
    }
    if (useCasesValue.includes('video-gen')) {
      outputModalities.add('video');
    }

    const currentInputStr = Array.from(inputModalities).sort().join(',');
    const originalInputStr = Array.from(originalInput).sort().join(',');
    const currentOutputStr = Array.from(outputModalities).sort().join(',');
    const originalOutputStr = Array.from(originalOutput).sort().join(',');

    if (currentInputStr !== originalInputStr) {
      form.setValue('input_modalities', Array.from(inputModalities), { shouldDirty: true });
    }
    if (currentOutputStr !== originalOutputStr) {
      form.setValue('output_modalities', Array.from(outputModalities), { shouldDirty: true });
    }
  }, [useCasesValue, form]);

  useEffect(() => {
    if (initialData) {
      form.reset({
        model: initialData.model,
        model_name: initialData.model_name,
        use_cases: initialData.use_cases || [],
        family: initialData.family || '',
        family_name: initialData.family_name || '',
        status: initialData.status || 'active',
        tagline: initialData.tagline || '',
        description: initialData.description || initialData.tagline || '',
        tier: normalizeTier(initialData.tier),
        context_window: initialData.context_window,
        max_output_tokens: initialData.max_output_tokens,
        input_price: billingDisplayInputValueFromUSD(
          initialData.input_price,
          initialData.input_price_configured,
          billingDisplay
        ),
        output_price: billingDisplayInputValueFromUSD(
          initialData.output_price,
          initialData.output_price_configured,
          billingDisplay
        ),
        cache_read_price: billingDisplayInputValueFromUSD(
          initialData.cache_read_price,
          initialData.cache_read_price_configured,
          billingDisplay
        ),
        cache_write_price: billingDisplayInputValueFromUSD(
          initialData.cache_write_price,
          initialData.cache_write_price_configured,
          billingDisplay
        ),
        cache_write_5m_price: billingDisplayInputValueFromUSD(
          initialData.cache_write_5m_price,
          initialData.cache_write_5m_price_configured,
          billingDisplay
        ),
        cache_write_1h_price: billingDisplayInputValueFromUSD(
          initialData.cache_write_1h_price,
          initialData.cache_write_1h_price_configured,
          billingDisplay
        ),
        price_per_image: displayPriceInputValue(
          readCustomModelPricePerImage(initialData.pricing),
          billingDisplay
        ),
        image_billing_mode: getImageBillingMode(initialData.pricing),
        image_token_text_price: displayPriceInputValue(
          readImageTokenPrice(initialData.pricing, 'text_input_price_per_million'),
          billingDisplay
        ),
        image_token_image_price: displayPriceInputValue(
          readImageTokenPrice(initialData.pricing, 'image_input_price_per_million'),
          billingDisplay
        ),
        image_token_output_price: displayPriceInputValue(
          readImageTokenPrice(initialData.pricing, 'output_price_per_million'),
          billingDisplay
        ),
        video_rates: readCustomModelVideoRates(initialData.pricing).map(rate => ({
          resolution: rate.resolution,
          price_per_second: displayPriceInputValue(rate.pricePerSecond, billingDisplay),
        })),
        speech_to_text_price: displayPriceInputValue(
          readCustomModelMeteredPrice(initialData.pricing, 'transcription'),
          billingDisplay
        ),
        text_to_speech_price: displayPriceInputValue(
          readCustomModelMeteredPrice(initialData.pricing, 'speech_generation'),
          billingDisplay
        ),
        music_price_per_track: displayPriceInputValue(
          readCustomModelMeteredPrice(initialData.pricing, 'music_generation'),
          billingDisplay
        ),
        realtime_audio_billing_mode: getRealtimeAudioBillingMode(initialData.pricing),
        realtime_audio_input_price: displayPriceInputValue(
          readRealtimeAudioPrice(initialData.pricing, 'input_token'),
          billingDisplay
        ),
        realtime_audio_output_price: displayPriceInputValue(
          readRealtimeAudioPrice(initialData.pricing, 'output_token'),
          billingDisplay
        ),
        input_modalities: initialData.input_modalities || ['text'],
        output_modalities: initialData.output_modalities || ['text'],
        knowledge_cutoff: initialData.training_data?.cutoff_date || '',
        is_active: initialData.is_enabled,
        endpoints: {
          chat_completions: !!initialData.endpoints?.chat_completions,
          responses: !!initialData.endpoints?.responses,
          realtime: !!initialData.endpoints?.realtime,
          assistants: !!initialData.endpoints?.assistants,
          batch: !!initialData.endpoints?.batch,
          embeddings: !!initialData.endpoints?.embeddings,
          fine_tuning: !!initialData.endpoints?.fine_tuning,
          image_generation: !!initialData.endpoints?.image_generation,
          vision: !!initialData.endpoints?.vision,
          speech_generation: !!initialData.endpoints?.speech_generation,
          transcription: !!initialData.endpoints?.transcription,
          translation: !!initialData.endpoints?.translation,
          moderation: !!initialData.endpoints?.moderation,
          videos: !!initialData.endpoints?.videos,
          image_edit: !!initialData.endpoints?.image_edit,
        },
        features: {
          streaming: !!initialData.features?.streaming,
          function_calling: !!initialData.features?.function_calling,
          structured_output: !!initialData.features?.structured_output,
          json_mode: !!initialData.features?.json_mode,
          distillation: !!initialData.features?.distillation,
          reasoning: !!initialData.features?.reasoning,
          system_prompt: !!initialData.features?.system_prompt,
        },
        tools: {
          web_search: !!initialData.tools?.web_search,
          file_search: !!initialData.tools?.file_search,
          image_generation: !!initialData.tools?.image_generation,
          code_interpreter: !!initialData.tools?.code_interpreter,
          computer_use: !!initialData.tools?.computer_use,
          mcp: !!initialData.tools?.mcp,
          parallel_tool_calls: !!initialData.tools?.parallel_tool_calls,
        },
        parameters: {
          temperature: !!initialData.parameters?.supports_temperature,
          top_p: !!initialData.parameters?.supports_top_p,
          presence_penalty: !!initialData.parameters?.supports_presence_penalty,
          frequency_penalty: !!initialData.parameters?.supports_frequency_penalty,
          logit_bias: !!initialData.parameters?.supports_logit_bias,
          seed: !!initialData.parameters?.supports_seed,
          stop: !!initialData.parameters?.supports_stop,
          max_stop_sequences: initialData.parameters?.max_stop_sequences || 4,
        },
        config_parameters: [],
      });
    } else {
      form.reset({
        model: '',
        model_name: '',
        use_cases: ['text-chat'],
        family: '',
        family_name: '',
        status: 'active',
        tagline: '',
        description: '',
        tier: 'standard',
        context_window: 128000,
        max_output_tokens: 4096,
        input_price: '',
        output_price: '',
        cache_read_price: '',
        cache_write_price: '',
        cache_write_5m_price: '',
        cache_write_1h_price: '',
        input_modalities: ['text'],
        output_modalities: ['text'],
        knowledge_cutoff: '',
        is_active: true,
        endpoints: {
          chat_completions: true,
        },
        features: {
          streaming: true,
          system_prompt: true,
        },
        tools: {},
        parameters: {
          temperature: true,
          top_p: true,
          stop: true,
          max_stop_sequences: 4,
        },
        config_parameters: [],
      });
    }
    appliedConfigParametersRef.current = null;
  }, [billingDisplay, initialData, form, open]);

  useEffect(() => {
    if (!open || !initialData?.id || !currentSchemaModel) {
      return;
    }

    if (!hasLoadedConfigParameters && !isConfigParametersNotFound) {
      return;
    }

    const applyKey = `${initialData.id}:${currentSchemaProvider}:${currentSchemaModel}`;
    if (appliedConfigParametersRef.current === applyKey) {
      return;
    }

    replaceConfigParameters(existingConfigParameters.map(toConfigParameterFormValue));
    appliedConfigParametersRef.current = applyKey;
  }, [
    currentSchemaModel,
    currentSchemaProvider,
    existingConfigParameters,
    hasLoadedConfigParameters,
    initialData?.id,
    isConfigParametersNotFound,
    open,
    replaceConfigParameters,
  ]);

  const handleSubmit = async (values: CustomModelFormValues) => {
    // Map internal parameter flags to API structure
    const { parameters, config_parameters, ...rest } = values;
    const mappedParameters: ModelParameters = {
      supports_temperature: !!parameters?.temperature,
      supports_top_p: !!parameters?.top_p,
      supports_presence_penalty: !!parameters?.presence_penalty,
      supports_frequency_penalty: !!parameters?.frequency_penalty,
      supports_logit_bias: !!parameters?.logit_bias,
      supports_seed: !!parameters?.seed,
      supports_stop: !!parameters?.stop,
      max_stop_sequences: parameters?.max_stop_sequences || 4,
    };

    const pricing = buildCustomModelPricing({
      useCases: values.use_cases,
      currency: billingDisplay.currency,
      pricePerImage: billingDisplayInputToUSD(values.price_per_image ?? '', billingDisplay),
      imageBillingMode: values.image_billing_mode,
      imageTextInputPricePerMillion: billingDisplayInputToUSD(
        values.image_token_text_price ?? '',
        billingDisplay
      ),
      imageImageInputPricePerMillion: billingDisplayInputToUSD(
        values.image_token_image_price ?? '',
        billingDisplay
      ),
      imageOutputPricePerMillion: billingDisplayInputToUSD(
        values.image_token_output_price ?? '',
        billingDisplay
      ),
      videoRates: (values.video_rates ?? []).map(rate => ({
        resolution: rate.resolution,
        pricePerSecond: billingDisplayInputToUSD(rate.price_per_second ?? '', billingDisplay),
      })),
      speechToTextPricePerHour: billingDisplayInputToUSD(values.speech_to_text_price ?? '', billingDisplay),
      textToSpeechPricePer10kChars: billingDisplayInputToUSD(values.text_to_speech_price ?? '', billingDisplay),
      musicPricePerTrack: billingDisplayInputToUSD(values.music_price_per_track ?? '', billingDisplay),
      realtimeAudioBillingMode: values.realtime_audio_billing_mode,
      realtimeAudioInputPricePerMillion: billingDisplayInputToUSD(
        values.realtime_audio_input_price ?? '',
        billingDisplay
      ),
      realtimeAudioOutputPricePerMillion: billingDisplayInputToUSD(
        values.realtime_audio_output_price ?? '',
        billingDisplay
      ),
    });

    const data = {
      ...rest,
      input_price: showInputPrice
        ? billingDisplayInputToUSD(values.input_price ?? '', billingDisplay)
        : '',
      output_price: showOutputPrice
        ? billingDisplayInputToUSD(values.output_price ?? '', billingDisplay)
        : '',
      cache_read_price: showCachePricing
        ? billingDisplayInputToUSD(values.cache_read_price ?? '', billingDisplay)
        : '',
      cache_write_price: showCachePricing
        ? billingDisplayInputToUSD(values.cache_write_price ?? '', billingDisplay)
        : '',
      cache_write_5m_price: showCachePricing
        ? billingDisplayInputToUSD(values.cache_write_5m_price ?? '', billingDisplay)
        : '',
      cache_write_1h_price: showCachePricing
        ? billingDisplayInputToUSD(values.cache_write_1h_price ?? '', billingDisplay)
        : '',
      pricing,
      use_cases: values.use_cases as ModelUseCase[],
      parameters: mappedParameters,
      config_parameters: buildConfigParameters(config_parameters),
      provider: providerId,
    };
    await onSubmit(data);
    onOpenChange(false);
  };

  const handleAddConfigParameter = () => {
    appendConfigParameter(createEmptyConfigParameter());
  };

  const handleConfigParameterTypeChange = (index: number, nextType: ParameterValueType) => {
    form.setValue(`config_parameters.${index}.type`, nextType, { shouldDirty: true });

    if (!isNumericParameterType(nextType)) {
      form.setValue(`config_parameters.${index}.min`, '', { shouldDirty: true });
      form.setValue(`config_parameters.${index}.max`, '', { shouldDirty: true });
      form.setValue(`config_parameters.${index}.precision`, '', { shouldDirty: true });
    }

    if (nextType === 'boolean') {
      const currentDefault = form.getValues(`config_parameters.${index}.default`) ?? '';
      if (!['', 'true', 'false'].includes(currentDefault)) {
        form.setValue(`config_parameters.${index}.default`, '', { shouldDirty: true });
      }
      return;
    }

    const currentDefault = form.getValues(`config_parameters.${index}.default`) ?? '';
    if (currentDefault === 'true' || currentDefault === 'false') {
      form.setValue(`config_parameters.${index}.default`, '', { shouldDirty: true });
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[700px] p-0 gap-0 overflow-hidden">
        <Form {...form}>
          <form onSubmit={form.handleSubmit(handleSubmit)} className="flex flex-col h-[85vh]">
            <DialogHeader className="p-6 pb-2">
              <div className="flex items-center gap-3">
                <div className="h-10 w-10 rounded-xl bg-primary/10 flex items-center justify-center text-primary">
                  <Box className="h-6 w-6" />
                </div>
                <div>
                  <DialogTitle className="text-xl font-bold">
                    {initialData
                      ? t('aiProviders.models.actions.edit')
                      : t('aiProviders.models.actions.add')}
                  </DialogTitle>
                  <DialogDescription>
                    {t('aiProviders.management.description', { provider: providerId })}
                  </DialogDescription>
                </div>
              </div>
            </DialogHeader>

            <Tabs defaultValue="general" className="flex-1 flex flex-col overflow-hidden">
              <div className="px-6 py-4 border-b bg-muted/20">
                <TabsList className="w-full justify-start gap-2">
                  <TabsTrigger value="general" className="font-medium">
                    {t('aiProviders.overview')}
                  </TabsTrigger>
                  <TabsTrigger value="details" className="font-medium">
                    {t('aiProviders.details')}
                  </TabsTrigger>
                  <TabsTrigger value="capabilities" className="font-medium">
                    {t('aiProviders.pricing')}
                  </TabsTrigger>
                  <TabsTrigger value="features" className="font-medium">
                    {t('aiProviders.models.table.features')}
                  </TabsTrigger>
                  <TabsTrigger value="settings" className="font-medium">
                    {t('aiProviders.configuration')}
                  </TabsTrigger>
                </TabsList>
              </div>

              <DialogBody className="space-y-6 py-4">
                {/* General Tab - Required Fields Only */}
                <TabsContent value="general" className="mt-0 space-y-5">
                  <Alert className="bg-primary/5 border-primary/20">
                    <Info className="h-4 w-4 text-primary" />
                    <AlertDescription className="text-xs text-primary/80 font-medium">
                      {t('aiProviders.custom.dialog.bannerTip')}
                    </AlertDescription>
                  </Alert>

                  <div className="grid grid-cols-2 gap-4">
                    <FormField
                      control={form.control}
                      name="model"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('aiProviders.custom.dialog.fields.model')}</FormLabel>
                          <FormControl>
                            <Input
                              placeholder={t('aiProviders.customModel.fields.modelPlaceholder')}
                              className="bg-muted/30"
                              {...field}
                              disabled={!!initialData}
                            />
                          </FormControl>
                          <FormDescription>
                            {t('aiProviders.customModel.fields.modelHint')}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormField
                      control={form.control}
                      name="model_name"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>
                            {t('aiProviders.custom.dialog.fields.providerName')}
                          </FormLabel>
                          <FormControl>
                            <Input placeholder="e.g. GPT-4o" {...field} />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>

                  <FormField
                    control={form.control}
                    name="use_cases"
                    render={({ field }) => (
                      <FormItem className="space-y-3">
                        <FormLabel>{t('aiProviders.supportedTypes')}</FormLabel>
                        <div className="space-y-4">
                          <div className="flex flex-wrap gap-2 min-h-[32px] p-2 border rounded-lg bg-accent/20">
                            {field.value.length === 0 && (
                              <span className="text-xs text-muted-foreground italic">
                                {t('aiProviders.models.empty.noUseCases')}
                              </span>
                            )}
                            {(field.value as ModelUseCase[]).map(uc => (
                              <Badge
                                key={uc}
                                variant="outline"
                                className={cn(
                                  'pl-2 pr-1 h-7 text-xs flex items-center gap-1',
                                  USE_CASE_BADGE_COLORS[uc]
                                )}
                              >
                                {t(`aiProviders.models.usecases.${uc}`)}
                                <button
                                  type="button"
                                  onClick={() => {
                                    const nextValue = field.value.filter((v: string) => v !== uc);
                                    field.onChange(nextValue);
                                  }}
                                  className="hover:bg-black/10 dark:hover:bg-white/10 rounded-full p-0.5 transition-colors"
                                >
                                  <X className="w-3 h-3" />
                                </button>
                              </Badge>
                            ))}
                          </div>

                          <div className="flex flex-wrap gap-2">
                            {useCases
                              .filter(uc => !field.value.includes(uc))
                              .map(uc => (
                                <Badge
                                  key={uc}
                                  variant="outline"
                                  interactive
                                  className={cn(
                                    'h-7 px-3 text-xs cursor-pointer opacity-60 hover:opacity-100 transition-opacity',
                                    USE_CASE_BADGE_COLORS[uc]
                                  )}
                                  onClick={() => {
                                    let nextValue = [...field.value];
                                    if (EXCLUSIVE_USE_CASES.includes(uc)) {
                                      nextValue = nextValue.filter(
                                        v => !EXCLUSIVE_USE_CASES.includes(v)
                                      );
                                    }
                                    field.onChange([...nextValue, uc]);
                                  }}
                                >
                                  {t(`aiProviders.models.usecases.${uc}`)}
                                </Badge>
                              ))}
                          </div>
                        </div>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <div className="grid grid-cols-2 gap-8 pt-4 border-t border-dashed">
                    <FormField
                      control={form.control}
                      name="input_modalities"
                      render={({ field }) => (
                        <FormItem className="space-y-3">
                          <FormLabel className="text-xs uppercase text-muted-foreground font-bold tracking-wider">
                            {t('aiProviders.models.fields.inputModalities')}
                          </FormLabel>
                          <div className="flex flex-wrap gap-2">
                            {MODALITY_OPTIONS.map(m => (
                              <Badge
                                key={m}
                                variant={field.value.includes(m) ? 'default' : 'outline'}
                                className="cursor-pointer transition-all h-7 px-3 capitalize"
                                onClick={() => {
                                  const next = field.value.includes(m)
                                    ? field.value.filter((v: string) => v !== m)
                                    : [...field.value, m];
                                  field.onChange(next);
                                }}
                              >
                                {t(`aiProviders.models.modalities.${m}` as const)}
                              </Badge>
                            ))}
                          </div>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormField
                      control={form.control}
                      name="output_modalities"
                      render={({ field }) => (
                        <FormItem className="space-y-3">
                          <FormLabel className="text-xs uppercase text-muted-foreground font-bold tracking-wider">
                            {t('aiProviders.models.fields.outputModalities')}
                          </FormLabel>
                          <div className="flex flex-wrap gap-2">
                            {MODALITY_OPTIONS.map(m => (
                              <Badge
                                key={m}
                                variant={field.value.includes(m) ? 'default' : 'outline'}
                                className="cursor-pointer transition-all h-7 px-3 capitalize"
                                onClick={() => {
                                  const next = field.value.includes(m)
                                    ? field.value.filter((v: string) => v !== m)
                                    : [...field.value, m];
                                  field.onChange(next);
                                }}
                              >
                                {t(`aiProviders.models.modalities.${m}` as const)}
                              </Badge>
                            ))}
                          </div>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>
                </TabsContent>

                {/* Details Tab - Optional Info */}
                <TabsContent value="details" className="mt-0 space-y-5">
                  <div className="grid grid-cols-2 gap-4">
                    <FormField
                      control={form.control}
                      name="family"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('aiProviders.models.fields.family')}</FormLabel>
                          <FormControl>
                            <Input placeholder="e.g. gpt-4" {...field} />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                    <FormField
                      control={form.control}
                      name="family_name"
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('aiProviders.models.fields.familyName')}</FormLabel>
                          <FormControl>
                            <Input placeholder="e.g. GPT-4" {...field} />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>

                  <FormField
                    control={form.control}
                    name="tagline"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('aiProviders.models.fields.tagline')}</FormLabel>
                        <FormControl>
                          <Input
                            placeholder={t('aiProviders.models.placeholders.tagline')}
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name="description"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('aiProviders.custom.dialog.fields.description')}</FormLabel>
                        <FormControl>
                          <Textarea
                            placeholder={t('aiProviders.models.placeholders.description')}
                            className="resize-none"
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </TabsContent>

                {/* Capabilities Tab */}
                <TabsContent value="capabilities" className="mt-0 space-y-6">
                  <div className="space-y-4">
                    <h3 className="text-sm font-semibold flex items-center gap-2">
                      <CreditCard className="h-4 w-4 text-primary" />
                      {t('aiProviders.models.sections.pricing')}
                    </h3>
                    <p className="text-xs text-muted-foreground">
                      {t('aiProviders.models.fields.pointsPerUsdHint', { points: pointsPerUsd })}
                    </p>
                    <div className="grid grid-cols-2 gap-4">
                      {showInputPrice && (
                      <FormField
                        control={form.control}
                        name="input_price"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('aiProviders.models.fields.inputPrice')}</FormLabel>
                            <FormControl>
                              <div className="relative">
                                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                  {currencySymbol}
                                </span>
                                <Input
                                  className="pl-6 pr-24"
                                  type="number"
                                  min="0"
                                  step="any"
                                  {...field}
                                />
                                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">
                                  {perMillionUnit}
                                </span>
                              </div>
                            </FormControl>
                            <FormDescription>
                              {t('aiProviders.models.fields.priceConfiguredHint')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      )}
                      {showOutputPrice && (
                      <FormField
                        control={form.control}
                        name="output_price"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('aiProviders.models.fields.outputPrice')}</FormLabel>
                            <FormControl>
                              <div className="relative">
                                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                  {currencySymbol}
                                </span>
                                <Input
                                  className="pl-6 pr-24"
                                  type="number"
                                  min="0"
                                  step="any"
                                  {...field}
                                />
                                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">
                                  {perMillionUnit}
                                </span>
                              </div>
                            </FormControl>
                            <FormDescription>
                              {t('aiProviders.models.fields.priceConfiguredHint')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      )}
                      {showCachePricing && (
                        <>
                      <FormField
                        control={form.control}
                        name="cache_read_price"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('aiProviders.models.fields.cacheReadPrice')}</FormLabel>
                            <FormControl>
                              <div className="relative">
                                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                  {currencySymbol}
                                </span>
                                <Input
                                  className="pl-6 pr-24"
                                  type="number"
                                  min="0"
                                  step="any"
                                  {...field}
                                />
                                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">
                                  {perMillionUnit}
                                </span>
                              </div>
                            </FormControl>
                            <FormDescription>
                              {t('aiProviders.models.fields.priceConfiguredHint')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name="cache_write_price"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('aiProviders.models.fields.cacheWritePrice')}</FormLabel>
                            <FormControl>
                              <div className="relative">
                                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                  {currencySymbol}
                                </span>
                                <Input
                                  className="pl-6 pr-24"
                                  type="number"
                                  min="0"
                                  step="any"
                                  {...field}
                                />
                                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">
                                  {perMillionUnit}
                                </span>
                              </div>
                            </FormControl>
                            <FormDescription>
                              {t('aiProviders.models.fields.priceConfiguredHint')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name="cache_write_5m_price"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              {t('aiProviders.models.fields.cacheWrite5mPrice')}
                            </FormLabel>
                            <FormControl>
                              <div className="relative">
                                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                  {currencySymbol}
                                </span>
                                <Input
                                  className="pl-6 pr-24"
                                  type="number"
                                  min="0"
                                  step="any"
                                  {...field}
                                />
                                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">
                                  {perMillionUnit}
                                </span>
                              </div>
                            </FormControl>
                            <FormDescription>
                              {t('aiProviders.models.fields.priceConfiguredHint')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name="cache_write_1h_price"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              {t('aiProviders.models.fields.cacheWrite1hPrice')}
                            </FormLabel>
                            <FormControl>
                              <div className="relative">
                                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                  {currencySymbol}
                                </span>
                                <Input
                                  className="pl-6 pr-24"
                                  type="number"
                                  min="0"
                                  step="any"
                                  {...field}
                                />
                                <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">
                                  {perMillionUnit}
                                </span>
                              </div>
                            </FormControl>
                            <FormDescription>
                              {t('aiProviders.models.fields.priceConfiguredHint')}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                        </>
                      )}
                    </div>

                    {hasRealtimeAudio && (
                      <div className="space-y-3 rounded-lg border border-primary/20 bg-primary/5 p-4">
                        <FormField
                          control={form.control}
                          name="realtime_audio_billing_mode"
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>
                                {t('aiProviders.models.fields.realtimeAudioBillingMode')}
                              </FormLabel>
                              <div className="flex gap-2">
                                <Button
                                  type="button"
                                  size="sm"
                                  variant={field.value === 'text' ? 'default' : 'outline'}
                                  onClick={() => field.onChange('text')}
                                >
                                  {t('aiProviders.models.fields.realtimeAudioBillingText')}
                                </Button>
                                <Button
                                  type="button"
                                  size="sm"
                                  variant={field.value === 'audio' ? 'default' : 'outline'}
                                  onClick={() => field.onChange('audio')}
                                >
                                  {t('aiProviders.models.fields.realtimeAudioBillingAudio')}
                                </Button>
                              </div>
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                        {showRealtimeAudioFields && (
                          <div className="grid gap-4 sm:grid-cols-2">
                            <FormField
                              control={form.control}
                              name="realtime_audio_input_price"
                              render={({ field }) => (
                                <FormItem>
                                  <FormLabel>
                                    {t('aiProviders.models.fields.realtimeAudioInputPrice')}
                                  </FormLabel>
                                  <FormControl>
                                    <div className="relative">
                                      <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                        {currencySymbol}
                                      </span>
                                      <Input className="pl-6 pr-24" type="number" min="0" step="any" {...field} />
                                      <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">
                                        {perMillionUnit}
                                      </span>
                                    </div>
                                  </FormControl>
                                  <FormMessage />
                                </FormItem>
                              )}
                            />
                            <FormField
                              control={form.control}
                              name="realtime_audio_output_price"
                              render={({ field }) => (
                                <FormItem>
                                  <FormLabel>
                                    {t('aiProviders.models.fields.realtimeAudioOutputPrice')}
                                  </FormLabel>
                                  <FormControl>
                                    <div className="relative">
                                      <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                        {currencySymbol}
                                      </span>
                                      <Input className="pl-6 pr-24" type="number" min="0" step="any" {...field} />
                                      <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">
                                        {perMillionUnit}
                                      </span>
                                    </div>
                                  </FormControl>
                                  <FormMessage />
                                </FormItem>
                              )}
                            />
                          </div>
                        )}
                      </div>
                    )}

                    {useCasesValue?.includes('image-gen') && (
                      <div className="space-y-3 rounded-lg border border-primary/20 bg-primary/5 p-4">
                        <FormField
                          control={form.control}
                          name="image_billing_mode"
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>{t('aiProviders.models.fields.imageBillingMode')}</FormLabel>
                              <div className="flex gap-2">
                                <Button
                                  type="button"
                                  size="sm"
                                  variant={field.value === 'per_image' ? 'default' : 'outline'}
                                  onClick={() => field.onChange('per_image')}
                                >
                                  {t('aiProviders.models.fields.imageBillingPerImage')}
                                </Button>
                                <Button
                                  type="button"
                                  size="sm"
                                  variant={field.value === 'token' ? 'default' : 'outline'}
                                  onClick={() => field.onChange('token')}
                                >
                                  {t('aiProviders.models.fields.imageBillingToken')}
                                </Button>
                              </div>
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                        {imageBillingMode === 'token' ? (
                          <div className="space-y-3">
                            {(
                              [
                                { name: 'image_token_text_price', label: t('aiProviders.models.fields.imageTokenTextPrice') },
                                { name: 'image_token_image_price', label: t('aiProviders.models.fields.imageTokenImagePrice') },
                                { name: 'image_token_output_price', label: t('aiProviders.models.fields.imageTokenOutputPrice') },
                              ] as const
                            ).map(item => (
                              <FormField
                                key={item.name}
                                control={form.control}
                                name={item.name}
                                render={({ field }) => (
                                  <FormItem>
                                    <FormLabel>{item.label}</FormLabel>
                                    <FormControl>
                                      <div className="relative">
                                        <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                          {currencySymbol}
                                        </span>
                                        <Input
                                          className="pl-6 pr-24"
                                          type="number"
                                          min="0"
                                          step="any"
                                          {...field}
                                        />
                                        <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">
                                          {perMillionUnit}
                                        </span>
                                      </div>
                                    </FormControl>
                                    <FormMessage />
                                  </FormItem>
                                )}
                              />
                            ))}
                          </div>
                        ) : (
                          <FormField
                            control={form.control}
                            name="price_per_image"
                            render={({ field }) => (
                              <FormItem>
                                <FormLabel>{t('aiProviders.models.fields.pricePerImage')}</FormLabel>
                                <FormControl>
                                  <div className="relative">
                                    <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                      {currencySymbol}
                                    </span>
                                    <Input className="pl-6" type="number" min="0" step="any" {...field} />
                                  </div>
                                </FormControl>
                                <FormMessage />
                              </FormItem>
                            )}
                          />
                        )}
                      </div>
                    )}

                    {useCasesValue?.includes('video-gen') && (
                      <div className="space-y-3 rounded-lg border border-primary/20 bg-primary/5 p-4">
                        <div className="flex items-center justify-between gap-3">
                          <FormLabel>{t('aiProviders.models.fields.videoPricePerSecond')}</FormLabel>
                          <Button
                            type="button"
                            size="sm"
                            variant="outline"
                            onClick={() => appendVideoRate({ resolution: '', price_per_second: '' })}
                          >
                            {t('aiProviders.models.actions.addResolution')}
                          </Button>
                        </div>
                        {videoRateFields.map((rateField, index) => (
                          <div
                            key={rateField.id}
                            className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-end"
                          >
                            <FormField
                              control={form.control}
                              name={`video_rates.${index}.resolution` as const}
                              render={({ field }) => (
                                <FormItem>
                                  <FormLabel>{t('aiProviders.models.fields.resolution')}</FormLabel>
                                  <FormControl>
                                    <Input placeholder="720p" {...field} />
                                  </FormControl>
                                  <FormMessage />
                                </FormItem>
                              )}
                            />
                            <FormField
                              control={form.control}
                              name={`video_rates.${index}.price_per_second` as const}
                              render={({ field }) => (
                                <FormItem>
                                  <FormLabel>{t('aiProviders.models.fields.perSecondPrice')}</FormLabel>
                                  <FormControl>
                                    <div className="relative">
                                      <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                        {currencySymbol}
                                      </span>
                                      <Input className="pl-6" type="number" min="0" step="any" {...field} />
                                    </div>
                                  </FormControl>
                                  <FormMessage />
                                </FormItem>
                              )}
                            />
                            <Button
                              type="button"
                              variant="ghost"
                              size="sm"
                              disabled={videoRateFields.length <= 1}
                              onClick={() => removeVideoRate(index)}
                            >
                              {t('aiProviders.models.actions.remove')}
                            </Button>
                          </div>
                        ))}
                      </div>
                    )}

                    {useCasesValue?.includes('speech-to-text') && (
                      <FormField
                        control={form.control}
                        name="speech_to_text_price"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              {t('aiProviders.models.fields.speechToTextPricePerHour')}
                            </FormLabel>
                            <FormControl>
                              <div className="relative">
                                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                  {currencySymbol}
                                </span>
                                <Input className="pl-6" type="number" min="0" step="any" {...field} />
                              </div>
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    )}

                    {useCasesValue?.includes('text-to-speech') && (
                      <FormField
                        control={form.control}
                        name="text_to_speech_price"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              {t('aiProviders.models.fields.textToSpeechPricePer10kChars')}
                            </FormLabel>
                            <FormControl>
                              <div className="relative">
                                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                  {currencySymbol}
                                </span>
                                <Input className="pl-6" type="number" min="0" step="any" {...field} />
                              </div>
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    )}

                    {useCasesValue?.includes('music-gen') && (
                      <FormField
                        control={form.control}
                        name="music_price_per_track"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>
                              {t('aiProviders.models.fields.musicPricePerTrack')}
                            </FormLabel>
                            <FormControl>
                              <div className="relative">
                                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                  {currencySymbol}
                                </span>
                                <Input className="pl-6" type="number" min="0" step="any" {...field} />
                              </div>
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    )}
                  </div>

                  <div className="space-y-4 border-t pt-4">
                    <h3 className="text-sm font-semibold flex items-center gap-2">
                      <Layers className="h-4 w-4 text-primary" />
                      {t('aiProviders.models.sections.window')}
                    </h3>
                    <div className="grid grid-cols-2 gap-4">
                      <FormField
                        control={form.control}
                        name="context_window"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('aiProviders.models.fields.contextWindow')}</FormLabel>
                            <FormControl>
                              <Input type="number" {...field} />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name="max_output_tokens"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('aiProviders.models.fields.maxTokens')}</FormLabel>
                            <FormControl>
                              <Input type="number" {...field} />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </div>
                  </div>
                </TabsContent>

                {/* Features Tab */}
                <TabsContent value="features" className="mt-0 space-y-6">
                  <div className="space-y-4">
                    <h3 className="text-sm font-semibold flex items-center gap-2">
                      <Activity className="h-4 w-4 text-primary" />
                      {t('aiProviders.models.sections.endpoints')}
                    </h3>
                    <div className="grid grid-cols-3 gap-y-4 gap-x-2">
                      {ENDPOINT_KEYS.map(key => (
                        <FormField
                          key={key}
                          control={form.control}
                          name={`endpoints.${key}` as const}
                          render={({ field }) => (
                            <FormItem className="flex items-center space-x-2 space-y-0">
                              <FormControl>
                                <Checkbox checked={field.value} onCheckedChange={field.onChange} />
                              </FormControl>
                              <FormLabel className="text-xs font-medium cursor-pointer capitalize">
                                {t(`aiProviders.models.endpoints.${key}` as const)}
                              </FormLabel>
                            </FormItem>
                          )}
                        />
                      ))}
                    </div>
                  </div>

                  <div className="space-y-4 border-t pt-4">
                    <h3 className="text-sm font-semibold flex items-center gap-2">
                      <Zap className="h-4 w-4 text-amber-500" />
                      {t('aiProviders.models.sections.features')}
                    </h3>
                    <div className="grid grid-cols-2 gap-4">
                      {FEATURE_KEYS.map(key => (
                        <FormField
                          key={key}
                          control={form.control}
                          name={`features.${key}` as const}
                          render={({ field }) => (
                            <FormItem className="flex flex-row items-center justify-between p-2 rounded-lg border bg-muted/20 space-y-0">
                              <FormLabel className="text-xs font-medium capitalize">
                                {t(`aiProviders.models.features.${key}` as const)}
                              </FormLabel>
                              <FormControl>
                                <Switch checked={field.value} onCheckedChange={field.onChange} />
                              </FormControl>
                            </FormItem>
                          )}
                        />
                      ))}
                    </div>
                  </div>

                  <div className="space-y-4 border-t pt-4">
                    <h3 className="text-sm font-semibold flex items-center gap-2">
                      <Terminal className="h-4 w-4 text-blue-500" />
                      {t('aiProviders.models.sections.tools')}
                    </h3>
                    <div className="grid grid-cols-3 gap-3">
                      {TOOL_KEYS.map(key => (
                        <FormField
                          key={key}
                          control={form.control}
                          name={`tools.${key}` as const}
                          render={({ field }) => (
                            <ToolItem
                              icon={<Terminal className="h-3.5 w-3.5" />}
                              label={t(`aiProviders.models.tools.${key}` as const)}
                              checked={Boolean(field.value)}
                              onChange={field.onChange}
                            />
                          )}
                        />
                      ))}
                    </div>
                  </div>
                </TabsContent>

                {/* Settings Tab */}
                <TabsContent value="settings" className="mt-0 space-y-6">
                  <div className="space-y-4">
                    <h3 className="text-sm font-semibold flex items-center gap-2">
                      <Settings className="h-4 w-4 text-primary" />
                      {t('aiProviders.configuration')}
                    </h3>
                    <div className="grid grid-cols-2 gap-4">
                      <FormField
                        control={form.control}
                        name="is_active"
                        render={({ field }) => (
                          <FormItem className="flex flex-row items-center justify-between rounded-lg border p-3 shadow-sm space-y-0">
                            <div className="space-y-0.5">
                              <FormLabel>{t('aiProviders.enabled')}</FormLabel>
                            </div>
                            <FormControl>
                              <Switch checked={field.value} onCheckedChange={field.onChange} />
                            </FormControl>
                          </FormItem>
                        )}
                      />
                      <FormField
                        control={form.control}
                        name="status"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('aiProviders.models.table.status')}</FormLabel>
                            <FormControl>
                              <Input
                                placeholder={t('aiProviders.models.placeholders.status')}
                                {...field}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </div>
                  </div>

                  <div className="space-y-4 border-t pt-4">
                    <h3 className="text-sm font-semibold flex items-center gap-2">
                      <List className="h-4 w-4 text-primary" />
                      {t('aiProviders.models.sections.parameters')}
                    </h3>
                    <div className="grid grid-cols-2 gap-4">
                      <div className="space-y-4">
                        {LEGACY_PARAMETER_KEYS.map(key => (
                          <FormField
                            key={key}
                            control={form.control}
                            name={`parameters.${key}` as const}
                            render={({ field }) => (
                              <FormItem className="flex items-center justify-between space-y-0">
                                <FormLabel className="text-xs font-medium capitalize">
                                  {t('aiProviders.models.fields.supports', { parameter: key })}
                                </FormLabel>
                                <FormControl>
                                  <Switch checked={field.value} onCheckedChange={field.onChange} />
                                </FormControl>
                              </FormItem>
                            )}
                          />
                        ))}
                      </div>
                      <FormField
                        control={form.control}
                        name="parameters.max_stop_sequences"
                        render={({ field }) => (
                          <FormItem className="space-y-2">
                            <FormLabel>{t('aiProviders.models.fields.maxStopSequences')}</FormLabel>
                            <FormControl>
                              <Input type="number" {...field} />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </div>
                  </div>

                  <div className="space-y-4 border-t pt-4">
                    <div className="flex items-start justify-between gap-4">
                      <div className="space-y-1">
                        <h3 className="text-sm font-semibold flex items-center gap-2">
                          <Settings className="h-4 w-4 text-primary" />
                          {t('aiProviders.customModel.configParameters.title')}
                        </h3>
                        <p className="text-sm text-muted-foreground">
                          {t('aiProviders.customModel.configParameters.description')}
                        </p>
                      </div>
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={handleAddConfigParameter}
                      >
                        <Plus className="h-4 w-4" />
                        {t('aiProviders.customModel.configParameters.add')}
                      </Button>
                    </div>

                    {open && initialData ? (
                      isLoadingConfigParameters ? (
                        <Alert>
                          <AlertDescription>
                            {t('aiProviders.customModel.configParameters.loading')}
                          </AlertDescription>
                        </Alert>
                      ) : configParametersError ? (
                        <Alert variant="destructive">
                          <AlertDescription>
                            {t('models.configParameters.states.loadFailed.description')}
                          </AlertDescription>
                        </Alert>
                      ) : isConfigParametersNotFound ? (
                        <Alert>
                          <AlertDescription>
                            {t('models.configParameters.states.notFound.description')}
                          </AlertDescription>
                        </Alert>
                      ) : null
                    ) : null}

                    {configParameterFields.length === 0 ? (
                      <div className="rounded-lg border border-dashed p-4 text-sm text-muted-foreground">
                        {t('aiProviders.customModel.configParameters.empty')}
                      </div>
                    ) : null}

                    <div className="space-y-4">
                      {configParameterFields.map((field, index) => {
                        const rowValue =
                          configParameterValues?.[index] ?? createEmptyConfigParameter();
                        const isNumericType = isNumericParameterType(rowValue.type);
                        const { help, label } = resolveModelConfigParameterCopy({
                          parameter: {
                            name: rowValue.name,
                            template_key: rowValue.template_key,
                          },
                          translate: key => t(key as never),
                        });

                        return (
                          <div
                            key={field.fieldId}
                            className="rounded-xl border p-4 space-y-4 bg-muted/10"
                          >
                            <div className="flex items-center justify-between gap-3">
                              <div className="text-sm font-medium">
                                {rowValue.name ||
                                  t('aiProviders.customModel.configParameters.title')}
                              </div>
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                onClick={() => removeConfigParameter(index)}
                              >
                                <Trash2 className="h-4 w-4" />
                                {t('aiProviders.customModel.configParameters.remove')}
                              </Button>
                            </div>

                            <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
                              <FormField
                                control={form.control}
                                name={`config_parameters.${index}.name` as const}
                                render={({ field }) => (
                                  <FormItem>
                                    <FormLabel>
                                      {t('aiProviders.customModel.configParameters.fields.name')}
                                    </FormLabel>
                                    <FormControl>
                                      <Input
                                        {...field}
                                        placeholder={t(
                                          'aiProviders.customModel.configParameters.placeholders.name'
                                        )}
                                      />
                                    </FormControl>
                                    <FormMessage />
                                  </FormItem>
                                )}
                              />
                              <FormField
                                control={form.control}
                                name={`config_parameters.${index}.template_key` as const}
                                render={({ field }) => (
                                  <FormItem>
                                    <FormLabel>
                                      {t(
                                        'aiProviders.customModel.configParameters.fields.templateKey'
                                      )}
                                    </FormLabel>
                                    <FormControl>
                                      <Input
                                        {...field}
                                        placeholder={t(
                                          'aiProviders.customModel.configParameters.placeholders.templateKey'
                                        )}
                                      />
                                    </FormControl>
                                    <FormMessage />
                                  </FormItem>
                                )}
                              />
                              <FormField
                                control={form.control}
                                name={`config_parameters.${index}.type` as const}
                                render={({ field }) => (
                                  <FormItem>
                                    <FormLabel>
                                      {t('aiProviders.customModel.configParameters.fields.type')}
                                    </FormLabel>
                                    <FormControl>
                                      <Select
                                        value={field.value}
                                        onValueChange={value =>
                                          handleConfigParameterTypeChange(
                                            index,
                                            value as ParameterValueType
                                          )
                                        }
                                      >
                                        <SelectTrigger>
                                          <SelectValue />
                                        </SelectTrigger>
                                        <SelectContent>
                                          {CONFIG_PARAMETER_TYPES.map(type => (
                                            <SelectItem key={type} value={type}>
                                              {t(
                                                `aiProviders.customModel.configParameters.types.${type}` as never
                                              )}
                                            </SelectItem>
                                          ))}
                                        </SelectContent>
                                      </Select>
                                    </FormControl>
                                    <FormMessage />
                                  </FormItem>
                                )}
                              />
                              <FormField
                                control={form.control}
                                name={`config_parameters.${index}.required` as const}
                                render={({ field }) => (
                                  <FormItem className="rounded-lg border p-3 space-y-2">
                                    <FormLabel>
                                      {t(
                                        'aiProviders.customModel.configParameters.fields.required'
                                      )}
                                    </FormLabel>
                                    <FormControl>
                                      <Switch
                                        checked={field.value}
                                        onCheckedChange={field.onChange}
                                      />
                                    </FormControl>
                                  </FormItem>
                                )}
                              />
                            </div>

                            <div
                              className={cn(
                                'grid gap-4',
                                isNumericType ? 'lg:grid-cols-4' : 'lg:grid-cols-2'
                              )}
                            >
                              <FormField
                                control={form.control}
                                name={`config_parameters.${index}.default` as const}
                                render={({ field }) => (
                                  <FormItem>
                                    <FormLabel>
                                      {t(
                                        'aiProviders.customModel.configParameters.fields.defaultValue'
                                      )}
                                    </FormLabel>
                                    <FormControl>
                                      {rowValue.type === 'boolean' ? (
                                        <Select
                                          value={field.value || '__unset__'}
                                          onValueChange={value =>
                                            field.onChange(value === '__unset__' ? '' : value)
                                          }
                                        >
                                          <SelectTrigger>
                                            <SelectValue />
                                          </SelectTrigger>
                                          <SelectContent>
                                            <SelectItem value="__unset__">
                                              {t(
                                                'aiProviders.customModel.configParameters.booleanValues.unset'
                                              )}
                                            </SelectItem>
                                            <SelectItem value="true">
                                              {t(
                                                'aiProviders.customModel.configParameters.booleanValues.true'
                                              )}
                                            </SelectItem>
                                            <SelectItem value="false">
                                              {t(
                                                'aiProviders.customModel.configParameters.booleanValues.false'
                                              )}
                                            </SelectItem>
                                          </SelectContent>
                                        </Select>
                                      ) : (
                                        <Input
                                          {...field}
                                          type={isNumericType ? 'number' : 'text'}
                                          placeholder={t(
                                            'aiProviders.customModel.configParameters.placeholders.defaultValue'
                                          )}
                                        />
                                      )}
                                    </FormControl>
                                    <FormMessage />
                                  </FormItem>
                                )}
                              />

                              {isNumericType ? (
                                <>
                                  <FormField
                                    control={form.control}
                                    name={`config_parameters.${index}.min` as const}
                                    render={({ field }) => (
                                      <FormItem>
                                        <FormLabel>
                                          {t('aiProviders.customModel.configParameters.fields.min')}
                                        </FormLabel>
                                        <FormControl>
                                          <Input
                                            {...field}
                                            type="number"
                                            placeholder={t(
                                              'aiProviders.customModel.configParameters.placeholders.min'
                                            )}
                                          />
                                        </FormControl>
                                        <FormMessage />
                                      </FormItem>
                                    )}
                                  />
                                  <FormField
                                    control={form.control}
                                    name={`config_parameters.${index}.max` as const}
                                    render={({ field }) => (
                                      <FormItem>
                                        <FormLabel>
                                          {t('aiProviders.customModel.configParameters.fields.max')}
                                        </FormLabel>
                                        <FormControl>
                                          <Input
                                            {...field}
                                            type="number"
                                            placeholder={t(
                                              'aiProviders.customModel.configParameters.placeholders.max'
                                            )}
                                          />
                                        </FormControl>
                                        <FormMessage />
                                      </FormItem>
                                    )}
                                  />
                                  <FormField
                                    control={form.control}
                                    name={`config_parameters.${index}.precision` as const}
                                    render={({ field }) => (
                                      <FormItem>
                                        <FormLabel>
                                          {t(
                                            'aiProviders.customModel.configParameters.fields.precision'
                                          )}
                                        </FormLabel>
                                        <FormControl>
                                          <Input
                                            {...field}
                                            type="number"
                                            placeholder={t(
                                              'aiProviders.customModel.configParameters.placeholders.precision'
                                            )}
                                          />
                                        </FormControl>
                                        <FormMessage />
                                      </FormItem>
                                    )}
                                  />
                                </>
                              ) : null}
                            </div>

                            <div className="rounded-lg bg-background border p-3 space-y-1">
                              <div className="text-xs font-medium text-muted-foreground">
                                {t('aiProviders.customModel.configParameters.previewLabel')}
                              </div>
                              <div className="text-sm font-medium">{label}</div>
                              {help ? (
                                <>
                                  <div className="text-xs font-medium text-muted-foreground pt-2">
                                    {t('aiProviders.customModel.configParameters.previewHelp')}
                                  </div>
                                  <FormDescription className="!mt-0">{help}</FormDescription>
                                </>
                              ) : null}
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                </TabsContent>
              </DialogBody>

              <DialogFooter className="p-6 border-t bg-muted/5">
                <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
                  {t('aiProviders.cancel')}
                </Button>
                <Button type="submit" className="min-w-[120px]" disabled={isSubmitting}>
                  {isSubmitting ? t('aiProviders.actions.saving') : t('aiProviders.save')}
                </Button>
              </DialogFooter>
            </Tabs>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

function ToolItem({
  icon,
  label,
  checked,
  onChange,
}: {
  icon: React.ReactNode;
  label: string;
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center p-3 rounded-xl border transition-all cursor-pointer gap-2 hover:bg-muted/50',
        {
          'border-primary bg-primary/5 text-primary shadow-[0_0_0_1px_inset_rgba(var(--primary),0.1)]':
            checked,
          'bg-background text-muted-foreground opacity-70': !checked,
        }
      )}
      onClick={() => onChange(!checked)}
    >
      <div
        className={cn(
          'p-1.5 rounded-lg border',
          checked ? 'bg-primary/10 border-primary/20' : 'bg-muted/50 border-transparent'
        )}
      >
        {icon}
      </div>
      <span className="text-[10px] font-bold uppercase tracking-tight">{label}</span>
    </div>
  );
}
