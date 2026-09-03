'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { ArrowLeft } from 'lucide-react';
import { toast } from 'sonner';
import { ConnectorPickerDialog } from '@/components/connectors/picker';
import type {
  ConnectorPickerActionRequest,
  ConnectorPickerBindingRef,
} from '@/components/connectors/picker';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import { ConnectorKeySetupPanel } from '@/components/connectors/setup/connector-key-setup-panel';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { useT } from '@/i18n';
import { useLocale } from '@/hooks/use-locale';
import type { AgentConnectorBinding } from '@/services/types/agent';
import type { ConnectorCatalogItem, PersonalConnection } from '@/services/types/connector';
import type { ConnectorReferenceRepairProps } from '@/components/connectors/unavailable-references';
import { normalizeAgentConnectorBindings } from './binding-rebase-merge';

interface AgentRuntimeConnectorDialogProps extends ConnectorReferenceRepairProps {
  agentId: string;
  targetName: string;
  open: boolean;
  anchorElement: HTMLElement | null;
  connectorsEnabled: boolean;
  connectorBindings: AgentConnectorBinding[];
  onOpenChange: (open: boolean) => void;
  onConfirmConnectorBindings: (bindings: AgentConnectorBinding[]) => void;
}

interface AgentConnectorSetupContext {
  connector: ConnectorCatalogItem;
  replacedBinding?: ConnectorPickerBindingRef;
}

function draftBindingRef(
  agentID: string,
  binding: AgentConnectorBinding
): ConnectorPickerBindingRef {
  return {
    id: `agent-draft:${agentID}:${binding.connector_id}:${binding.connection_id}`,
    connection_id: binding.connection_id,
    connector_id: binding.connector_id,
    status: 'active',
  };
}

function connectorDetailURL(request: ConnectorPickerActionRequest): string {
  const params = new URLSearchParams({
    target_type: 'agent',
    target_id: request.target?.id ?? '',
  });
  if (request.connection?.id) params.set('connection_id', request.connection.id);
  return `/console/connectors/${encodeURIComponent(request.connector.id)}?${params.toString()}`;
}

function EnabledAgentRuntimeConnectorDialog({
  agentId,
  targetName,
  open,
  anchorElement,
  connectorBindings,
  onOpenChange,
  onConfirmConnectorBindings,
  onReferenceCommand,
  pendingReferenceCommands,
}: Omit<AgentRuntimeConnectorDialogProps, 'connectorsEnabled'>) {
  const router = useRouter();
  const t = useT('connectors');
  const { locale } = useLocale();
  const [setup, setSetup] = useState<AgentConnectorSetupContext>();
  const pickerFocusRef = useRef({ open, anchorElement });
  pickerFocusRef.current = { open, anchorElement };

  useEffect(() => {
    setSetup(undefined);
  }, [open, agentId]);

  const pickerBindings = useMemo(
    () => connectorBindings.map(binding => draftBindingRef(agentId, binding)),
    [agentId, connectorBindings]
  );
  const selectConnectionInDraft = (
    connector: ConnectorCatalogItem,
    connection: PersonalConnection
  ) => {
    onConfirmConnectorBindings(
      normalizeAgentConnectorBindings([
        ...connectorBindings,
        { connector_id: connector.id, connection_id: connection.id },
      ])
    );
  };

  if (setup) {
    const setupConnector = localizeConnectorCatalogItem(setup.connector, locale);
    return (
      <Dialog
        open={open}
        onOpenChange={nextOpen => {
          if (!nextOpen) setSetup(undefined);
          onOpenChange(nextOpen);
        }}
      >
        <DialogContent
          size="md"
          className="overflow-hidden p-0"
          onCloseAutoFocus={event => {
            event.preventDefault();
            const current = pickerFocusRef.current;
            if (!current.open && current.anchorElement?.isConnected) {
              current.anchorElement.focus({ preventScroll: true });
            }
          }}
        >
          <DialogHeader className="border-b pb-4 pr-12">
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="mb-2 w-fit px-2"
              onClick={() => setSetup(undefined)}
            >
              <ArrowLeft aria-hidden="true" className="size-4" />
              {t('picker.backToChoices')}
            </Button>
            <DialogTitle>{t('picker.setupTitle', { connector: setupConnector.name })}</DialogTitle>
            <DialogDescription>{t('picker.subtitle', { target: targetName })}</DialogDescription>
          </DialogHeader>
          <DialogBody className="p-5">
            <ConnectorKeySetupPanel
              connector={setupConnector}
              submitLabel={t(
                setup.replacedBinding
                  ? 'picker.actions.addConnection'
                  : 'picker.actions.connectAndAdd'
              )}
              onReady={connection => {
                if (
                  setup.replacedBinding ||
                  connectorBindings.some(binding => binding.connector_id === setup.connector.id)
                ) {
                  setSetup(undefined);
                  toast.success(t('picker.connectionReadyNotAdded'));
                  return;
                }
                selectConnectionInDraft(setup.connector, connection);
                setSetup(undefined);
              }}
            />
          </DialogBody>
        </DialogContent>
      </Dialog>
    );
  }

  return (
    <ConnectorPickerDialog
      groupByConnector
      open={open}
      anchorElement={anchorElement}
      target={{ type: 'agent', id: agentId }}
      targetName={targetName}
      bindings={pickerBindings}
      onReferenceCommand={onReferenceCommand}
      pendingReferenceCommands={pendingReferenceCommands}
      onOpenChange={onOpenChange}
      onRequestAdd={request => {
        if (connectorBindings.some(binding => binding.connector_id === request.connector.id)) {
          throw new Error('connector switch confirmation is required');
        }
        selectConnectionInDraft(request.connector, request.connection);
      }}
      onRequestSwitch={request => selectConnectionInDraft(request.connector, request.connection)}
      onRequestConnectAndAdd={request =>
        setSetup({ connector: request.connector, replacedBinding: request.replacedBinding })
      }
      onRequestUpdateKey={request => {
        onOpenChange(false);
        router.push(connectorDetailURL(request));
      }}
      onRequestViewReason={request => {
        onOpenChange(false);
        router.push(connectorDetailURL(request));
      }}
      onRequestDetach={request => {
        onConfirmConnectorBindings(
          connectorBindings.filter(
            binding =>
              !(
                binding.connection_id === request.binding.connection_id &&
                binding.connector_id === request.binding.connector_id
              )
          )
        );
      }}
    />
  );
}

export function AgentRuntimeConnectorDialog(props: AgentRuntimeConnectorDialogProps) {
  return <EnabledAgentRuntimeConnectorDialog {...props} />;
}
