'use client';

import { forwardRef, useEffect, useImperativeHandle, useMemo, useState } from 'react';
import { CheckCircle2, CircleAlert, Loader2, Pencil, Search, ShieldCheck } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
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
  IntegrationActionPolicy,
  IntegrationCatalogItem,
  IntegrationConnection,
} from '@/services/types/integration';
import { actionsForAuthMethod } from './action-auth-compatibility';
import { isReadCapability } from './provider-capability-view';
import { useIntegrationMetadata } from './metadata-i18n';

export interface ConnectionActionSetupState {
  ready: boolean;
  loading: boolean;
  dirty: boolean;
  saving: boolean;
  enabledActionIDs: string[];
}

export interface ConnectionActionSetupHandle {
  save: () => Promise<void>;
}

interface ConnectionActionSetupProps {
  connection: IntegrationConnection;
  provider: IntegrationCatalogItem;
  canManageShared: boolean;
  enabled: boolean;
  onStateChange: (state: ConnectionActionSetupState) => void;
}

function defaultPolicy(
  integrationId: string,
  action: IntegrationActionDefinition
): IntegrationActionPolicy {
  return {
    integration_id: integrationId,
    action_id: action.id,
    enabled: action.default_policy?.enabled ?? true,
    approval_policy:
      action.default_policy?.approval_policy === 'always_ask' ? 'always_ask' : 'inherit',
    data_egress_allowed: action.default_policy?.data_egress_allowed ?? true,
  };
}

export const IntegrationConnectionActionSetup = forwardRef<
  ConnectionActionSetupHandle,
  ConnectionActionSetupProps
>(function IntegrationConnectionActionSetup(
  { connection, provider, canManageShared, enabled, onStateChange },
  ref
) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const integrationId = connection.integration_id;
  const personal = connection.credential_source === 'account';
  const actions = useMemo(
    () => actionsForAuthMethod(provider.actions ?? [], connection.auth_method_id),
    [connection.auth_method_id, provider.actions]
  );
  const policyQuery = useIntegrationActionPolicies(canManageShared && enabled ? integrationId : '');
  const capabilityQuery = useIntegrationProviderCapabilities(
    integrationId,
    personal ? 'account' : 'organization',
    enabled
  );
  const updateMutation = useUpdateIntegrationActionPolicies(integrationId);
  const [draft, setDraft] = useState<IntegrationActionPolicy[]>([]);
  const [baseline, setBaseline] = useState<IntegrationActionPolicy[]>([]);
  const [revision, setRevision] = useState('');
  const dirty = JSON.stringify(draft) !== JSON.stringify(baseline);

  useEffect(() => {
    if (!canManageShared || !policyQuery.isSuccess || policyQuery.isFetching) return;
    if (dirty || updateMutation.isPending) return;
    const saved = new Map(
      (policyQuery.data?.data.items ?? policyQuery.data?.data.policies ?? []).map(policy => [
        policy.action_id,
        policy,
      ])
    );
    const next = actions.map(
      action => saved.get(action.id) ?? defaultPolicy(integrationId, action)
    );
    setDraft(next);
    setBaseline(next);
    setRevision(policyQuery.data?.data.revision ?? '');
  }, [
    actions,
    canManageShared,
    dirty,
    integrationId,
    policyQuery.data,
    policyQuery.isFetching,
    policyQuery.isSuccess,
    updateMutation.isPending,
  ]);

  const livePolicyByAction = useMemo(
    () =>
      new Map(
        (capabilityQuery.data?.data.actions ?? []).map(action => [
          action.id,
          {
            integration_id: integrationId,
            action_id: action.id,
            enabled: action.enabled,
            approval_policy: action.approval_policy,
            data_egress_allowed: action.data_egress_allowed,
          } satisfies IntegrationActionPolicy,
        ])
      ),
    [capabilityQuery.data?.data.actions, integrationId]
  );
  const policyByAction = useMemo(
    () =>
      new Map(
        (canManageShared
          ? draft
          : actions.map(
              action => livePolicyByAction.get(action.id) ?? defaultPolicy(integrationId, action)
            )
        ).map(policy => [policy.action_id, policy])
      ),
    [actions, canManageShared, draft, integrationId, livePolicyByAction]
  );
  const connectionCapabilityByAction = useMemo(
    () =>
      new Map(
        (connection.permission_summary?.adapted_capabilities ?? []).map(capability => [
          capability.action_id,
          capability,
        ])
      ),
    [connection.permission_summary?.adapted_capabilities]
  );
  const rows = useMemo(
    () =>
      actions.map(action => {
        const policy = policyByAction.get(action.id) ?? defaultPolicy(integrationId, action);
        const permission = connectionCapabilityByAction.get(action.id);
        const scopeSatisfied = permission?.scope_satisfied ?? false;
        const policyAllows = policy.enabled && (!action.data_egress || policy.data_egress_allowed);
        return {
          action,
          policy,
          read: isReadCapability(action),
          scopeSatisfied,
          available: scopeSatisfied && policyAllows,
        };
      }),
    [actions, connectionCapabilityByAction, integrationId, policyByAction]
  );
  const loading = canManageShared ? policyQuery.isLoading : capabilityQuery.isLoading;
  const enabledActionIDs = useMemo(
    () => rows.filter(row => row.available).map(row => row.action.id),
    [rows]
  );
  const ready = enabledActionIDs.length > 0;

  useEffect(() => {
    onStateChange({
      ready,
      loading,
      dirty,
      saving: updateMutation.isPending,
      enabledActionIDs,
    });
  }, [dirty, enabledActionIDs, loading, onStateChange, ready, updateMutation.isPending]);

  useImperativeHandle(
    ref,
    () => ({
      save: async () => {
        if (!canManageShared || !dirty) return;
        const response = await updateMutation.mutateAsync({ revision, policies: draft });
        const saved = response.data.items ?? response.data.policies ?? [];
        setDraft(saved);
        setBaseline(saved);
        setRevision(response.data.revision);
      },
    }),
    [canManageShared, dirty, draft, revision, updateMutation]
  );

  const updatePolicy = (action: IntegrationActionDefinition, checked: boolean) => {
    setDraft(current =>
      current.map(policy =>
        policy.action_id === action.id
          ? {
              ...policy,
              enabled: checked,
              data_egress_allowed:
                checked && action.data_egress ? true : policy.data_egress_allowed,
            }
          : policy
      )
    );
  };

  const applyPreset = (mode: 'recommended' | 'read_only') => {
    setDraft(current =>
      current.map(policy => {
        const action = actions.find(candidate => candidate.id === policy.action_id);
        if (!action || !connectionCapabilityByAction.get(action.id)?.scope_satisfied) return policy;
        const shouldEnable = mode === 'recommended' || isReadCapability(action);
        return {
          ...policy,
          enabled: shouldEnable,
          data_egress_allowed:
            shouldEnable && action.data_egress ? true : policy.data_egress_allowed,
        };
      })
    );
  };

  const enabledCount = rows.filter(row => row.available).length;
  const supportedCount = rows.filter(row => row.scopeSatisfied).length;
  const groups = [
    { id: 'read', rows: rows.filter(row => row.read) },
    { id: 'write', rows: rows.filter(row => !row.read) },
  ].filter(group => group.rows.length > 0);

  if (loading) {
    return (
      <div className="space-y-3">
        <Skeleton className="h-20 rounded-xl" />
        <div className="grid gap-3 md:grid-cols-2">
          <Skeleton className="h-44 rounded-xl" />
          <Skeleton className="h-44 rounded-xl" />
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 rounded-xl border bg-muted/20 p-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
            <ShieldCheck className="size-5" />
          </div>
          <div className="min-w-0">
            <p className="font-medium">{t('setup.actions.summaryTitle')}</p>
            <p className="mt-1 text-sm text-muted-foreground">
              {t('setup.actions.summary', { supported: supportedCount, enabled: enabledCount })}
            </p>
          </div>
        </div>
        {canManageShared ? (
          <div className="flex shrink-0 flex-wrap gap-2">
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => applyPreset('read_only')}
            >
              {t('setup.actions.readOnlyPreset')}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => applyPreset('recommended')}
            >
              {t('setup.actions.recommendedPreset')}
            </Button>
          </div>
        ) : null}
      </div>

      <div className="grid gap-4 lg:grid-cols-2">
        {groups.map(group => (
          <section key={group.id} className="overflow-hidden rounded-xl border bg-background">
            <div className="flex items-center justify-between border-b bg-muted/20 px-4 py-3">
              <div>
                <h4 className="text-sm font-semibold">
                  {t(
                    group.id === 'read'
                      ? 'setup.actions.groups.read.title'
                      : 'setup.actions.groups.write.title'
                  )}
                </h4>
                <p className="mt-0.5 text-xs text-muted-foreground">
                  {t(
                    group.id === 'read'
                      ? 'setup.actions.groups.read.description'
                      : 'setup.actions.groups.write.description'
                  )}
                </p>
              </div>
              <Badge variant={group.id === 'write' ? 'warning' : 'subtle'}>
                {t('setup.actions.count', { count: group.rows.length })}
              </Badge>
            </div>
            <div className="divide-y">
              {group.rows.map(({ action, policy, read, scopeSatisfied, available }) => {
                const Icon = read ? Search : Pencil;
                return (
                  <div
                    key={action.id}
                    className={cn(
                      'flex min-h-20 items-center gap-3 px-4 py-3',
                      !scopeSatisfied && 'bg-muted/20'
                    )}
                  >
                    <div
                      className={cn(
                        'flex size-9 shrink-0 items-center justify-center rounded-lg',
                        read ? 'bg-primary/8 text-primary' : 'bg-warning/10 text-warning'
                      )}
                    >
                      <Icon className="size-4" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <p className="truncate text-sm font-medium">
                          {metadata.actionName(action)}
                        </p>
                        {!read ? (
                          <Badge variant="outline" className="font-normal">
                            {t('setup.actions.confirmEveryTime')}
                          </Badge>
                        ) : null}
                      </div>
                      <p className="mt-1 line-clamp-1 text-xs text-muted-foreground">
                        {metadata.actionDescription(action)}
                      </p>
                      {!scopeSatisfied ? (
                        <p className="mt-1 flex items-center gap-1 text-xs text-warning">
                          <CircleAlert className="size-3.5" />
                          {t('setup.actions.scopeMissing')}
                        </p>
                      ) : !canManageShared ? (
                        <p
                          className={cn(
                            'mt-1 flex items-center gap-1 text-xs',
                            available ? 'text-success' : 'text-warning'
                          )}
                        >
                          {available ? (
                            <CheckCircle2 className="size-3.5" />
                          ) : (
                            <CircleAlert className="size-3.5" />
                          )}
                          {t(available ? 'setup.actions.enabled' : 'setup.actions.policyDisabled')}
                        </p>
                      ) : null}
                    </div>
                    {canManageShared ? (
                      <Switch
                        checked={
                          policy.enabled && (!action.data_egress || policy.data_egress_allowed)
                        }
                        disabled={!scopeSatisfied || updateMutation.isPending}
                        onCheckedChange={checked => updatePolicy(action, checked)}
                        aria-label={t('setup.actions.toggle', {
                          action: metadata.actionName(action),
                        })}
                      />
                    ) : null}
                  </div>
                );
              })}
            </div>
          </section>
        ))}
      </div>

      {canManageShared && dirty ? (
        <p className="flex items-center gap-2 text-xs text-primary" role="status">
          {updateMutation.isPending ? (
            <Loader2 className="size-3.5 animate-spin" />
          ) : (
            <CircleAlert className="size-3.5" />
          )}
          {t(updateMutation.isPending ? 'setup.actions.saving' : 'setup.actions.unsaved')}
        </p>
      ) : null}
      {updateMutation.isError ? (
        <p className="text-sm text-destructive" role="alert">
          {t('setup.actions.saveFailed')}
        </p>
      ) : null}
    </div>
  );
});
