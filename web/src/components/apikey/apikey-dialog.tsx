'use client';

import React from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
  DialogFooter,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Switch } from '@/components/ui/switch';
import { Button } from '@/components/ui/button';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useCreateApiKey, useUpdateApiKey } from '@/hooks';
import {
  ApiKeyQuotaType,
  type ApiKeyItem,
  type CreateApiKeyRequest,
  type UpdateApiKeyRequest,
} from '@/services/types/apikey';
import { useT } from '@/i18n';
import { Copy } from 'lucide-react';
import { toast } from 'sonner';
import ModelMultiSelector from '@/components/common/model-multi-selector/model-multi-selector';
import { DEFAULT_AI_CREDIT_EDIT_MAX, sanitizeAiCreditIntegerInput } from '@/utils/ai-credits';
import { datetimeLocalToISO, formatDateTimeLocalInput } from '@/utils/date-input';

const BATCH_COUNT_MAX = 20;

type CreatedApiKey = ApiKeyItem & { key: string };
type ExpirationPreset = 'never' | 'hour' | 'day' | 'month';

const parseIntegerInput = (value: string): number | null => {
  const trimmed = value.trim();
  if (!/^\d+$/.test(trimmed)) return null;
  const parsed = Number.parseInt(trimmed, 10);
  return Number.isSafeInteger(parsed) ? parsed : null;
};

/**
 * Props for ApiKeyDialog component
 */
export interface ApiKeyDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: 'create' | 'edit';
  initial?: ApiKeyItem | null;
}

export default function ApiKeyDialog({
  open,
  onOpenChange,
  mode,
  initial,
}: ApiKeyDialogProps): JSX.Element {
  const t = useT('apikeys');
  const tCommon = useT('common');
  const quotaAmountMaxLabel = DEFAULT_AI_CREDIT_EDIT_MAX.toLocaleString();

  const [name, setName] = React.useState<string>(initial?.name ?? '');
  const [count, setCount] = React.useState<string>('1');
  const [createdKeys, setCreatedKeys] = React.useState<CreatedApiKey[]>([]);
  const [quotaType, setQuotaType] = React.useState<ApiKeyQuotaType>(ApiKeyQuotaType.Unlimited);
  const [quotaAmount, setQuotaAmount] = React.useState<string>('');
  const [allowAllModels, setAllowAllModels] = React.useState<boolean>(true);
  const [modelNames, setModelNames] = React.useState<string[]>([]);
  const [expiresAt, setExpiresAt] = React.useState<string>('');

  const { createApiKey, isCreating } = useCreateApiKey();
  const { updateApiKey, isUpdating } = useUpdateApiKey();

  const getMinDateTime = React.useCallback((): string => formatDateTimeLocalInput(new Date()), []);

  const closeDialog = React.useCallback((): void => {
    setCreatedKeys([]);
    onOpenChange(false);
  }, [onOpenChange]);

  const handleOpenChange = React.useCallback(
    (nextOpen: boolean): void => {
      if (!nextOpen) {
        setCreatedKeys([]);
      }
      onOpenChange(nextOpen);
    },
    [onOpenChange]
  );

  React.useEffect(() => {
    if (!open) return;

    setCreatedKeys([]);
    if (mode === 'create') {
      setName('');
      setCount('1');
      setQuotaType(ApiKeyQuotaType.Unlimited);
      setQuotaAmount('');
      setAllowAllModels(true);
      setModelNames([]);
      setExpiresAt('');
      return;
    }

    if (!initial) return;

    setName(initial.name ?? '');
    setCount('1');
    setModelNames(initial.model_limits ?? []);
    setAllowAllModels(!initial.model_limits_enabled);

    const hasQuotaLimit = initial.quota_limit !== null && initial.quota_limit !== undefined;
    setQuotaType(hasQuotaLimit ? ApiKeyQuotaType.Custom : ApiKeyQuotaType.Unlimited);
    setQuotaAmount(hasQuotaLimit ? String(initial.quota_limit) : '');
    setExpiresAt(initial.expires_at ? formatDateTimeLocalInput(initial.expires_at) : '');
  }, [open, mode, initial]);

  const parsedCount = parseIntegerInput(count);
  const parsedQuotaAmount = parseIntegerInput(quotaAmount);
  const isCountValid =
    mode === 'edit' || (parsedCount !== null && parsedCount >= 1 && parsedCount <= BATCH_COUNT_MAX);
  const isQuotaValid =
    quotaType !== ApiKeyQuotaType.Custom ||
    (parsedQuotaAmount !== null &&
      parsedQuotaAmount > 0 &&
      parsedQuotaAmount <= DEFAULT_AI_CREDIT_EDIT_MAX);
  const disabled = isCreating || isUpdating;
  const canSubmit = !disabled && name.trim() !== '' && isCountValid && isQuotaValid;

  const copyText = (value: string): void => {
    void navigator.clipboard.writeText(value).then(() => {
      toast.success(tCommon('toasts.copySuccess'));
    });
  };

  const applyExpirationPreset = (preset: ExpirationPreset): void => {
    if (preset === 'never') {
      setExpiresAt('');
      return;
    }

    const next = new Date();
    if (preset === 'hour') {
      next.setHours(next.getHours() + 1);
    } else if (preset === 'day') {
      next.setDate(next.getDate() + 1);
    } else {
      next.setMonth(next.getMonth() + 1);
    }
    setExpiresAt(formatDateTimeLocalInput(next));
  };

  const onSubmit = async (): Promise<void> => {
    if (mode === 'create') {
      if (parsedCount === null || parsedCount < 1 || parsedCount > BATCH_COUNT_MAX) {
        toast.error(t('dialog.errors.countInvalid', { max: BATCH_COUNT_MAX }));
        return;
      }
    }

    if (quotaType === ApiKeyQuotaType.Custom) {
      if (parsedQuotaAmount === null || parsedQuotaAmount <= 0) {
        toast.error(t('dialog.errors.quotaAmountRequired'));
        return;
      }
      if (parsedQuotaAmount > DEFAULT_AI_CREDIT_EDIT_MAX) {
        toast.error(t('dialog.errors.quotaAmountMax', { max: quotaAmountMaxLabel }));
        return;
      }
    }

    if (!allowAllModels && modelNames.length === 0) {
      toast.error(t('dialog.errors.modelRequired'));
      return;
    }

    if (expiresAt) {
      const expiresAtISO = datetimeLocalToISO(expiresAt);
      if (!expiresAtISO || new Date(expiresAtISO) <= new Date()) {
        toast.error(t('dialog.errors.expiresInPast'));
        return;
      }
    }

    if (mode === 'create') {
      const payload: CreateApiKeyRequest = {
        name: name.trim(),
        count: parsedCount ?? 1,
        quota_type: quotaType,
        quota_amount:
          quotaType === ApiKeyQuotaType.Custom ? (parsedQuotaAmount ?? undefined) : undefined,
        allow_all_models: allowAllModels,
        model_names: allowAllModels ? undefined : modelNames,
        expires_at: datetimeLocalToISO(expiresAt),
      };

      const res = await createApiKey(payload);
      if (!res || res.keys.length === 0) {
        closeDialog();
        return;
      }

      const visibleKeys = res.keys.filter((key): key is CreatedApiKey => Boolean(key.key));
      if (visibleKeys.length !== res.keys.length) {
        toast.error(t('dialog.errors.createdKeysMissing'));
        return;
      }
      setCreatedKeys(visibleKeys);
      return;
    }

    if (!initial) return;

    const update: UpdateApiKeyRequest = {
      name: name.trim() !== initial.name ? name.trim() : undefined,
      model_limits_enabled: !allowAllModels,
      model_limits: allowAllModels ? [] : modelNames,
      quota_limit:
        quotaType === ApiKeyQuotaType.Custom && parsedQuotaAmount !== null
          ? parsedQuotaAmount
          : null,
      expires_at: expiresAt ? (datetimeLocalToISO(expiresAt) ?? null) : null,
    };
    await updateApiKey(initial.id, update);
    closeDialog();
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        className={
          createdKeys.length > 0
            ? 'w-[calc(100vw-2rem)] max-w-[720px] p-0'
            : 'w-[calc(100vw-2rem)] max-w-[760px] p-0'
        }
      >
        {createdKeys.length > 0 ? (
          <div className="flex min-h-0 flex-col">
            <DialogHeader className="px-6 pb-3 pt-6">
              <DialogTitle>
                {createdKeys.length > 1
                  ? t('dialog.createdCountTitle', { count: createdKeys.length })
                  : t('dialog.createdTitle')}
              </DialogTitle>
              <DialogDescription>
                {createdKeys.length > 1
                  ? t('dialog.createdBatchNotice')
                  : t('dialog.createdNotice')}
              </DialogDescription>
            </DialogHeader>
            <DialogBody className="max-h-[520px] space-y-3 px-6 py-2">
              {createdKeys.map(createdKey => (
                <div
                  key={createdKey.id}
                  className="flex items-start gap-3 rounded-lg border bg-muted/30 p-3"
                >
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="truncate text-sm font-medium text-foreground">
                      {createdKey.name}
                    </div>
                    <div className="break-all font-mono text-xs leading-5 text-foreground">
                      {createdKey.key}
                    </div>
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    isIcon
                    aria-label={tCommon('copy')}
                    onClick={() => copyText(createdKey.key)}
                  >
                    <Copy className="h-4 w-4" />
                  </Button>
                </div>
              ))}
            </DialogBody>
            <DialogFooter className="border-t bg-muted/30 px-6 py-4">
              <Button variant="ghost" onClick={closeDialog}>
                {tCommon('close')}
              </Button>
              <Button
                onClick={() =>
                  copyText(createdKeys.map(key => `${key.name}: ${key.key}`).join('\n'))
                }
              >
                <Copy className="h-4 w-4" />
                {t('dialog.buttons.copyAll')}
              </Button>
            </DialogFooter>
          </div>
        ) : (
          <div className="flex min-h-0 flex-col">
            <DialogHeader className="px-6 pb-3 pt-6">
              <DialogTitle>
                {mode === 'create' ? t('dialog.titleCreate') : t('dialog.titleEdit')}
              </DialogTitle>
              <DialogDescription>{t('description')}</DialogDescription>
            </DialogHeader>

            <DialogBody className="space-y-4 px-6 py-2">
              <section className="space-y-4 rounded-lg border bg-muted/10 p-4">
                <div className="space-y-1">
                  <h3 className="text-sm font-semibold text-foreground">
                    {t('dialog.sections.basic')}
                  </h3>
                </div>

                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="name">
                      {t('dialog.labels.name')} <span className="text-destructive">*</span>
                    </Label>
                    <Input
                      id="name"
                      value={name}
                      onChange={e => setName(e.target.value)}
                      placeholder={t('dialog.placeholders.name')}
                      maxLength={30}
                    />
                  </div>

                  {mode === 'create' && (
                    <div className="space-y-2">
                      <Label htmlFor="count">{t('dialog.labels.count')}</Label>
                      <Input
                        id="count"
                        type="number"
                        min={1}
                        max={BATCH_COUNT_MAX}
                        step={1}
                        value={count}
                        onChange={e => setCount(e.target.value.replace(/\D/g, ''))}
                        placeholder={t('dialog.placeholders.count')}
                      />
                      <p className="text-xs text-muted-foreground">
                        {t('dialog.hints.countMax', { max: BATCH_COUNT_MAX })}
                      </p>
                    </div>
                  )}
                </div>
              </section>

              <section className="space-y-4 rounded-lg border bg-muted/10 p-4">
                <div className="space-y-1">
                  <h3 className="text-sm font-semibold text-foreground">
                    {t('dialog.sections.quota')}
                  </h3>
                </div>

                <div className="grid gap-4 sm:grid-cols-2">
                  <div className="space-y-2">
                    <Label htmlFor="quotaType">{t('dialog.labels.quotaType')}</Label>
                    <Select
                      value={quotaType}
                      onValueChange={v => setQuotaType(v as ApiKeyQuotaType)}
                    >
                      <SelectTrigger id="quotaType">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value={ApiKeyQuotaType.Unlimited}>
                          {t('dialog.quotaTypes.unlimited')}
                        </SelectItem>
                        <SelectItem value={ApiKeyQuotaType.Custom}>
                          {t('dialog.quotaTypes.custom')}
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  {quotaType === ApiKeyQuotaType.Custom && (
                    <div className="space-y-2">
                      <Label htmlFor="quotaAmount">
                        {t('dialog.labels.quotaAmount')}
                        <span className="ml-0.5 text-destructive">*</span>
                      </Label>
                      <Input
                        id="quotaAmount"
                        type="number"
                        min={1}
                        max={DEFAULT_AI_CREDIT_EDIT_MAX}
                        step={1}
                        value={quotaAmount}
                        onChange={e =>
                          setQuotaAmount(
                            sanitizeAiCreditIntegerInput(e.target.value, DEFAULT_AI_CREDIT_EDIT_MAX)
                          )
                        }
                        placeholder={t('dialog.placeholders.quotaAmount')}
                      />
                      <p className="text-xs text-muted-foreground">
                        {t('dialog.hints.quotaAmountMax', { max: quotaAmountMaxLabel })}
                      </p>
                    </div>
                  )}
                </div>
              </section>

              <section className="space-y-4 rounded-lg border bg-muted/10 p-4">
                <div className="space-y-1">
                  <h3 className="text-sm font-semibold text-foreground">
                    {t('dialog.sections.access')}
                  </h3>
                </div>

                <div className="flex items-center justify-between gap-4">
                  <Label htmlFor="allowAllModels">{t('dialog.labels.allowAllModels')}</Label>
                  <Switch
                    id="allowAllModels"
                    checked={allowAllModels}
                    onCheckedChange={v => setAllowAllModels(Boolean(v))}
                  />
                </div>

                {allowAllModels ? (
                  <p className="text-xs text-muted-foreground">
                    {t('dialog.hints.allowAllModelsEnabled')}
                  </p>
                ) : (
                  <div className="space-y-2">
                    <Label>{t('dialog.labels.modelNames')}</Label>
                    <p className="text-xs text-muted-foreground">
                      {t('dialog.hints.modelLimitsSelected')}
                    </p>
                    <div className="h-[320px] overflow-hidden rounded-lg border bg-background">
                      <ModelMultiSelector
                        value={modelNames}
                        onChange={setModelNames}
                        isEnabled
                        columns={2}
                        className="h-full overflow-hidden"
                      />
                    </div>
                  </div>
                )}

                <div className="space-y-2">
                  <Label htmlFor="expiresAt">{t('dialog.labels.expiresAt')}</Label>
                  <div className="flex flex-wrap gap-2">
                    <Button
                      type="button"
                      variant={expiresAt ? 'outline' : 'secondary'}
                      size="sm"
                      onClick={() => applyExpirationPreset('never')}
                    >
                      {t('dialog.expirationPresets.never')}
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => applyExpirationPreset('hour')}
                    >
                      {t('dialog.expirationPresets.hour')}
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => applyExpirationPreset('day')}
                    >
                      {t('dialog.expirationPresets.day')}
                    </Button>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => applyExpirationPreset('month')}
                    >
                      {t('dialog.expirationPresets.month')}
                    </Button>
                  </div>
                  <Input
                    id="expiresAt"
                    type="datetime-local"
                    value={expiresAt}
                    min={getMinDateTime()}
                    onChange={e => setExpiresAt(e.target.value)}
                  />
                  <p className="text-xs text-muted-foreground">{t('dialog.hints.expiresAt')}</p>
                </div>
              </section>
            </DialogBody>

            <DialogFooter className="border-t bg-muted/30 px-6 py-4">
              <Button variant="outline" onClick={closeDialog} disabled={disabled}>
                {t('dialog.buttons.cancel')}
              </Button>
              <Button onClick={onSubmit} disabled={!canSubmit} loading={disabled}>
                {mode === 'create' ? t('dialog.buttons.create') : t('dialog.buttons.save')}
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
