'use client';

import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type Dispatch,
  type SetStateAction,
} from 'react';
import { CircleAlert, Settings2, TriangleAlert } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
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
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { ModelSelector, type ModelSelectorValue } from '@/components/common/model-selector';
import { useModelParameterRules } from '@/hooks/model/use-model-parameter-rules';
import type { ModelUseCase, ParameterRuleItem } from '@/services/types/model';
import { computeInitialNumber } from '@/utils/number';
import { useT } from '@/i18n';
import {
  applyTextChatParameterPreset,
  hasTextChatParameterPresetTargets,
  isTextChatParameterPresetId,
  TEXT_CHAT_PARAMETER_PRESETS,
  type TextChatParameterPresetId,
} from '@/utils/text-chat-parameter-presets';
import ParameterForm from './parameter-form';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { cn } from '@/lib/utils';

// Helper to compare params shallowly to avoid redundant onChange
function shallowEqualParams(
  a: Record<string, number | string | boolean>,
  b: Record<string, number | string | boolean>
): boolean {
  const aKeys = Object.keys(a);
  const bKeys = Object.keys(b);
  if (aKeys.length !== bKeys.length) return false;
  for (const k of aKeys) {
    if (a[k] !== b[k]) return false;
  }
  return true;
}

type ParameterStateKind = 'empty' | 'notFound' | 'loadFailed';

interface ParameterStateCardContent {
  badge: string;
  title: string;
  description: string;
  hint: string;
}

export interface ModelSelectorParameterValue {
  params: Record<string, number | string | boolean>;
  model: string;
  provider: string;
}

export interface ModelSelectorParameterProps {
  modelType: ModelUseCase;
  value: ModelSelectorParameterValue;
  onChange: (v: ModelSelectorParameterValue) => void;
  className?: string;
  /** When true, show parameter form inline under selector without dialog or save button. */
  parameterShow?: boolean;
  /** Filter models by capability requirements. Multiple capabilities can be required. */
  capabilityFilter?: {
    /** Require vision support */
    features_vision?: boolean;
    /** Require tool call support */
    features_tool_call?: boolean;
    /** Require attachment support */
    features_attachment?: boolean;
    /** Require reasoning support */
    features_reasoning?: boolean;
    /** Require structured output support */
    features_structured_output?: boolean;
  };
  /** When true, display an error border on the select trigger. */
  hasError?: boolean;
  /** Disable model selection and parameter editing. */
  disabled?: boolean;
  /** Within each provider, place models for this use case first and highlight them. */
  preferredUseCase?: ModelUseCase;
}

export default function ModelSelectorParameter({
  modelType,
  value,
  onChange,
  className,
  parameterShow = false,
  capabilityFilter,
  hasError = false,
  disabled = false,
  preferredUseCase,
}: ModelSelectorParameterProps) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);

  // Local form state
  const [enabledMap, setEnabledMap] = useState<Record<string, boolean>>({});
  const [localValues, setLocalValues] = useState<Record<string, number | string | boolean>>({});
  const [initializedAtOpen, setInitializedAtOpen] = useState(false);
  const [selectedPresetId, setSelectedPresetId] = useState<TextChatParameterPresetId | null>(null);
  const [lastSelectionKey, setLastSelectionKey] = useState<string>(
    `${value.provider}::${value.model}`
  );

  // Fetch rules when dialog opens or inline mode is active
  const {
    data: rules,
    isLoading,
    error,
    isNotFound,
  } = useModelParameterRules({
    provider: value?.provider,
    model: value?.model,
    enabled: parameterShow || open,
  });

  useEffect(() => {
    const shouldInit = parameterShow || open;
    if (!shouldInit) return;
    if (initializedAtOpen) return;
    if (!rules || rules.length === 0) return;
    // Initialize enable map and values from existing params or defaults
    const nextEnabled: Record<string, boolean> = {};
    const nextValues: Record<string, number | string | boolean> = {};
    rules.forEach((r: ParameterRuleItem) => {
      const hasValue = Object.prototype.hasOwnProperty.call(value?.params ?? {}, r.name);
      const required = !!r.required;
      const enabled = required || hasValue;
      const numericDefault = typeof r.default === 'number' ? r.default : null;
      const numericMin = typeof r.min === 'number' ? r.min : null;
      const numericMax = typeof r.max === 'number' ? r.max : null;
      nextEnabled[r.name] = enabled;
      if (enabled) {
        const incoming = (value?.params ?? {})[r.name];
        if (incoming !== undefined) {
          nextValues[r.name] = incoming as number | string | boolean;
        } else if (r.default !== null && r.default !== undefined) {
          nextValues[r.name] = r.default as number | string | boolean;
        } else if (r.type === 'int' || r.type === 'float') {
          nextValues[r.name] = computeInitialNumber({
            defaultValue: numericDefault,
            min: numericMin,
            max: numericMax,
            isInt: r.type === 'int',
          });
        } else if (r.type === 'boolean') {
          nextValues[r.name] = false;
        } else {
          nextValues[r.name] = '';
        }
      }
    });
    setEnabledMap(nextEnabled);
    setLocalValues(nextValues);
    setSelectedPresetId(null);
    setInitializedAtOpen(true);
  }, [parameterShow, open, rules, initializedAtOpen, value?.params]);

  // Reset init flag when dialog closes
  useEffect(() => {
    if (!open && !parameterShow && initializedAtOpen) {
      setInitializedAtOpen(false);
      setSelectedPresetId(null);
    }
  }, [open, parameterShow, initializedAtOpen]);

  // When provider/model changes while dialog is open, prepare to re-initialize when rules arrive
  useEffect(() => {
    const key = `${value.provider}::${value.model}`;
    if ((open || parameterShow) && key !== lastSelectionKey) {
      setInitializedAtOpen(false);
      setLastSelectionKey(key);
      setSelectedPresetId(null);
    }
  }, [open, parameterShow, value.provider, value.model, lastSelectionKey]);
  // In inline mode, sync parameter changes immediately to parent value
  useEffect(() => {
    if (!parameterShow) return;
    if (!rules) return;
    const nextParams: Record<string, number | string | boolean> = {};
    for (const r of rules) {
      if (!enabledMap[r.name]) continue;
      const v = localValues[r.name];
      if (v === undefined || v === null) continue;
      if ((r.type === 'int' || r.type === 'float') && v === '') continue;
      nextParams[r.name] = v as number | string | boolean;
    }
    if (!shallowEqualParams(value.params ?? {}, nextParams)) {
      onChange({ ...value, params: nextParams });
    }
  }, [parameterShow, rules, enabledMap, localValues, onChange, value]);

  const handleSave = useCallback(() => {
    const nextParams: Record<string, number | string | boolean> = {};
    for (const r of rules || []) {
      if (!enabledMap[r.name]) continue;
      const v = localValues[r.name];
      // For options/boolean/number types allow falsy values (e.g., false, 0) to be saved
      if (v === undefined || v === null) continue;
      if ((r.type === 'int' || r.type === 'float') && v === '') continue;
      nextParams[r.name] = v as number | string | boolean;
    }
    onChange({ ...value, params: nextParams });
    setOpen(false);
  }, [rules, enabledMap, localValues, onChange, value]);

  const computeParamsFromLocal = useCallback((): Record<string, number | string | boolean> => {
    const out: Record<string, number | string | boolean> = {};
    for (const r of rules || []) {
      if (!enabledMap[r.name]) continue;
      const v = localValues[r.name];
      if (v === undefined || v === null) continue;
      if ((r.type === 'int' || r.type === 'float') && v === '') continue;
      out[r.name] = v as number | string | boolean;
    }
    return out;
  }, [rules, enabledMap, localValues]);

  const hasUnsaved = useMemo(() => {
    if (!open) return false;
    if (!rules) return false;
    const nextParams = computeParamsFromLocal();
    return !shallowEqualParams(value.params ?? {}, nextParams);
  }, [open, rules, computeParamsFromLocal, value.params]);

  const handleDialogOpenChange = useCallback(
    (next: boolean) => {
      if (next) {
        setOpen(true);
        return;
      }
      if (hasUnsaved) {
        setConfirmOpen(true);
        return;
      }
      setOpen(false);
    },
    [hasUnsaved]
  );

  const onModelChange = useCallback(
    (v: ModelSelectorValue) => {
      setSelectedPresetId(null);
      onChange({ ...value, model: v.model, provider: v.provider });
    },
    [onChange, value]
  );

  const handleEnabledMapChange = useCallback<Dispatch<SetStateAction<Record<string, boolean>>>>(
    next => {
      setSelectedPresetId(null);
      setEnabledMap(next);
    },
    [setEnabledMap]
  );

  const handleLocalValuesChange = useCallback<
    Dispatch<SetStateAction<Record<string, number | string | boolean>>>
  >(
    next => {
      setSelectedPresetId(null);
      setLocalValues(next);
    },
    [setLocalValues]
  );

  const showTextChatPresets = useMemo(() => {
    if (modelType !== 'text-chat') {
      return false;
    }

    if (!rules || rules.length === 0) {
      return false;
    }

    return hasTextChatParameterPresetTargets(rules);
  }, [modelType, rules]);

  const handleApplyPreset = useCallback(
    (presetId: TextChatParameterPresetId) => {
      if (!rules || rules.length === 0) {
        return;
      }

      const nextState = applyTextChatParameterPreset({
        presetId,
        rules,
        enabledMap,
        localValues,
      });

      if (nextState.appliedCount === 0) {
        return;
      }

      setEnabledMap(nextState.enabledMap);
      setLocalValues(nextState.localValues);
      setSelectedPresetId(presetId);
    },
    [rules, enabledMap, localValues]
  );

  const getParameterStateCardContent = useCallback(
    (kind: ParameterStateKind): ParameterStateCardContent => ({
      badge: t(`models.configParameters.states.${kind}.badge` as never),
      title: t(`models.configParameters.states.${kind}.title` as never),
      description: t(`models.configParameters.states.${kind}.description` as never),
      hint: t(`models.configParameters.states.${kind}.hint` as never),
    }),
    [t]
  );

  const renderParameterStateCard = useCallback(
    (kind: ParameterStateKind) => {
      const copy = getParameterStateCardContent(kind);
      const Icon =
        kind === 'loadFailed' ? TriangleAlert : kind === 'notFound' ? CircleAlert : Settings2;
      const iconWrapperClassName =
        kind === 'loadFailed'
          ? 'border-warning/20 bg-warning/10 text-warning'
          : kind === 'notFound'
            ? 'border-border/70 bg-background text-muted-foreground'
            : 'border-primary/15 bg-primary/10 text-primary';
      const badgeVariant = kind === 'loadFailed' ? 'warning' : kind === 'empty' ? 'info' : 'subtle';
      const modelLabel =
        value.provider && value.model ? `${value.provider} / ${value.model}` : null;

      return (
        <div className="flex min-h-[180px] flex-col items-center justify-center rounded-2xl border border-dashed border-border/70 bg-muted/10 px-6 py-8 text-center sm:min-h-[220px] sm:px-8 sm:py-10">
          <Badge variant={badgeVariant} className="mb-4 rounded-full px-3 py-1">
            {copy.badge}
          </Badge>
          <div
            className={cn(
              'mb-4 flex size-14 items-center justify-center rounded-2xl border',
              iconWrapperClassName
            )}
          >
            <Icon className="h-5 w-5" />
          </div>
          <div className="space-y-2">
            <div className="text-base font-semibold text-foreground">{copy.title}</div>
            <p className="max-w-md text-sm leading-6 text-muted-foreground">{copy.description}</p>
          </div>
          {modelLabel ? (
            <Badge variant="outline" className="mt-4 max-w-full truncate px-3 py-1">
              {modelLabel}
            </Badge>
          ) : null}
          <p className="mt-4 max-w-md text-xs leading-5 text-muted-foreground">{copy.hint}</p>
        </div>
      );
    },
    [getParameterStateCardContent, value.model, value.provider]
  );

  const canSaveParameters = Boolean(rules && rules.length > 0);

  const parameterFormContent =
    !rules || isLoading ? (
      <div className="space-y-3">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-9 w-full" />
        ))}
      </div>
    ) : rules.length === 0 ? (
      renderParameterStateCard(isNotFound ? 'notFound' : error ? 'loadFailed' : 'empty')
    ) : (
      <div className="space-y-4">
        {showTextChatPresets ? (
          <div className="rounded-lg border bg-muted/20 px-3 py-2.5">
            <div className="flex items-center gap-3">
              <div className="min-w-0 flex-1">
                <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
                  {t('models.configParameters.presets.title')}
                </div>
              </div>
              <div className="w-full max-w-[220px]">
                <Select
                  value={selectedPresetId ?? undefined}
                  onValueChange={value => {
                    if (!isTextChatParameterPresetId(value)) {
                      return;
                    }

                    handleApplyPreset(value);
                  }}
                  disabled={disabled}
                >
                  <SelectTrigger className="h-8 bg-background text-xs">
                    <SelectValue placeholder={t('models.configParameters.presets.placeholder')} />
                  </SelectTrigger>
                  <SelectContent>
                    {TEXT_CHAT_PARAMETER_PRESETS.map(preset => (
                      <SelectItem key={preset.id} value={preset.id} className="text-xs">
                        {t(`models.configParameters.presets.items.${preset.id}.label` as never)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>
          </div>
        ) : null}
        <ParameterForm
          rules={rules}
          enabledMap={enabledMap}
          setEnabledMap={handleEnabledMapChange}
          localValues={localValues}
          setLocalValues={handleLocalValuesChange}
        />
      </div>
    );

  return (
    <div className={className}>
      <div className="flex items-center gap-2">
        <div className="flex-1">
          <ModelSelector
            modelType={modelType}
            value={
              value?.provider && value?.model
                ? { provider: value.provider, model: value.model }
                : undefined
            }
            onChange={onModelChange}
            capabilityFilter={capabilityFilter}
            hasError={hasError}
            disabled={disabled}
            preferredUseCase={preferredUseCase}
          />
        </div>
        {!parameterShow ? (
          <Button
            type="button"
            variant="ghost"
            isIcon
            aria-label={t('models.modelParameters')}
            onClick={() => setOpen(true)}
            disabled={disabled || !value.model || !value.provider}
          >
            <Settings2 className="h-4 w-4" />
          </Button>
        ) : null}
      </div>
      {parameterShow ? (
        <div className="mt-3">{parameterFormContent}</div>
      ) : (
        <>
          <Dialog open={open} onOpenChange={handleDialogOpenChange}>
            <DialogContent aria-describedby={undefined} className="max-w-3xl">
              <DialogHeader>
                <DialogTitle className="flex items-center gap-2">
                  <Settings2 className="h-4 w-4" />
                  <span>{t('models.modelParameters')}</span>
                </DialogTitle>
              </DialogHeader>

              <DialogBody className="font-medium">{parameterFormContent}</DialogBody>

              <DialogFooter>
                {canSaveParameters ? (
                  <Button onClick={handleSave}>{t('common.save')}</Button>
                ) : (
                  <Button variant="outline" onClick={() => setOpen(false)}>
                    {t('common.close')}
                  </Button>
                )}
              </DialogFooter>
            </DialogContent>
          </Dialog>
          <ConfirmDialog
            open={confirmOpen}
            onOpenChange={setConfirmOpen}
            title={t('models.unsavedChanges.title')}
            description={t('models.unsavedChanges.description')}
            cancelText={t('models.unsavedChanges.discard')}
            confirmText={t('models.unsavedChanges.save')}
            onCancel={() => {
              setConfirmOpen(false);
              setOpen(false);
            }}
            onConfirm={handleSave}
            variant="warning"
          />
        </>
      )}
    </div>
  );
}
