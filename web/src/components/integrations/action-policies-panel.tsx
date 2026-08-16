'use client';

import { useEffect, useMemo, useState } from 'react';
import { ShieldCheck } from 'lucide-react';
import { Alert, AlertDescription } from '@/components/ui/alert';
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
import { useT } from '@/i18n';
import {
  integrationCatalogItems,
  useIntegrationActionPolicies,
  useIntegrationCatalog,
  useUpdateIntegrationActionPolicies,
} from '@/hooks';
import type { IntegrationActionPolicy } from '@/services/types/integration';
import { safeOptionalIntegrationDisplayText } from './display-utils';
import { useIntegrationMetadata } from './metadata-i18n';

function integrationID(item: { id: string; integration_id?: string }): string {
  return item.integration_id || item.id;
}

export function IntegrationActionPoliciesPanel({ integrationId }: { integrationId?: string } = {}) {
  const t = useT('integrations');
  const metadata = useIntegrationMetadata();
  const catalogQuery = useIntegrationCatalog(true, 'organization');
  const catalog = integrationCatalogItems(catalogQuery.data?.data).filter(item => item.enabled);
  const [selectedIntegrationId, setSelectedIntegrationId] = useState(integrationId ?? '');
  const policyQuery = useIntegrationActionPolicies(selectedIntegrationId);
  const updateMutation = useUpdateIntegrationActionPolicies(selectedIntegrationId);
  const [draft, setDraft] = useState<IntegrationActionPolicy[]>([]);
  const [baseline, setBaseline] = useState<IntegrationActionPolicy[]>([]);
  const [revision, setRevision] = useState('');
  const isDirty = JSON.stringify(draft) !== JSON.stringify(baseline);

  useEffect(() => {
    if (integrationId) {
      if (selectedIntegrationId !== integrationId) setSelectedIntegrationId(integrationId);
      return;
    }
    if (!selectedIntegrationId && catalog.length > 0) {
      setSelectedIntegrationId(integrationID(catalog[0]));
    }
  }, [catalog, integrationId, selectedIntegrationId]);

  const selectedIntegration = useMemo(
    () => catalog.find(item => integrationID(item) === selectedIntegrationId),
    [catalog, selectedIntegrationId]
  );

  useEffect(() => {
    if (!selectedIntegration) {
      setDraft([]);
      setBaseline([]);
      setRevision('');
      return;
    }
    if (!policyQuery.isSuccess || policyQuery.isFetching || isDirty || updateMutation.isPending) {
      return;
    }
    const saved = new Map(
      (policyQuery.data?.data.items ?? policyQuery.data?.data.policies ?? []).map(policy => [
        policy.action_id,
        policy,
      ])
    );
    const nextPolicies: IntegrationActionPolicy[] = selectedIntegration.actions.map(
      action =>
        saved.get(action.id) ?? {
          integration_id: selectedIntegrationId,
          action_id: action.id,
          enabled: true,
          approval_policy: 'inherit',
          data_egress_allowed: true,
        }
    );
    setDraft(nextPolicies);
    setBaseline(nextPolicies);
    setRevision(policyQuery.data?.data.revision ?? '');
  }, [
    policyQuery.data,
    policyQuery.isFetching,
    policyQuery.isSuccess,
    isDirty,
    selectedIntegration,
    selectedIntegrationId,
    updateMutation.isPending,
  ]);

  const updatePolicy = (actionId: string, update: Partial<IntegrationActionPolicy>) => {
    setDraft(current =>
      current.map(policy => (policy.action_id === actionId ? { ...policy, ...update } : policy))
    );
  };

  return (
    <div className="space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div className="w-full max-w-sm space-y-2">
          <div className="text-sm font-medium">{t('policies.integration')}</div>
          <Select
            value={selectedIntegrationId}
            disabled={Boolean(integrationId)}
            onValueChange={value => {
              setSelectedIntegrationId(value);
              setDraft([]);
              setBaseline([]);
              setRevision('');
            }}
          >
            <SelectTrigger>
              <SelectValue placeholder={t('policies.selectIntegration')} />
            </SelectTrigger>
            <SelectContent>
              {catalog.map(item => (
                <SelectItem key={integrationID(item)} value={integrationID(item)}>
                  {metadata.providerName(item)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button
          disabled={
            !selectedIntegrationId ||
            revision.length !== 64 ||
            draft.length === 0 ||
            !isDirty ||
            !policyQuery.isSuccess ||
            policyQuery.isFetching ||
            policyQuery.isError ||
            updateMutation.isPending
          }
          onClick={() =>
            updateMutation.mutate(
              { revision, policies: draft },
              {
                onSuccess: response => {
                  const savedPolicies = response.data.items ?? response.data.policies ?? [];
                  setDraft(savedPolicies);
                  setBaseline(savedPolicies);
                  setRevision(response.data.revision);
                },
              }
            )
          }
        >
          {t('policies.save')}
        </Button>
      </div>

      <Alert>
        <ShieldCheck className="size-4" />
        <AlertDescription>
          {t('policies.description')} {t('policies.immutable')}
        </AlertDescription>
      </Alert>

      {policyQuery.isLoading || catalogQuery.isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 2 }).map((_, index) => (
            <Skeleton key={index} className="h-32 w-full rounded-lg" />
          ))}
        </div>
      ) : policyQuery.isError ? (
        <div className="rounded-lg border border-destructive/30 p-8 text-center text-sm text-destructive">
          {t('policies.loadFailed')}
        </div>
      ) : draft.length === 0 ? (
        <div className="rounded-lg border p-10 text-center text-sm text-muted-foreground">
          {t('policies.empty')}
        </div>
      ) : (
        <div className="space-y-3">
          {draft.map(policy => {
            const action = selectedIntegration?.actions.find(item => item.id === policy.action_id);
            return (
              <div key={policy.action_id} className="rounded-lg border bg-card p-4">
                <div className="flex flex-wrap items-start justify-between gap-3">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium">
                        {action
                          ? metadata.actionName(action)
                          : metadata.actionNameByID(policy.action_id, t('common.unknownAction'))}
                      </span>
                      {action?.risk_level ? (
                        <Badge variant="secondary">{metadata.risk(action.risk_level)}</Badge>
                      ) : null}
                    </div>
                    {action && metadata.actionDescription(action) ? (
                      <p className="mt-1 text-sm text-muted-foreground">
                        {metadata.actionDescription(action)}
                      </p>
                    ) : null}
                    {safeOptionalIntegrationDisplayText(action?.external_destination) ? (
                      <p className="mt-2 text-xs text-muted-foreground">
                        {safeOptionalIntegrationDisplayText(action?.external_destination)}
                      </p>
                    ) : null}
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-sm">{t('policies.enabled')}</span>
                    <Switch
                      checked={policy.enabled}
                      disabled={updateMutation.isPending}
                      onCheckedChange={enabled => updatePolicy(policy.action_id, { enabled })}
                    />
                  </div>
                </div>
                <div className="mt-4 grid gap-4 border-t pt-4 md:grid-cols-2">
                  <div className="flex items-center justify-between gap-4 rounded-md bg-muted/30 p-3">
                    <span className="text-sm">{t('policies.dataEgress')}</span>
                    <Switch
                      checked={policy.data_egress_allowed}
                      disabled={updateMutation.isPending || !policy.enabled || !action?.data_egress}
                      onCheckedChange={dataEgressAllowed =>
                        updatePolicy(policy.action_id, {
                          data_egress_allowed: dataEgressAllowed,
                        })
                      }
                    />
                  </div>
                  <div className="space-y-1">
                    <div className="text-xs text-muted-foreground">{t('policies.approval')}</div>
                    <Select
                      value={policy.approval_policy}
                      disabled={updateMutation.isPending || !policy.enabled}
                      onValueChange={approvalPolicy =>
                        updatePolicy(policy.action_id, {
                          approval_policy:
                            approvalPolicy as IntegrationActionPolicy['approval_policy'],
                        })
                      }
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="inherit">{t('policies.inherit')}</SelectItem>
                        <SelectItem value="always_ask">{t('policies.alwaysAsk')}</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
