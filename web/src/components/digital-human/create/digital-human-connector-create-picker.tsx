'use client';

import { useMemo, useState } from 'react';
import { ArrowLeft, PlugZap, Plus } from 'lucide-react';
import {
  DigitalHumanConnectorCards,
  DigitalHumanConnectorOperationDetails,
} from '@/components/digital-human/digital-human-connector-dialog';
import { ConnectionAppDialog } from '@/components/connectors/connections/connection-app-dialog';
import { ConnectionReconnectDialog } from '@/components/connectors/connections/connection-reconnect-dialog';
import { ConnectorPickerDialog } from '@/components/connectors/picker';
import type {
  ConnectorPickerActionRequest,
  ConnectorPickerBindingRef,
} from '@/components/connectors/picker';
import { ConnectorKeySetupPanel } from '@/components/connectors/setup/connector-key-setup-panel';
import {
  supportsAppSetup,
  supportsMessagingContract,
} from '@/components/connectors/setup/app-setup-state';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useConnectorCatalog, connectorCatalogItems } from '@/hooks/connectors/use-connectors';
import { useDigitalHumanConnectorCandidates } from '@/hooks/digital-human/use-digital-humans';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import type { ConnectorCatalogItem, PersonalConnection } from '@/services/types/connector';
import {
  addCreateConnectorSelection,
  createConnectorCandidateKey,
  createConnectorCandidateSelectable,
  createConnectorSelectionBindings,
  createConnectorSelectionCandidates,
  removeCreateConnectorSelection,
  setCreateConnectorSelectionApprovalPolicy,
  setCreateConnectorSelectionOperation,
  type DigitalHumanCreateConnectorSelection,
} from './digital-human-connector-create-model';

interface DigitalHumanConnectorCreateSectionProps {
  selections: readonly DigitalHumanCreateConnectorSelection[];
  onChangeSelections: (selections: DigitalHumanCreateConnectorSelection[]) => void;
  disabled?: boolean;
}

/**
 * Create-time host UI. The connector list, setup handoff, and connection state
 * all come from the shared ConnectorPickerDialog; this section only owns the
 * draft selection that will be submitted with the new Digital Human.
 */
export function DigitalHumanConnectorCreateSection({
  selections,
  onChangeSelections,
  disabled = false,
}: DigitalHumanConnectorCreateSectionProps) {
  const t = useT('digitalHumans');
  const connectorT = useT('connectors');
  const [dialogOpen, setDialogOpen] = useState(false);
  const items = useMemo(() => createConnectorSelectionBindings(selections), [selections]);
  const candidates = useMemo(() => createConnectorSelectionCandidates(selections), [selections]);

  return (
    <div className="space-y-2">
      <p className="text-xs leading-5 text-muted-foreground">
        {t('createFlow.form.connectorsHint')}
      </p>
      <section className="flex min-h-[180px] flex-col overflow-hidden rounded-xl border bg-card shadow-sm">
        <div className="flex items-center justify-between gap-3 border-b bg-muted/15 px-3 py-3">
          <div className="flex min-w-0 items-center gap-2.5">
            <span className="flex size-8 shrink-0 items-center justify-center rounded-lg border bg-background text-primary">
              <PlugZap className="size-4" />
            </span>
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              <h2 className="font-medium">{connectorT('title')}</h2>
              <Badge variant="subtle">
                {t('bindings.selectedCount', { count: selections.length })}
              </Badge>
            </div>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            isIcon
            className="size-8 shrink-0"
            aria-label={connectorT('picker.actions.addConnection')}
            aria-haspopup="dialog"
            aria-expanded={dialogOpen}
            disabled={disabled}
            onClick={() => setDialogOpen(true)}
          >
            <Plus className="size-4" />
          </Button>
        </div>
        <div className="min-h-0 flex-1 p-3">
          {selections.length === 0 ? (
            <button
              type="button"
              className="flex min-h-[130px] w-full flex-col items-center justify-center rounded-xl border border-dashed bg-muted/10 px-5 py-6 text-center transition-colors motion-reduce:transition-none hover:border-primary/40 hover:bg-primary/[0.03] disabled:pointer-events-none"
              disabled={disabled}
              onClick={() => setDialogOpen(true)}
            >
              <Plus className="mb-2 size-5 text-muted-foreground" />
              <span className="text-sm font-medium text-foreground">
                {connectorT('picker.actions.addConnection')}
              </span>
              <span className="mt-1 text-xs leading-5 text-muted-foreground">
                {t('createFlow.form.connectorsOptional')}
              </span>
            </button>
          ) : (
            <DigitalHumanConnectorCards
              items={items}
              candidates={candidates}
              disabled={disabled}
              onRemove={item =>
                onChangeSelections([
                  ...removeCreateConnectorSelection(selections, createConnectorCandidateKey(item)),
                ])
              }
              onOperation={(item, operationId, checked) =>
                onChangeSelections([
                  ...setCreateConnectorSelectionOperation(
                    selections,
                    createConnectorCandidateKey(item),
                    operationId,
                    checked
                  ),
                ])
              }
              onApprovalPolicy={(item, policy) =>
                onChangeSelections([
                  ...setCreateConnectorSelectionApprovalPolicy(
                    selections,
                    createConnectorCandidateKey(item),
                    policy
                  ),
                ])
              }
            />
          )}
        </div>
      </section>
      {dialogOpen ? (
        <DigitalHumanConnectorCreateDialog
          selections={selections}
          onClose={() => setDialogOpen(false)}
          onSave={next => {
            onChangeSelections(next);
            setDialogOpen(false);
          }}
        />
      ) : null}
    </div>
  );
}

interface DigitalHumanConnectorCreateDialogProps {
  selections: readonly DigitalHumanCreateConnectorSelection[];
  onSave: (selections: DigitalHumanCreateConnectorSelection[]) => void;
  onClose: () => void;
}

interface SetupState {
  connector: ConnectorCatalogItem;
  connection?: PersonalConnection;
}

/**
 * Draft creation uses the completed ConnectorPicker host adapter. It does not
 * persist a target until the Digital Human exists; the picker only edits local
 * snapshots and the create flow submits them later.
 */
function DigitalHumanConnectorCreateDialog({
  selections,
  onSave,
  onClose,
}: DigitalHumanConnectorCreateDialogProps) {
  const t = useT('digitalHumans');
  const connectorT = useT('connectors');
  const { locale } = useLocale();
  const [pending, setPending] =
    useState<readonly DigitalHumanCreateConnectorSelection[]>(selections);
  const [setup, setSetup] = useState<SetupState>();
  const [setupFailed, setSetupFailed] = useState(false);
  const candidatesQuery = useDigitalHumanConnectorCandidates({ limit: 512 });
  const catalogQuery = useConnectorCatalog();
  const candidates = candidatesQuery.data?.data.items ?? [];
  const catalog = connectorCatalogItems(catalogQuery.data?.data).map(item =>
    localizeConnectorCatalogItem(item, locale)
  );
  const pendingBindings = useMemo(
    () =>
      pending.map<ConnectorPickerBindingRef>(item => ({
        id: `create:${createConnectorCandidateKey(item.candidate)}`,
        connector_id: item.binding.parent_resource_id ?? '',
        connection_id: item.binding.resource_id,
        status: 'active',
      })),
    [pending]
  );
  const hostEligibility = {
    status:
      catalogQuery.isError || candidatesQuery.isError
        ? ('error' as const)
        : catalogQuery.isLoading || candidatesQuery.isLoading
          ? ('loading' as const)
          : ('ready' as const),
    allowedConnectorIds: catalog
      .filter(item => item.supported_surfaces?.includes('digital_human'))
      .map(item => item.id),
    connections: Object.fromEntries(
      candidates.map(candidate => [
        candidate.resource_id,
        {
          available: candidate.available && createConnectorCandidateSelectable(candidate),
          reason: t('bindings.connectors.unavailable'),
        },
      ])
    ),
    onRetry: () => Promise.all([catalogQuery.refetch(), candidatesQuery.refetch()]),
  };

  const add = (request: ConnectorPickerActionRequest & { connection: PersonalConnection }) => {
    const candidate = candidates.find(
      item =>
        item.resource_id === request.connection.id &&
        item.parent_resource_id === request.connector.id
    );
    if (!candidate) throw new Error('Connection candidate unavailable');
    setPending(current => addCreateConnectorSelection(current, candidate));
  };

  const ready = async (connection: PersonalConnection) => {
    setSetupFailed(false);
    const result = await candidatesQuery.refetch();
    const candidate = result.data?.data.items.find(
      item =>
        item.resource_id === connection.id && item.parent_resource_id === connection.connector_id
    );
    if (!candidate || !createConnectorCandidateSelectable(candidate)) {
      setSetupFailed(true);
      return;
    }
    setPending(current => addCreateConnectorSelection(current, candidate));
    setSetup(undefined);
  };

  const maintain = (request: ConnectorPickerActionRequest) => {
    if (request.connection) {
      setSetup({
        connector: localizeConnectorCatalogItem(request.connector, locale),
        connection: request.connection,
      });
    } else {
      setSetupFailed(true);
    }
  };

  if (setup?.connection) {
    const onMaintenanceClose = (open: boolean) => {
      if (!open) {
        setSetup(undefined);
        void candidatesQuery.refetch();
      }
    };
    return supportsAppSetup(setup.connector) || supportsMessagingContract(setup.connector) ? (
      <ConnectionAppDialog
        open
        connector={setup.connector}
        connection={setup.connection}
        onOpenChange={onMaintenanceClose}
      />
    ) : (
      <ConnectionReconnectDialog
        open
        connector={setup.connector}
        connectionId={setup.connection.id}
        connection={setup.connection}
        onOpenChange={onMaintenanceClose}
      />
    );
  }

  if (setup) {
    return (
      <Dialog open onOpenChange={open => (open ? undefined : onClose())}>
        <DialogContent size="md">
          <DialogHeader>
            <Button
              type="button"
              variant="ghost"
              className="mb-2 w-fit"
              onClick={() => setSetup(undefined)}
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
              embedded
              onReady={connection => void ready(connection)}
            />
          </DialogBody>
        </DialogContent>
      </Dialog>
    );
  }

  return (
    <ConnectorPickerDialog
      open
      draft
      target={{ type: 'digital_human', id: 'draft', scope: 'draft' }}
      targetName={t('bindings.connectors.target')}
      selectionMode="multiple-connections"
      groupByConnector
      bindings={pendingBindings}
      hostEligibility={hostEligibility}
      showUnavailableReferences={false}
      onOpenChange={open => {
        if (!open) onClose();
      }}
      onRequestAdd={add}
      onRequestSwitch={() => {
        throw new Error('Draft Digital Human connections never switch an existing slot');
      }}
      onRequestConnectAndAdd={request => {
        setSetupFailed(false);
        setSetup({ connector: localizeConnectorCatalogItem(request.connector, locale) });
      }}
      onRequestUpdateKey={maintain}
      onRequestViewReason={() => setSetupFailed(true)}
      renderConnectionDetails={row => {
        const item = pending.find(
          value =>
            value.binding.resource_id === row.connection?.id &&
            value.binding.parent_resource_id === row.connector?.id
        );
        if (!item) return null;
        const candidate = candidates.find(
          value =>
            value.resource_id === item.binding.resource_id &&
            value.parent_resource_id === item.binding.parent_resource_id
        );
        return (
          <DigitalHumanConnectorOperationDetails
            item={item.binding}
            candidate={candidate}
            connector={row.connector}
            name={row.connection?.name || item.candidate.display_name}
            checking={candidatesQuery.isLoading}
            candidatesError={candidatesQuery.isError}
            onOperation={(operation, checked) =>
              setPending(current =>
                setCreateConnectorSelectionOperation(
                  current,
                  createConnectorCandidateKey(item.candidate),
                  operation,
                  checked
                )
              )
            }
            onApprovalPolicy={policy =>
              setPending(current =>
                setCreateConnectorSelectionApprovalPolicy(
                  current,
                  createConnectorCandidateKey(item.candidate),
                  policy
                )
              )
            }
          />
        );
      }}
      onRequestDetach={request => {
        setPending(current =>
          removeCreateConnectorSelection(
            current,
            `${request.binding.connector_id}:${request.binding.connection_id}`
          )
        );
      }}
      footer={
        <div className="flex w-full flex-wrap items-center justify-end gap-2">
          {setupFailed ? (
            <p role="alert" className="mr-auto text-sm text-destructive">
              {t('bindings.connectors.unavailable')}
            </p>
          ) : null}
          <Button variant="outline" onClick={onClose}>
            {t('common.cancel')}
          </Button>
          <Button onClick={() => onSave([...pending])}>{t('bindings.saveSelection')}</Button>
        </div>
      }
    >
      {pending.length > 0 ? (
        <div className="px-6 pb-4">
          <p className="mb-2 text-xs text-muted-foreground">
            {t('bindings.connectors.operationsHint')}
          </p>
        </div>
      ) : null}
    </ConnectorPickerDialog>
  );
}
