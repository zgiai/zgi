'use client';

import { useEffect, useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import { Bot, ChevronDown, ExternalLink, Pencil, Save, Search, Sparkles } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import {
  useIntegrationActionPolicies,
  useIntegrationProviderCapabilities,
  useUpdateIntegrationActionPolicies,
} from '@/hooks';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  IntegrationActionDefinition,
  IntegrationActionCapability,
  IntegrationActionPolicy,
  IntegrationCatalogItem,
} from '@/services/types/integration';
import { integrationCatalogID, resolveIntegrationAuthDefinitions } from './integration-utils';
import { useIntegrationMetadata } from './metadata-i18n';
import { ProviderCapabilityAvailability } from './provider-capability-availability';

type CapabilityFilter = 'all' | 'read' | 'write';

interface IntegrationProviderCapabilitiesInlineProps {
  provider: IntegrationCatalogItem;
  canManageShared?: boolean;
}

function isReadCapability(action: IntegrationActionDefinition): boolean {
  return action.effect === 'read' || action.effect === 'none';
}

function hasActionScopes(action: IntegrationActionDefinition): boolean {
  return Boolean(action.required_scopes?.length || action.required_any_scopes?.length);
}

function actionDefaultPolicy(
  integrationId: string,
  action: IntegrationActionDefinition
): IntegrationActionPolicy {
  return {
    integration_id: integrationId,
    action_id: action.id,
    // Provider defaults seed missing organization policy rows; persisted
    // organization policy overrides are merged into the editable draft below.
    enabled: action.default_policy?.enabled ?? true,
    approval_policy:
      action.default_policy?.approval_policy === 'always_ask' ? 'always_ask' : 'inherit',
    data_egress_allowed: action.default_policy?.data_egress_allowed ?? true,
  };
}

export function IntegrationProviderCapabilitiesInline({
  provider,
  canManageShared = false,
}: IntegrationProviderCapabilitiesInlineProps) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const [filter, setFilter] = useState<CapabilityFilter>('all');
  const integrationId = integrationCatalogID(provider);
  const actions = useMemo(() => provider.actions ?? [], [provider.actions]);
  // This connected view describes what the current signed-in account can use.
  // Administrators may edit shared policies here, but that must not hide their
  // personal connections from the live capability calculation.
  const capabilityQuery = useIntegrationProviderCapabilities(integrationId, 'account');
  const policyQuery = useIntegrationActionPolicies(canManageShared ? integrationId : '');
  const updateMutation = useUpdateIntegrationActionPolicies(integrationId);
  const [draft, setDraft] = useState<IntegrationActionPolicy[]>([]);
  const [baseline, setBaseline] = useState<IntegrationActionPolicy[]>([]);
  const [revision, setRevision] = useState('');
  const isDirty = JSON.stringify(draft) !== JSON.stringify(baseline);
  const liveCapabilities =
    capabilityQuery.isSuccess && !capabilityQuery.isFetching
      ? capabilityQuery.data?.data
      : undefined;
  const liveCapabilityByAction = useMemo(
    () =>
      new Map<string, IntegrationActionCapability>(
        (liveCapabilities?.actions ?? []).map(action => [action.id, action])
      ),
    [liveCapabilities?.actions]
  );

  useEffect(() => {
    if (!canManageShared) return;
    if (!policyQuery.isSuccess || policyQuery.isFetching || isDirty || updateMutation.isPending) {
      return;
    }
    const saved = new Map(
      (policyQuery.data?.data.items ?? policyQuery.data?.data.policies ?? []).map(policy => [
        policy.action_id,
        policy,
      ])
    );
    const next = actions.map(
      action => saved.get(action.id) ?? actionDefaultPolicy(integrationId, action)
    );
    setDraft(next);
    setBaseline(next);
    setRevision(policyQuery.data?.data.revision ?? '');
  }, [
    actions,
    canManageShared,
    integrationId,
    isDirty,
    policyQuery.data,
    policyQuery.isFetching,
    policyQuery.isSuccess,
    updateMutation.isPending,
  ]);

  const policyByAction = useMemo(
    () =>
      new Map(
        (canManageShared
          ? draft
          : actions.map(action => {
              const live = liveCapabilityByAction.get(action.id);
              if (!live) return actionDefaultPolicy(integrationId, action);
              return {
                integration_id: integrationId,
                action_id: action.id,
                enabled: live.enabled,
                approval_policy: live.approval_policy,
                data_egress_allowed: live.data_egress_allowed,
              } satisfies IntegrationActionPolicy;
            })
        ).map(policy => [policy.action_id, policy])
      ),
    [actions, canManageShared, draft, integrationId, liveCapabilityByAction]
  );
  const summary = useMemo(
    () => ({
      total: actions.length,
      read: actions.filter(isReadCapability).length,
      write: actions.filter(action => !isReadCapability(action)).length,
    }),
    [actions]
  );
  const filteredActions = useMemo(
    () =>
      actions.filter(action => {
        if (filter === 'read') return isReadCapability(action);
        if (filter === 'write') return !isReadCapability(action);
        return true;
      }),
    [actions, filter]
  );
  const authenticationMethods = useMemo(
    () => resolveIntegrationAuthDefinitions(provider),
    [provider]
  );
  const documentationURL = metadata.documentationURL(provider);

  const updatePolicy = (actionId: string, update: Partial<IntegrationActionPolicy>) => {
    setDraft(current =>
      current.map(policy => (policy.action_id === actionId ? { ...policy, ...update } : policy))
    );
  };

  return (
    <section aria-label={t('capabilities.connectedViewLabel')} className="space-y-4">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <p className="text-sm font-medium">
            {liveCapabilities
              ? t('capabilities.connectedSummary', {
                  total: liveCapabilities.summary.total,
                  available: liveCapabilities.summary.available,
                  attention: liveCapabilities.summary.needs_attention,
                })
              : t('capabilities.catalogSummary', {
                  total: summary.total,
                  read: summary.read,
                  write: summary.write,
                })}
          </p>
          <p className="mt-1 text-xs text-muted-foreground">{t('capabilities.catalogNotice')}</p>
        </div>
        <div className="flex shrink-0 flex-wrap items-center gap-2">
          <div
            role="tablist"
            aria-label={t('capabilities.filterLabel')}
            className="inline-flex rounded-md border bg-background p-0.5"
          >
            {(['all', 'read', 'write'] as const).map(value => (
              <button
                key={value}
                type="button"
                role="tab"
                aria-selected={filter === value}
                className={cn(
                  'h-7 rounded px-2.5 text-[11px] font-medium transition-colors',
                  filter === value
                    ? 'bg-primary text-primary-foreground'
                    : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                )}
                onClick={() => setFilter(value)}
              >
                {t(`capabilities.filters.${value}`, {
                  count:
                    value === 'all'
                      ? summary.total
                      : value === 'read'
                        ? summary.read
                        : summary.write,
                })}
              </button>
            ))}
          </div>
          {documentationURL ? (
            <Button asChild variant="ghost" size="xs">
              <a href={documentationURL} target="_blank" rel="noreferrer">
                {t('capabilities.documentation')}
                <ExternalLink className="size-3.5" />
              </a>
            </Button>
          ) : null}
          {canManageShared ? (
            <Button
              size="xs"
              disabled={
                revision.length !== 64 ||
                !isDirty ||
                policyQuery.isFetching ||
                policyQuery.isError ||
                updateMutation.isPending
              }
              onClick={() =>
                updateMutation.mutate(
                  { revision, policies: draft },
                  {
                    onSuccess: response => {
                      const saved = response.data.items ?? response.data.policies ?? [];
                      setDraft(saved);
                      setBaseline(saved);
                      setRevision(response.data.revision);
                    },
                  }
                )
              }
            >
              <Save className="size-3.5" />
              {t('capabilities.saveExecutionSettings')}
            </Button>
          ) : null}
        </div>
      </div>

      {capabilityQuery.isError ? (
        <div className="rounded-lg border border-warning/30 bg-warning/5 p-4 text-sm text-warning">
          <p className="font-medium">{t('capabilities.loadFailed')}</p>
          <p className="mt-1 text-xs leading-5">{t('capabilities.liveStatusUnavailable')}</p>
        </div>
      ) : null}

      {canManageShared && policyQuery.isError ? (
        <div className="rounded-lg border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
          {t('policies.loadFailed')}
        </div>
      ) : null}

      {canManageShared && policyQuery.isLoading && actions.length > 0 ? (
        <div className="space-y-2">
          {Array.from({ length: Math.min(actions.length, 3) }).map((_, index) => (
            <Skeleton key={index} className="h-20 rounded-lg" />
          ))}
        </div>
      ) : filteredActions.length === 0 ? (
        <div className="rounded-lg border border-dashed bg-background p-8 text-center text-sm text-muted-foreground">
          {t('capabilities.empty')}
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border bg-background">
          {filteredActions.map((action, index) => {
            const read = isReadCapability(action);
            const Icon = read ? Search : Pencil;
            const policy =
              policyByAction.get(action.id) ?? actionDefaultPolicy(integrationId, action);
            const liveCapability = liveCapabilityByAction.get(action.id);
            const currentPolicyKnown = canManageShared
              ? policyQuery.isSuccess
              : Boolean(liveCapability);
            const availabilityState =
              capabilityQuery.isLoading || capabilityQuery.isFetching
                ? 'checking'
                : capabilityQuery.isError || !liveCapability
                  ? 'status_unavailable'
                  : liveCapability.availability;
            const approvalLockedByProvider =
              action.default_policy?.approval_policy === 'always_ask';
            const egressLockedByProvider =
              action.data_egress && action.default_policy?.data_egress_allowed === false;
            const supportedAuthMethodIDs = new Set(action.supported_auth_method_ids ?? []);
            const supportedAuthMethods =
              supportedAuthMethodIDs.size > 0
                ? authenticationMethods.filter(method => supportedAuthMethodIDs.has(method.id))
                : authenticationMethods;
            return (
              <details key={action.id} className={cn('group', index > 0 && 'border-t')}>
                <summary className="grid cursor-pointer list-none gap-3 p-4 marker:hidden hover:bg-muted/20 @[1050px]/connections:grid-cols-[minmax(180px,.85fr)_minmax(230px,1.35fr)_minmax(80px,.35fr)_minmax(90px,.4fr)_minmax(150px,.65fr)_minmax(150px,.7fr)_20px] @[1050px]/connections:items-center">
                  <div className="flex min-w-0 items-start gap-3">
                    <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-primary/5 text-primary">
                      <Icon className="size-4" />
                    </span>
                    <div className="min-w-0">
                      <h4 className="text-sm font-medium">{metadata.actionName(action)}</h4>
                      <p className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground">
                        {action.id}
                      </p>
                    </div>
                  </div>
                  <p className="text-xs leading-5 text-muted-foreground">
                    {metadata.actionDescription(action)}
                  </p>
                  <div>
                    <span className="mb-1 block text-[11px] font-medium text-muted-foreground @[1050px]/connections:hidden">
                      {t('capabilities.columns.effect')}
                    </span>
                    <Badge
                      variant="subtle"
                      className={cn(
                        'text-[11px]',
                        read ? 'bg-primary/8 text-primary' : 'bg-destructive/8 text-destructive'
                      )}
                    >
                      {read ? t('capabilities.access.read') : t('capabilities.access.write')}
                    </Badge>
                  </div>
                  <div>
                    <span className="mb-1 block text-[11px] font-medium text-muted-foreground @[1050px]/connections:hidden">
                      {t('capabilities.columns.risk')}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {metadata.risk(action.risk_level)}
                    </span>
                  </div>
                  <div>
                    <span className="mb-1 block text-[11px] font-medium text-muted-foreground @[1050px]/connections:hidden">
                      {t('capabilities.columns.availability')}
                    </span>
                    <ProviderCapabilityAvailability
                      state={availabilityState}
                      compatibleConnectionCount={liveCapability?.compatible_connection_count}
                    />
                  </div>
                  <div>
                    <span className="mb-1 block text-[11px] font-medium text-muted-foreground @[1050px]/connections:hidden">
                      {t('capabilities.columns.execution')}
                    </span>
                    <span className="text-xs text-muted-foreground">
                      {currentPolicyKnown
                        ? policy.approval_policy === 'always_ask'
                          ? t('capabilities.approvalAlways')
                          : t('capabilities.approvalInherit')
                        : t('capabilities.currentPolicyUnavailable')}
                    </span>
                  </div>
                  <ChevronDown className="size-4 text-muted-foreground transition-transform group-open:rotate-180" />
                </summary>

                <div className="grid gap-5 border-t bg-muted/10 px-4 py-4 @[760px]/connections:grid-cols-2">
                  <div className="space-y-4">
                    <CapabilityDetail
                      label={t('capabilities.authentication')}
                      empty={t('capabilities.noAuthenticationMethods')}
                    >
                      {supportedAuthMethods.map(method => (
                        <Badge key={method.id} variant="outline" className="font-normal">
                          {metadata.authMethodLabel(integrationId, method)}
                        </Badge>
                      ))}
                    </CapabilityDetail>
                    <CapabilityDetail
                      label={t('capabilities.requiredScopes')}
                      empty={t('capabilities.noAdditionalScopes')}
                    >
                      {(action.required_scopes ?? []).map(scope => (
                        <Badge key={scope} variant="subtle" className="font-normal">
                          {metadata.scope(scope, provider)}
                        </Badge>
                      ))}
                      {(action.required_any_scopes ?? []).length > 0 ? (
                        <Badge variant="subtle" className="font-normal">
                          {action.required_any_scopes
                            ?.map(scope => metadata.scope(scope, provider))
                            .join(' / ')}
                        </Badge>
                      ) : null}
                      {!hasActionScopes(action) ? (
                        <span className="text-xs text-muted-foreground">
                          {t('capabilities.noAdditionalScopes')}
                        </span>
                      ) : null}
                    </CapabilityDetail>
                    <CapabilityDetail
                      label={t('capabilities.surfaces')}
                      empty={t('capabilities.noSupportedSurfaces')}
                    >
                      {(action.supported_callers ?? []).map(caller => (
                        <Badge key={caller} variant="outline" className="gap-1 font-normal">
                          {caller === 'aichat' ? <Sparkles className="size-3" /> : null}
                          {caller === 'agent' ? <Bot className="size-3" /> : null}
                          {caller === 'aichat'
                            ? t('capabilities.surface.aichat')
                            : caller === 'agent'
                              ? t('capabilities.surface.agent')
                              : caller === 'workflow'
                                ? t('capabilities.surface.workflow')
                                : caller === 'api'
                                  ? t('capabilities.surface.api')
                                  : t('capabilities.surface.other')}
                        </Badge>
                      ))}
                    </CapabilityDetail>
                  </div>

                  <div className="space-y-3">
                    <p className="text-xs font-medium text-foreground">
                      {t('capabilities.executionSettings')}
                    </p>
                    <div className="flex items-center justify-between gap-4 rounded-md border bg-background p-3">
                      <div>
                        <p className="text-xs font-medium">{t('capabilities.actionStatus')}</p>
                        <p className="mt-1 text-[11px] text-muted-foreground">
                          {t('capabilities.actionStatusDescription')}
                        </p>
                      </div>
                      {canManageShared ? (
                        <Switch
                          checked={policy.enabled}
                          disabled={
                            updateMutation.isPending || policyQuery.isLoading || policyQuery.isError
                          }
                          aria-label={t('capabilities.actionStatus')}
                          onCheckedChange={enabled => updatePolicy(action.id, { enabled })}
                        />
                      ) : (
                        <Badge variant="outline">
                          {currentPolicyKnown
                            ? policy.enabled
                              ? t('capabilities.actionEnabled')
                              : t('capabilities.actionDisabled')
                            : t('capabilities.currentPolicyUnavailable')}
                        </Badge>
                      )}
                    </div>
                    <div className="space-y-1">
                      <span className="text-[11px] text-muted-foreground">
                        {t('policies.approval')}
                      </span>
                      {canManageShared ? (
                        <Select
                          value={policy.approval_policy}
                          disabled={
                            policyQuery.isLoading ||
                            policyQuery.isError ||
                            updateMutation.isPending ||
                            !policy.enabled ||
                            approvalLockedByProvider
                          }
                          onValueChange={approvalPolicy =>
                            updatePolicy(action.id, {
                              approval_policy:
                                approvalPolicy as IntegrationActionPolicy['approval_policy'],
                            })
                          }
                        >
                          <SelectTrigger className="h-9">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="inherit">{t('policies.inherit')}</SelectItem>
                            <SelectItem value="always_ask">{t('policies.alwaysAsk')}</SelectItem>
                          </SelectContent>
                        </Select>
                      ) : (
                        <p className="text-xs text-muted-foreground">
                          {currentPolicyKnown
                            ? policy.approval_policy === 'always_ask'
                              ? t('capabilities.approvalAlways')
                              : t('capabilities.approvalInherit')
                            : t('capabilities.currentPolicyUnavailable')}
                        </p>
                      )}
                    </div>
                    <div className="flex items-center justify-between gap-4 rounded-md border bg-background p-3">
                      <div>
                        <p className="text-xs font-medium">{t('capabilities.dataEgress')}</p>
                        <p className="mt-1 text-[11px] text-muted-foreground">
                          {action.external_destination
                            ? t('capabilities.externalDestination', {
                                destination: action.external_destination,
                              })
                            : t('capabilities.noExternalDestination')}
                        </p>
                      </div>
                      {canManageShared ? (
                        <Switch
                          checked={policy.data_egress_allowed}
                          disabled={
                            updateMutation.isPending ||
                            policyQuery.isLoading ||
                            policyQuery.isError ||
                            !policy.enabled ||
                            !action.data_egress ||
                            egressLockedByProvider
                          }
                          onCheckedChange={dataEgressAllowed =>
                            updatePolicy(action.id, {
                              data_egress_allowed: dataEgressAllowed,
                            })
                          }
                        />
                      ) : (
                        <Badge variant="outline">
                          {!currentPolicyKnown
                            ? t('capabilities.currentPolicyUnavailable')
                            : !action.data_egress
                              ? t('capabilities.dataEgressNotRequired')
                              : policy.data_egress_allowed
                                ? t('capabilities.dataEgressAllowed')
                                : t('capabilities.dataEgressBlocked')}
                        </Badge>
                      )}
                    </div>
                    {approvalLockedByProvider || egressLockedByProvider ? (
                      <p className="text-[11px] leading-5 text-muted-foreground">
                        {t('policies.immutable')}
                      </p>
                    ) : null}
                  </div>
                </div>
              </details>
            );
          })}
        </div>
      )}
    </section>
  );
}

function CapabilityDetail({
  label,
  empty,
  children,
}: {
  label: string;
  empty: string;
  children: ReactNode;
}) {
  const items = Array.isArray(children) ? children.filter(Boolean) : children ? [children] : [];
  return (
    <div>
      <p className="text-xs font-medium text-foreground">{label}</p>
      {items.length > 0 ? (
        <div className="mt-2 flex flex-wrap gap-1.5">{children}</div>
      ) : (
        <p className="mt-1.5 text-xs text-muted-foreground">{empty}</p>
      )}
    </div>
  );
}
