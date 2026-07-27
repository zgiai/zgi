'use client';

import { useEffect, useMemo, useState } from 'react';
import {
  CheckCircle2,
  Copy,
  ExternalLink,
  KeyRound,
  Loader2,
  RotateCw,
  ShieldCheck,
  Trash2,
  Users,
} from 'lucide-react';
import { toast } from 'sonner';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Switch } from '@/components/ui/switch';
import { Textarea } from '@/components/ui/textarea';
import {
  useDeleteIntegrationOAuthClientConfig,
  useIntegrationOAuthClientConfig,
  useIntegrationOAuthClientConfigImpact,
  useUpdateIntegrationOAuthClientConfig,
} from '@/hooks';
import { useT } from '@/i18n';
import type { IntegrationAuthDefinition } from '@/services/types/integration';
import { AuthSetupGuide } from './auth-setup-guide';
import { useIntegrationMetadata } from './metadata-i18n';
import { resolveOAuthClientFields } from './oauth-client-fields';

interface IntegrationOAuthClientConfigDialogProps {
  open: boolean;
  integrationId: string;
  providerName: string;
  auth: IntegrationAuthDefinition | null;
  relatedAuthMethods?: IntegrationAuthDefinition[];
  onConfigured?: () => void | Promise<void>;
  onOpenChange: (open: boolean) => void;
}

type ClientFieldValue = string | boolean;
type EditMode = 'client_id' | 'client_secret' | 'additional' | null;

export function IntegrationOAuthClientConfigDialog({
  open,
  integrationId,
  providerName,
  auth,
  relatedAuthMethods = [],
  onConfigured,
  onOpenChange,
}: IntegrationOAuthClientConfigDialogProps) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const authMethodId = auth?.id ?? '';
  const clientConfigId = auth?.oauth?.client_config_id ?? authMethodId;
  const configQuery = useIntegrationOAuthClientConfig(
    integrationId,
    authMethodId,
    open,
    clientConfigId
  );
  const config = configQuery.data?.data;
  const impactQuery = useIntegrationOAuthClientConfigImpact(
    integrationId,
    authMethodId,
    open && Boolean(config?.configured),
    clientConfigId
  );
  const updateMutation = useUpdateIntegrationOAuthClientConfig(
    integrationId,
    authMethodId,
    clientConfigId
  );
  const deleteMutation = useDeleteIntegrationOAuthClientConfig(
    integrationId,
    authMethodId,
    clientConfigId
  );
  const [values, setValues] = useState<Record<string, ClientFieldValue>>({});
  const [invalidFieldNames, setInvalidFieldNames] = useState<string[]>([]);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [editMode, setEditMode] = useState<EditMode>(null);
  const fields = useMemo(() => resolveOAuthClientFields(auth), [auth]);

  useEffect(() => {
    if (!open) return;
    const next: Record<string, ClientFieldValue> = {};
    for (const field of fields) {
      if (field.secret) {
        next[field.name] = '';
        continue;
      }
      const saved = config?.config?.[field.name];
      next[field.name] =
        typeof saved === 'string' || typeof saved === 'boolean'
          ? saved
          : (field.default_value ?? '');
    }
    setValues(next);
    setInvalidFieldNames([]);
    setEditMode(null);
  }, [config?.config, fields, open]);

  const clearFieldError = (fieldName: string) => {
    setInvalidFieldNames(current => current.filter(name => name !== fieldName));
  };

  const clearSecrets = () => {
    setValues(current => ({
      ...current,
      ...Object.fromEntries(fields.filter(field => field.secret).map(field => [field.name, ''])),
    }));
  };

  const handleOpenChange = (nextOpen: boolean) => {
    if (!nextOpen) {
      clearSecrets();
      setEditMode(null);
    }
    onOpenChange(nextOpen);
  };

  const save = async () => {
    if (!config) return;
    const hasOrganizationConfig = config?.source === 'organization' && Boolean(config.configured);
    const missingFields = fields.filter(field => {
      if (!field.required) return false;
      if (field.secret && hasOrganizationConfig && config?.has_client_secret) return false;
      if (field.name === 'client_id' && hasOrganizationConfig) return false;
      const value = values[field.name];
      return typeof value === 'boolean' ? false : !value?.trim();
    });
    if (missingFields.length > 0) {
      const names = missingFields.map(field => field.name);
      setInvalidFieldNames(names);
      requestAnimationFrame(() => document.getElementById(`oauth-client-${names[0]}`)?.focus());
      return;
    }

    const clientIdValue = values.client_id;
    const clientSecretValue = values.client_secret;
    const requestConfig: Record<string, unknown> = {};
    for (const field of fields) {
      if (field.name === 'client_id' || field.name === 'client_secret') continue;
      const value = values[field.name];
      if (typeof value === 'boolean') requestConfig[field.name] = value;
      else if (value?.trim()) requestConfig[field.name] = value.trim();
    }

    try {
      await updateMutation.mutateAsync({
        revision: config?.revision,
        client_id:
          typeof clientIdValue === 'string' && clientIdValue.trim()
            ? clientIdValue.trim()
            : undefined,
        client_secret:
          typeof clientSecretValue === 'string' && clientSecretValue.trim()
            ? clientSecretValue.trim()
            : undefined,
        config: requestConfig,
      });
      clearSecrets();
      setEditMode(null);
      await onConfigured?.();
      if (onConfigured) onOpenChange(false);
    } catch {
      // The mutation owns the localized error toast. Keep secret values out of
      // rejected promises and component state.
    } finally {
      clearSecrets();
    }
  };

  const configured = Boolean(config?.configured);
  const impact = impactQuery.data?.data;
  const clientIDField = fields.find(field => field.name === 'client_id');
  const clientSecretField = fields.find(field => field.name === 'client_secret');
  const additionalFields = fields.filter(
    field => field.name !== 'client_id' && field.name !== 'client_secret'
  );
  const editableFields = !configured
    ? fields
    : editMode === 'client_id'
      ? clientIDField
        ? [clientIDField]
        : []
      : editMode === 'client_secret'
        ? clientSecretField
          ? [clientSecretField]
          : []
        : editMode === 'additional'
          ? additionalFields
          : [];
  const updatedAt = config?.updated_at
    ? new Intl.DateTimeFormat(undefined, {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
      }).format(new Date(config.updated_at))
    : null;

  const renderEditableFields = () =>
    editableFields.map(field => {
      const id = `oauth-client-${field.name}`;
      const value = values[field.name];
      const label =
        metadata.credentialFieldLabel(integrationId, field) ??
        (field.name === 'client_secret'
          ? t('oauth.clientConfig.clientSecret')
          : t('oauth.clientConfig.clientID'));
      const description =
        metadata.credentialFieldDescription(integrationId, field) ??
        (field.name === 'client_secret'
          ? t('oauth.clientConfig.clientSecretDescription')
          : field.name === 'client_id'
            ? t('oauth.clientConfig.clientIDDescription')
            : null);
      const placeholder =
        metadata.credentialFieldPlaceholder(integrationId, field) ??
        (field.name === 'client_secret'
          ? t('oauth.clientConfig.clientSecretPlaceholder')
          : field.name === 'client_id'
            ? t('oauth.clientConfig.clientIDPlaceholder')
            : null);
      const invalid = invalidFieldNames.includes(field.name);
      const keepsExistingValue =
        config?.source === 'organization' &&
        configured &&
        ((field.secret && Boolean(config.has_client_secret)) || field.name === 'client_id');
      const requiredForInput = field.required && !keepsExistingValue;
      const descriptionId = description ? `${id}-description` : undefined;
      const errorId = invalid ? 'oauth-client-validation-error' : undefined;
      const describedBy = [descriptionId, errorId].filter(Boolean).join(' ') || undefined;

      if (field.type === 'boolean') {
        return (
          <div
            key={field.name}
            className="flex items-start justify-between gap-4 rounded-lg border p-3"
          >
            <div>
              <Label htmlFor={id}>{label}</Label>
              {description ? (
                <p id={descriptionId} className="mt-1 text-xs text-muted-foreground">
                  {description}
                </p>
              ) : null}
            </div>
            <Switch
              id={id}
              aria-required={requiredForInput}
              aria-invalid={invalid}
              aria-describedby={describedBy}
              checked={Boolean(value)}
              onCheckedChange={checked => {
                setValues(current => ({ ...current, [field.name]: checked }));
                clearFieldError(field.name);
              }}
            />
          </div>
        );
      }

      return (
        <div key={field.name} className="space-y-2">
          <Label htmlFor={id}>
            {label}
            {requiredForInput ? ' *' : ''}
          </Label>
          {field.type === 'select' ? (
            <Select
              value={typeof value === 'string' ? value : ''}
              onValueChange={next => {
                setValues(current => ({ ...current, [field.name]: next }));
                clearFieldError(field.name);
              }}
            >
              <SelectTrigger
                id={id}
                aria-required={requiredForInput}
                aria-invalid={invalid}
                aria-describedby={describedBy}
              >
                <SelectValue placeholder={placeholder ?? undefined} />
              </SelectTrigger>
              <SelectContent>
                {(field.options ?? []).map(option => (
                  <SelectItem key={option.value} value={option.value}>
                    {metadata.optionLabel(option)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : field.type === 'textarea' ? (
            <Textarea
              id={id}
              aria-required={requiredForInput}
              aria-invalid={invalid}
              aria-describedby={describedBy}
              value={typeof value === 'string' ? value : ''}
              placeholder={placeholder ?? undefined}
              onChange={event => {
                setValues(current => ({ ...current, [field.name]: event.target.value }));
                clearFieldError(field.name);
              }}
            />
          ) : (
            <Input
              id={id}
              type={field.secret ? 'password' : 'text'}
              required={requiredForInput}
              aria-invalid={invalid}
              aria-describedby={describedBy}
              autoComplete={field.secret ? 'new-password' : 'off'}
              value={typeof value === 'string' ? value : ''}
              placeholder={
                field.secret && config?.source === 'organization' && config?.has_client_secret
                  ? t('oauth.clientConfig.keepExistingSecret')
                  : (placeholder ?? undefined)
              }
              onChange={event => {
                setValues(current => ({ ...current, [field.name]: event.target.value }));
                clearFieldError(field.name);
              }}
            />
          )}
          {description ? (
            <p id={descriptionId} className="text-xs text-muted-foreground">
              {description}
            </p>
          ) : null}
        </div>
      );
    });

  return (
    <>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent className="p-0">
          <DialogHeader>
            <DialogTitle>{t('oauth.clientConfig.title', { provider: providerName })}</DialogTitle>
            <DialogDescription>
              {t('oauth.clientConfig.description', { provider: providerName })}
            </DialogDescription>
          </DialogHeader>

          <DialogBody className="space-y-5 pb-5">
            {configQuery.isLoading ? (
              <div className="flex min-h-48 items-center justify-center">
                <Loader2 className="size-6 animate-spin text-primary" />
              </div>
            ) : configQuery.isError ? (
              <div className="rounded-lg border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
                <p>{t('oauth.clientConfig.loadFailed')}</p>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  className="mt-3"
                  onClick={() => void configQuery.refetch()}
                >
                  {t('oauth.clientConfig.retry')}
                </Button>
              </div>
            ) : configured && !editMode ? (
              <>
                <div className="flex flex-wrap items-center gap-2 text-sm">
                  <span className="inline-flex items-center gap-1.5 rounded-full bg-success/10 px-2.5 py-1 font-medium text-success">
                    <CheckCircle2 className="size-3.5" />
                    {t('oauth.clientConfig.configured')}
                  </span>
                  <span className="text-muted-foreground">
                    {t('oauth.clientConfig.sharedByMethods', {
                      count: relatedAuthMethods.length || 1,
                    })}
                  </span>
                  {impact ? (
                    <span className="text-muted-foreground">
                      ·{' '}
                      {t('oauth.clientConfig.connectedAccounts', {
                        count: impact.dependent_connections,
                      })}
                    </span>
                  ) : null}
                </div>

                <AuthSetupGuide
                  key={`${integrationId}:${authMethodId}:configured`}
                  providerName={providerName}
                  guide={auth?.setup_guide}
                  callbackURL={config?.callback_url}
                />

                <div className="overflow-hidden rounded-xl border">
                  {clientIDField ? (
                    <div className="grid gap-2 border-b px-4 py-3 sm:grid-cols-[9rem_minmax(0,1fr)_auto] sm:items-center">
                      <span className="text-sm font-medium">
                        {t('oauth.clientConfig.clientID')}
                      </span>
                      <span className="font-mono text-xs text-muted-foreground">
                        {config?.client_id_masked ?? t('oauth.clientConfig.valueUnavailable')}
                      </span>
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        onClick={() => setEditMode('client_id')}
                      >
                        {t('oauth.clientConfig.changeClientID')}
                      </Button>
                    </div>
                  ) : null}
                  {clientSecretField ? (
                    <div className="grid gap-2 border-b px-4 py-3 sm:grid-cols-[9rem_minmax(0,1fr)_auto] sm:items-center">
                      <span className="text-sm font-medium">
                        {t('oauth.clientConfig.clientSecret')}
                      </span>
                      <span className="inline-flex items-center gap-1.5 text-sm text-success">
                        <ShieldCheck className="size-4" />
                        {config?.has_client_secret
                          ? t('oauth.clientConfig.secretStored')
                          : t('oauth.clientConfig.secretNotRequired')}
                      </span>
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        onClick={() => setEditMode('client_secret')}
                      >
                        <RotateCw className="size-4" />
                        {t('oauth.clientConfig.rotateSecret')}
                      </Button>
                    </div>
                  ) : null}
                  <div className="grid gap-2 border-b px-4 py-3 sm:grid-cols-[9rem_minmax(0,1fr)_auto] sm:items-center">
                    <span className="text-sm font-medium">
                      {t('oauth.clientConfig.callbackURL')}
                    </span>
                    <span className="min-w-0 truncate font-mono text-xs text-muted-foreground">
                      {config?.callback_url}
                    </span>
                    <Button
                      type="button"
                      size="sm"
                      isIcon
                      variant="ghost"
                      disabled={!config?.callback_url}
                      aria-label={t('oauth.clientConfig.copyCallbackURL')}
                      onClick={() => {
                        if (!config?.callback_url) return;
                        void navigator.clipboard
                          .writeText(config.callback_url)
                          .then(() => toast.success(t('oauth.clientConfig.callbackCopied')))
                          .catch(() => undefined);
                      }}
                    >
                      <Copy className="size-4" />
                    </Button>
                  </div>
                  <div className="grid gap-2 px-4 py-3 sm:grid-cols-[9rem_minmax(0,1fr)_auto] sm:items-center">
                    <span className="text-sm font-medium">
                      {t('oauth.clientConfig.supportedMethods')}
                    </span>
                    <div className="flex flex-wrap gap-1.5">
                      {(relatedAuthMethods.length > 0
                        ? relatedAuthMethods
                        : auth
                          ? [auth]
                          : []
                      ).map(method => (
                        <span
                          key={method.id}
                          className="rounded-full border bg-muted/30 px-2.5 py-1 text-xs"
                        >
                          {metadata.authMethodLabel(integrationId, method)}
                        </span>
                      ))}
                    </div>
                    {additionalFields.length > 0 ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        onClick={() => setEditMode('additional')}
                      >
                        {t('oauth.clientConfig.editAdditional')}
                      </Button>
                    ) : null}
                  </div>
                </div>

                <div className="flex flex-wrap items-center justify-between gap-3 text-xs text-muted-foreground">
                  <span>
                    {updatedAt
                      ? t('oauth.clientConfig.updatedAt', { time: updatedAt })
                      : t('oauth.clientConfig.writeOnlyNotice')}
                  </span>
                  {!auth?.setup_guide && config?.provider_setup_url ? (
                    <Button asChild variant="link" size="sm" className="h-auto px-0">
                      <a href={config.provider_setup_url} target="_blank" rel="noreferrer noopener">
                        <ExternalLink className="size-4" />
                        {t('oauth.clientConfig.openProviderConsole', { provider: providerName })}
                      </a>
                    </Button>
                  ) : null}
                </div>

                {config?.source === 'organization' ? (
                  <div className="rounded-xl border border-destructive/25 bg-destructive/[0.03] p-4">
                    <div className="flex items-start gap-3">
                      <Trash2 className="mt-0.5 size-4 shrink-0 text-destructive" />
                      <div className="min-w-0 flex-1">
                        <p className="text-sm font-medium text-destructive">
                          {t('oauth.clientConfig.dangerTitle')}
                        </p>
                        <p className="mt-1 text-xs leading-5 text-muted-foreground">
                          {impactQuery.isLoading
                            ? t('oauth.clientConfig.impactLoading')
                            : impact
                              ? t('oauth.clientConfig.impactSummary', {
                                  accounts: impact.dependent_connections,
                                  flows: impact.pending_flows,
                                })
                              : t('oauth.clientConfig.impactUnavailable')}
                        </p>
                        {impact && !impact.can_remove ? (
                          <p className="mt-2 inline-flex items-center gap-1.5 text-xs font-medium text-warning">
                            <Users className="size-3.5" />
                            {t('oauth.clientConfig.removeBlocked')}
                          </p>
                        ) : null}
                      </div>
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        className="shrink-0 border-destructive/40 text-destructive hover:bg-destructive/5 hover:text-destructive"
                        disabled={impactQuery.isLoading || !impact?.can_remove}
                        onClick={() => setDeleteOpen(true)}
                      >
                        {t('oauth.clientConfig.remove')}
                      </Button>
                    </div>
                  </div>
                ) : null}
              </>
            ) : (
              <>
                <Alert>
                  <ShieldCheck className="size-4" />
                  <AlertDescription>{t('oauth.clientConfig.adminOnlyNotice')}</AlertDescription>
                </Alert>
                {!configured && auth ? (
                  <div className="rounded-xl border border-primary/20 bg-primary/5 p-4">
                    <p className="text-sm font-medium">{t('oauth.clientConfig.sharedTitle')}</p>
                    <p className="mt-1 text-xs leading-5 text-muted-foreground">
                      {t('oauth.clientConfig.sharedDescription', { provider: providerName })}
                    </p>
                  </div>
                ) : null}
                {!configured ? (
                  <AuthSetupGuide
                    key={`${integrationId}:${authMethodId}:setup`}
                    providerName={providerName}
                    guide={auth?.setup_guide}
                    callbackURL={config?.callback_url}
                  />
                ) : null}
                {configured && editMode ? (
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <p className="text-sm font-medium">
                        {editMode === 'client_secret'
                          ? t('oauth.clientConfig.rotateSecret')
                          : editMode === 'client_id'
                            ? t('oauth.clientConfig.changeClientID')
                            : t('oauth.clientConfig.editAdditional')}
                      </p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {t('oauth.clientConfig.editDescription')}
                      </p>
                    </div>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={() => setEditMode(null)}
                    >
                      {t('oauth.clientConfig.backToOverview')}
                    </Button>
                  </div>
                ) : null}
                {!configured ? (
                  <div className="rounded-lg border bg-muted/20 p-3">
                    <div className="flex items-center gap-2 text-sm font-medium">
                      <KeyRound className="size-4" />
                      {t('oauth.clientConfig.callbackURL')}
                    </div>
                    <p className="mt-1 text-xs leading-5 text-muted-foreground">
                      {t('oauth.clientConfig.callbackURLDescription')}
                    </p>
                    <div className="mt-3 flex gap-2">
                      <Input
                        value={config?.callback_url ?? ''}
                        readOnly
                        className="font-mono text-xs"
                      />
                      <Button
                        type="button"
                        size="sm"
                        isIcon
                        variant="outline"
                        disabled={!config?.callback_url}
                        aria-label={t('oauth.clientConfig.copyCallbackURL')}
                        onClick={() => {
                          if (!config?.callback_url) return;
                          void navigator.clipboard
                            .writeText(config.callback_url)
                            .then(() => toast.success(t('oauth.clientConfig.callbackCopied')))
                            .catch(() => undefined);
                        }}
                      >
                        <Copy className="size-4" />
                      </Button>
                    </div>
                  </div>
                ) : null}
                {renderEditableFields()}
                {invalidFieldNames.length > 0 ? (
                  <p
                    id="oauth-client-validation-error"
                    role="alert"
                    className="text-sm text-destructive"
                  >
                    {t('oauth.clientConfig.required')}
                  </p>
                ) : null}
              </>
            )}
          </DialogBody>

          <DialogFooter className="border-t bg-muted/30">
            <Button
              variant="ghost"
              onClick={() => (editMode ? setEditMode(null) : handleOpenChange(false))}
              disabled={updateMutation.isPending}
            >
              {editMode ? t('oauth.clientConfig.cancelEdit') : t('oauth.clientConfig.close')}
            </Button>
            {!configured || editMode ? (
              <Button
                onClick={() => void save()}
                disabled={
                  configQuery.isLoading ||
                  configQuery.isError ||
                  !config ||
                  updateMutation.isPending
                }
              >
                {updateMutation.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
                {onConfigured && !configured
                  ? t('oauth.clientConfig.saveAndContinue')
                  : t('oauth.clientConfig.saveChanges')}
              </Button>
            ) : null}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        variant="danger"
        title={t('oauth.clientConfig.removeTitle')}
        description={t('oauth.clientConfig.removeDescription', { provider: providerName })}
        cancelText={t('oauth.clientConfig.cancel')}
        confirmText={t('oauth.clientConfig.remove')}
        loading={deleteMutation.isPending}
        onConfirm={() => {
          void deleteMutation
            .mutateAsync()
            .then(() => {
              setDeleteOpen(false);
              onOpenChange(false);
            })
            .catch(() => undefined);
        }}
      />
    </>
  );
}
