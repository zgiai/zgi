'use client';

import { useEffect, useRef, useState, type MouseEvent } from 'react';
import { useRouter } from 'next/navigation';
import { ArrowLeft, ArrowRightLeft, RefreshCw, Trash2 } from 'lucide-react';
import { ResourceBindingCard } from '@/components/common/resource-binding';
import { ConnectorIcon } from '@/components/connectors/connector-icon';
import { ConnectorStatusBadge } from '@/components/connectors/connector-status-badge';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import { ConnectionAppDialog } from '@/components/connectors/connections/connection-app-dialog';
import { ConnectionReconnectDialog } from '@/components/connectors/connections/connection-reconnect-dialog';
import { ConnectorPickerDialog } from '@/components/connectors/picker';
import { ZgiMarketConnectorPickerGroup } from '@/components/zgi-market/zgi-market-connector-picker-group';
import type {
  ConnectorPickerActionRequest,
  ConnectorPickerBindingRef,
} from '@/components/connectors/picker';
import { ConnectorKeySetupPanel } from '@/components/connectors/setup/connector-key-setup-panel';
import {
  supportsAppSetup,
  supportsMessagingContract,
} from '@/components/connectors/setup/app-setup-state';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  connectorCatalogItems,
  personalConnectionItems,
  useConnectorCatalog,
  usePersonalConnections,
} from '@/hooks/connectors/use-connectors';
import { useDigitalHumanBindingCandidates } from '@/hooks/digital-human/use-digital-humans';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import type { ConnectorCatalogItem, PersonalConnection } from '@/services/types/connector';
import type {
  DigitalHumanResourceBindingInput,
  DigitalHumanResourceCandidate,
} from '@/services/types/digital-human';
import {
  connectorOperationIds,
  connectorSelectionChanged,
  connectorSelectionInvalid,
  digitalHumanResourceKey,
  isPerUserConnectorSlot,
  selectDigitalHumanConnection,
  updateDigitalHumanOperations,
  type DigitalHumanConnectorSelection,
} from './digital-human-connector-model';

interface ConnectorCardsProps {
  items: DigitalHumanResourceBindingInput[];
  candidates: DigitalHumanResourceCandidate[];
  candidatesLoading?: boolean;
  candidatesError?: boolean;
  disabled?: boolean;
  onRemove?: (item: DigitalHumanResourceBindingInput, trigger: HTMLElement) => void;
  onReplace?: (item: DigitalHumanResourceBindingInput, trigger: HTMLElement) => void;
  onOperation?: (
    item: DigitalHumanResourceBindingInput,
    operationId: string,
    checked: boolean
  ) => void;
}

export function DigitalHumanConnectorOperationDetails({
  item,
  candidate,
  connector,
  name,
  disabled,
  checking,
  candidatesError,
  onOperation,
}: {
  item: DigitalHumanResourceBindingInput;
  candidate?: DigitalHumanResourceCandidate;
  connector?: ConnectorCatalogItem;
  name?: string;
  disabled?: boolean;
  checking?: boolean;
  candidatesError?: boolean;
  onOperation?: (operationId: string, checked: boolean) => void;
}) {
  const t = useT('digitalHumans');
  const connectorT = useT('connectors');
  const selected = connectorOperationIds(item);
  const available = candidate ? connectorOperationIds(candidate) : [];
  const operations = [...new Set([...selected, ...(onOperation ? available : [])])];
  return (
    <details className="text-xs">
      <summary className="cursor-pointer py-1 font-medium">
        {name ? `${name} · ` : null}
        {t('bindings.connectors.operations', { count: selected.length })}
      </summary>
      <p className="my-2 break-all text-muted-foreground">
        {t('bindings.connectors.identity', { id: item.resource_id })}
      </p>
      <div className="space-y-2">
        {operations.map(operationId => {
          const operation = connector?.capabilities.find(value => value.id === operationId);
          const checked = selected.includes(operationId);
          return (
            <label key={operationId} className="flex items-start gap-2 rounded-md border p-2">
              {onOperation ? (
                <Checkbox
                  checked={checked}
                  disabled={
                    disabled ||
                    checking ||
                    candidatesError ||
                    !candidate?.available ||
                    (!checked && !available.includes(operationId))
                  }
                  onCheckedChange={value => onOperation(operationId, value === true)}
                />
              ) : null}
              <span className="min-w-0">
                <span className="block font-medium">{operation?.name || operationId}</span>
                <span className="mt-0.5 block text-muted-foreground">
                  {operation?.description ||
                    connectorT('metadata.capabilityDescriptionUnavailable')}
                </span>
                {!checking && !candidatesError && !available.includes(operationId) ? (
                  <span className="mt-1 block text-destructive">
                    {t('bindings.connectors.operationUnavailable')}
                  </span>
                ) : null}
              </span>
            </label>
          );
        })}
        {onOperation && selected.length === 0 ? (
          <p role="alert" className="text-destructive">
            {t('bindings.connectors.operationRequired')}
          </p>
        ) : null}
      </div>
    </details>
  );
}

export function DigitalHumanConnectorCards({
  items,
  candidates,
  candidatesLoading = false,
  candidatesError = false,
  disabled,
  onRemove,
  onReplace,
  onOperation,
}: ConnectorCardsProps) {
  const t = useT('digitalHumans');
  const connectorT = useT('connectors');
  const { locale } = useLocale();
  const catalogQuery = useConnectorCatalog(undefined, items.length > 0);
  const connectionsQuery = usePersonalConnections(undefined, items.length > 0);
  const catalog = connectorCatalogItems(catalogQuery.data?.data).map(item =>
    localizeConnectorCatalogItem(item, locale)
  );
  const connections = personalConnectionItems(connectionsQuery.data?.data);
  return (
    <div className="space-y-3">
      {catalogQuery.isError || connectionsQuery.isError ? (
        <div role="alert" className="text-sm">
          <p>{t('bindings.connectors.loadFailed')}</p>
          <Button
            variant="ghost"
            onClick={() => void Promise.all([catalogQuery.refetch(), connectionsQuery.refetch()])}
          >
            {t('bindings.retry')}
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
        const legacy = item.type === 'integration_connection';
        const perUser = isPerUserConnectorSlot(item);
        const selected = connectorOperationIds(item);
        const available = candidate ? connectorOperationIds(candidate) : [];
        const checking = candidatesLoading || catalogQuery.isLoading || connectionsQuery.isLoading;
        const unavailable = perUser
          ? !connector ||
            !connector.supported_surfaces?.includes('digital_human') ||
            selected.length === 0
          : legacy ||
            !connector ||
            !connector.supported_surfaces?.includes('digital_human') ||
            !connection ||
            connection.state !== 'ready' ||
            candidatesError ||
            !candidate?.available ||
            selected.length === 0 ||
            selected.some(operation => !available.includes(operation));
        const name = perUser
          ? item.display_name || connector?.name || t('bindings.connectors.unnamed')
          : connection?.name || item.display_name || t('bindings.connectors.unnamed');
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
            description={
              perUser
                ? t('bindings.connectors.perUserDescription')
                : connection?.external_display_name
                  ? `${connector?.name || item.parent_resource_id} · ${connection.external_display_name}`
                  : connector?.name || item.parent_resource_id || t('bindings.connectors.legacy')
            }
            status={
              legacy ? (
                <Badge variant="outline">{t('bindings.connectors.needsReplacement')}</Badge>
              ) : perUser && !unavailable ? (
                <Badge variant="subtle">{t('bindings.connectors.perUserBadge')}</Badge>
              ) : checking ? (
                <Badge variant="subtle">{connectorT('picker.loading')}</Badge>
              ) : (
                <ConnectorStatusBadge
                  status={
                    unavailable
                      ? connection?.state === 'reconnect_required'
                        ? 'reconnect_required'
                        : 'unavailable'
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
                  onClick={event => onRemove(item, event.currentTarget)}
                >
                  <Trash2 className="size-4" />
                </Button>
              ) : undefined
            }
          >
            {legacy ? (
              <p className="text-xs text-muted-foreground">
                {t('bindings.connectors.legacyDescription')}
              </p>
            ) : (
              <DigitalHumanConnectorOperationDetails
                item={item}
                candidate={candidate}
                connector={connector}
                checking={checking}
                candidatesError={candidatesError}
                disabled={disabled}
                onOperation={
                  onOperation
                    ? (operationId, checked) => onOperation(item, operationId, checked)
                    : undefined
                }
              />
            )}
            {onReplace && unavailable ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                className="mt-2"
                disabled={disabled}
                onClick={event => onReplace(item, event.currentTarget)}
              >
                <ArrowRightLeft className="size-3.5" />
                {t('bindings.connectors.replace')}
              </Button>
            ) : null}
          </ResourceBindingCard>
        );
      })}
    </div>
  );
}

interface DigitalHumanConnectorDialogProps {
  id: string;
  selection: DigitalHumanConnectorSelection;
  anchorElement: HTMLElement | null;
  readOnly: boolean;
  initialReplacement?: DigitalHumanResourceBindingInput;
  onChange: (selection: DigitalHumanConnectorSelection) => void;
  onClose: () => void;
  onSave: () => void | Promise<void>;
  onReload: () => void | Promise<void>;
}

export function DigitalHumanConnectorDialog({
  id,
  selection,
  anchorElement,
  readOnly,
  initialReplacement,
  onChange,
  onClose,
  onSave,
  onReload,
}: DigitalHumanConnectorDialogProps) {
  const router = useRouter();
  const t = useT('digitalHumans');
  const connectorT = useT('connectors');
  const { locale } = useLocale();
  const catalogQuery = useConnectorCatalog();
  const candidatesQuery = useDigitalHumanBindingCandidates(id, { type: 'connector', limit: 512 });
  const candidates = candidatesQuery.data?.data.items ?? [];
  const [setup, setSetup] = useState<{
    connector: ConnectorCatalogItem;
    connection?: PersonalConnection;
  }>();
  const [replacement, setReplacement] = useState(initialReplacement);
  const [setupFailed, setSetupFailed] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [navigationUrl, setNavigationUrl] = useState<string>();
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
  const hostEligibility = {
    status:
      catalogQuery.isError || candidatesQuery.isError
        ? ('error' as const)
        : catalogQuery.isLoading || candidatesQuery.isLoading
          ? ('loading' as const)
          : ('ready' as const),
    allowedConnectorIds: connectorCatalogItems(catalogQuery.data?.data)
      .filter(item => item.supported_surfaces?.includes('digital_human'))
      .map(item => item.id),
    connections: Object.fromEntries(
      candidates.map(candidate => [
        candidate.resource_id,
        {
          available: candidate.available && connectorOperationIds(candidate).length > 0,
          reason: t('bindings.connectors.unavailable'),
        },
      ])
    ),
    onRetry: () => Promise.all([catalogQuery.refetch(), candidatesQuery.refetch()]),
  };
  const pickerBindings: ConnectorPickerBindingRef[] = selection.pending
    .filter(item => item.type === 'connector')
    .map(item => ({
      id: digitalHumanResourceKey(item),
      connector_id: item.parent_resource_id || '',
      connection_id: item.resource_id,
      status: 'active',
    }));
  const changeItems = (items: DigitalHumanResourceBindingInput[]) => {
    if (busy) return;
    onChange({ ...currentRef.current, pending: items, status: 'editing' });
  };
  const add = (request: ConnectorPickerActionRequest & { connection: PersonalConnection }) => {
    if (busy) return;
    const candidate = candidates.find(
      value =>
        value.resource_id === request.connection.id &&
        value.parent_resource_id === request.connector.id
    );
    if (!candidate) throw new Error('Connection candidate unavailable');
    changeItems(selectDigitalHumanConnection(currentRef.current.pending, candidate, replacement));
    setReplacement(undefined);
  };
  const maintain = (request: ConnectorPickerActionRequest) => {
    if (busy) return;
    const connector = localizeConnectorCatalogItem(request.connector, locale);
    if (request.connection) setSetup({ connector, connection: request.connection });
    else setSetupFailed(true);
  };
  const ready = async (connection: PersonalConnection) => {
    const token = selection.token;
    if (busy) return;
    setRefreshing(true);
    setSetupFailed(false);
    const result = await candidatesQuery.refetch();
    if (!mountedRef.current || currentRef.current.token !== token) return;
    setRefreshing(false);
    if (
      currentRef.current.status === 'conflict' ||
      currentRef.current.status === 'saving' ||
      result.isError
    ) {
      setSetupFailed(true);
      return;
    }
    const candidate = result.data?.data.items.find(
      value =>
        value.resource_id === connection.id && value.parent_resource_id === connection.connector_id
    );
    if (!candidate?.available || !connectorOperationIds(candidate).length) {
      setSetupFailed(true);
      return;
    }
    try {
      onChange({
        ...currentRef.current,
        pending: selectDigitalHumanConnection(currentRef.current.pending, candidate, replacement),
        status: 'editing',
      });
      setSetup(undefined);
      setReplacement(undefined);
    } catch {
      setSetupFailed(true);
    }
  };
  const backFromMaintenance = () => {
    setSetup(undefined);
    void candidatesQuery.refetch();
  };

  const captureNavigation = (event: MouseEvent<HTMLDivElement>) => {
    const anchor = event.target instanceof Element ? event.target.closest('a') : null;
    const href = anchor?.getAttribute('href');
    if (!href?.startsWith('/console/') || anchor?.target === '_blank') return;
    event.preventDefault();
    event.stopPropagation();
    if (refreshing) return;
    if (connectorSelectionChanged(currentRef.current)) setNavigationUrl(href);
    else {
      onClose();
      router.push(href);
    }
  };
  const navigationDialog = (
    <ConfirmDialog
      open={Boolean(navigationUrl)}
      onOpenChange={open => {
        if (!open) setNavigationUrl(undefined);
      }}
      title={t('bindings.connectors.leaveTitle')}
      description={t('bindings.connectors.leaveDescription')}
      confirmText={t('bindings.connectors.leaveConfirm')}
      cancelText={t('common.cancel')}
      onConfirm={() => {
        if (!navigationUrl) return;
        onClose();
        router.push(navigationUrl);
      }}
    />
  );

  if (setup?.connection) {
    return (
      <div onClickCapture={captureNavigation}>
        {supportsAppSetup(setup.connector) || supportsMessagingContract(setup.connector) ? (
          <ConnectionAppDialog
            open
            connector={setup.connector}
            connection={setup.connection}
            onOpenChange={open => {
              if (!open) backFromMaintenance();
            }}
          />
        ) : (
          <ConnectionReconnectDialog
            open
            connector={setup.connector}
            connectionId={setup.connection.id}
            onOpenChange={open => {
              if (!open) backFromMaintenance();
            }}
          />
        )}
        {navigationDialog}
      </div>
    );
  }
  if (setup) {
    return (
      <div onClickCapture={captureNavigation}>
        <Dialog
          open
          onOpenChange={open => {
            if (!open && !refreshing) onClose();
          }}
        >
          <DialogContent
            size="md"
            onCloseAutoFocus={event => {
              event.preventDefault();
              if (mountedRef.current && anchorElement?.isConnected) {
                anchorElement.focus({ preventScroll: true });
              }
            }}
          >
            <DialogHeader>
              <Button
                variant="ghost"
                className="mb-2 w-fit"
                onClick={() => setSetup(undefined)}
                disabled={refreshing}
              >
                <ArrowLeft className="size-4" />
                {connectorT('picker.backToChoices')}
              </Button>
              <DialogTitle>
                {connectorT('picker.setupTitle', { connector: setup.connector.name })}
              </DialogTitle>
              <DialogDescription>{t('bindings.connectors.setupHint')}</DialogDescription>
            </DialogHeader>
            <DialogBody>
              {setupFailed ? (
                <p role="alert" className="mb-3 text-sm text-destructive">
                  {t('bindings.connectors.setupNotAdded')}
                </p>
              ) : null}
              <ConnectorKeySetupPanel
                connector={setup.connector}
                target={{ type: 'digital_human', id, scope: 'draft' }}
                embedded
                onReady={connection => void ready(connection)}
              />
            </DialogBody>
          </DialogContent>
        </Dialog>
        {navigationDialog}
      </div>
    );
  }

  const invalid = connectorSelectionInvalid(selection, candidates);
  return (
    <ConnectorPickerDialog
      open
      target={{ type: 'digital_human', id, scope: 'draft' }}
      targetName={t('bindings.connectors.target')}
      selectionMode="multiple-connections"
      groupByConnector
      bindings={pickerBindings}
      anchorElement={anchorElement}
      extraGroups={filter => (
        <ZgiMarketConnectorPickerGroup
          subjectType="digital_human"
          subjectId={id}
          readOnly={readOnly}
          filter={filter}
        />
      )}
      disabled={selection.status === 'saving' || refreshing}
      actionsDisabled={readOnly || selection.status === 'conflict'}
      hostEligibility={hostEligibility}
      renderConnectionDetails={row => {
        const item = currentRef.current.pending.find(
          value =>
            value.type === 'connector' &&
            value.resource_id === row.connection?.id &&
            value.parent_resource_id === row.connector?.id
        );
        if (!item) return null;
        const candidate = candidates.find(
          value =>
            value.resource_id === item.resource_id &&
            value.parent_resource_id === item.parent_resource_id
        );
        return (
          <DigitalHumanConnectorOperationDetails
            item={item}
            candidate={candidate}
            connector={row.connector}
            name={row.connection?.name || item.display_name}
            checking={candidatesQuery.isLoading}
            candidatesError={candidatesQuery.isError}
            disabled={busy}
            onOperation={(operation, checked) =>
              changeItems(
                updateDigitalHumanOperations(
                  currentRef.current.pending,
                  item,
                  candidate,
                  operation,
                  checked
                )
              )
            }
          />
        );
      }}
      showUnavailableReferences={false}
      onOpenChange={open => {
        if (!open) onClose();
      }}
      onRequestAdd={add}
      onRequestSwitch={() => {
        throw new Error('Digital human connections require an explicit source');
      }}
      onRequestDetach={request =>
        changeItems(
          currentRef.current.pending.filter(
            item =>
              !(
                item.type === 'connector' &&
                item.resource_id === request.binding.connection_id &&
                item.parent_resource_id === request.binding.connector_id
              )
          )
        )
      }
      onRequestConnectAndAdd={request => {
        if (!busy) {
          setSetupFailed(false);
          setSetup({ connector: localizeConnectorCatalogItem(request.connector, locale) });
        }
      }}
      onRequestUpdateKey={maintain}
      onRequestViewReason={() => setSetupFailed(true)}
      footer={
        <div className="flex w-full flex-wrap items-center justify-end gap-2">
          <Button variant="outline" disabled={selection.status === 'saving'} onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button
            disabled={
              busy ||
              hostEligibility.status !== 'ready' ||
              invalid ||
              !connectorSelectionChanged(selection)
            }
            loading={selection.status === 'saving'}
            onClick={() => void onSave()}
          >
            {selection.status === 'saving'
              ? t('bindings.savingSelection')
              : t('bindings.saveSelection')}
          </Button>
        </div>
      }
    >
      {selection.status === 'conflict' ? (
        <div role="alert" className="my-3 rounded-lg border border-warning/30 p-3 text-sm">
          <p>{t('bindings.connectors.conflict')}</p>
          <Button variant="outline" className="mt-2" onClick={() => void onReload()}>
            <RefreshCw className="size-4" />
            {t('bindings.connectors.reload')}
          </Button>
        </div>
      ) : null}
      {selection.status === 'failed' || setupFailed ? (
        <p role="alert" className="my-3 text-sm text-destructive">
          {t(
            selection.status === 'failed'
              ? 'bindings.connectors.saveFailed'
              : 'bindings.connectors.unavailable'
          )}
        </p>
      ) : null}
      {replacement ? (
        <div
          role="status"
          className="my-3 flex items-center justify-between gap-2 rounded-lg border p-3 text-sm"
        >
          <p>
            {t('bindings.connectors.replacing', {
              name: replacement.display_name || replacement.resource_id,
            })}
          </p>
          <Button variant="ghost" onClick={() => setReplacement(undefined)}>
            {t('common.cancel')}
          </Button>
        </div>
      ) : null}
      {invalid ? (
        <p role="alert" className="mt-3 text-xs text-destructive">
          {t('bindings.connectors.selectionInvalid')}
        </p>
      ) : null}
    </ConnectorPickerDialog>
  );
}
