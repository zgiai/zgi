'use client';

import {
  BotOff,
  CheckCircle2,
  Fingerprint,
  KeyRound,
  Pencil,
  Play,
  ShieldCheck,
  UserRound,
} from 'lucide-react';
import { Alert, AlertDescription } from '@/components/ui/alert';
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
import { Skeleton } from '@/components/ui/skeleton';
import { useIntegrationConnection } from '@/hooks';
import { useT } from '@/i18n';
import type { IntegrationCatalogItem, IntegrationConnection } from '@/services/types/integration';
import { actionsForAuthMethod } from './action-auth-compatibility';
import { IntegrationConnectionHealthBadge } from './health-badge';
import { IntegrationConnectionGrantsPanel } from './connection-grants-panel';
import { IntegrationConnectionHealthPanel } from './connection-health-panel';
import { IntegrationConnectionPermissionSummary } from './connection-permission-summary';
import { safeIntegrationDisplayText, safeOptionalIntegrationDisplayText } from './display-utils';
import { useIntegrationMetadata } from './metadata-i18n';

interface IntegrationConnectionDetailDialogProps {
  open: boolean;
  connection: IntegrationConnection | null;
  provider?: IntegrationCatalogItem;
  isTesting?: boolean;
  canManage?: boolean;
  onOpenChange: (open: boolean) => void;
  onEdit: (connection: IntegrationConnection) => void;
  onTest: (connection: IntegrationConnection) => void;
}

function DetailRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className="grid gap-1 border-b py-2.5 last:border-b-0 sm:grid-cols-[160px_minmax(0,1fr)] sm:gap-4">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="min-w-0 text-sm">{value}</dd>
    </div>
  );
}

export function IntegrationConnectionDetailDialog({
  open,
  connection,
  provider,
  isTesting = false,
  canManage = true,
  onOpenChange,
  onEdit,
  onTest,
}: IntegrationConnectionDetailDialogProps) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const isPersonal = connection?.credential_source === 'account';
  const detailQuery = useIntegrationConnection(
    connection?.id ?? '',
    open && Boolean(connection) && !isPersonal
  );
  const item = isPersonal ? connection : (detailQuery.data?.data ?? connection);
  const providerActions = actionsForAuthMethod(provider?.actions ?? [], item?.auth_method_id);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg" className="p-0">
        <DialogHeader>
          <DialogTitle>
            {safeIntegrationDisplayText(item?.name, t('connectionDetail.title'))}
          </DialogTitle>
          <DialogDescription>
            {t(
              item?.credential_source === 'account'
                ? 'connectionDetail.personalDescription'
                : 'connectionDetail.description'
            )}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-5 pb-5">
          {detailQuery.isLoading && !item ? (
            <div className="space-y-3">
              <Skeleton className="h-24 rounded-lg" />
              <Skeleton className="h-48 rounded-lg" />
            </div>
          ) : item ? (
            <>
              <div className="grid gap-3 sm:grid-cols-3">
                <div className="rounded-lg border bg-muted/20 p-3">
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <CheckCircle2 className="size-4" />
                    {t('connectionDetail.health')}
                  </div>
                  <div className="mt-2">
                    <IntegrationConnectionHealthBadge connection={item} />
                  </div>
                </div>
                <div className="rounded-lg border bg-muted/20 p-3">
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <KeyRound className="size-4" />
                    {t('connectionDetail.credential')}
                  </div>
                  <div className="mt-2">
                    <Badge variant={item.credential_configured === false ? 'warning' : 'success'}>
                      {item.credential_configured === false
                        ? t('connectionDetail.credentialMissing')
                        : t('connectionDetail.credentialStored')}
                    </Badge>
                  </div>
                </div>
                <div className="rounded-lg border bg-muted/20 p-3">
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    <UserRound className="size-4" />
                    {t('connectionDetail.authenticationStatus')}
                  </div>
                  <Badge className="mt-2" variant="outline">
                    {t(`connectionHealth.authStatus.${item.auth_status}`)}
                  </Badge>
                </div>
              </div>

              <Alert>
                <CheckCircle2 className="size-4" />
                <AlertDescription>{t('connectionDetail.secretNotice')}</AlertDescription>
              </Alert>

              <section>
                <h3 className="text-sm font-semibold">{t('connectionDetail.identityTitle')}</h3>
                <dl className="mt-2 rounded-lg border px-3">
                  <DetailRow
                    label={t('connectionDetail.provider')}
                    value={safeIntegrationDisplayText(
                      provider
                        ? metadata.providerName(provider)
                        : metadata.providerName(item.integration_id),
                      t('common.unknownExternalApp')
                    )}
                  />
                  <DetailRow
                    label={t('connectionDetail.externalIdentity')}
                    value={
                      <span className="inline-flex items-center gap-1.5">
                        <UserRound className="size-3.5 text-muted-foreground" />
                        {safeIntegrationDisplayText(
                          item.display_name || item.account_id,
                          t('connectionDetail.notReported')
                        )}
                      </span>
                    }
                  />
                  <DetailRow
                    label={t('connectionDetail.authType')}
                    value={t('connectionDetail.authSummary', {
                      source: t(`connections.credentialSource.${item.credential_source}`),
                      method: metadata.authType(item.auth_type),
                    })}
                  />
                  {item.credential_source === 'account' ? (
                    <DetailRow
                      label={t('connectionDetail.ownerAccount')}
                      value={t('connectionDetail.currentAccount')}
                    />
                  ) : null}
                </dl>
              </section>

              <IntegrationConnectionPermissionSummary
                summary={item.permission_summary}
                grantedScopes={item.granted_scopes}
                provider={provider}
              />

              {item.credential_source === 'account' ? (
                <section className="space-y-3">
                  <div>
                    <h3 className="text-sm font-semibold">
                      {t('connectionDetail.personalAccessTitle')}
                    </h3>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {t('connectionDetail.personalAccessDescription')}
                    </p>
                  </div>
                  <div className="rounded-lg border bg-muted/20 p-4">
                    <div className="flex items-start gap-3">
                      <span className="flex size-9 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
                        <ShieldCheck className="size-4" />
                      </span>
                      <div className="min-w-0 flex-1">
                        <Badge variant="success">{t('connectionDetail.personalAccessBadge')}</Badge>
                        <p className="mt-2 text-sm leading-6">
                          {t('connectionDetail.personalAccessAIChat')}
                        </p>
                        <p className="mt-1 flex items-center gap-1.5 text-xs leading-5 text-muted-foreground">
                          <BotOff className="size-3.5 shrink-0" />
                          {t('connectionDetail.personalAccessAgents')}
                        </p>
                      </div>
                    </div>
                  </div>
                </section>
              ) : (
                <IntegrationConnectionGrantsPanel
                  connectionId={item.id}
                  actions={providerActions}
                  enabled={open}
                />
              )}

              <IntegrationConnectionHealthPanel
                connection={item}
                provider={provider}
                enabled={open && item.credential_source !== 'account'}
                showHistory={item.credential_source !== 'account'}
              />

              <section>
                <h3 className="text-sm font-semibold">{t('connectionDetail.diagnosticsTitle')}</h3>
                <dl className="mt-2 rounded-lg border px-3">
                  <DetailRow
                    label={t('connectionDetail.lastTested')}
                    value={metadata.date(item.last_tested_at, t('connections.neverTested'))}
                  />
                  <DetailRow
                    label={t('connectionDetail.lastRuntimeSuccess')}
                    value={metadata.date(
                      item.last_runtime_success_at,
                      t('connectionDetail.neverUsed')
                    )}
                  />
                  <DetailRow
                    label={t('connectionDetail.expiresAt')}
                    value={metadata.date(item.expires_at, t('connectionDetail.noExpiry'))}
                  />
                  {item.auth_type === 'oauth2' ? (
                    <>
                      <DetailRow
                        label={t('connectionDetail.accessTokenExpiresAt')}
                        value={metadata.date(item.token_expires_at, t('connectionDetail.noExpiry'))}
                      />
                      <DetailRow
                        label={t('connectionDetail.refreshTokenExpiresAt')}
                        value={metadata.date(
                          item.refresh_token_expires_at,
                          t('connectionDetail.noExpiry')
                        )}
                      />
                    </>
                  ) : null}
                  <DetailRow
                    label={t('connectionDetail.lastError')}
                    value={
                      safeOptionalIntegrationDisplayText(item.last_error_code) ? (
                        <span className="text-destructive">
                          {metadata.error(item.last_error_code)}
                        </span>
                      ) : (
                        t('connectionDetail.noError')
                      )
                    }
                  />
                  <DetailRow
                    label={t('connectionDetail.updatedAt')}
                    value={metadata.date(item.updated_at, t('executions.noValue'))}
                  />
                  {item.credential_version ? (
                    <DetailRow
                      label={t('connectionDetail.credentialVersion')}
                      value={
                        <span className="inline-flex items-center gap-1.5">
                          <Fingerprint className="size-3.5 text-muted-foreground" />
                          {metadata.number(item.credential_version)}
                        </span>
                      }
                    />
                  ) : null}
                  {item.last_health_checked_at ? (
                    <DetailRow
                      label={t('connectionDetail.healthCheckedAt')}
                      value={
                        <span className="inline-flex items-center gap-1.5">
                          {metadata.date(item.last_health_checked_at, t('executions.noValue'))}
                        </span>
                      }
                    />
                  ) : null}
                </dl>
              </section>
            </>
          ) : (
            <p className="py-8 text-center text-sm text-destructive">
              {t('connectionDetail.loadFailed')}
            </p>
          )}
        </DialogBody>
        <DialogFooter className="border-t bg-muted/20">
          {item && canManage ? (
            <>
              <Button variant="outline" onClick={() => onEdit(item)}>
                <Pencil className="size-4" />
                {t('connections.actions.edit')}
              </Button>
              <Button disabled={isTesting} onClick={() => onTest(item)}>
                <Play className="size-4" />
                {t('connections.actions.test')}
              </Button>
            </>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
