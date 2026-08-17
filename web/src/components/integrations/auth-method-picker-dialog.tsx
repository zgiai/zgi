'use client';

import { useEffect, useMemo, useState } from 'react';
import {
  AppWindow,
  ArrowRight,
  Building2,
  Check,
  RadioTower,
  ServerCog,
  UserRound,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useT } from '@/i18n';
import type {
  IntegrationActionDefinition,
  IntegrationAuthDefinition,
} from '@/services/types/integration';
import { cn } from '@/lib/utils';
import { actionsForAuthMethod } from './action-auth-compatibility';
import {
  authMethodCanStart,
  authMethodsSharingOAuthClient,
  isOAuthAuthMethod,
  resolveAuthMethodPresentation,
} from './auth-method-selection';
import { useIntegrationMetadata } from './metadata-i18n';

interface IntegrationAuthMethodPickerDialogProps {
  open: boolean;
  integrationId: string;
  providerName: string;
  methods: IntegrationAuthDefinition[];
  actions: IntegrationActionDefinition[];
  recommendedAuthMethodId?: string;
  canConfigureOAuthClient: boolean;
  onOpenChange: (open: boolean) => void;
  onSelect: (auth: IntegrationAuthDefinition) => void;
}

function identityIcon(identity: ReturnType<typeof resolveAuthMethodPresentation>['identityKind']) {
  if (identity === 'user') return <UserRound className="size-5" />;
  if (identity === 'channel') return <RadioTower className="size-5" />;
  if (identity === 'service') return <ServerCog className="size-5" />;
  return <AppWindow className="size-5" />;
}

export function IntegrationAuthMethodPickerDialog({
  open,
  integrationId,
  providerName,
  methods,
  actions,
  recommendedAuthMethodId,
  canConfigureOAuthClient,
  onOpenChange,
  onSelect,
}: IntegrationAuthMethodPickerDialogProps) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const [selectedId, setSelectedId] = useState('');
  const selected = useMemo(
    () => methods.find(method => method.id === selectedId),
    [methods, selectedId]
  );
  const selectedPresentation = selected ? resolveAuthMethodPresentation(selected) : null;
  const selectedSharedOAuthMethods = useMemo(
    () => authMethodsSharingOAuthClient(methods, selected),
    [methods, selected]
  );
  const selectedNeedsOAuthClient =
    Boolean(selected && isOAuthAuthMethod(selected)) && !selected?.oauth?.client_configured;

  useEffect(() => {
    if (!open) return;
    const recommended = methods.find(method => method.id === recommendedAuthMethodId);
    const firstAvailable = methods.find(method =>
      authMethodCanStart(method, canConfigureOAuthClient)
    );
    setSelectedId(recommended?.id ?? firstAvailable?.id ?? methods[0]?.id ?? '');
  }, [canConfigureOAuthClient, methods, open, recommendedAuthMethodId]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="md" className="p-0">
        <DialogHeader>
          <DialogTitle>
            {t('authMethodPicker.title', {
              provider: providerName,
            })}
          </DialogTitle>
          <DialogDescription>{t('authMethodPicker.description')}</DialogDescription>
        </DialogHeader>

        <DialogBody className="space-y-3">
          {selectedNeedsOAuthClient && selectedSharedOAuthMethods.length > 0 ? (
            <div className="rounded-xl border border-primary/20 bg-primary/5 p-4">
              <p className="text-sm font-medium">{t('authMethodPicker.sharedOAuth.title')}</p>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {t('authMethodPicker.sharedOAuth.description')}
              </p>
              <div className="mt-3 flex flex-wrap gap-1.5">
                {selectedSharedOAuthMethods.map(method => (
                  <Badge key={method.id} variant={method.id === selectedId ? 'default' : 'outline'}>
                    {metadata.authMethodLabel(integrationId, method)}
                  </Badge>
                ))}
              </div>
            </div>
          ) : null}

          <div
            role="radiogroup"
            aria-label={t('authMethodPicker.listLabel')}
            className="grid grid-cols-1 gap-3"
          >
            {methods.map(method => {
              const presentation = resolveAuthMethodPresentation(method);
              const canStart = authMethodCanStart(method, canConfigureOAuthClient);
              const recommended = method.id === recommendedAuthMethodId;
              const methodDescription = metadata.authMethodDescription(integrationId, method);
              const supportedActionCount = actionsForAuthMethod(actions, method.id).length;
              const sourceLabel = t(
                `authMethodPicker.credentialSource.${presentation.credentialSource}`
              );
              const identityLabel = t(`authMethodPicker.identity.${presentation.identityKind}`);
              const unavailableReason = !canStart
                ? presentation.acquisitionStrategy === 'browser_redirect' &&
                  !method.oauth?.client_configured
                  ? t(
                      canConfigureOAuthClient
                        ? 'authMethodPicker.configureOAuth'
                        : 'authMethodPicker.adminSetupRequired'
                    )
                  : t('authMethodPicker.unavailable')
                : null;
              const resultKey =
                presentation.credentialSource === 'account'
                  ? 'personal'
                  : presentation.identityKind === 'user'
                    ? 'organizationUser'
                    : 'organizationApplication';

              return (
                <button
                  key={method.id}
                  type="button"
                  role="radio"
                  aria-checked={selectedId === method.id}
                  aria-label={metadata.authMethodLabel(integrationId, method)}
                  disabled={!canStart}
                  onClick={() => setSelectedId(method.id)}
                  className={cn(
                    'min-h-28 rounded-xl border p-4 text-left transition-colors',
                    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2',
                    selectedId === method.id
                      ? 'border-primary bg-primary/5'
                      : 'hover:border-primary/40 hover:bg-muted/30',
                    !canStart && 'cursor-not-allowed bg-muted/20 opacity-60'
                  )}
                >
                  <div className="flex items-start gap-3">
                    <span
                      aria-hidden="true"
                      className={cn(
                        'mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg border',
                        selectedId === method.id
                          ? 'border-primary/30 bg-primary/10 text-primary'
                          : 'bg-background text-muted-foreground'
                      )}
                    >
                      {identityIcon(presentation.identityKind)}
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="flex flex-wrap items-center gap-2">
                        <span className="font-medium">
                          {metadata.authMethodLabel(integrationId, method)}
                        </span>
                        {recommended ? (
                          <Badge variant="subtle" className="gap-1">
                            <Check className="size-3" />
                            {t('authMethodPicker.recommended')}
                          </Badge>
                        ) : null}
                      </span>
                      {methodDescription ? (
                        <span className="mt-1 block text-xs leading-5 text-muted-foreground">
                          {methodDescription}
                        </span>
                      ) : null}
                      <span className="mt-2 block text-xs leading-5 text-foreground">
                        {t(`authMethodPicker.result.${resultKey}`)}
                      </span>
                      {unavailableReason ? (
                        <span className="mt-1 block text-xs leading-5 text-warning">
                          {unavailableReason}
                        </span>
                      ) : null}
                    </span>
                    <span
                      aria-hidden="true"
                      className={cn(
                        'mt-1 flex size-4 shrink-0 items-center justify-center rounded-full border',
                        selectedId === method.id &&
                          'border-primary bg-primary text-primary-foreground'
                      )}
                    >
                      {selectedId === method.id ? <Check className="size-3" /> : null}
                    </span>
                  </div>
                  <div className="mt-3 flex flex-wrap gap-1.5 pl-12">
                    <Badge variant="outline">{identityLabel}</Badge>
                    <Badge variant="outline" className="gap-1">
                      {presentation.credentialSource === 'account' ? (
                        <UserRound className="size-3" />
                      ) : (
                        <Building2 className="size-3" />
                      )}
                      {sourceLabel}
                    </Badge>
                    <Badge variant="outline">
                      {t('authMethodPicker.actionCount', { count: supportedActionCount })}
                    </Badge>
                  </div>
                </button>
              );
            })}
          </div>

          {selected && selectedPresentation ? (
            <div className="rounded-lg border bg-muted/20 px-3 py-2.5 text-xs leading-5">
              <span className="font-medium">
                {t('authMethodPicker.selected', {
                  method: metadata.authMethodLabel(integrationId, selected),
                })}
              </span>
              {selectedNeedsOAuthClient ? (
                <span className="ml-1 text-muted-foreground">
                  {t(
                    canConfigureOAuthClient
                      ? 'authMethodPicker.configureOAuth'
                      : 'authMethodPicker.adminSetupRequired'
                  )}
                </span>
              ) : null}
            </div>
          ) : null}
        </DialogBody>

        <DialogFooter className="border-t bg-muted/30">
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t('authMethodPicker.cancel')}
          </Button>
          <Button
            disabled={!selected || !authMethodCanStart(selected, canConfigureOAuthClient)}
            onClick={() => {
              if (!selected) return;
              onOpenChange(false);
              onSelect(selected);
            }}
          >
            {t('authMethodPicker.continue')}
            <ArrowRight className="size-4" />
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
