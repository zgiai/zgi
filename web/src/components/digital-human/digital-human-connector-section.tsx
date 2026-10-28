'use client';

import { useEffect, useRef, useState } from 'react';
import { isAxiosError } from 'axios';
import { Plus, PlugZap, RefreshCw } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';
import { toast } from 'sonner';
import { ENABLE_DIGITAL_HUMAN_PER_USER_CONNECTORS } from '@/lib/config';
import { connectorCatalogItems, useConnectorCatalog } from '@/hooks/connectors/use-connectors';
import {
  useDigitalHumanDraftBindings,
  useDigitalHumanBindingCandidates,
  useDigitalHumanMutations,
  useDigitalHuman,
} from '@/hooks/digital-human/use-digital-humans';
import {
  connectorTargetBindingItems,
  useConnectorTargetBindings,
  useUpdateConnectorTargetBindingHostDelegation,
} from '@/hooks';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { useWorkspaceStore } from '@/store/workspace-store';
import type {
  DigitalHumanResourceBinding,
  DigitalHumanResourceBindingInput,
} from '@/services/types/digital-human';
import type { ConnectorHostAudience } from '@/services/types/connector';
import { getErrorMessage } from '@/utils/error-notifications';
import {
  DigitalHumanConnectorCards,
  DigitalHumanConnectorDialog,
} from './digital-human-connector-dialog';
import {
  beginConnectorSelection,
  digitalHumanBindingInput,
  digitalHumanResourceKey,
  isPerUserConnectorSlot,
  observeConnectorDraft,
  toPerUserConnectorSlot,
  updateDigitalHumanOperations,
  type DigitalHumanConnectorSelection,
} from './digital-human-connector-model';
import {
  useZgiMarketSelectedCount,
  ZgiMarketSelectedServers,
} from '@/components/zgi-market/zgi-market-selected-servers';
import { DigitalHumanBackgroundGrants } from './digital-human-background-grants';

interface DigitalHumanConnectorSectionProps {
  id: string;
  /** Published-version preview bindings; present means the draft is read-only. */
  preview?: DigitalHumanResourceBinding[];
  readOnly?: boolean;
}

export function DigitalHumanConnectorSection({
  id,
  preview,
  readOnly = false,
}: DigitalHumanConnectorSectionProps) {
  const account = useAuthStore.use.user();
  const organization = useOrganizationStore.use.currentOrganization();
  const workspace = useWorkspaceStore.use.currentWorkspace();
  const scope = JSON.stringify([account?.id, organization?.id, workspace?.id, id]);
  // Remount on a context switch so no draft session survives across scopes.
  return <ScopedConnectorSection key={scope} id={id} preview={preview} readOnly={readOnly} />;
}

function ScopedConnectorSection({
  id,
  preview,
  readOnly,
}: {
  id: string;
  preview?: DigitalHumanResourceBinding[];
  readOnly: boolean;
}) {
  const t = useT('digitalHumans');
  const connectorT = useT('connectors');
  const query = useDigitalHumanDraftBindings(id, !preview);
  const detailQuery = useDigitalHuman(id);
  const publishedVersionId = detailQuery.data?.data.published?.id;
  const publishedTarget = publishedVersionId
    ? {
        type: 'digital_human' as const,
        id,
        scope: 'published' as const,
        version_id: publishedVersionId,
      }
    : undefined;
  const publishedBindingsQuery = useConnectorTargetBindings(
    publishedTarget,
    Boolean(publishedTarget)
  );
  const updateHostDelegation = useUpdateConnectorTargetBindingHostDelegation();
  const publishedBindings = connectorTargetBindingItems(publishedBindingsQuery.data?.data);
  const previewItems = preview?.map(digitalHumanBindingInput);
  const items = (
    previewItems ?? (query.data?.data.items ?? []).map(digitalHumanBindingInput)
  ).filter(item => item.type === 'connector');
  const candidatesQuery = useDigitalHumanBindingCandidates(
    id,
    { type: 'connector', limit: 512 },
    items.length > 0
  );
  const mutations = useDigitalHumanMutations(id);
  const canUpdatePermission = useAccountPermissions().hasPermission('digital_human.update');
  const canUpdate = canUpdatePermission && !readOnly;
  const marketServerCount = useZgiMarketSelectedCount('digital_human', id);
  const [selection, setSelection] = useState<DigitalHumanConnectorSelection>();
  const [initialReplacement, setInitialReplacement] = useState<DigitalHumanResourceBindingInput>();
  const [opening, setOpening] = useState(false);
  const anchorRef = useRef<HTMLElement | null>(null);
  const currentRef = useRef<DigitalHumanConnectorSelection>();
  const mountedRef = useRef(false);
  const openingRef = useRef(false);
  const changeSelection = (next: DigitalHumanConnectorSelection | undefined) => {
    currentRef.current = next;
    setSelection(next);
  };
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      currentRef.current = undefined;
    };
  }, []);
  useEffect(() => {
    const current = currentRef.current;
    if (!current || !query.data?.data) return;
    const next = observeConnectorDraft(current, query.data.data);
    if (next !== current) changeSelection(next);
  }, [query.data?.data]);

  const open = async (removeId?: string, replaceId?: string, trigger?: HTMLElement | null) => {
    if (!canUpdate || openingRef.current || currentRef.current) return;
    anchorRef.current = trigger ?? null;
    openingRef.current = true;
    setOpening(true);
    try {
      const result = await query.refetch();
      if (mountedRef.current && !result.isError && result.data?.data) {
        const next = beginConnectorSelection(`${id}:${crypto.randomUUID()}`, result.data.data);
        setInitialReplacement(
          replaceId ? next.pending.find(item => item.id === replaceId) : undefined
        );
        if (removeId) next.pending = next.pending.filter(item => item.id !== removeId);
        changeSelection(next);
      }
    } finally {
      openingRef.current = false;
      if (mountedRef.current) setOpening(false);
    }
  };
  const close = () => {
    if (currentRef.current?.status === 'saving') return;
    changeSelection(undefined);
  };
  const save = async () => {
    const current = currentRef.current;
    if (!canUpdate || !current || current.status === 'saving' || current.status === 'conflict') {
      return;
    }
    // A publication or another editor replaced the draft generation this
    // session observed; refuse rather than authorize against stale bindings.
    if (
      query.data?.data.draft_version_id &&
      query.data.data.draft_version_id !== current.baseline.draft_version_id
    ) {
      changeSelection({ ...current, status: 'conflict' });
      return;
    }
    changeSelection({ ...current, status: 'saving' });
    try {
      await mutations.replaceBindings.mutateAsync({
        digitalHumanId: id,
        expectedDraftLockVersion: current.baseline.draft_lock_version,
        expectedDraftVersionId: current.baseline.draft_version_id,
        items: current.pending,
      });
      // The mutation installs the canonical response without downgrading a newer
      // cache value. Late completion never writes into a new picker session.
      if (mountedRef.current && currentRef.current?.token === current.token) {
        changeSelection(undefined);
      }
    } catch (error) {
      if (!mountedRef.current || currentRef.current?.token !== current.token) return;
      const conflict =
        isAxiosError(error) &&
        (error.response?.status === 409 ||
          error.response?.data?.code === 'digital_human.revision.conflict');
      changeSelection({ ...current, status: conflict ? 'conflict' : 'failed' });
    }
  };
  const catalogQuery = useConnectorCatalog(
    undefined,
    ENABLE_DIGITAL_HUMAN_PER_USER_CONNECTORS && items.length > 0
  );
  const catalog = connectorCatalogItems(catalogQuery.data?.data);
  const [modeSaving, setModeSaving] = useState(false);
  // A per-user slot acts as each requester with their own connection. Moving a
  // connection to per-user saves at once; moving back picks a connection.
  const makePerUser = async (item: DigitalHumanResourceBindingInput) => {
    if (!canUpdate || modeSaving || currentRef.current) return;
    setModeSaving(true);
    try {
      const result = await query.refetch();
      const draft = result.data?.data;
      if (!draft) return;
      const name = catalog.find(value => value.id === item.parent_resource_id)?.name ?? '';
      await mutations.replaceBindings.mutateAsync({
        digitalHumanId: id,
        expectedDraftLockVersion: draft.draft_lock_version,
        expectedDraftVersionId: draft.draft_version_id,
        items: draft.items
          .map(digitalHumanBindingInput)
          .map(value =>
            digitalHumanResourceKey(value) === digitalHumanResourceKey(item)
              ? toPerUserConnectorSlot(value, name)
              : value
          ),
      });
      toast.success(t('bindings.connectors.modeSaved'));
    } catch (error) {
      toast.error(getErrorMessage(error));
    } finally {
      if (mountedRef.current) setModeSaving(false);
    }
  };
  const [operationSaving, setOperationSaving] = useState(false);
  // An operation is ticked or cleared right on the connection's card, so an
  // operator opts into the actions that change something where they see them
  // (ADR 0023).
  const changeOperation = async (
    item: DigitalHumanResourceBindingInput,
    operationId: string,
    checked: boolean
  ) => {
    if (!canUpdate || operationSaving || currentRef.current) return;
    setOperationSaving(true);
    try {
      const result = await query.refetch();
      const draft = result.data?.data;
      if (!draft) return;
      const candidate = candidatesQuery.data?.data.items.find(
        value =>
          value.resource_id === item.resource_id &&
          value.parent_resource_id === item.parent_resource_id
      );
      await mutations.replaceBindings.mutateAsync({
        digitalHumanId: id,
        expectedDraftLockVersion: draft.draft_lock_version,
        expectedDraftVersionId: draft.draft_version_id,
        items: updateDigitalHumanOperations(
          draft.items.map(digitalHumanBindingInput),
          item,
          candidate,
          operationId,
          checked
        ),
      });
    } catch (error) {
      toast.error(getErrorMessage(error));
    } finally {
      if (mountedRef.current) setOperationSaving(false);
    }
  };
  const reload = async () => {
    const current = currentRef.current;
    if (!current || current.status === 'saving') return;
    const result = await query.refetch();
    if (
      mountedRef.current &&
      currentRef.current?.token === current.token &&
      !result.isError &&
      result.data?.data
    ) {
      changeSelection(beginConnectorSelection(`${id}:${crypto.randomUUID()}`, result.data.data));
    }
  };
  return (
    <section
      id="digital-human-bindings-connectors"
      className="flex h-full min-h-[280px] max-h-[520px] min-w-0 flex-col overflow-hidden rounded-xl border bg-card shadow-sm"
    >
      <div className="flex items-center justify-between gap-3 border-b bg-muted/15 px-3 py-3">
        <div className="flex min-w-0 items-center gap-2.5">
          <span className="flex size-8 shrink-0 items-center justify-center rounded-lg border bg-background text-primary">
            <PlugZap className="size-4" />
          </span>
          <h3 className="font-medium">{connectorT('title')}</h3>
          <Badge variant="subtle">
            {t('bindings.selectedCount', { count: items.length + marketServerCount })}
          </Badge>
        </div>
        <Button
          type="button"
          variant="outline"
          size="sm"
          isIcon
          aria-label={connectorT('picker.actions.addConnection')}
          aria-haspopup="dialog"
          aria-expanded={Boolean(selection)}
          disabled={!canUpdate || opening || query.isLoading || mutations.replaceBindings.isPending}
          onClick={event => void open(undefined, undefined, event.currentTarget)}
        >
          <Plus className="size-4" />
        </Button>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-3">
        {query.isLoading && !preview ? (
          <div className="h-24 animate-pulse rounded-lg bg-muted" />
        ) : query.isError && !preview ? (
          <div role="alert" className="space-y-2 p-3 text-sm">
            <p>{connectorT('connections.loadFailedTitle')}</p>
            <Button variant="outline" onClick={() => void query.refetch()}>
              <RefreshCw className="size-4" />
              {t('bindings.retry')}
            </Button>
          </div>
        ) : items.length === 0 && marketServerCount === 0 ? (
          <button
            type="button"
            className="flex min-h-full w-full flex-col items-center justify-center rounded-xl border border-dashed bg-muted/10 px-5 py-6 text-center transition-colors hover:border-primary/40 hover:bg-primary/[0.03] disabled:pointer-events-none"
            disabled={!canUpdate || opening}
            onClick={event => void open(undefined, undefined, event.currentTarget)}
          >
            <Plus className="mb-2 size-5 text-muted-foreground" />
            <span className="text-sm font-medium text-foreground">
              {connectorT('picker.actions.addConnection')}
            </span>
            <span className="mt-1 text-xs leading-5 text-muted-foreground">
              {connectorT('connections.emptyDescription')}
            </span>
          </button>
        ) : (
          <>
            {candidatesQuery.isError ? (
              <div role="alert" className="mb-3 text-sm">
                <p>{connectorT('connections.loadFailedTitle')}</p>
                <Button variant="ghost" onClick={() => void candidatesQuery.refetch()}>
                  {connectorT('actions.retry')}
                </Button>
              </div>
            ) : null}
            <DigitalHumanConnectorCards
              items={items}
              candidates={candidatesQuery.data?.data.items ?? []}
              candidatesLoading={candidatesQuery.isLoading}
              candidatesError={candidatesQuery.isError}
              disabled={
                opening || !canUpdate || operationSaving || mutations.replaceBindings.isPending
              }
              onOperation={
                canUpdate && !preview
                  ? (item, operationId, checked) => void changeOperation(item, operationId, checked)
                  : undefined
              }
              onRemove={
                canUpdate ? (item, trigger) => void open(item.id, undefined, trigger) : undefined
              }
              onReplace={
                canUpdate ? (item, trigger) => void open(undefined, item.id, trigger) : undefined
              }
            />
            {ENABLE_DIGITAL_HUMAN_PER_USER_CONNECTORS && canUpdate && !preview
              ? items.map(item => (
                  <div
                    key={`mode:${digitalHumanResourceKey(item)}`}
                    className="mt-3 flex min-w-0 flex-col items-start gap-1.5 text-xs"
                  >
                    <span className="max-w-full truncate text-muted-foreground">
                      {t('bindings.connectors.modeLabel', { name: item.display_name ?? '' })}
                    </span>
                    <ToggleGroup
                      type="single"
                      variant="outline"
                      size="sm"
                      value={isPerUserConnectorSlot(item) ? 'per_user' : 'shared'}
                      disabled={modeSaving || opening || mutations.replaceBindings.isPending}
                      onValueChange={value => {
                        if (value === 'per_user' && !isPerUserConnectorSlot(item)) {
                          void makePerUser(item);
                        } else if (value === 'shared' && isPerUserConnectorSlot(item)) {
                          void open(undefined, item.id);
                        }
                      }}
                    >
                      <ToggleGroupItem value="shared" className="px-2 text-xs">
                        {t('bindings.connectors.modeShared')}
                      </ToggleGroupItem>
                      <ToggleGroupItem value="per_user" className="px-2 text-xs">
                        {t('bindings.connectors.modePerUser')}
                      </ToggleGroupItem>
                    </ToggleGroup>
                  </div>
                ))
              : null}
            {items.map(item => {
              const publishedBinding = publishedBindings.find(
                binding =>
                  binding.connection_id === item.resource_id &&
                  binding.connector_id === item.parent_resource_id
              );
              if (!publishedBinding) return null;
              const audiences = new Set<ConnectorHostAudience>(
                publishedBinding.host_audiences ?? []
              );
              const options: Array<{ value: ConnectorHostAudience; label: string }> = [
                {
                  value: 'workspace',
                  label: connectorT('picker.authorization.hostAudienceWorkspace'),
                },
                { value: 'api', label: connectorT('picker.authorization.hostAudienceApi') },
                {
                  value: 'channel',
                  label: connectorT('picker.authorization.hostAudienceChannel'),
                },
              ];
              return (
                <div
                  key={`${item.parent_resource_id}:${item.resource_id}`}
                  className="mt-4 flex flex-wrap gap-3"
                >
                  {options.map(option => (
                    <label key={option.value} className="flex items-center gap-2 text-xs">
                      <Checkbox
                        checked={audiences.has(option.value)}
                        disabled={!canUpdate || updateHostDelegation.isPending}
                        onCheckedChange={async checked => {
                          const next = new Set(audiences);
                          if (checked === true) next.add(option.value);
                          else next.delete(option.value);
                          try {
                            await updateHostDelegation.mutateAsync({
                              bindingId: publishedBinding.id,
                              revision: publishedBinding.revision,
                              audiences: Array.from(next),
                            });
                            toast.success(connectorT('picker.authorization.hostDelegationSaved'));
                          } catch (error) {
                            toast.error(
                              getErrorMessage(error) ||
                                connectorT('picker.authorization.hostDelegationSaveFailed')
                            );
                          }
                        }}
                      />
                      {option.label}
                    </label>
                  ))}
                </div>
              );
            })}
            <div className={items.length > 0 ? 'mt-2' : undefined}>
              <ZgiMarketSelectedServers
                subjectType="digital_human"
                subjectId={id}
                readOnly={!canUpdate}
              />
            </div>
            {canUpdate ? (
              <Button
                type="button"
                variant="outline"
                className="mt-3 w-full"
                disabled={opening || mutations.replaceBindings.isPending}
                onClick={event => void open(undefined, undefined, event.currentTarget)}
              >
                {connectorT('picker.actions.addConnection')}
              </Button>
            ) : null}
          </>
        )}
        {publishedVersionId ? (
          <DigitalHumanBackgroundGrants
            employeeId={id}
            versionId={publishedVersionId}
            bindings={publishedBindings}
            canView={canUpdatePermission}
            canCreate={canUpdate}
          />
        ) : null}
      </div>
      {selection ? (
        <DigitalHumanConnectorDialog
          key={selection.token}
          id={id}
          selection={selection}
          anchorElement={anchorRef.current}
          readOnly={!canUpdate}
          initialReplacement={initialReplacement}
          onChange={changeSelection}
          onClose={close}
          onSave={save}
          onReload={reload}
        />
      ) : null}
    </section>
  );
}
