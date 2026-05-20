'use client';

import React from 'react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogBody,
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
import { ChevronDown, X, Plus, Copy } from 'lucide-react';
import { cn } from '@/lib/utils';
import { Badge } from '@/components/ui/badge';
import { toast } from 'sonner';
import ModelMultiSelector from '@/components/common/model-multi-selector/model-multi-selector';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { DEFAULT_AI_CREDIT_EDIT_MAX, sanitizeAiCreditIntegerInput } from '@/utils/ai-credits';

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

  // Get managed workspaces for default value
  const { currentOrganization } = useOrganizations();

  // Form state
  const [name, setName] = React.useState<string>(initial?.name ?? '');
  const [secretKey, setSecretKey] = React.useState<string | null>(null);
  const [quotaType, setQuotaType] = React.useState<ApiKeyQuotaType>(ApiKeyQuotaType.Unlimited);
  const [quotaAmount, setQuotaAmount] = React.useState<string>('');
  const [allowAllModels, setAllowAllModels] = React.useState<boolean>(true);
  const [modelNames, setModelNames] = React.useState<string[]>([]);
  const [allowIps, setAllowIps] = React.useState<string[]>([]);
  const [ipInput, setIpInput] = React.useState<string>('');
  const [expiresAt, setExpiresAt] = React.useState<string>('');
  const [advancedOpen, setAdvancedOpen] = React.useState<boolean>(false);

  // Get current datetime in local format for min validation
  const getMinDateTime = React.useCallback((): string => {
    const now = new Date();
    const year = now.getFullYear();
    const month = String(now.getMonth() + 1).padStart(2, '0');
    const day = String(now.getDate()).padStart(2, '0');
    const hours = String(now.getHours()).padStart(2, '0');
    const minutes = String(now.getMinutes()).padStart(2, '0');
    return `${year}-${month}-${day}T${hours}:${minutes}`;
  }, []);

  // Reset on open/initial change
  React.useEffect(() => {
    if (!open) return;
    if (mode === 'create') {
      setName('');
      setQuotaType(ApiKeyQuotaType.Unlimited);
      setQuotaAmount('');
      setAllowAllModels(true);
      setModelNames([]);
      setAllowIps([]);
      setIpInput('');
      setExpiresAt('');
      setSecretKey(null);
      setAdvancedOpen(false);
    } else if (initial) {
      setName(initial.name ?? '');
      // Parse model_limits from initial if it exists
      setModelNames(initial.model_limits ?? []);
      setAllowAllModels(!initial.model_limits_enabled);
      const hasQuotaLimit = initial.quota_limit !== null && initial.quota_limit !== undefined;
      setQuotaType(hasQuotaLimit ? ApiKeyQuotaType.Custom : ApiKeyQuotaType.Unlimited);
      setQuotaAmount(hasQuotaLimit ? String(initial.quota_limit) : '');
      // Parse allow_ips from comma-separated string to array
      const ips = initial.allow_ips
        ? initial.allow_ips
            .split(',')
            .map(ip => ip.trim())
            .filter(Boolean)
        : [];
      setAllowIps(ips);
      setIpInput('');
      setAdvancedOpen(false);
    }
  }, [open, mode, initial, currentOrganization]);

  const { createApiKey, isCreating } = useCreateApiKey();
  const { updateApiKey, isUpdating } = useUpdateApiKey();

  // Add IP to list
  const addIp = (): void => {
    const ip = ipInput.trim();
    if (ip && !allowIps.includes(ip)) {
      setAllowIps([...allowIps, ip]);
    }
    setIpInput('');
  };

  // Remove IP from list
  const removeIp = (ip: string): void => {
    setAllowIps(allowIps.filter(i => i !== ip));
  };

  // Check if quota amount is valid when custom quota type
  const isQuotaValid =
    quotaType !== ApiKeyQuotaType.Custom ||
    (quotaAmount.trim() !== '' &&
      !isNaN(parseInt(quotaAmount, 10)) &&
      parseInt(quotaAmount, 10) >= 0 &&
      parseInt(quotaAmount, 10) <= DEFAULT_AI_CREDIT_EDIT_MAX);

  const onSubmit = async (): Promise<void> => {
    // Validate quota amount when custom quota type
    if (quotaType === ApiKeyQuotaType.Custom) {
      const amount = parseInt(quotaAmount, 10);
      if (quotaAmount.trim() === '' || isNaN(amount) || amount < 0) {
        toast.error(t('dialog.errors.quotaAmountRequired'));
        return;
      }
      if (amount > DEFAULT_AI_CREDIT_EDIT_MAX) {
        toast.error(t('dialog.errors.quotaAmountMax', { max: quotaAmountMaxLabel }));
        return;
      }
    }

    // Validate expiration time is in the future
    if (expiresAt) {
      const expiresDate = new Date(expiresAt);
      if (expiresDate <= new Date()) {
        toast.error(t('dialog.errors.expiresInPast'));
        return;
      }
    }

    // Convert allowIps array to comma-separated string
    const allowIpsStr = allowIps.length > 0 ? allowIps.join(',') : undefined;

    if (mode === 'create') {
      const payload: CreateApiKeyRequest = {
        name,
        count: 1,
        quota_type: quotaType,
        quota_amount: quotaType === ApiKeyQuotaType.Custom ? parseInt(quotaAmount, 10) : undefined,
        allow_all_models: allowAllModels,
        model_names: allowAllModels || modelNames.length === 0 ? undefined : modelNames,
        allow_ips: allowIpsStr,
        expires_at: expiresAt || undefined,
      };
      const res = await createApiKey(payload);
      if (res && res.keys && res.keys.length > 0) {
        setSecretKey(res.keys[0].key);
      } else {
        onOpenChange(false);
      }
      return;
    }

    // Edit mode
    if (!initial) return;
    const update: UpdateApiKeyRequest = {
      name: name !== initial.name ? name : undefined,
      model_limits_enabled: !allowAllModels,
      allow_ips: allowIpsStr,
      model_limits: !allowAllModels && modelNames.length > 0 ? modelNames : undefined,
      quota_limit:
        quotaType === ApiKeyQuotaType.Custom && quotaAmount.trim() !== ''
          ? parseInt(quotaAmount, 10)
          : undefined,
      expires_at: expiresAt || undefined,
    };
    await updateApiKey(initial.id, update);
    onOpenChange(false);
  };

  const disabled = isCreating || isUpdating;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className={cn(
          'p-0 transition-all duration-300',
          secretKey
            ? 'w-[400px] max-w-[400px] h-auto'
            : allowAllModels
              ? 'w-[400px] max-w-[400px] h-[50vh]'
              : 'w-[80vw] max-w-[80vw] h-[80vh]'
        )}
      >
        {secretKey ? (
          <div className="flex flex-col h-full">
            <DialogHeader className="pb-2">
              <DialogTitle className="text-xl font-bold tracking-tight">
                {t('dialog.createdTitle')}
              </DialogTitle>
              <DialogDescription className="text-sm text-muted-foreground font-medium">
                {t('dialog.createdNotice')}
              </DialogDescription>
            </DialogHeader>
            <DialogBody className="py-6 space-y-6">
              <div className="flex items-center gap-3 rounded-lg border bg-muted/40 p-4 shadow-sm">
                <div className="font-mono break-all text-sm flex-1 leading-relaxed text-foreground">
                  {secretKey}
                </div>
                <Button
                  variant="ghost"
                  isIcon
                  onClick={() => {
                    navigator.clipboard.writeText(secretKey);
                    toast.success(tCommon('toasts.copySuccess'));
                  }}
                  className="h-9 w-9 text-muted-foreground hover:text-foreground"
                >
                  <Copy className="h-4 w-4" />
                </Button>
              </div>
            </DialogBody>
            <div className="bg-muted/50 pb-6 px-6 pt-4 border-t shrink-0 flex items-center justify-end gap-2">
              <Button
                variant="ghost"
                onClick={() => {
                  setSecretKey(null);
                  onOpenChange(false);
                }}
                className="font-semibold"
              >
                {tCommon('close')}
              </Button>
              <Button
                onClick={() => {
                  navigator.clipboard.writeText(secretKey);
                  toast.success(tCommon('toasts.copySuccess'));
                }}
                className="font-bold"
              >
                <Copy className="h-4 w-4 mr-2" /> {tCommon('copy')}
              </Button>
            </div>
          </div>
        ) : (
          <div className="h-full overflow-hidden flex flex-col gap-2">
            <DialogHeader className="px-6 pt-6">
              <DialogTitle>
                {mode === 'create' ? t('dialog.titleCreate') : t('dialog.titleEdit')}
              </DialogTitle>
              <DialogDescription>{t('description')}</DialogDescription>
            </DialogHeader>

            <DialogBody className="flex gap-6 h-0 grow overflow-x-hidden">
              {/* Left column: Basic settings */}
              <div
                className={cn(
                  'space-y-4 w-[352px] shrink-0 transition-all duration-300',
                  allowAllModels ? '' : 'pr-3 overflow-y-auto shrink-0'
                )}
              >
                <div className="text-sm font-semibold text-foreground">{t('dialog.basic')}</div>

                {/* Name field */}
                <div className="space-y-2">
                  <Label htmlFor="name">
                    {t('dialog.labels.name')} <span className="text-red-500">*</span>
                  </Label>
                  <Input
                    id="name"
                    value={name}
                    onChange={e => setName(e.target.value)}
                    placeholder={t('dialog.placeholders.name')}
                    maxLength={30}
                  />
                </div>

                {/* Quota type */}
                <div className="space-y-2">
                  <Label htmlFor="quotaType">{t('dialog.labels.quotaType')}</Label>
                  <Select value={quotaType} onValueChange={v => setQuotaType(v as ApiKeyQuotaType)}>
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
                      <span className="text-destructive ml-0.5">*</span>
                    </Label>
                    <Input
                      id="quotaAmount"
                      type="number"
                      min={0}
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

                {/* Allow all models toggle */}
                <div className="flex items-center justify-between py-2">
                  <Label htmlFor="allowAllModels">{t('dialog.labels.allowAllModels')}</Label>
                  <Switch
                    id="allowAllModels"
                    checked={allowAllModels}
                    onCheckedChange={v => setAllowAllModels(Boolean(v))}
                  />
                </div>

                {/* Advanced section */}
                <div
                  className="flex items-center justify-between cursor-pointer py-2"
                  onClick={() => setAdvancedOpen(v => !v)}
                >
                  <div className="text-sm font-semibold text-foreground">
                    {t('dialog.advanced')}
                  </div>
                  <ChevronDown
                    className={`h-4 w-4 transition-transform ${advancedOpen ? '' : 'rotate-90'}`}
                  />
                </div>

                <div
                  className={cn(
                    'space-y-4 transition-all duration-300 overflow-hidden',
                    advancedOpen ? 'h-auto opacity-100 pb-3' : 'h-0 opacity-0'
                  )}
                >
                  <div className="space-y-2">
                    <Label>{t('dialog.labels.allowIps')}</Label>
                    {allowIps.length > 0 && (
                      <div className="flex flex-wrap gap-2 mb-2 max-h-20 overflow-y-auto">
                        {allowIps.map(ip => (
                          <Badge key={ip} variant="secondary" className="gap-1 pr-1">
                            {ip}
                            <button
                              type="button"
                              className="ml-1 hover:bg-muted rounded-full p-0.5"
                              onClick={() => removeIp(ip)}
                            >
                              <X className="h-3 w-3" />
                            </button>
                          </Badge>
                        ))}
                      </div>
                    )}
                    <div className="flex gap-2">
                      <Input
                        id="allowIps"
                        value={ipInput}
                        onChange={e => setIpInput(e.target.value)}
                        onKeyDown={e => {
                          if (e.key === 'Enter') {
                            e.preventDefault();
                            const ip = ipInput.trim();
                            if (!ip) return;
                            const isValidIp =
                              /^(?:(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])\.){3}(?:25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9]?[0-9])$/.test(
                                ip
                              );
                            if (!isValidIp) {
                              toast.error(t('dialog.errors.invalidIp'));
                              return;
                            }
                            if (allowIps.includes(ip)) {
                              toast.error(t('dialog.errors.duplicateIp'));
                              return;
                            }
                            addIp();
                          }
                        }}
                        placeholder={t('dialog.placeholders.allowIps')}
                        className="flex-1"
                      />
                      <Button
                        type="button"
                        variant="outline"
                        isIcon
                        onClick={addIp}
                        disabled={!ipInput.trim()}
                      >
                        <Plus className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="expiresAt">{t('dialog.labels.expiresAt')}</Label>
                    <Input
                      id="expiresAt"
                      type="datetime-local"
                      value={expiresAt}
                      min={getMinDateTime()}
                      onChange={e => setExpiresAt(e.target.value)}
                    />
                    <p className="text-xs text-muted-foreground">{t('dialog.hints.expiresAt')}</p>
                  </div>
                </div>
              </div>

              {/* Right column: Model selector (expandable) */}
              <div
                className={cn(
                  'overflow-hidden flex flex-col transition-all duration-300',
                  allowAllModels ? 'w-0 opacity-0' : 'flex-1 opacity-100 pl-3'
                )}
              >
                <div className="text-sm font-semibold text-foreground mb-2">
                  {t('dialog.labels.modelNames')}
                </div>
                <ModelMultiSelector
                  value={modelNames}
                  onChange={setModelNames}
                  isEnabled
                  columns={2}
                  className="h-0 grow overflow-hidden"
                />
              </div>
            </DialogBody>

            <div className="pb-6 px-6 pt-4 border-t shrink-0 flex items-center justify-end gap-2">
              <Button variant="outline" onClick={() => onOpenChange(false)} disabled={disabled}>
                {t('dialog.buttons.cancel')}
              </Button>
              <Button
                onClick={onSubmit}
                disabled={disabled || !isQuotaValid || (mode === 'create' && !name.trim())}
              >
                {mode === 'create' ? t('dialog.buttons.create') : t('dialog.buttons.save')}
              </Button>
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
