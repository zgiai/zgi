'use client';

import { useEffect, useMemo, useState } from 'react';
import { AlertCircle, Check, PlugZap, RefreshCw } from 'lucide-react';
import { useQuery } from '@tanstack/react-query';
import { safeIntegrationDisplayText } from '@/components/integrations/display-utils';
import { actionSupportsAuthMethod } from '@/components/integrations/action-auth-compatibility';
import { useIntegrationMetadata } from '@/components/integrations/metadata-i18n';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Pagination } from '@/components/ui/pagination';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { integrationCatalogItems, useIntegrationCatalog } from '@/hooks';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import agentService from '@/services/agent.service';
import type {
  AgentIntegrationConnectionBinding,
  AgentIntegrationConnectionCandidate,
} from '@/services/types/integration';
import {
  AgentRuntimeSelectionDialog,
  AgentRuntimeSelectionEmptyState,
  AgentRuntimeSelectionGrid,
  AgentRuntimeSelectionSkeleton,
} from './selection-dialog';
import { useSelectionDialogDraftGuard } from './use-selection-dialog-draft-guard';
import { normalizeAgentIntegrationBindings } from './binding-rebase-merge';

interface AgentRuntimeIntegrationDialogProps {
  agentId: string;
  open: boolean;
  bindings: AgentIntegrationConnectionBinding[];
  onOpenChange: (open: boolean) => void;
  onConfirmBindings: (bindings: AgentIntegrationConnectionBinding[]) => void;
}

const CANDIDATE_PAGE_SIZE = 24;

export function AgentRuntimeIntegrationDialog({
  agentId,
  open,
  bindings,
  onOpenChange,
  onConfirmBindings,
}: AgentRuntimeIntegrationDialogProps) {
  const t = useT('agents.agentRuntime');
  const integrationMetadata = useIntegrationMetadata();
  const catalogQuery = useIntegrationCatalog(true, 'shared');
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState('');
  const candidateQuery = useQuery({
    queryKey: [
      ...AGENT_KEYS.integrationConnectionCandidates(agentId),
      'dialog',
      page,
      search.trim(),
    ],
    queryFn: () =>
      agentService.getAgentIntegrationConnectionCandidates(agentId, {
        query: search.trim() || undefined,
        page,
        limit: CANDIDATE_PAGE_SIZE,
        include_selected: true,
      }),
    enabled: open && Boolean(agentId),
    staleTime: 30_000,
    retry: false,
  });
  const [draft, setDraft] = useState<AgentIntegrationConnectionBinding[]>([]);

  useEffect(() => {
    if (open) {
      setDraft(normalizeAgentIntegrationBindings(bindings));
      setSearch('');
      setPage(1);
    }
  }, [bindings, open]);

  const candidates = useMemo(
    () => candidateQuery.data?.data.data ?? [],
    [candidateQuery.data?.data.data]
  );
  const candidateResponse = candidateQuery.data?.data;
  const candidateTotal = candidateResponse?.total ?? candidates.length;
  const candidateTotalPages = Math.max(1, Math.ceil(candidateTotal / CANDIDATE_PAGE_SIZE));
  const catalogItems = useMemo(
    () => integrationCatalogItems(catalogQuery.data?.data),
    [catalogQuery.data?.data]
  );
  const catalogByIntegration = useMemo(
    () => new Map(catalogItems.map(item => [item.integration_id || item.id, item])),
    [catalogItems]
  );
  const actionsByIntegration = useMemo(
    () => new Map(catalogItems.map(item => [item.integration_id || item.id, item.actions])),
    [catalogItems]
  );
  const draftByIntegration = useMemo(
    () => new Map(draft.map(binding => [binding.integration_id, binding])),
    [draft]
  );
  const original = useMemo(() => normalizeAgentIntegrationBindings(bindings), [bindings]);
  const isDirty =
    JSON.stringify(original) !== JSON.stringify(normalizeAgentIntegrationBindings(draft));
  const commit = () => onConfirmBindings(normalizeAgentIntegrationBindings(draft));
  const { requestOpenChange, requestClose, saveAndClose, closeGuard } =
    useSelectionDialogDraftGuard({ open, isDirty, onOpenChange, onSave: commit });

  const chooseConnection = (candidate: AgentIntegrationConnectionCandidate) => {
    const current = draftByIntegration.get(candidate.integration_id);
    if (current?.connection_id === candidate.connection_id) {
      setDraft(items => items.filter(item => item.integration_id !== candidate.integration_id));
      return;
    }
    const availableAccessMode = candidate.available_access_mode;
    if (
      candidate.status !== 'active' ||
      !availableAccessMode ||
      candidate.available_action_ids.length === 0
    ) {
      return;
    }
    const availableActionIDs = new Set(candidate.available_action_ids);
    const availableActions = (actionsByIntegration.get(candidate.integration_id) ?? []).filter(
      action =>
        availableActionIDs.has(action.id) &&
        actionSupportsAuthMethod(action, candidate.auth_method_id)
    );
    if (availableActions.length === 0) return;
    const readActionIDs = availableActions
      .filter(action => action.effect === 'read')
      .map(action => action.id);
    const actionIds =
      readActionIDs.length > 0 ? readActionIDs : availableActions.map(action => action.id);
    setDraft(items => [
      ...items.filter(item => item.integration_id !== candidate.integration_id),
      {
        connection_id: candidate.connection_id,
        integration_id: candidate.integration_id,
        access_mode: readActionIDs.length > 0 ? 'read' : availableAccessMode,
        allowed_action_ids: actionIds,
      },
    ]);
  };

  const toggleAction = (
    integrationId: string,
    actionId: string,
    checked: boolean,
    actionEffects: Map<string, string | undefined>
  ) => {
    setDraft(items =>
      items.map(binding =>
        binding.integration_id !== integrationId
          ? binding
          : (() => {
              const allowedActionIDs = checked
                ? Array.from(new Set([...binding.allowed_action_ids, actionId]))
                : binding.allowed_action_ids.filter(id => id !== actionId);
              return {
                ...binding,
                access_mode: allowedActionIDs.some(id => actionEffects.get(id) !== 'read')
                  ? ('write' as const)
                  : ('read' as const),
                allowed_action_ids: allowedActionIDs,
              };
            })()
      )
    );
  };
  const hasEmptyActions = draft.some(binding => binding.allowed_action_ids.length === 0);
  const visibleCandidatesByConnection = useMemo(
    () => new Map(candidates.map(candidate => [candidate.connection_id, candidate])),
    [candidates]
  );
  const hasUnavailableActions = draft.some(binding => {
    const candidate = visibleCandidatesByConnection.get(binding.connection_id);
    if (!candidate) return false;
    const available = new Set(
      (actionsByIntegration.get(candidate.integration_id) ?? [])
        .filter(
          action =>
            candidate.available_action_ids.includes(action.id) &&
            actionSupportsAuthMethod(action, candidate.auth_method_id)
        )
        .map(action => action.id)
    );
    return (
      candidate.status !== 'active' ||
      !candidate.available_access_mode ||
      (binding.access_mode === 'write' && candidate.available_access_mode !== 'write') ||
      binding.allowed_action_ids.some(actionId => !available.has(actionId))
    );
  });

  return (
    <>
      <AgentRuntimeSelectionDialog
        open={open}
        title={t('integration.dialogTitle')}
        description={t('integration.dialogDescription')}
        selectedCount={draft.length}
        search={search}
        searchPlaceholder={t('integration.searchPlaceholder')}
        onChangeSearch={value => {
          setSearch(value);
          setPage(1);
        }}
        onOpenChange={requestOpenChange}
        footer={
          <>
            <Button type="button" variant="ghost" onClick={requestClose}>
              {t('integration.cancel')}
            </Button>
            <Button
              type="button"
              disabled={hasEmptyActions || hasUnavailableActions}
              onClick={saveAndClose}
            >
              {t('integration.confirm')}
            </Button>
          </>
        }
      >
        {candidateQuery.isLoading || catalogQuery.isLoading ? (
          <AgentRuntimeSelectionSkeleton />
        ) : candidateQuery.isError || catalogQuery.isError ? (
          <AgentRuntimeSelectionEmptyState
            icon={<AlertCircle />}
            title={t('integration.loadFailedTitle')}
            description={t('integration.loadFailedDescription')}
            action={
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => {
                  void candidateQuery.refetch();
                  void catalogQuery.refetch();
                }}
              >
                <RefreshCw className="size-4" />
                {t('integration.retryLoad')}
              </Button>
            }
          />
        ) : candidates.length === 0 ? (
          <AgentRuntimeSelectionEmptyState
            icon={<PlugZap />}
            title={t('integration.emptyTitle')}
            description={t('integration.emptyDescription')}
          />
        ) : (
          <div className="space-y-4">
            <AgentRuntimeSelectionGrid>
              {candidates.map(candidate => {
                const selected =
                  draftByIntegration.get(candidate.integration_id)?.connection_id ===
                  candidate.connection_id;
                const binding = selected
                  ? draftByIntegration.get(candidate.integration_id)
                  : undefined;
                const availableActionIDs = new Set(candidate.available_action_ids);
                const actions = (actionsByIntegration.get(candidate.integration_id) ?? []).filter(
                  action =>
                    (availableActionIDs.has(action.id) &&
                      actionSupportsAuthMethod(action, candidate.auth_method_id)) ||
                    Boolean(binding?.allowed_action_ids.includes(action.id))
                );
                const actionEffects = new Map(actions.map(action => [action.id, action.effect]));
                const connectionName = safeIntegrationDisplayText(
                  candidate.name,
                  t('integration.unnamedConnection')
                );
                const catalogItem = catalogByIntegration.get(candidate.integration_id);
                const integrationName = catalogItem
                  ? integrationMetadata.providerName(
                      catalogItem,
                      t('integration.unknownExternalApp')
                    )
                  : t('integration.unknownExternalApp');
                return (
                  <div
                    key={candidate.connection_id}
                    className={cn(
                      'rounded-lg border bg-background p-3.5 shadow-sm transition-colors',
                      selected && 'border-primary bg-primary/5',
                      candidate.status !== 'active' && 'opacity-60'
                    )}
                  >
                    <button
                      type="button"
                      className="flex w-full items-start gap-3 text-left"
                      disabled={candidate.status !== 'active' && !selected}
                      onClick={() => chooseConnection(candidate)}
                    >
                      <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-primary">
                        <PlugZap className="size-4" />
                      </span>
                      <span className="min-w-0 flex-1">
                        <span className="block truncate text-sm font-semibold">
                          {connectionName}
                        </span>
                        <span className="mt-1 block text-xs text-muted-foreground">
                          {t('integration.connectionStatus', {
                            integration: integrationName,
                            status: t(`integration.status.${candidate.status}`),
                          })}
                        </span>
                      </span>
                      <span
                        className={cn(
                          'flex size-5 shrink-0 items-center justify-center rounded-full border',
                          selected && 'border-primary bg-primary text-primary-foreground'
                        )}
                      >
                        {selected ? <Check className="size-3.5" /> : null}
                      </span>
                    </button>
                    {selected ? (
                      <div className="mt-3 space-y-2 border-t pt-3">
                        <div className="text-xs font-medium">{t('integration.allowedActions')}</div>
                        {actions.map(action => {
                          const checked = Boolean(binding?.allowed_action_ids.includes(action.id));
                          const available =
                            availableActionIDs.has(action.id) &&
                            actionSupportsAuthMethod(action, candidate.auth_method_id);
                          const actionName = integrationMetadata.actionName(
                            action,
                            t('integration.unknownAction')
                          );
                          const actionDescription =
                            integrationMetadata.actionDescription(action) ??
                            t('integration.noActionDescription');
                          return (
                            <label
                              key={action.id}
                              className={cn(
                                'flex items-start gap-2 rounded p-1.5',
                                available || checked
                                  ? 'cursor-pointer hover:bg-muted/50'
                                  : 'cursor-not-allowed opacity-60'
                              )}
                            >
                              <Checkbox
                                checked={checked}
                                disabled={!available && !checked}
                                onCheckedChange={value =>
                                  toggleAction(
                                    candidate.integration_id,
                                    action.id,
                                    value === true,
                                    actionEffects
                                  )
                                }
                              />
                              <span className="min-w-0">
                                <span className="block text-xs font-medium">{actionName}</span>
                                <span className="mt-0.5 block text-[11px] leading-4 text-muted-foreground">
                                  {actionDescription}
                                </span>
                                <span className="mt-1 flex flex-wrap gap-1">
                                  {!available ? (
                                    <Badge variant="destructive" className="text-[10px]">
                                      {t('integration.actionUnavailable')}
                                    </Badge>
                                  ) : null}
                                </span>
                              </span>
                            </label>
                          );
                        })}
                        {binding?.allowed_action_ids.length === 0 ? (
                          <p className="text-xs text-destructive">
                            {t('integration.actionRequired')}
                          </p>
                        ) : null}
                      </div>
                    ) : null}
                  </div>
                );
              })}
            </AgentRuntimeSelectionGrid>
            {candidateTotalPages > 1 ? (
              <Pagination
                currentPage={page}
                totalPages={candidateTotalPages}
                total={candidateTotal}
                pageSize={CANDIDATE_PAGE_SIZE}
                onPageChange={setPage}
                showInfo={false}
              />
            ) : null}
          </div>
        )}
      </AgentRuntimeSelectionDialog>
      {closeGuard}
    </>
  );
}
