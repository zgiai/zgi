'use client';

import { ResourceSelectionDialogContent } from '@/components/chat/ui/resource-selection-dialog-content';

import { groupMailPickerProviders, isMailPickerGroup } from './mail-picker-groups';
import { MailPickerGroupRow } from './mail-picker-group-row';
import { personalConnectionDisplayName } from '../connections/personal-connections-model';

import { listedConnectorGroups, listedConnectorRows } from '../connector-visibility';

import {
  ConnectorUnavailableReferences,
  type ConnectorReferenceRepairProps,
} from '../unavailable-references';

import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { usePendingSetups } from '@/hooks/connectors/use-pending-setups';
import { ConnectorKeySetupPanel } from '@/components/connectors/setup/connector-key-setup-panel';
import { AlertCircle, RefreshCw, SearchX } from 'lucide-react';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { SearchInput } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { useChatConnectorPreferences } from '@/hooks/connectors/use-chat-connector-preferences';
import {
  connectorCatalogItems,
  personalConnectionItems,
  useConnectorCatalog,
  usePersonalConnections,
} from '@/hooks/connectors/use-connectors';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import type { ConnectorCatalogStatus, ConnectorTargetRef } from '@/services/types/connector';
import {
  createConnectorPickerRows,
  filterConnectorPickerRows,
  isConnectorPickerSwitchCurrent,
  requestConnectorPickerAction,
  type ConnectorPickerActionHandlers,
  type ConnectorPickerBindingRef,
  type ConnectorPickerRow,
  type ConnectorPickerSwitchRequest,
} from './connector-picker-model';
import { ConnectorPickerRow as ConnectorPickerRowView } from './connector-picker-row';
import { ConnectorPickerGroupRow } from './connector-picker-group-row';
import {
  createConnectorPickerMenuGroups,
  filterConnectorPickerGroups,
  requestConnectorPickerMenuAccount,
  requestConnectorPickerMenuToggle,
  type ConnectorPickerMenuGroup,
} from './connector-picker-menu-model';

export interface ConnectorPickerDialogProps extends ConnectorReferenceRepairProps {
  open: boolean;
  target: ConnectorTargetRef;
  targetName: string;
  /** Limit choices to one Connector when the host already owns the capability tuple. */
  connectorId?: string;
  bindings: readonly ConnectorPickerBindingRef[];
  /** Chat/Agent group accounts beneath the service; Workflow retains its node picker. */
  groupByConnector?: boolean;
  /** Local first-turn selection: query personal connections without a persisted target. */
  draft?: boolean;
  anchorElement?: HTMLElement | null;
  onOpenChange: (open: boolean) => void;
  onRequestAdd: NonNullable<ConnectorPickerActionHandlers['onRequestAdd']>;
  onRequestSwitch: NonNullable<ConnectorPickerActionHandlers['onRequestSwitch']>;
  onRequestConnectAndAdd: NonNullable<ConnectorPickerActionHandlers['onRequestConnectAndAdd']>;
  onRequestUpdateKey: NonNullable<ConnectorPickerActionHandlers['onRequestUpdateKey']>;
  onRequestViewReason: NonNullable<ConnectorPickerActionHandlers['onRequestViewReason']>;
  onRequestDetach: NonNullable<ConnectorPickerActionHandlers['onRequestDetach']>;
  headerExtra?: ReactNode;
  footerExtra?: ReactNode;
}

const FILTER_VALUES: readonly ConnectorCatalogStatus[] = ['all', 'connected', 'needs_attention'];

export function ConnectorPickerDialog({
  open,
  target,
  targetName,
  connectorId,
  bindings,
  groupByConnector = false,
  draft = false,
  anchorElement,
  onOpenChange,
  onRequestAdd,
  onRequestSwitch,
  onRequestConnectAndAdd,
  onRequestUpdateKey,
  onRequestViewReason,
  onRequestDetach,
  headerExtra,
  footerExtra,
  onReferenceCommand,
  pendingReferenceCommands,
}: ConnectorPickerDialogProps) {
  const t = useT('connectors');
  const { locale } = useLocale();
  const [query, setQuery] = useState('');
  const [status, setStatus] = useState<ConnectorCatalogStatus>('all');
  const [pendingKey, setPendingKey] = useState<string | null>(null);
  const [pendingSwitch, setPendingSwitch] = useState<ConnectorPickerSwitchRequest>();
  const [switchPending, setSwitchPending] = useState(false);
  const [actionFailed, setActionFailed] = useState(false);
  const [resumingID, setResumingID] = useState<string>();
  const pendingQuery = usePendingSetups('feishu', open);
  const pendingSetups = useMemo(() => pendingQuery.data?.data.items ?? [], [pendingQuery.data]);
  const preferencesQuery = useChatConnectorPreferences(open && groupByConnector);
  const preferences = preferencesQuery.isError ? undefined : preferencesQuery.data?.data;
  const busyRef = useRef(false);
  const mountedRef = useRef(false);
  const restoreFocusRef = useRef(true);
  const switchTriggerRef = useRef<HTMLButtonElement | null>(null);
  const scope = JSON.stringify([
    preferencesQuery.scope,
    open,
    target.type,
    target.id,
    target.binding_instance_id,
    connectorId,
  ]);
  const sessionRef = useRef({ scope });
  if (sessionRef.current.scope !== scope) sessionRef.current = { scope };
  const session = sessionRef.current;
  const catalogQuery = useConnectorCatalog(undefined, open);
  const connectionsQuery = usePersonalConnections(
    {
      connector_id: connectorId,
      ...(draft ? {} : { target_type: target.type, target_id: target.id }),
    },
    open && (draft || Boolean(target.id))
  );
  const catalog = useMemo(
    () =>
      connectorCatalogItems(catalogQuery.data?.data).map(item =>
        localizeConnectorCatalogItem(item, locale, {
          description: t('metadata.descriptionUnavailable'),
          capabilityDescription: t('metadata.capabilityDescriptionUnavailable'),
        })
      ),
    [catalogQuery.data?.data, locale, t]
  );
  const connections = personalConnectionItems(connectionsQuery.data?.data);
  const targetLabel = targetName.trim() || t(`picker.targetTypes.${target.type}`);
  const eligibleCatalog = useMemo(
    () => (connectorId ? catalog.filter(item => item.id === connectorId) : catalog),
    [catalog, connectorId]
  );

  useEffect(() => {
    setQuery('');
    setStatus('all');
    setPendingKey(null);
    setPendingSwitch(undefined);
    setSwitchPending(false);
    setActionFailed(false);
    setResumingID(undefined);
    busyRef.current = false;
    if (open) restoreFocusRef.current = true;
  }, [scope, open]);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const rows = useMemo(
    () => listedConnectorRows(createConnectorPickerRows(eligibleCatalog, connections, bindings)),
    [bindings, connections, eligibleCatalog]
  );
  const filteredRows = useMemo(
    () => filterConnectorPickerRows(rows, { query, status }),
    [query, rows, status]
  );
  const nativeRows = filteredRows;
  const allGroups = useMemo(
    () =>
      listedConnectorGroups(
        createConnectorPickerMenuGroups(eligibleCatalog, connections, bindings)
      ),
    [eligibleCatalog, connections, bindings]
  );
  const groups = useMemo(
    () =>
      filterConnectorPickerGroups(
        allGroups,
        { query, status },
        new Set(pendingSetups.map(item => item.connector_id))
      ),
    [allGroups, query, status, pendingSetups]
  );
  const displayGroups = groupMailPickerProviders(groups, t('mail.title'), allGroups);
  const isLoading = catalogQuery.isLoading || connectionsQuery.isLoading;
  const isError = catalogQuery.isError || connectionsQuery.isError;
  const hasResults = groupByConnector ? groups.length > 0 : nativeRows.length > 0;

  const perform = async (key: string, action: () => void | Promise<void>, handoff = false) => {
    if (busyRef.current || !mountedRef.current || sessionRef.current !== session || !open) return;
    busyRef.current = true;
    setActionFailed(false);
    setPendingKey(key);
    restoreFocusRef.current = !handoff;
    try {
      await action();
    } catch {
      if (mountedRef.current && sessionRef.current === session) {
        restoreFocusRef.current = true;
        setPendingSwitch(undefined);
        setActionFailed(true);
      }
    } finally {
      if (mountedRef.current && sessionRef.current === session) {
        busyRef.current = false;
        setPendingKey(null);
        setSwitchPending(false);
      }
    }
  };

  const handlers: ConnectorPickerActionHandlers = {
    onRequestAdd,
    onRequestSwitch: request => {
      setPendingSwitch(request);
    },
    onRequestConnectAndAdd,
    onRequestUpdateKey,
    onRequestViewReason,
    onRequestDetach,
  };
  const runAction = (row: ConnectorPickerRow, trigger?: HTMLButtonElement | null) => {
    if (pendingSwitch) return;
    if (trigger) switchTriggerRef.current = trigger;
    const action = groupByConnector
      ? requestConnectorPickerMenuAccount
      : requestConnectorPickerAction;
    void perform(
      row.key,
      () => action(row, handlers, target),
      row.state === 'not_connected' || row.state === 'reconnect' || row.state === 'unavailable'
    );
  };

  const detachGroup = (group: ConnectorPickerMenuGroup) => {
    if (!group.currentBinding || pendingSwitch) return;
    void perform(group.id, () => requestConnectorPickerMenuToggle(group, handlers, target));
  };

  const confirmSwitch = async () => {
    const request = pendingSwitch;
    if (!request || busyRef.current) return;
    if (!isConnectorPickerSwitchCurrent(request, connections, bindings, target)) {
      setPendingSwitch(undefined);
      setActionFailed(true);
      return;
    }
    setSwitchPending(true);
    await perform(request.connection.id, async () => {
      await onRequestSwitch(request);
      if (mountedRef.current && sessionRef.current === session) {
        setPendingSwitch(undefined);
      }
    });
  };

  const retry = () => {
    void Promise.all([catalogQuery.refetch(), connectionsQuery.refetch()]);
  };

  const resetFilters = () => {
    setQuery('');
    setStatus('all');
  };

  const switchConnectorName = pendingSwitch
    ? localizeConnectorCatalogItem(pendingSwitch.connector, locale).name
    : '';
  const currentConnectionName = pendingSwitch?.replacedConnection
    ? personalConnectionDisplayName(pendingSwitch.replacedConnection, switchConnectorName, locale)
    : switchConnectorName;
  const nextConnectionName = pendingSwitch?.connection
    ? personalConnectionDisplayName(pendingSwitch.connection, switchConnectorName, locale)
    : switchConnectorName;
  const resumingConnector = resumingID
    ? eligibleCatalog.find(item => item.id === 'feishu')
    : undefined;

  const pickerBody = (
    <>
      {open && resumingID && resumingConnector ? (
        <div className="min-h-0 flex-1 overflow-y-auto px-6 pb-5">
          <Button className="mb-4" variant="ghost" onClick={() => setResumingID(undefined)}>
            {t('pending.back')}
          </Button>
          <ConnectorKeySetupPanel
            connector={resumingConnector}
            pendingSetupID={resumingID}
            embedded
            onReady={connection => {
              if (!mountedRef.current || sessionRef.current !== session || !open) return;
              setResumingID(undefined);
              const row = createConnectorPickerRows(
                eligibleCatalog,
                [...connections.filter(item => item.id !== connection.id), connection],
                bindings
              ).find(item => item.connection?.id === connection.id);
              if (row) {
                void perform(row.key, () => requestConnectorPickerAction(row, handlers, target));
              }
            }}
          />
        </div>
      ) : (
        <>
          <div className="shrink-0 space-y-3 border-b px-6 pb-4">
            <SearchInput
              value={query}
              onChange={event => setQuery(event.target.value)}
              placeholder={t('picker.searchPlaceholder')}
              aria-label={t('picker.searchAria')}
              className="h-10 rounded-lg bg-background"
            />
            <div
              className="flex gap-2 overflow-x-auto pb-0.5"
              role="group"
              aria-label={t('picker.filtersLabel')}
            >
              {FILTER_VALUES.map(value => (
                <Button
                  key={value}
                  type="button"
                  size="sm"
                  variant={status === value ? 'default' : 'outline'}
                  className="rounded-full"
                  aria-pressed={status === value}
                  onClick={() => setStatus(value)}
                >
                  {value === 'all'
                    ? t('filters.all')
                    : value === 'connected'
                      ? t('filters.connected')
                      : t('filters.needsAttention')}
                </Button>
              ))}
            </div>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto px-6 py-4">
            {groupByConnector && preferencesQuery.isError ? (
              <div
                role="alert"
                className="mb-3 rounded-lg border border-warning/30 px-3 py-2 text-sm"
              >
                <p>{t('defaults.loadFailed')}</p>
                <Button size="sm" variant="ghost" onClick={() => void preferencesQuery.refetch()}>
                  {t('actions.retry')}
                </Button>
              </div>
            ) : null}
            {actionFailed ? (
              <div
                role="alert"
                className="mb-3 flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm text-destructive"
              >
                <AlertCircle aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
                <span>{t('picker.actionFailed')}</span>
              </div>
            ) : null}

            {isLoading ? (
              <div className="space-y-3" role="status" aria-label={t('picker.loading')}>
                <span className="sr-only">{t('picker.loading')}</span>
                {Array.from({ length: 4 }, (_, index) => (
                  <Skeleton key={index} className="h-[82px] rounded-xl" />
                ))}
              </div>
            ) : null}

            {!isLoading && isError ? (
              <div className="flex min-h-52 flex-col items-center justify-center text-center">
                <AlertCircle aria-hidden="true" className="size-8 text-muted-foreground" />
                <h3 className="mt-3 text-sm font-semibold">{t('picker.loadFailedTitle')}</h3>
                <p className="mt-1 max-w-sm text-sm leading-5 text-muted-foreground">
                  {t('picker.loadFailedDescription')}
                </p>
                <Button type="button" variant="outline" className="mt-4" onClick={retry}>
                  <RefreshCw aria-hidden="true" className="size-4" />
                  {t('actions.retry')}
                </Button>
              </div>
            ) : null}

            {!isLoading && !isError && !hasResults ? (
              <div className="flex min-h-52 flex-col items-center justify-center text-center">
                <SearchX aria-hidden="true" className="size-8 text-muted-foreground" />
                <h3 className="mt-3 text-sm font-semibold">{t('picker.emptyTitle')}</h3>
                <p className="mt-1 max-w-sm text-sm leading-5 text-muted-foreground">
                  {t('picker.emptyDescription')}
                </p>
                <Button type="button" variant="outline" className="mt-4" onClick={resetFilters}>
                  {t('catalog.resetFilters')}
                </Button>
              </div>
            ) : null}

            {!isLoading && !isError && hasResults ? (
              <div className="space-y-3">
                {groupByConnector
                  ? displayGroups.map(group =>
                      isMailPickerGroup(group) ? (
                        <MailPickerGroupRow
                          key={group.id}
                          group={group}
                          targetLabel={targetLabel}
                          disabled={Boolean(pendingKey || pendingSwitch)}
                          preferences={preferences?.defaults}
                          onSelect={runAction}
                          onDetach={detachGroup}
                        />
                      ) : (
                        <ConnectorPickerGroupRow
                          key={group.id}
                          group={group}
                          targetLabel={targetLabel}
                          disabled={Boolean(pendingKey || pendingSwitch)}
                          preference={preferences?.defaults[group.id]}
                          onSelect={runAction}
                          onDetach={detachGroup}
                          pendingSetups={pendingSetups.filter(
                            item => item.connector_id === group.id
                          )}
                          onResume={setResumingID}
                        />
                      )
                    )
                  : nativeRows.map(row => (
                      <ConnectorPickerRowView
                        key={row.key}
                        row={row}
                        pending={Boolean(pendingKey || pendingSwitch)}
                        onAction={item => void runAction(item)}
                      />
                    ))}
              </div>
            ) : null}
          </div>

          <p className="sr-only" role="status" aria-live="polite">
            {pendingKey ? t('picker.updating') : ''}
          </p>
          <ConnectorUnavailableReferences
            target={target}
            enabled={open && !draft}
            onReferenceCommand={onReferenceCommand}
            pendingReferenceCommands={pendingReferenceCommands}
          />
        </>
      )}
    </>
  );

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        {target.type === 'agent' ? (
          <DialogContent
            size="lg"
            className="h-[min(720px,calc(100vh-2rem))] max-w-[min(760px,calc(100vw-2rem))] overflow-hidden p-0"
            onCloseAutoFocus={event => {
              if (anchorElement || !restoreFocusRef.current) {
                event.preventDefault();
                if (restoreFocusRef.current && anchorElement?.isConnected) {
                  anchorElement.focus({ preventScroll: true });
                }
              }
            }}
          >
          <DialogHeader className="shrink-0 pb-3 pr-12">
            <DialogTitle>{t('picker.title')}</DialogTitle>
            <DialogDescription>
                {t(groupByConnector ? 'picker.group.subtitle' : 'picker.subtitle', {
                  target: targetLabel,
              })}
            </DialogDescription>
            {headerExtra}
            </DialogHeader>

            <DialogBody className="flex min-h-0 flex-1 flex-col overflow-hidden p-0">
              {pickerBody}
              {footerExtra}
            </DialogBody>
          </DialogContent>
        ) : (
          <ResourceSelectionDialogContent
            size="lg"
            className="h-[min(720px,calc(100vh-2rem))] max-w-[min(760px,calc(100vw-2rem))] overflow-hidden p-0"
            onCloseAutoFocus={event => {
              if (anchorElement || !restoreFocusRef.current) {
                event.preventDefault();
                if (restoreFocusRef.current && anchorElement?.isConnected) {
                  anchorElement.focus({ preventScroll: true });
                }
              }
            }}
            title={t('picker.title')}
            description={t(groupByConnector ? 'picker.group.subtitle' : 'picker.subtitle', {
              target: targetLabel,
            })}
            headerClassName="shrink-0 pb-3 pr-12"
            bodyClassName="flex min-h-0 flex-1 flex-col overflow-hidden p-0"
            headerExtra={headerExtra}
          >
            {pickerBody}
            {footerExtra}
          </ResourceSelectionDialogContent>
        )}
      </Dialog>
      <ConfirmDialog
        onCloseAutoFocus={event => {
          if (groupByConnector) {
            event.preventDefault();
            if (open && switchTriggerRef.current?.isConnected) switchTriggerRef.current.focus();
          }
        }}
        open={open && Boolean(pendingSwitch)}
        onOpenChange={nextOpen => {
          if (!nextOpen && !switchPending) setPendingSwitch(undefined);
        }}
        title={t('picker.switchTitle', { connector: switchConnectorName })}
        description={t('picker.switchDescription', {
          current: currentConnectionName,
          next: nextConnectionName,
          target: targetLabel,
        })}
        confirmText={t('picker.switchConfirm')}
        cancelText={t('picker.switchCancel')}
        loading={switchPending}
        confirmDisabled={switchPending}
        closeOnConfirm={false}
        onConfirm={() => void confirmSwitch()}
        onCancel={() => {
          if (!switchPending) setPendingSwitch(undefined);
        }}
      />
    </>
  );
}
