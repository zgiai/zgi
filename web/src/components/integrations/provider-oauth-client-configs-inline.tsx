'use client';

import { useMemo, useState } from 'react';
import {
  AlertCircle,
  CheckCircle2,
  ExternalLink,
  KeyRound,
  Settings2,
  ShieldCheck,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { useIntegrationOAuthClientConfig, useIntegrationOAuthClientConfigImpact } from '@/hooks';
import { useT } from '@/i18n';
import type {
  IntegrationAuthDefinition,
  IntegrationCatalogItem,
} from '@/services/types/integration';
import {
  authMethodsSharingOAuthClient,
  isOAuthAuthMethod,
  oauthClientConfigID,
} from './auth-method-selection';
import { IntegrationOAuthClientConfigDialog } from './oauth-client-config-dialog';
import { integrationCatalogID, resolveIntegrationAuthDefinitions } from './integration-utils';
import { useIntegrationMetadata } from './metadata-i18n';

export interface ProviderOAuthClientConfigGroup {
  id: string;
  auth: IntegrationAuthDefinition;
  methods: IntegrationAuthDefinition[];
}

interface IntegrationProviderOAuthClientConfigsInlineProps {
  provider: IntegrationCatalogItem;
}

export function resolveProviderOAuthClientConfigGroups(
  provider: IntegrationCatalogItem
): ProviderOAuthClientConfigGroup[] {
  const methods = resolveIntegrationAuthDefinitions(provider).filter(isOAuthAuthMethod);
  const groups = new Map<string, ProviderOAuthClientConfigGroup>();
  for (const method of methods) {
    const id = oauthClientConfigID(method);
    if (groups.has(id)) continue;
    groups.set(id, {
      id,
      auth: method,
      methods: authMethodsSharingOAuthClient(methods, method),
    });
  }
  return [...groups.values()];
}

export function IntegrationProviderOAuthClientConfigsInline({
  provider,
}: IntegrationProviderOAuthClientConfigsInlineProps) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const integrationId = integrationCatalogID(provider);
  const groups = useMemo(() => resolveProviderOAuthClientConfigGroups(provider), [provider]);
  const [selectedGroup, setSelectedGroup] = useState<ProviderOAuthClientConfigGroup | null>(null);

  return (
    <section aria-label={t('oauth.clientConfig.connectedViewLabel')} className="space-y-3">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <h4 className="text-sm font-semibold">{t('oauth.clientConfig.connectedViewTitle')}</h4>
          <p className="mt-1 max-w-3xl text-xs leading-5 text-muted-foreground">
            {t('oauth.clientConfig.connectedViewDescription')}
          </p>
        </div>
        <Badge variant="outline" className="w-fit shrink-0 gap-1.5 font-normal">
          <ShieldCheck className="size-3.5 text-success" />
          {t('oauth.clientConfig.writeOnlyBadge')}
        </Badge>
      </div>

      <div className="space-y-3">
        {groups.map(group => (
          <OAuthClientConfigSummaryCard
            key={group.id}
            integrationId={integrationId}
            providerName={metadata.providerName(provider)}
            group={group}
            onManage={() => setSelectedGroup(group)}
          />
        ))}
      </div>

      <div className="flex items-start gap-2 rounded-lg border border-primary/15 bg-primary/[0.03] p-3 text-xs leading-5 text-muted-foreground">
        <KeyRound className="mt-0.5 size-4 shrink-0 text-primary" />
        <p>{t('oauth.clientConfig.connectedViewSecurityNote')}</p>
      </div>

      <IntegrationOAuthClientConfigDialog
        open={Boolean(selectedGroup)}
        integrationId={integrationId}
        providerName={metadata.providerName(provider)}
        auth={selectedGroup?.auth ?? null}
        relatedAuthMethods={selectedGroup?.methods ?? []}
        onConfigured={() => undefined}
        onOpenChange={open => {
          if (!open) setSelectedGroup(null);
        }}
      />
    </section>
  );
}

function OAuthClientConfigSummaryCard({
  integrationId,
  providerName,
  group,
  onManage,
}: {
  integrationId: string;
  providerName: string;
  group: ProviderOAuthClientConfigGroup;
  onManage: () => void;
}) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const configQuery = useIntegrationOAuthClientConfig(integrationId, group.auth.id, true, group.id);
  const config = configQuery.data?.data;
  const impactQuery = useIntegrationOAuthClientConfigImpact(
    integrationId,
    group.auth.id,
    Boolean(config?.configured),
    group.id
  );
  const impact = impactQuery.data?.data;
  const updatedAt = config?.updated_at
    ? metadata.date(config.updated_at, t('oauth.clientConfig.valueUnavailable'))
    : null;

  if (configQuery.isLoading) {
    return <Skeleton className="h-48 rounded-xl" />;
  }

  if (configQuery.isError || !config) {
    return (
      <div className="rounded-xl border border-destructive/30 bg-destructive/[0.03] p-4">
        <div className="flex items-start gap-3">
          <AlertCircle className="mt-0.5 size-4 shrink-0 text-destructive" />
          <div className="min-w-0 flex-1">
            <p className="text-sm font-medium text-destructive">
              {t('oauth.clientConfig.loadFailed')}
            </p>
            <Button
              type="button"
              variant="outline"
              size="xs"
              className="mt-3"
              onClick={() => void configQuery.refetch()}
            >
              {t('oauth.clientConfig.retry')}
            </Button>
          </div>
        </div>
      </div>
    );
  }

  return (
    <article className="overflow-hidden rounded-xl border bg-background">
      <div className="flex flex-col gap-3 border-b bg-muted/[0.12] px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span
            className={
              config.configured
                ? 'inline-flex items-center gap-1.5 rounded-full bg-success/10 px-2.5 py-1 text-xs font-medium text-success'
                : 'inline-flex items-center gap-1.5 rounded-full bg-warning/10 px-2.5 py-1 text-xs font-medium text-warning'
            }
          >
            {config.configured ? (
              <CheckCircle2 className="size-3.5" />
            ) : (
              <AlertCircle className="size-3.5" />
            )}
            {t(
              config.configured
                ? 'oauth.clientConfig.configured'
                : 'oauth.clientConfig.notConfigured'
            )}
          </span>
          <span className="text-xs text-muted-foreground">
            {t('oauth.clientConfig.sharedByMethods', { count: group.methods.length || 1 })}
          </span>
          {impact ? (
            <span className="text-xs text-muted-foreground">
              ·{' '}
              {t('oauth.clientConfig.connectedAccounts', {
                count: impact.dependent_connections,
              })}
            </span>
          ) : null}
        </div>
        <Button type="button" variant="outline" size="sm" onClick={onManage}>
          <Settings2 className="size-4" />
          {t(
            config.configured
              ? 'oauth.clientConfig.manageAction'
              : 'oauth.clientConfig.configureAction'
          )}
        </Button>
      </div>

      <div className="grid divide-y @[760px]/connections:grid-cols-2 @[760px]/connections:divide-x @[760px]/connections:divide-y-0">
        <div className="space-y-3 p-4">
          <div className="grid gap-1.5 sm:grid-cols-[8rem_minmax(0,1fr)] sm:items-center">
            <span className="text-xs font-medium">{t('oauth.clientConfig.clientID')}</span>
            <span className="truncate font-mono text-xs text-muted-foreground">
              {config.client_id_masked ?? t('oauth.clientConfig.valueUnavailable')}
            </span>
          </div>
          <div className="grid gap-1.5 sm:grid-cols-[8rem_minmax(0,1fr)] sm:items-center">
            <span className="text-xs font-medium">{t('oauth.clientConfig.clientSecret')}</span>
            <span className="inline-flex items-center gap-1.5 text-xs text-success">
              <ShieldCheck className="size-3.5" />
              {config.has_client_secret
                ? t('oauth.clientConfig.secretStored')
                : t('oauth.clientConfig.secretNotRequired')}
            </span>
          </div>
          <div className="grid gap-1.5 sm:grid-cols-[8rem_minmax(0,1fr)] sm:items-center">
            <span className="text-xs font-medium">{t('oauth.clientConfig.sourceLabel')}</span>
            <span className="text-xs text-muted-foreground">
              {t(`oauth.clientConfig.source.${config.source}`)}
            </span>
          </div>
        </div>

        <div className="space-y-3 p-4">
          <div className="grid gap-1.5 sm:grid-cols-[8rem_minmax(0,1fr)] sm:items-center">
            <span className="text-xs font-medium">{t('oauth.clientConfig.callbackURL')}</span>
            <span className="truncate font-mono text-xs text-muted-foreground">
              {config.callback_url ?? t('oauth.clientConfig.valueUnavailable')}
            </span>
          </div>
          <div className="grid gap-1.5 sm:grid-cols-[8rem_minmax(0,1fr)] sm:items-start">
            <span className="text-xs font-medium">{t('oauth.clientConfig.supportedMethods')}</span>
            <div className="flex flex-wrap gap-1.5">
              {group.methods.map(method => (
                <Badge key={method.id} variant="outline" className="font-normal">
                  {metadata.authMethodLabel(integrationId, method)}
                </Badge>
              ))}
            </div>
          </div>
          <div className="grid gap-1.5 sm:grid-cols-[8rem_minmax(0,1fr)] sm:items-center">
            <span className="text-xs font-medium">{t('oauth.clientConfig.updatedLabel')}</span>
            <span className="text-xs text-muted-foreground">
              {updatedAt ?? t('oauth.clientConfig.neverUpdated')}
            </span>
          </div>
        </div>
      </div>

      {config.provider_setup_url ? (
        <div className="flex justify-end border-t px-4 py-2">
          <Button asChild variant="ghost" size="xs">
            <a href={config.provider_setup_url} target="_blank" rel="noreferrer noopener">
              <ExternalLink className="size-3.5" />
              {t('oauth.clientConfig.openProviderConsole', { provider: providerName })}
            </a>
          </Button>
        </div>
      ) : null}
    </article>
  );
}
