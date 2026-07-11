'use client';

import React, { useEffect, useRef } from 'react';
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

const useCases: ModelUseCase[] = USE_CASE_ORDER;
const EXCLUSIVE_USE_CASES: string[] = ['text-chat', 'embedding', 'rerank'];
const CONFIG_PARAMETER_TYPES = ['int', 'float', 'string', 'boolean', 'text'] as const satisfies readonly ParameterValueType[];

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
const ENDPOINT_KEYS = Object.keys(endpointSchema.shape) as Array<keyof z.infer<typeof endpointSchema>>;
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

const createSchema = (
  t: (key: string, values?: Record<string, string | number>) => string
) =>
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
      cached_input_price: z.string().optional(),

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
            name: z.string().trim().min(1, t('aiProviders.models.validation.configParameterNameRequired')),
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

            const numericFields: Array<keyof Pick<ConfigParameterFormValue, 'default' | 'min' | 'max'>> =
              ['default', 'min', 'max'];

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
    );

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
      cached_input_price: '0',
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
        input_price: initialData.input_price_configured ? String(initialData.input_price) : '',
        output_price: initialData.output_price_configured ? String(initialData.output_price) : '',
        cached_input_price: String(initialData.cached_input_price || '0'),
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
        cached_input_price: '0',
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
  }, [initialData, form, open]);

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

    const data = {
      ...rest,
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
                    <div className="grid grid-cols-3 gap-4">
                      <FormField
                        control={form.control}
                        name="input_price"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('aiProviders.models.fields.inputPrice')}</FormLabel>
                            <FormControl>
                              <div className="relative">
                                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                  $
                                </span>
                                <Input className="pl-6" {...field} />
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
                        name="output_price"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('aiProviders.models.fields.outputPrice')}</FormLabel>
                            <FormControl>
                              <div className="relative">
                                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                  $
                                </span>
                                <Input className="pl-6" {...field} />
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
                        name="cached_input_price"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('aiProviders.models.fields.cachedPrice')}</FormLabel>
                            <FormControl>
                              <div className="relative">
                                <span className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground text-sm">
                                  $
                                </span>
                                <Input className="pl-6" {...field} />
                              </div>
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </div>
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
                        const rowValue = configParameterValues?.[index] ?? createEmptyConfigParameter();
                        const isNumericType = isNumericParameterType(rowValue.type);
                        const { help, label } = resolveModelConfigParameterCopy({
                          parameter: {
                            name: rowValue.name,
                            template_key: rowValue.template_key,
                          },
                          translate: key => t(key as never),
                        });

                        return (
                          <div key={field.fieldId} className="rounded-xl border p-4 space-y-4 bg-muted/10">
                            <div className="flex items-center justify-between gap-3">
                              <div className="text-sm font-medium">
                                {rowValue.name || t('aiProviders.customModel.configParameters.title')}
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

                            <div className={cn('grid gap-4', isNumericType ? 'lg:grid-cols-4' : 'lg:grid-cols-2')}>
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
                                          {t(
                                            'aiProviders.customModel.configParameters.fields.min'
                                          )}
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
                                          {t(
                                            'aiProviders.customModel.configParameters.fields.max'
                                          )}
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
