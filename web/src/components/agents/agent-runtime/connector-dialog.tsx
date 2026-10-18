'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { ArrowLeft } from 'lucide-react';
import { toast } from 'sonner';
import { ConnectorPickerDialog } from '@/components/connectors/picker';
import { ZgiMarketConnectorPickerGroup } from '@/components/zgi-market/zgi-market-connector-picker-group';
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
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
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

interface AgentConnectorAuthorizationContext {
  connector: ConnectorCatalogItem;
  connection: PersonalConnection;
}

function draftBindingRef(
  agentID: string,
  binding: AgentConnectorBinding
): ConnectorPickerBindingRef {
  return {
    id:
      binding.binding_id ||
      `agent-draft:${agentID}:${binding.connector_id}:${binding.connection_id}`,
    connection_id: binding.connection_id,
    connector_id: binding.connector_id,
    status: 'active',
    revision: binding.revision,
    authorization_mode: binding.authorization_mode,
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
  const [authorization, setAuthorization] = useState<AgentConnectorAuthorizationContext>();
  const [authorizationMode, setAuthorizationMode] = useState<'ask_each_time' | 'direct'>(
    'ask_each_time'
  );
  const pickerFocusRef = useRef({ open, anchorElement });
  pickerFocusRef.current = { open, anchorElement };

  useEffect(() => {
    setSetup(undefined);
    setAuthorization(undefined);
    setAuthorizationMode('ask_each_time');
  }, [open, agentId]);

  const pickerBindings = useMemo(
    () => connectorBindings.map(binding => draftBindingRef(agentId, binding)),
    [agentId, connectorBindings]
  );
  const beginAuthorization = (connector: ConnectorCatalogItem, connection: PersonalConnection) => {
    const current = connectorBindings.find(binding => binding.connector_id === connector.id);
    setAuthorizationMode(current?.authorization_mode === 'direct' ? 'direct' : 'ask_each_time');
    setAuthorization({ connector, connection });
  };

  if (authorization) {
    const connectionName =
      authorization.connection.name ||
      authorization.connection.external_display_name ||
      authorization.connector.name;
    return (
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent size="sm" className="overflow-hidden p-0">
          <DialogHeader className="border-b pb-4 pr-12">
            <DialogTitle>{t('picker.authorization.addToAgentTitle')}</DialogTitle>
            <DialogDescription>
              {t('picker.authorization.selectedConnection', { connection: connectionName })}
            </DialogDescription>
          </DialogHeader>
          <DialogBody className="space-y-4 p-5">
            <div className="space-y-2 rounded-lg border border-primary/30 bg-primary/5 p-3">
              <div className="flex items-center justify-between gap-3">
                <Label htmlFor="agent-connector-authorization-mode">
                  {t('picker.authorization.modeLabel')}
                </Label>
                <span className="text-xs text-muted-foreground">
                  {t('picker.authorization.required')}
                </span>
              </div>
              <Select
                value={authorizationMode}
                onValueChange={value =>
                  setAuthorizationMode(value === 'direct' ? 'direct' : 'ask_each_time')
                }
              >
                <SelectTrigger id="agent-connector-authorization-mode">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="ask_each_time">
                    {t('picker.authorization.askShort')}
                  </SelectItem>
                  <SelectItem value="direct">{t('picker.authorization.directShort')}</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <p className="rounded-lg bg-primary/10 px-3 py-2 text-xs leading-5 text-primary">
              {t('picker.authorization.agentScopeHint')}
            </p>
            <div className="flex justify-end gap-2">
              <Button type="button" variant="outline" onClick={() => setAuthorization(undefined)}>
                {t('picker.authorization.cancel')}
              </Button>
              <Button
                type="button"
                onClick={() => {
                  const next = normalizeAgentConnectorBindings([
                    ...connectorBindings.filter(
                      binding => binding.connector_id !== authorization.connector.id
                    ),
                    {
                      connector_id: authorization.connector.id,
                      connection_id: authorization.connection.id,
                      authorization_mode: authorizationMode,
                    },
                  ]);
                  onConfirmConnectorBindings(next);
                  setAuthorization(undefined);
                  onOpenChange(false);
                }}
              >
                {t('picker.authorization.confirmAdd')}
              </Button>
            </div>
          </DialogBody>
        </DialogContent>
      </Dialog>
    );
  }

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
              embedded
              target={{ type: 'agent', id: agentId }}
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
                beginAuthorization(setup.connector, connection);
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
        beginAuthorization(request.connector, request.connection);
      }}
      onRequestSwitch={request => beginAuthorization(request.connector, request.connection)}
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
      extraGroups={filter => (
        <ZgiMarketConnectorPickerGroup subjectType="agent" subjectId={agentId} filter={filter} />
      )}
    />
  );
}

export function AgentRuntimeConnectorDialog(props: AgentRuntimeConnectorDialogProps) {
  return <EnabledAgentRuntimeConnectorDialog {...props} />;
}
