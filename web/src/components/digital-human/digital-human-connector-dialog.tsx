'use client';

import { useEffect, useRef, useState, type ReactNode } from 'react';
import Link from 'next/link';
import { AlertCircle, ArrowRightLeft, RefreshCw, Trash2 } from 'lucide-react';
import { ResourceBindingCard } from '@/components/common/resource-binding';
import {
  ResourceSelectionDialog,
  ResourceSelectionEmptyState,
  ResourceSelectionSkeleton,
} from '@/components/common/resource-selection-dialog';
import { ConnectorIcon } from '@/components/connectors/connector-icon';
import { ConnectorStatusBadge } from '@/components/connectors/connector-status-badge';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import { ConnectionAppDialog } from '@/components/connectors/connections/connection-app-dialog';
import { personalConnectionDisplayName } from '@/components/connectors/connections/personal-connections-model';
import { ConnectionReconnectDialog } from '@/components/connectors/connections/connection-reconnect-dialog';
import {
  supportsAppSetup,
  supportsMessagingContract,
} from '@/components/connectors/setup/app-setup-state';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  connectorCatalogItems,
  personalConnectionItems,
  useConnectorCatalog,
  usePersonalConnections,
} from '@/hooks/connectors/use-connectors';
import { useDigitalHumanBindingCandidates } from '@/hooks/digital-human/use-digital-humans';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import type {
  ConnectorCatalogItem,
  ConnectorReadiness,
  PersonalConnection,
} from '@/services/types/connector';
import type {
  DigitalHumanResourceBindingInput,
  DigitalHumanResourceCandidate,
} from '@/services/types/digital-human';
import {
  connectorOperationIds,
  connectorSelectionChanged,
  connectorSelectionInvalid,
  digitalHumanResourceKey,
  selectDigitalHumanConnection,
  updateDigitalHumanOperations,
  type DigitalHumanConnectorSelection,
} from './digital-human-connector-model';

function connectorReadiness(candidate: DigitalHumanResourceCandidate): ConnectorReadiness {
  if (candidate.available) return 'ready';
  return candidate.status === 'reconnect_required' ? 'reconnect_required' : 'unavailable';
}

/** Stable React key for a candidate that has no draft row identity yet. */
function candidateKey(candidate: DigitalHumanResourceCandidate) {
  return `${candidate.parent_resource_id ?? ''}:${candidate.resource_id}`;
}

interface ConnectorOperationScopeProps {
  item: DigitalHumanResourceBindingInput;
  candidate?: DigitalHumanResourceCandidate;
  connector?: ConnectorCatalogItem;
  checking?: boolean;
  candidatesError?: boolean;
  disabled?: boolean;
  onOperation?: (operationId: string, checked: boolean) => void;
}

/**
 * Operation scope for one bound connection.
 *
 * Only operation identifiers are stored; credential values stay on the server
 * and never enter this draft DTO.
 */
function ConnectorOperationScope({
  item,
  candidate,
  connector,
  checking,
  candidatesError,
  disabled,
  onOperation,
}: ConnectorOperationScopeProps) {
  const t = useT('digitalHumans');
  const connectorT = useT('connectors');
  const selected = connectorOperationIds(item);
  const available = candidate ? connectorOperationIds(candidate) : [];
  const operationIds = [...new Set([...selected, ...(onOperation ? available : [])])];
  return (
    <div className="space-y-2">
      <div className="text-xs font-medium">{t('bindings.integrations.allowedActions')}</div>
      {operationIds.length === 0 ? (
        <p className="text-xs text-destructive">{t('bindings.integrations.noAvailableActions')}</p>
      ) : (
        operationIds.map(operationId => {
          const capability = connector?.capabilities.find(value => value.id === operationId);
          const checked = selected.includes(operationId);
          const usable = available.includes(operationId);
          return (
            <label
              key={operationId}
              className="flex items-start gap-2 rounded-md border p-2 text-xs"
            >
              {onOperation ? (
                <Checkbox
                  checked={checked}
                  disabled={disabled || !candidate?.available || (!checked && !usable)}
                  onCheckedChange={value => onOperation(operationId, value === true)}
                />
              ) : null}
              <span className="min-w-0">
                <span className="block font-medium">{capability?.name || operationId}</span>
                <span className="mt-0.5 block text-muted-foreground">
                  {capability?.description ||
                    connectorT('metadata.capabilityDescriptionUnavailable')}
                </span>
                {!checking && !candidatesError && !usable ? (
                  <span className="mt-1 block text-destructive">
                    {connectorT('detail.capabilityUnavailable')}
                  </span>
                ) : null}
              </span>
            </label>
          );
        })
      )}
      {onOperation && selected.length === 0 ? (
        <p role="alert" className="text-xs text-destructive">
          {t('bindings.integrations.actionRequired')}
        </p>
      ) : null}
    </div>
  );
}

interface ConnectorCardsProps {
  items: DigitalHumanResourceBindingInput[];
  candidates: DigitalHumanResourceCandidate[];
  candidatesLoading?: boolean;
  candidatesError?: boolean;
  disabled?: boolean;
  onRemove?: (item: DigitalHumanResourceBindingInput) => void;
  onReplace?: (item: DigitalHumanResourceBindingInput) => void;
}

/**
 * Bound Connector connections rendered from the versioned digital human draft.
 * Their status and operations are read from the same candidate query the editor
 * uses, so a card can never imply authority the server would reject.
 */
export function DigitalHumanConnectorCards({
  items,
  candidates,
  candidatesLoading = false,
  candidatesError = false,
  disabled,
  onRemove,
  onReplace,
}: ConnectorCardsProps) {
  const t = useT('digitalHumans');
  const connectorT = useT('connectors');
  const { locale } = useLocale();
  const catalogQuery = useConnectorCatalog(undefined, items.length > 0);
  const connectionsQuery = usePersonalConnections(undefined, items.length > 0);
  const catalog = connectorCatalogItems(catalogQuery.data?.data).map(item =>
    localizeConnectorCatalogItem(item, locale, {
      capabilityDescription: connectorT('metadata.capabilityDescriptionUnavailable'),
    })
  );
  const connections = personalConnectionItems(connectionsQuery.data?.data);
  return (
    <div className="space-y-3">
      {catalogQuery.isError || connectionsQuery.isError ? (
        <div role="alert" className="text-sm">
          <p>{connectorT('connections.loadFailedTitle')}</p>
          <Button
            variant="ghost"
            onClick={() => void Promise.all([catalogQuery.refetch(), connectionsQuery.refetch()])}
          >
            {connectorT('actions.retry')}
          </Button>
        </div>
      ) : null}
      {items.map(item => {
        const connector = catalog.find(value => value.id === item.parent_resource_id);
        const connection = connections.find(
          value => value.id === item.resource_id && value.connector_id === item.parent_resource_id
        );
        const candidate = candidates.find(
          value =>
            value.resource_id === item.resource_id &&
            value.parent_resource_id === item.parent_resource_id
        );
        const connectorName =
          connector?.name || item.parent_resource_id || connectorT('connection.connectorFallback');
        const name = connection
          ? personalConnectionDisplayName(connection, connectorName, locale)
          : item.display_name || connectorT('connections.unknownConnection');
        const selected = connectorOperationIds(item);
        const available = candidate ? connectorOperationIds(candidate) : [];
        const checking = candidatesLoading || catalogQuery.isLoading || connectionsQuery.isLoading;
        const unavailable =
          !connector ||
          !connection ||
          connection.state !== 'ready' ||
          candidatesError ||
          !candidate?.available ||
          selected.length === 0 ||
          selected.some(operation => !available.includes(operation));
        return (
          <ResourceBindingCard
            key={digitalHumanResourceKey(item)}
            icon={
              <ConnectorIcon
                connectorId={item.parent_resource_id || ''}
                name={name}
                icon={connector?.icon}
                classification={connector?.classification ?? 'connector'}
                className="size-8 border-0 shadow-none"
              />
            }
            title={name}
            description={connectorName}
            status={
              checking ? (
                <Badge variant="subtle">{connectorT('picker.loading')}</Badge>
              ) : (
                <ConnectorStatusBadge
                  status={
                    unavailable && connection?.state === 'reconnect_required'
                      ? 'reconnect_required'
                      : unavailable
                        ? 'unavailable'
                        : 'ready'
                  }
                />
              )
            }
            action={
              onRemove ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  isIcon
                  disabled={disabled}
                  aria-label={t('bindings.removeNamed', { name })}
                  onClick={() => onRemove(item)}
                >
                  <Trash2 className="size-4" />
                </Button>
              ) : undefined
            }
          >
            <ConnectorOperationScope
              item={item}
              candidate={candidate}
              connector={connector}
              checking={checking}
              candidatesError={candidatesError}
            />
            {onReplace && unavailable ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="mt-2"
                disabled={disabled}
                onClick={() => onReplace(item)}
              >
                <ArrowRightLeft className="size-3.5" />
                {connectorT('picker.actions.switch')}
              </Button>
            ) : null}
          </ResourceBindingCard>
        );
      })}
    </div>
  );
}

interface MaintenanceContext {
  connector: ConnectorCatalogItem;
  connection: PersonalConnection;
}

interface DigitalHumanConnectorDialogProps {
  id: string;
  selection: DigitalHumanConnectorSelection;
  readOnly: boolean;
  initialReplacement?: DigitalHumanResourceBindingInput;
  onChange: (selection: DigitalHumanConnectorSelection) => void;
  onClose: () => void;
  onSave: () => void | Promise<void>;
  onReload: () => void | Promise<void>;
}

/**
 * Digital human Connector selection hosted on the shared candidate and
 * personal-connection contracts. It edits a local draft session; the section
 * owns the versioned save.
 */
export function DigitalHumanConnectorDialog({
  id,
  selection,
  readOnly,
  initialReplacement,
  onChange,
  onClose,
  onSave,
  onReload,
}: DigitalHumanConnectorDialogProps) {
  const t = useT('digitalHumans');
  const connectorT = useT('connectors');
  const { locale } = useLocale();
  const candidatesQuery = useDigitalHumanBindingCandidates(id, { type: 'connector', limit: 512 });
  const catalogQuery = useConnectorCatalog();
  const connectionsQuery = usePersonalConnections();
  const candidates = candidatesQuery.data?.data.items ?? [];
  const catalog = connectorCatalogItems(catalogQuery.data?.data).map(item =>
    localizeConnectorCatalogItem(item, locale, {
      capabilityDescription: connectorT('metadata.capabilityDescriptionUnavailable'),
    })
  );
  const connections = personalConnectionItems(connectionsQuery.data?.data);
  const [query, setQuery] = useState('');
  const [replacement, setReplacement] = useState(initialReplacement);
  const [maintenance, setMaintenance] = useState<MaintenanceContext>();
  const [refreshing, setRefreshing] = useState(false);
  const [actionFailed, setActionFailed] = useState(false);
  const currentRef = useRef(selection);
  currentRef.current = selection;
  const mountedRef = useRef(false);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const busy =
    readOnly || selection.status === 'saving' || selection.status === 'conflict' || refreshing;
  const changeItems = (items: DigitalHumanResourceBindingInput[]) => {
    if (busy) return;
    setActionFailed(false);
    onChange({ ...currentRef.current, pending: items, status: 'editing' });
  };
  const selectedItem = (candidate: DigitalHumanResourceCandidate) =>
    currentRef.current.pending.find(
      value =>
        value.type === 'connector' &&
        value.resource_id === candidate.resource_id &&
        value.parent_resource_id === candidate.parent_resource_id
    );
  const selectedKeys = new Set(
    currentRef.current.pending
      .filter(item => item.type === 'connector')
      .map(item => `${item.parent_resource_id ?? ''}:${item.resource_id}`)
  );
  const visibleCandidates = candidates
    .filter(candidate => candidate.available || selectedKeys.has(candidateKey(candidate)))
    .filter(candidate => {
      const term = query.trim().toLocaleLowerCase();
      if (!term) return true;
      const connector = catalog.find(value => value.id === candidate.parent_resource_id);
      return [candidate.display_name, candidate.description, candidate.resource_id, connector?.name]
        .filter((value): value is string => Boolean(value))
        .some(value => value.toLocaleLowerCase().includes(term));
    });
  const pendingConnectorCount = currentRef.current.pending.filter(
    item => item.type === 'connector'
  ).length;
  const invalid = connectorSelectionInvalid(selection, candidates);
  const changeFailed = selection.status === 'failed' || actionFailed;
  const toggleCandidate = (candidate: DigitalHumanResourceCandidate) => {
    const existing = selectedItem(candidate);
    if (existing) {
      changeItems(
        currentRef.current.pending.filter(
          item => digitalHumanResourceKey(item) !== digitalHumanResourceKey(existing)
        )
      );
      return;
    }
    try {
      changeItems(selectDigitalHumanConnection(currentRef.current.pending, candidate, replacement));
      setReplacement(undefined);
    } catch {
      setActionFailed(true);
    }
  };
  const maintain = (candidate: DigitalHumanResourceCandidate) => {
    if (busy) return;
    const connector = catalog.find(value => value.id === candidate.parent_resource_id);
    const connection = connections.find(
      value =>
        value.id === candidate.resource_id && value.connector_id === candidate.parent_resource_id
    );
    if (!connector || !connection) {
      setActionFailed(true);
      return;
    }
    setMaintenance({ connector, connection });
  };
  const backFromMaintenance = () => {
    setMaintenance(undefined);
    setRefreshing(true);
    void candidatesQuery.refetch().finally(() => {
      if (mountedRef.current) setRefreshing(false);
    });
  };

  if (maintenance) {
    const appSetup =
      supportsAppSetup(maintenance.connector) || supportsMessagingContract(maintenance.connector);
    return appSetup ? (
      <ConnectionAppDialog
        open
        connector={maintenance.connector}
        connection={maintenance.connection}
        onOpenChange={open => {
          if (!open) backFromMaintenance();
        }}
      />
    ) : (
      <ConnectionReconnectDialog
        open
        connector={maintenance.connector}
        connectionId={maintenance.connection.id}
        connection={maintenance.connection}
        onOpenChange={open => {
          if (!open) backFromMaintenance();
        }}
      />
    );
  }

  let body: ReactNode;
  if (candidatesQuery.isLoading || catalogQuery.isLoading || connectionsQuery.isLoading) {
    body = <ResourceSelectionSkeleton />;
  } else if (candidatesQuery.isError) {
    body = (
      <ResourceSelectionEmptyState
        icon={<AlertCircle />}
        title={connectorT('picker.loadFailedTitle')}
        description={connectorT('picker.loadFailedDescription')}
        action={
          <Button variant="outline" size="sm" onClick={() => void candidatesQuery.refetch()}>
            <RefreshCw className="size-4" />
            {connectorT('actions.retry')}
          </Button>
        }
      />
    );
  } else if (visibleCandidates.length === 0) {
    body = (
      <ResourceSelectionEmptyState
        icon={<AlertCircle />}
        title={connectorT('picker.emptyTitle')}
        description={connectorT('picker.emptyDescription')}
        variant={query.trim() ? 'search' : 'resource'}
      />
    );
  } else {
    body = (
      <div className="space-y-3">
        {visibleCandidates.map(candidate => {
          const connector = catalog.find(value => value.id === candidate.parent_resource_id);
          const connection = connections.find(
            value =>
              value.id === candidate.resource_id &&
              value.connector_id === candidate.parent_resource_id
          );
          const item = selectedItem(candidate);
          const connectorName =
            connector?.name ||
            candidate.parent_resource_id ||
            connectorT('connection.connectorFallback');
          const name = connection
            ? personalConnectionDisplayName(connection, connectorName, locale)
            : candidate.display_name || connectorT('connections.unknownConnection');
          const maintenanceAvailable = Boolean(connection) && !candidate.available;
          return (
            <article
              key={candidateKey(candidate)}
              className="rounded-xl border bg-background p-3.5 shadow-xs"
            >
              <div className="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-center">
                <ConnectorIcon
                  connectorId={candidate.parent_resource_id || ''}
                  name={name}
                  icon={connector?.icon}
                  classification={connector?.classification ?? 'connector'}
                />
                <div className="min-w-0 flex-1">
                  <div className="flex min-w-0 flex-wrap items-center gap-2">
                    <h3 className="truncate text-sm font-semibold">{name}</h3>
                    <ConnectorStatusBadge status={connectorReadiness(candidate)} />
                    {item ? (
                      <Badge variant="success">{connectorT('picker.states.inUse')}</Badge>
                    ) : null}
                  </div>
                  <p className="mt-1 truncate text-xs leading-5 text-muted-foreground">
                    {connectorName}
                  </p>
                </div>
                <div className="flex shrink-0 items-center gap-2">
                  {maintenanceAvailable ? (
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={busy}
                      onClick={() => maintain(candidate)}
                    >
                      {connectorT('picker.actions.updateKey')}
                    </Button>
                  ) : null}
                  <Button
                    type="button"
                    size="sm"
                    variant={item ? 'outline' : 'default'}
                    disabled={busy || (!candidate.available && !item)}
                    onClick={() => toggleCandidate(candidate)}
                  >
                    {item ? connectorT('picker.actions.remove') : connectorT('picker.actions.add')}
                  </Button>
                </div>
              </div>
              {item ? (
                <div className="mt-3 border-t pt-3">
                  <ConnectorOperationScope
                    item={item}
                    candidate={candidate}
                    connector={connector}
                    checking={candidatesQuery.isLoading}
                    candidatesError={candidatesQuery.isError}
                    disabled={busy}
                    onOperation={(operationId, checked) =>
                      changeItems(
                        updateDigitalHumanOperations(
                          currentRef.current.pending,
                          item,
                          candidate,
                          operationId,
                          checked
                        )
                      )
                    }
                  />
                </div>
              ) : null}
            </article>
          );
        })}
      </div>
    );
  }

  return (
    <ResourceSelectionDialog
      open
      title={connectorT('picker.title')}
      description={t('bindings.integrations.dialogDescription')}
      selectedCountLabel={t('bindings.selectedCount', { count: pendingConnectorCount })}
      search={query}
      searchPlaceholder={connectorT('picker.searchPlaceholder')}
      onOpenChange={open => {
        if (!open) onClose();
      }}
      onChangeSearch={setQuery}
      toolbar={
        <Button
          variant="outline"
          className="shrink-0"
          disabled={refreshing}
          onClick={() => void candidatesQuery.refetch()}
        >
          <RefreshCw className="size-4" />
          {connectorT('actions.retry')}
        </Button>
      }
      footer={
        <>
          <Button asChild variant="ghost" className="mr-auto">
            <Link href="/console/connectors/connections" target="_blank" rel="noreferrer">
              {connectorT('manageConnections')}
            </Link>
          </Button>
          <Button variant="outline" disabled={selection.status === 'saving'} onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            disabled={busy || invalid || !connectorSelectionChanged(selection)}
            loading={selection.status === 'saving'}
            onClick={() => void onSave()}
          >
            {selection.status === 'saving'
              ? t('bindings.savingSelection')
              : t('bindings.saveSelection')}
          </Button>
        </>
      }
    >
      {selection.status === 'conflict' ? (
        <div role="alert" className="mb-3 rounded-lg border border-warning/30 p-3 text-sm">
          <p>{t('errors.conflict')}</p>
          <Button
            variant="outline"
            className="mt-2"
            disabled={refreshing}
            onClick={() => void onReload()}
          >
            <RefreshCw className="size-4" />
            {t('bindings.retry')}
          </Button>
        </div>
      ) : null}
      {changeFailed ? (
        <p role="alert" className="mb-3 text-sm text-destructive">
          {t('errors.binding')}
        </p>
      ) : null}
      {replacement ? (
        <div
          role="status"
          className="mb-3 flex items-center justify-between gap-2 rounded-lg border p-3 text-sm"
        >
          <p>
            {`${connectorT('picker.actions.switch')}: ${
              replacement.display_name || replacement.resource_id
            }`}
          </p>
          <Button variant="ghost" disabled={busy} onClick={() => setReplacement(undefined)}>
            {t('common.cancel')}
          </Button>
        </div>
      ) : null}
      {body}
      {invalid ? (
        <p role="alert" className="mt-3 text-xs text-destructive">
          {t('bindings.integrations.actionRequired')}
        </p>
      ) : null}
    </ResourceSelectionDialog>
  );
}
