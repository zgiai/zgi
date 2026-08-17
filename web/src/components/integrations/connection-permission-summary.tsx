'use client';

import { useState } from 'react';
import { useLocale } from 'next-intl';
import {
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  KeyRound,
  RefreshCw,
  ShieldCheck,
} from 'lucide-react';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { useT } from '@/i18n';
import type {
  IntegrationCatalogItem,
  IntegrationConnectionPermissionSummary as IntegrationConnectionPermissionSummaryData,
  IntegrationConnectionProviderPermission,
  IntegrationLocalizedText,
} from '@/services/types/integration';
import { safeIntegrationDisplayText } from './display-utils';
import { useIntegrationMetadata } from './metadata-i18n';

interface IntegrationConnectionPermissionSummaryProps {
  summary?: IntegrationConnectionPermissionSummaryData | null;
  grantedScopes?: string[] | null;
  provider?: IntegrationCatalogItem;
  upgradePending?: boolean;
  onUpgradeAction?: (actionID: string) => void;
}

function localizedValue(
  values: IntegrationLocalizedText | undefined,
  locale: string,
  fallback: string
): string {
  const normalized = locale.toLowerCase();
  const candidates = normalized.startsWith('zh')
    ? ['zh-Hans', 'zh-CN', 'zh', 'en-US']
    : [locale, 'en-US', 'zh-Hans'];
  for (const candidate of candidates) {
    const value = values?.[candidate]?.trim();
    if (value) return value;
  }
  return fallback;
}

function PermissionBadge({ permission }: { permission: IntegrationConnectionProviderPermission }) {
  const t = useT('integrations');
  const locale = useLocale();
  const label = safeIntegrationDisplayText(
    localizedValue(permission.label_i18n, locale, permission.label),
    permission.id
  );

  return (
    <div className="flex min-w-0 items-center justify-between gap-3 rounded-md border px-3 py-2">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="text-sm font-medium">{label}</span>
          {permission.broad ? (
            <Badge variant="warning">{t('permissionSummary.broad')}</Badge>
          ) : null}
          {!permission.known ? (
            <Badge variant="subtle">{t('permissionSummary.providerNative')}</Badge>
          ) : null}
        </div>
        {permission.id !== label ? (
          <code className="mt-0.5 block break-all text-[11px] text-muted-foreground">
            {permission.id}
          </code>
        ) : null}
      </div>
      <Badge variant="outline" className="shrink-0">
        {t(`permissionSummary.access.${permission.access}`)}
      </Badge>
    </div>
  );
}

function PermissionGroup({
  title,
  icon,
  permissions,
}: {
  title: string;
  icon: React.ReactNode;
  permissions: IntegrationConnectionProviderPermission[];
}) {
  if (permissions.length === 0) return null;
  return (
    <div className="space-y-2">
      <div className="flex items-center gap-2 text-xs font-medium text-muted-foreground">
        {icon}
        <span>{title}</span>
        <Badge variant="subtle">{permissions.length}</Badge>
      </div>
      <div className="grid gap-2">
        {permissions.map(permission => (
          <PermissionBadge key={permission.id} permission={permission} />
        ))}
      </div>
    </div>
  );
}

export function IntegrationConnectionPermissionSummary({
  summary,
  grantedScopes,
  provider,
  upgradePending = false,
  onUpgradeAction,
}: IntegrationConnectionPermissionSummaryProps) {
  const t = useT('integrations');
  const locale = useLocale();
  const metadata = useIntegrationMetadata();
  const [detailsOpen, setDetailsOpen] = useState(false);
  const grantedScopeItems = grantedScopes ?? [];
  const identityPermissions = summary?.identity_permissions ?? [];
  const lifecyclePermissions = summary?.lifecycle_permissions ?? [];
  const providerPermissionItems = summary?.provider_permissions ?? [];
  const unknownPermissions = summary?.unknown_permissions ?? [];
  const missingPermissions = summary?.missing_permissions ?? [];

  const legacyPermissions: IntegrationConnectionProviderPermission[] = grantedScopeItems
    .map(value => value.trim())
    .filter(Boolean)
    .map(id => ({
      id,
      label: metadata.scope(id, provider),
      category: '' as const,
      access: 'unknown' as const,
      broad: false,
      known: metadata.scope(id, provider) !== id,
    }));

  const capabilities = summary?.adapted_capabilities ?? [];
  const connectorDeclaredScopes = summary?.scope_evidence === 'connector_declared';
  const scopeVerified = (capability: (typeof capabilities)[number]) =>
    capability.scope_verified ?? !connectorDeclaredScopes;
  const availableCapabilities = capabilities.filter(item => item.scope_satisfied);
  const runtimeVerifiedCapabilities = availableCapabilities.filter(scopeVerified);
  const runtimeVerificationCapabilities = availableCapabilities.filter(
    item => !scopeVerified(item)
  );
  const blockedCapabilities = capabilities.filter(item => !item.scope_satisfied);
  const providerPermissions = summary
    ? [
        ...identityPermissions,
        ...lifecyclePermissions,
        ...providerPermissionItems,
        ...unknownPermissions,
      ]
    : legacyPermissions;
  const providerScopesReported = summary?.provider_scopes_reported ?? grantedScopeItems.length > 0;

  return (
    <section className="space-y-3">
      <div>
        <h3 className="text-sm font-semibold">{t('permissionSummary.title')}</h3>
        <p className="mt-1 text-xs leading-5 text-muted-foreground">
          {t('permissionSummary.description')}
        </p>
      </div>

      <div className="grid gap-2 sm:grid-cols-3">
        <div className="rounded-lg border bg-muted/20 p-3">
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            {connectorDeclaredScopes ? (
              <ShieldCheck className="size-4 text-primary" />
            ) : (
              <CheckCircle2 className="size-4 text-success" />
            )}
            {t(
              connectorDeclaredScopes
                ? 'permissionSummary.configuredCapabilities'
                : 'permissionSummary.availableCapabilities'
            )}
          </div>
          <p className="mt-1.5 text-lg font-semibold">{availableCapabilities.length}</p>
        </div>
        <div className="rounded-lg border bg-muted/20 p-3">
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <KeyRound className="size-4" />
            {t(
              connectorDeclaredScopes
                ? 'permissionSummary.providerRequirementGroups'
                : 'permissionSummary.providerScopeCount'
            )}
          </div>
          <p className="mt-1.5 text-lg font-semibold">{providerPermissions.length}</p>
        </div>
        <div className="rounded-lg border bg-muted/20 p-3">
          <div className="flex items-center gap-2 text-xs text-muted-foreground">
            <AlertTriangle className="size-4 text-warning" />
            {t('permissionSummary.needsAttention')}
          </div>
          <p className="mt-1.5 text-lg font-semibold">
            {blockedCapabilities.length + missingPermissions.length}
          </p>
        </div>
      </div>

      <div className="rounded-lg border p-3">
        <div className="flex items-center justify-between gap-3">
          <div>
            <p className="text-sm font-medium">{t('permissionSummary.capabilitiesTitle')}</p>
            <p className="mt-0.5 text-xs text-muted-foreground">
              {t('permissionSummary.capabilitiesDescription')}
            </p>
          </div>
          <Badge
            variant={
              connectorDeclaredScopes
                ? 'subtle'
                : blockedCapabilities.length > 0
                  ? 'warning'
                  : 'success'
            }
          >
            {connectorDeclaredScopes
              ? t('permissionSummary.configuredOfTotal', {
                  configured: availableCapabilities.length,
                  total: capabilities.length,
                })
              : t('permissionSummary.availableOfTotal', {
                  available: availableCapabilities.length,
                  total: capabilities.length,
                })}
          </Badge>
        </div>
        {capabilities.length > 0 ? (
          <div className="mt-3 grid gap-2 sm:grid-cols-2">
            {capabilities.map(capability => {
              const name = safeIntegrationDisplayText(
                localizedValue(capability.name_i18n, locale, capability.name),
                capability.action_id
              );
              const description = safeIntegrationDisplayText(
                localizedValue(capability.description_i18n, locale, capability.description ?? ''),
                ''
              );
              return (
                <div
                  key={capability.action_id}
                  className="flex min-w-0 items-start gap-2.5 rounded-md border bg-muted/10 p-3"
                >
                  <span
                    className={
                      !capability.scope_satisfied
                        ? 'mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-warning/10 text-warning'
                        : scopeVerified(capability)
                          ? 'mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-success/10 text-success'
                          : 'mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary'
                    }
                  >
                    {capability.scope_satisfied ? (
                      <CheckCircle2 className="size-3.5" />
                    ) : (
                      <AlertTriangle className="size-3.5" />
                    )}
                  </span>
                  <div className="min-w-0">
                    <p className="text-sm font-medium">{name}</p>
                    {description ? (
                      <p className="mt-0.5 line-clamp-2 text-xs leading-5 text-muted-foreground">
                        {description}
                      </p>
                    ) : null}
                    <div className="mt-1 flex flex-wrap gap-1.5">
                      <Badge variant="outline">{metadata.effect(capability.effect)}</Badge>
                      <Badge variant="subtle">{metadata.risk(capability.risk_level)}</Badge>
                      {!capability.scope_satisfied ? (
                        <Badge variant="warning">{t('permissionSummary.missingAccess')}</Badge>
                      ) : !scopeVerified(capability) ? (
                        <Badge variant="subtle">
                          {t('permissionSummary.runtimeVerificationRequired')}
                        </Badge>
                      ) : null}
                    </div>
                    {!capability.scope_satisfied && capability.can_upgrade && onUpgradeAction ? (
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        className="mt-2"
                        disabled={upgradePending}
                        onClick={() => onUpgradeAction(capability.action_id)}
                      >
                        <RefreshCw
                          className={upgradePending ? 'size-3.5 animate-spin' : 'size-3.5'}
                        />
                        {t('permissionSummary.upgradeAction')}
                      </Button>
                    ) : null}
                  </div>
                </div>
              );
            })}
          </div>
        ) : (
          <p className="mt-3 text-sm text-muted-foreground">
            {t('permissionSummary.noCapabilities')}
          </p>
        )}
      </div>

      {connectorDeclaredScopes && runtimeVerificationCapabilities.length > 0 ? (
        <Alert className="border-primary/25 bg-primary/5 text-foreground">
          <ShieldCheck className="size-4 text-primary" />
          <AlertDescription>
            {t('permissionSummary.connectorDeclaredNotice', {
              configured: availableCapabilities.length,
              verified: runtimeVerifiedCapabilities.length,
            })}
          </AlertDescription>
        </Alert>
      ) : null}

      {summary?.has_broad_permissions ? (
        <Alert className="border-warning/40 bg-warning/5 text-warning">
          <AlertTriangle className="size-4" />
          <AlertDescription>{t('permissionSummary.broadWarning')}</AlertDescription>
        </Alert>
      ) : null}

      {missingPermissions.length > 0 ? (
        <Alert className="border-warning/40 bg-warning/5 text-warning">
          <RefreshCw className="size-4" />
          <AlertDescription>
            {t('permissionSummary.missingWarning', {
              count: missingPermissions.length,
            })}
          </AlertDescription>
        </Alert>
      ) : null}

      <Collapsible open={detailsOpen} onOpenChange={setDetailsOpen}>
        <div className="rounded-lg border">
          <CollapsibleTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              className="h-auto w-full justify-between rounded-lg px-3 py-3"
            >
              <span className="flex min-w-0 items-center gap-2 text-left">
                <ShieldCheck className="size-4 shrink-0 text-muted-foreground" />
                <span>
                  <span className="block text-sm font-medium">
                    {t(
                      connectorDeclaredScopes
                        ? 'permissionSummary.connectorDeclaredDetailsTitle'
                        : 'permissionSummary.providerDetailsTitle'
                    )}
                  </span>
                  <span className="mt-0.5 block text-xs font-normal text-muted-foreground">
                    {connectorDeclaredScopes
                      ? t('permissionSummary.connectorDeclaredDetailsCount', {
                          count: providerPermissions.length,
                        })
                      : providerScopesReported
                        ? t('permissionSummary.providerDetailsCount', {
                            count: providerPermissions.length,
                          })
                        : t('permissionSummary.providerScopesNotReported')}
                  </span>
                </span>
              </span>
              {detailsOpen ? (
                <ChevronUp className="size-4 shrink-0" />
              ) : (
                <ChevronDown className="size-4 shrink-0" />
              )}
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <div className="space-y-4 border-t p-3">
              {providerPermissions.length > 0 ? (
                <>
                  <PermissionGroup
                    title={t('permissionSummary.groups.identity')}
                    icon={<ShieldCheck className="size-3.5" />}
                    permissions={identityPermissions}
                  />
                  <PermissionGroup
                    title={t('permissionSummary.groups.lifecycle')}
                    icon={<RefreshCw className="size-3.5" />}
                    permissions={lifecyclePermissions}
                  />
                  <PermissionGroup
                    title={t(
                      connectorDeclaredScopes
                        ? 'permissionSummary.groups.connectorRequired'
                        : 'permissionSummary.groups.provider'
                    )}
                    icon={<KeyRound className="size-3.5" />}
                    permissions={
                      summary
                        ? [...providerPermissionItems, ...unknownPermissions]
                        : providerPermissions
                    }
                  />
                </>
              ) : (
                <p className="text-sm text-muted-foreground">
                  {t('permissionSummary.providerScopesNotReportedDescription')}
                </p>
              )}
            </div>
          </CollapsibleContent>
        </div>
      </Collapsible>
    </section>
  );
}
