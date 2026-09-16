'use client';

import { useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { toast } from 'sonner';
import {
  ConnectorPickerDialog,
  type ConnectorPickerBindingRef,
} from '@/components/connectors/picker';
import { ConnectorCreateContent } from '@/components/connectors/setup/connector-create-dialog';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import { Dialog, DialogContent } from '@/components/ui/dialog';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import type { AIChatInitialConnectorBinding } from '@/services/types/aichat';
import type { ConnectorCatalogItem, PersonalConnection } from '@/services/types/connector';
import { selectChatDraftConnection, type ChatConnectorDraft } from './connector-draft-model';

export function AIChatConnectorDraftDialog({
  draft,
  open,
  anchorElement,
  onOpenChange,
  onChange,
}: {
  draft: ChatConnectorDraft;
  open: boolean;
  anchorElement: HTMLElement | null;
  onOpenChange: (open: boolean) => void;
  onChange: (bindings: AIChatInitialConnectorBinding[]) => void;
}) {
  const t = useT('connectors');
  const { locale } = useLocale();
  const router = useRouter();
  const [setup, setSetup] = useState<ConnectorCatalogItem>();
  const latest = useRef({ draft, open });
  latest.current = { draft, open };
  const sessionScope = JSON.stringify([draft.id, open, draft.scope]);
  const sessionRef = useRef({ scope: sessionScope });
  if (sessionRef.current.scope !== sessionScope) {
    sessionRef.current = { scope: sessionScope };
    setSetup(undefined);
  }
  const session = sessionRef.current;
  const bindings: ConnectorPickerBindingRef[] = draft.bindings.map(binding => ({
    ...binding,
    id: `${draft.id}:${binding.connector_id}`,
    revision: draft.revision,
    status: 'active',
  }));
  const select = (connection: PersonalConnection) => {
    if (sessionRef.current !== session || !latest.current.open) return;
    onChange(
      selectChatDraftConnection(latest.current.draft, {
        connector_id: connection.connector_id,
        connection_id: connection.id,
      })
    );
  };
  const navigate = (connector: ConnectorCatalogItem, connectionID?: string) => {
    onOpenChange(false);
    router.push(
      `/console/connectors/${encodeURIComponent(connector.id)}${connectionID ? `?connection_id=${encodeURIComponent(connectionID)}` : ''}`
    );
  };

  if (setup) {
    const connector = localizeConnectorCatalogItem(setup, locale);
    return (
      <Dialog
        open={open}
        onOpenChange={next => {
          if (!next) setSetup(undefined);
          onOpenChange(next);
        }}
      >
        <DialogContent
          size="md"
          className="overflow-hidden p-0"
          onCloseAutoFocus={event => {
            event.preventDefault();
            if (!latest.current.open && anchorElement?.isConnected) anchorElement.focus();
          }}
        >
          <ConnectorCreateContent
            onBack={() => setSetup(undefined)}
            description={t('picker.group.draftFooter')}
            connector={connector}
            onReady={connection => {
              if (sessionRef.current !== session || !latest.current.open) return;
              if (
                latest.current.draft.bindings.some(
                  binding => binding.connector_id === connection.connector_id
                )
              ) {
                toast.success(t('picker.connectionReadyNotAdded'));
              } else select(connection);
              setSetup(undefined);
            }}
          />
        </DialogContent>
      </Dialog>
    );
  }
  return (
    <ConnectorPickerDialog
      draft
      groupByConnector
      open={open}
      anchorElement={anchorElement}
      target={{ type: 'chat', id: draft.id }}
      targetName={t('picker.targetTypes.chat')}
      bindings={bindings}
      onOpenChange={onOpenChange}
      onRequestAdd={({ connection }) => {
        if (draft.bindings.some(binding => binding.connector_id === connection.connector_id)) {
          throw new Error('connector switch confirmation is required');
        }
        select(connection);
      }}
      onRequestSwitch={({ connection }) => select(connection)}
      onRequestDetach={({ binding }) =>
        onChange(
          draft.bindings.filter(
            item =>
              item.connection_id !== binding.connection_id ||
              item.connector_id !== binding.connector_id
          )
        )
      }
      onRequestConnectAndAdd={({ connector }) => setSetup(connector)}
      onRequestUpdateKey={({ connector, connection }) => navigate(connector, connection.id)}
      onRequestViewReason={({ connector }) => navigate(connector)}
    />
  );
}
