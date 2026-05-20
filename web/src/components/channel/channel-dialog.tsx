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
import { useCreateChannel, useUpdateChannel } from '@/hooks';
import type {
  CreateChannelRequest,
  ChannelDetail,
  UpdateChannelRequest,
} from '@/services/types/channel';
import { useT } from '@/i18n';
import ModelMultiSelector from '@/components/common/model-multi-selector/model-multi-selector';
import { ChevronDown } from 'lucide-react';
import { cn } from '@/lib/utils';
import ChannelProviderSelector, {
  getMappedProvider,
  getChannelProviderOption,
} from '@/components/channel/channel-provider-selector';
import {
  CHANNEL_INITIAL_CREDIT_MAX,
  CHANNEL_POINTS_PER_USD,
  formatChannelCreditPoints,
  usdToChannelPoints,
} from '@/utils/ai-credits';

const DEFAULT_INITIAL_FUNDS_USD = '100.00';

/**
 * Props for ChannelDialog component.
 * Only supports fields that exist in current channel API.
 */
export interface ChannelDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: 'create' | 'edit';
  initial?: ChannelDetail | null;
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

interface ChannelFormProps {
  mode: 'create' | 'edit';
  initial?: ChannelDetail | null;
  onOpenChange: (open: boolean) => void;
}

function ChannelForm({ mode, initial, onOpenChange }: ChannelFormProps) {
  const t = useT('channels');
  const initialChannelProvider =
    initial?.channel_provider ?? initial?.provider ?? 'openai-compatible';
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
  const [priority, setPriority] = React.useState<string>(
    initial?.priority != null ? String(initial.priority) : ''
  );
  const [weight, setWeight] = React.useState<string>(
    initial?.weight != null ? String(initial.weight) : ''
  );
  const [isEnabled, setIsEnabled] = React.useState<boolean>(initial?.is_enabled ?? true);
  const [advancedOpen, setAdvancedOpen] = React.useState<boolean>(false);
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

  // Stable callbacks to prevent child component re-renders when parent re-renders

  const handleModelsChange = useCallback((models: string[]) => {
    setModelsSelected(models);
  }, []);

  const { createChannel, isCreating } = useCreateChannel();
  const { updateChannel, isUpdating } = useUpdateChannel();

  const handleNumericInput = (
    value: string,
    setter: React.Dispatch<React.SetStateAction<string>>,
    max: number
  ) => {
    setter(sanitizeNonNegativeInt(value, max));
  };

  const onSubmit = async (): Promise<void> => {
    // Build payloads strictly from allowed fields
    const common = {
      channel_provider: channelProvider.trim(),
      api_base_url: apiBaseUrl || undefined,
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
    if (apiBaseUrl !== (initial.api_base_url ?? '')) {
      update.api_base_url = apiBaseUrl || undefined;
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

  const disabled = isCreating || isUpdating;

  const selectedProviderOption = React.useMemo(
    () => getChannelProviderOption(channelProvider),
    [channelProvider]
  );
  const mappedProvider = React.useMemo(
    () => selectedProviderOption?.provider || getMappedProvider(channelProvider),
    [channelProvider, selectedProviderOption]
  );
  const apiKeyPlaceholder =
    selectedProviderOption?.apiKeyPlaceholder || t('dialog.placeholders.apiKey');
  const apiBaseUrlPlaceholder =
    selectedProviderOption?.defaultApiBaseUrl ||
    selectedProviderOption?.apiBaseUrlPlaceholder ||
    t('dialog.placeholders.apiBaseUrl');

  return (
    <div className="h-full overflow-hidden flex flex-col gap-2">
      <DialogHeader className="px-6 pt-6">
        <DialogTitle>
          {mode === 'create' ? t('dialog.titleCreate') : t('dialog.titleEdit')}
        </DialogTitle>
        <DialogDescription>{t('description')}</DialogDescription>
      </DialogHeader>

      <DialogBody className="grid grid-cols-3 gap-4 h-0 grow">
        <div className="col-span-1 space-y-3 overflow-y-auto pr-3">
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
                setChannelProvider(option.value);
                if (option.defaultApiBaseUrl) {
                  setApiBaseUrl(option.defaultApiBaseUrl);
                  return;
                }
                setApiBaseUrl('');
              }}
              disabled={disabled}
            />
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
            <div className="space-y-2 rounded-md border border-primary/30 bg-primary/5 p-3">
              <div className="flex items-center justify-between gap-2">
                <div className="text-sm font-medium">
                  {t('dialog.labels.initialFunds')}
                  <span className="text-destructive ml-0.5">*</span>
                </div>
                <div className="text-xs text-muted-foreground">{t('credit.rate')}</div>
              </div>
              <p className="text-xs text-muted-foreground">
                {t('dialog.hints.initialFundsDefault')}
              </p>
              <div className="flex items-center gap-2">
                <div className="flex h-10 shrink-0 items-center rounded-md border bg-background px-3 text-sm text-muted-foreground">
                  $
                </div>
                <Input
                  inputMode="decimal"
                  value={initialFundsUsd}
                  onChange={e =>
                    setInitialFundsUsd(sanitizeUsdInput(e.target.value, maxInitialFundsUsd))
                  }
                  placeholder={t('dialog.placeholders.initialFunds')}
                />
                <div className="flex h-10 shrink-0 items-center rounded-md border bg-background px-3 text-sm text-muted-foreground">
                  USD
                </div>
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
            <div id="advanced-section" className="grid grid-cols-2 gap-3">
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
              <div className="flex items-center gap-3 col-span-2">
                <div className="text-sm font-medium">{t('dialog.labels.enabled')}</div>
                <Switch checked={isEnabled} onCheckedChange={v => setIsEnabled(Boolean(v))} />
              </div>
            </div>
          </div>
        </div>
        <div className="col-span-2 overflow-y-auto pl-3">
          <ModelMultiSelector
            value={modelsSelected}
            onChange={handleModelsChange}
            placeholder={t('dialog.placeholders.modelsCsv')}
            className="max-h-[calc(92vh-12rem)] overflow-hidden"
            preferredProvider={mappedProvider}
            autoCollapseOthers={mappedProvider !== 'all'}
          />
        </div>
      </DialogBody>

      <div className="pb-6 px-6 pt-4 border-t shrink-0 flex items-center justify-end gap-2">
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
            (mode === 'create' && apiKeyRequired && !apiKey.trim()) ||
            (mode === 'edit' && updateApiKey && !apiKey.trim())
          }
        >
          {mode === 'create' ? t('dialog.buttons.create') : t('dialog.buttons.save')}
        </Button>
      </div>
    </div>
  );
}

export default function ChannelDialog({
  open,
  onOpenChange,
  mode,
  initial,
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
          <ChannelForm mode={mode} initial={normalizedInitial} onOpenChange={onOpenChange} />
        )}
      </DialogContent>
    </Dialog>
  );
}
