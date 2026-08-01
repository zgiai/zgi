'use client';

import { useMemo, useState } from 'react';
import {
  ArrowRight,
  Bot,
  ChevronDown,
  ExternalLink,
  Info,
  KeyRound,
  ShieldCheck,
  Sparkles,
} from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { useIntegrationProviderCapabilities } from '@/hooks';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  IntegrationActionDefinition,
  IntegrationActionCapability,
  IntegrationCatalogItem,
} from '@/services/types/integration';
import { integrationCatalogID, resolveIntegrationAuthDefinitions } from './integration-utils';
import { useIntegrationMetadata } from './metadata-i18n';
import { ProviderCapabilityAvailability } from './provider-capability-availability';
import { IntegrationProviderIcon } from './provider-icon';

type CapabilityFilter = 'all' | 'read' | 'write';

interface IntegrationProviderCapabilitiesSheetProps {
  provider: IntegrationCatalogItem | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConnect: (provider: IntegrationCatalogItem) => void;
  audience?: 'account' | 'organization';
}

function isReadCapability(action: IntegrationActionDefinition): boolean {
  return action.effect === 'read' || action.effect === 'none';
}

function hasActionScopes(action: IntegrationActionDefinition): boolean {
  return Boolean(action.required_scopes?.length || action.required_any_scopes?.length);
}

export function IntegrationProviderCapabilitiesSheet({
  provider,
  open,
  onOpenChange,
  onConnect,
  audience = 'account',
}: IntegrationProviderCapabilitiesSheetProps) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const [filter, setFilter] = useState<CapabilityFilter>('all');
  const integrationId = provider ? integrationCatalogID(provider) : '';
  const capabilityQuery = useIntegrationProviderCapabilities(integrationId, audience, open);
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
  const actions = useMemo(() => provider?.actions ?? [], [provider?.actions]);
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
  const supportedCallers = useMemo(
    () => new Set(actions.flatMap(action => action.supported_callers ?? [])),
    [actions]
  );
  const documentationURL = provider ? metadata.documentationURL(provider) : null;

  return (
    <Sheet
      open={open}
      onOpenChange={nextOpen => {
        if (!nextOpen) setFilter('all');
        onOpenChange(nextOpen);
      }}
    >
      <SheetContent
        side="right"
        className="flex w-full flex-col gap-0 overflow-hidden p-0 sm:max-w-[500px]"
      >
        <SheetHeader className="border-b px-6 py-5 pr-14 text-left">
          <SheetTitle>
            {t('capabilities.title', {
              provider: provider ? metadata.providerName(provider) : '',
            })}
          </SheetTitle>
          <SheetDescription className="sr-only">{t('capabilities.description')}</SheetDescription>
          {provider ? (
            <div className="flex items-start gap-3 pt-2">
              <div className="flex size-12 shrink-0 items-center justify-center rounded-xl border bg-muted/30 text-primary">
                <IntegrationProviderIcon
                  integrationId={integrationId}
                  driverId={provider.driver_id}
                  className="size-7"
                />
              </div>
              <div className="min-w-0 flex-1">
                <p className="line-clamp-2 text-sm leading-5 text-muted-foreground">
                  {metadata.providerDescription(provider)}
                </p>
                <p className="mt-2 text-sm font-medium text-foreground">
                  {liveCapabilities
                    ? t('capabilities.connectedSummary', {
                        total: liveCapabilities.summary.total,
                        available: liveCapabilities.summary.available,
                        attention: liveCapabilities.summary.needs_attention,
                      })
                    : t('capabilities.summary', {
                        count: summary.total,
                        access:
                          summary.write > 0
                            ? t('capabilities.access.readWrite')
                            : t('capabilities.access.readOnly'),
                      })}
                </p>
              </div>
            </div>
          ) : null}
        </SheetHeader>

        <div className="min-h-0 flex-1 overflow-y-auto">
          <div className="sticky top-0 z-10 border-b bg-background px-6 py-3">
            <div
              role="tablist"
              aria-label={t('capabilities.filterLabel')}
              className="grid grid-cols-3 overflow-hidden rounded-md border"
            >
              {(['all', 'read', 'write'] as const).map(value => {
                const count =
                  value === 'all' ? summary.total : value === 'read' ? summary.read : summary.write;
                return (
                  <button
                    key={value}
                    type="button"
                    role="tab"
                    aria-selected={filter === value}
                    className={cn(
                      'h-9 border-r px-3 text-xs font-medium transition-colors last:border-r-0',
                      filter === value
                        ? 'bg-primary/5 text-primary shadow-[inset_0_0_0_1px_var(--primary)]'
                        : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground'
                    )}
                    onClick={() => setFilter(value)}
                  >
                    {t(`capabilities.filters.${value}`, { count })}
                  </button>
                );
              })}
            </div>
          </div>

          <div className="space-y-6 px-6 py-5">
            {capabilityQuery.isError ? (
              <div className="rounded-lg border border-warning/30 bg-warning/5 p-4 text-sm text-warning">
                <p className="font-medium">{t('capabilities.loadFailed')}</p>
                <p className="mt-1 text-xs leading-5">{t('capabilities.liveStatusUnavailable')}</p>
              </div>
            ) : null}
            {filteredActions.length === 0 ? (
              <div className="rounded-lg border border-dashed p-8 text-center text-sm text-muted-foreground">
                {t('capabilities.empty')}
              </div>
            ) : (
              <div className="overflow-hidden rounded-lg border">
                {filteredActions.map((action, index) => {
                  const liveCapability = liveCapabilityByAction.get(action.id);
                  const availabilityState =
                    capabilityQuery.isLoading || capabilityQuery.isFetching
                      ? 'checking'
                      : capabilityQuery.isError || !liveCapability
                        ? 'status_unavailable'
                        : liveCapability.availability;
                  return (
                    <details key={action.id} className="group border-b last:border-b-0">
                      <summary className="flex cursor-pointer list-none items-start gap-3 px-4 py-4 marker:hidden hover:bg-muted/20">
                        <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary/5 text-xs font-semibold text-primary">
                          {index + 1}
                        </span>
                        <span className="min-w-0 flex-1">
                          <span className="flex flex-wrap items-center gap-2">
                            <span className="font-medium text-foreground">
                              {metadata.actionName(action)}
                            </span>
                            <Badge variant="outline" className="text-[11px]">
                              {isReadCapability(action)
                                ? t('capabilities.access.read')
                                : t('capabilities.access.write')}
                            </Badge>
                          </span>
                          <span className="mt-1 block font-mono text-[11px] text-muted-foreground">
                            {action.id}
                          </span>
                          <span className="mt-1.5 line-clamp-2 block text-sm leading-5 text-muted-foreground">
                            {metadata.actionDescription(action)}
                          </span>
                          <ProviderCapabilityAvailability
                            className="mt-2"
                            state={availabilityState}
                            compatibleConnectionCount={liveCapability?.compatible_connection_count}
                            showGuidance={false}
                          />
                        </span>
                        <ChevronDown className="mt-1 size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-180" />
                      </summary>
                      <div className="space-y-3 border-t bg-muted/10 px-4 py-4 pl-[3.25rem] text-xs">
                        <ProviderCapabilityAvailability
                          state={availabilityState}
                          compatibleConnectionCount={liveCapability?.compatible_connection_count}
                        />
                        <div>
                          <p className="font-medium text-foreground">
                            {t('capabilities.currentPolicy')}
                          </p>
                          <p className="mt-1.5 text-muted-foreground">
                            {liveCapability
                              ? t('capabilities.currentPolicySummary', {
                                  approval:
                                    liveCapability.approval_policy === 'always_ask'
                                      ? t('capabilities.approvalAlways')
                                      : t('capabilities.approvalInherit'),
                                  egress: !action.data_egress
                                    ? t('capabilities.dataEgressNotRequired')
                                    : liveCapability.data_egress_allowed
                                      ? t('capabilities.dataEgressAllowed')
                                      : t('capabilities.dataEgressBlocked'),
                                })
                              : t('capabilities.currentPolicyUnavailable')}
                          </p>
                        </div>
                        <div>
                          <p className="font-medium text-foreground">
                            {t('capabilities.authentication')}
                          </p>
                          <div className="mt-2 flex flex-wrap gap-1.5">
                            {provider
                              ? resolveIntegrationAuthDefinitions(provider)
                                  .filter(
                                    method =>
                                      !action.supported_auth_method_ids?.length ||
                                      action.supported_auth_method_ids.includes(method.id)
                                  )
                                  .map(method => (
                                    <Badge
                                      key={method.id}
                                      variant="outline"
                                      className="font-normal"
                                    >
                                      {metadata.authMethodLabel(integrationId, method)}
                                    </Badge>
                                  ))
                              : null}
                          </div>
                        </div>
                        <div>
                          <p className="font-medium text-foreground">
                            {t('capabilities.requiredScopes')}
                          </p>
                          <div className="mt-2 flex flex-wrap gap-1.5">
                            {hasActionScopes(action) ? (
                              <>
                                {action.required_scopes?.map(scope => (
                                  <Badge key={scope} variant="subtle" className="font-normal">
                                    {metadata.scope(scope, provider ?? undefined)}
                                  </Badge>
                                ))}
                                {(action.required_any_scopes ?? []).length > 0 ? (
                                  <Badge variant="subtle" className="font-normal">
                                    {action.required_any_scopes
                                      ?.map(scope => metadata.scope(scope, provider ?? undefined))
                                      .join(' / ')}
                                  </Badge>
                                ) : null}
                              </>
                            ) : (
                              <span className="text-muted-foreground">
                                {t('capabilities.noAdditionalScopes')}
                              </span>
                            )}
                          </div>
                        </div>
                        <div className="grid gap-2 sm:grid-cols-2">
                          <span className="text-muted-foreground">
                            {t('capabilities.risk', { risk: metadata.risk(action.risk_level) })}
                          </span>
                          <span className="text-muted-foreground">
                            {t('capabilities.approval', {
                              approval:
                                liveCapability?.approval_policy === 'always_ask'
                                  ? t('capabilities.approvalAlways')
                                  : liveCapability
                                    ? t('capabilities.approvalInherit')
                                    : t('capabilities.currentPolicyUnavailable'),
                            })}
                          </span>
                        </div>
                        {action.data_egress ? (
                          <p className="text-muted-foreground">
                            {t('capabilities.externalDestination', {
                              destination:
                                action.external_destination ?? t('capabilities.unknownDestination'),
                            })}
                          </p>
                        ) : null}
                      </div>
                    </details>
                  );
                })}
              </div>
            )}

            <section className="space-y-3 border-t pt-5">
              <div className="flex items-center gap-2">
                <KeyRound className="size-4 text-muted-foreground" />
                <h3 className="text-sm font-semibold">{t('capabilities.authentication')}</h3>
              </div>
              <div className="flex flex-wrap gap-2">
                {provider
                  ? resolveIntegrationAuthDefinitions(provider).map(auth => (
                      <Badge key={auth.id} variant="outline" className="font-normal">
                        {metadata.authMethodLabel(integrationId, auth)}
                      </Badge>
                    ))
                  : null}
              </div>
            </section>

            <section className="space-y-3 border-t pt-5">
              <div className="flex items-center gap-2">
                <ShieldCheck className="size-4 text-muted-foreground" />
                <h3 className="text-sm font-semibold">{t('capabilities.surfaces')}</h3>
              </div>
              <div className="flex flex-wrap items-center gap-2">
                {supportedCallers.has('aichat') ? (
                  <Badge variant="outline" className="gap-1 text-primary">
                    <Sparkles className="size-3" />
                    AIChat
                  </Badge>
                ) : null}
                {supportedCallers.has('agent') ? (
                  <Badge variant="outline" className="gap-1 text-primary">
                    <Bot className="size-3" />
                    Agent
                  </Badge>
                ) : null}
                {supportedCallers.has('workflow') ? (
                  <Badge variant="outline">{t('capabilities.surface.workflow')}</Badge>
                ) : (
                  <span className="text-xs text-muted-foreground">
                    {t('capabilities.workflowUnavailable')}
                  </span>
                )}
              </div>
            </section>

            <div className="flex gap-2 rounded-lg border border-primary/20 bg-primary/5 p-3 text-xs leading-5 text-primary">
              <Info className="mt-0.5 size-4 shrink-0" />
              <p>{t('capabilities.notice')}</p>
            </div>
          </div>
        </div>

        <SheetFooter className="gap-2 border-t bg-background px-6 py-4 sm:space-x-0">
          {documentationURL ? (
            <Button variant="outline" asChild>
              <a href={documentationURL} target="_blank" rel="noreferrer">
                {t('capabilities.documentation')}
                <ExternalLink className="size-4" />
              </a>
            </Button>
          ) : null}
          <Button
            type="button"
            onClick={() => {
              if (!provider) return;
              onOpenChange(false);
              onConnect(provider);
            }}
            disabled={!provider}
          >
            {t('capabilities.connect', {
              provider: provider ? metadata.providerName(provider) : '',
            })}
            <ArrowRight className="size-4" />
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
