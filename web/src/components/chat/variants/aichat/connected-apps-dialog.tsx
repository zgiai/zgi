'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useRouter } from 'next/navigation';
import { useQueryClient } from '@tanstack/react-query';
import { AlertCircle, RefreshCw } from 'lucide-react';
import { toast } from 'sonner';
import { localizeConnectorCatalogItem } from '@/components/connectors/connector-metadata';
import {
  ConnectorPickerDialog,
  type ConnectorPickerActionRequest,
  type ConnectorPickerBindingRef,
} from '@/components/connectors/picker';
import { ConnectorCreateContent } from '@/components/connectors/setup/connector-create-dialog';
import { useChatConnectorPreferences } from '@/hooks/connectors/use-chat-connector-preferences';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { connectorTargetBindingItems, useConnectorTargetBindings } from '@/hooks';
import { CONNECTOR_KEYS } from '@/hooks/query-keys';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n/translations';
import { connectorService } from '@/services/connector.service';
import type { AIChatInitialConnectorBinding } from '@/services/types/aichat';
import type { ChatConnectorDraft } from './connector-draft-model';
import { AIChatConnectorDraftDialog } from './connector-draft-dialog';
import type {
  ConnectorCatalogItem,
  ConnectorTargetBinding,
  ConnectorTargetRef,
  PersonalConnection,
} from '@/services/types/connector';

export interface AIChatConnectedAppsSummary {
  selectedConnectionCount: number;
  hasAttentionRequired: boolean;
}

interface AIChatConnectedAppsDialogProps {
  draft?: ChatConnectorDraft;
  onDraftBindingsChange?: (bindings: AIChatInitialConnectorBinding[]) => void;
  open: boolean;
  anchorElement: HTMLElement | null;
  connectorsEnabled: boolean;
  conversationId: string;
  targetName: string;
  onOpenChange: (open: boolean) => void;
  onSummaryChange?: (summary: AIChatConnectedAppsSummary) => void;
}

interface SetupContext {
  connector: ConnectorCatalogItem;
  target: ConnectorTargetRef;
  targetName: string;
  replacedBinding?: ConnectorPickerBindingRef;
}

function newIdempotencyKey(): string {
  return globalThis.crypto.randomUUID();
}

function pickerBinding(binding: ConnectorTargetBinding): ConnectorPickerBindingRef {
  return {
    id: binding.id,
    connection_id: binding.connection_id,
    connector_id: binding.connector_id,
    status: binding.status,
    revision: binding.revision,
    authorization_mode: binding.authorization_mode,
  };
}

function connectorDetailURL(request: ConnectorPickerActionRequest): string {
  const params = new URLSearchParams({
    target_type: 'chat',
    target_id: request.target?.id ?? '',
  });
  if (request.connection?.id) params.set('connection_id', request.connection.id);
  return `/console/connectors/${encodeURIComponent(request.connector.id)}?${params.toString()}`;
}

export function summarizeAIChatConnectorBindings(
  bindings: readonly ConnectorTargetBinding[]
): AIChatConnectedAppsSummary {
  return {
    selectedConnectionCount: bindings.filter(binding => binding.status === 'active').length,
    hasAttentionRequired: bindings.some(binding => binding.status !== 'active'),
  };
}

function AIChatConnectorBindingStateDialog({
  open,
  anchorElement,
  targetName,
  missingTarget,
  loadFailed,
  onOpenChange,
  onRetry,
}: {
  open: boolean;
  anchorElement: HTMLElement | null;
  targetName: string;
  missingTarget: boolean;
  loadFailed: boolean;
  onOpenChange: (open: boolean) => void;
  onRetry: () => void;
}) {
  const t = useT('connectors');
  const stateDialogOpenRef = useRef(open);
  stateDialogOpenRef.current = open;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        size="lg"
        className="h-[min(720px,calc(100vh-2rem))] max-w-[min(760px,calc(100vw-2rem))] overflow-hidden p-0"
        onCloseAutoFocus={event => {
          event.preventDefault();
          if (!stateDialogOpenRef.current && anchorElement?.isConnected) {
            anchorElement.focus({ preventScroll: true });
          }
        }}
      >
        <DialogHeader className="pr-12">
          <DialogTitle>{t('picker.title')}</DialogTitle>
          <DialogDescription>
            {t('picker.group.subtitle', { target: targetName })}
          </DialogDescription>
        </DialogHeader>
        <DialogBody>
          {missingTarget ? (
            <div className="flex items-start gap-2 px-2 py-3 text-sm">
              <AlertCircle
                aria-hidden="true"
                className="mt-0.5 size-4 shrink-0 text-muted-foreground"
              />
              <p className="text-muted-foreground">{t('picker.conversationRequired')}</p>
            </div>
          ) : loadFailed ? (
            <div className="flex min-h-52 flex-col items-center justify-center text-center">
              <p role="alert" className="text-sm text-muted-foreground">
                {t('picker.loadFailedTitle')}
              </p>
              <Button type="button" variant="outline" size="sm" className="mt-4" onClick={onRetry}>
                <RefreshCw aria-hidden="true" className="size-4" />
                {t('actions.retry')}
              </Button>
            </div>
          ) : (
            <div className="space-y-3" role="status" aria-label={t('picker.loading')}>
              <span className="sr-only">{t('picker.loading')}</span>
              <Skeleton className="h-[92px] rounded-xl" />
              <Skeleton className="h-[92px] rounded-xl" />
            </div>
          )}
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

function AIChatConnectorDialog({
  open,
  anchorElement,
  conversationId,
  targetName,
  onOpenChange,
  onSummaryChange,
}: Pick<
  AIChatConnectedAppsDialogProps,
  'open' | 'anchorElement' | 'conversationId' | 'targetName' | 'onOpenChange' | 'onSummaryChange'
>) {
  const router = useRouter();
  const queryClient = useQueryClient();
  const t = useT('connectors');
  const { locale } = useLocale();
  const target = useMemo<ConnectorTargetRef>(
    () => ({ type: 'chat', id: conversationId }),
    [conversationId]
  );
  const bindingsQuery = useConnectorTargetBindings(target, Boolean(conversationId));
  const bindings = useMemo(
    () => connectorTargetBindingItems(bindingsQuery.data?.data),
    [bindingsQuery.data?.data]
  );
  const nativeBindings = useMemo(() => bindings.map(pickerBinding), [bindings]);
  const [setup, setSetup] = useState<SetupContext>();
  const preferencesQuery = useChatConnectorPreferences(open);
  const sessionScope = JSON.stringify([open, conversationId, preferencesQuery.scope]);
  const sessionRef = useRef({ scope: sessionScope });
  if (sessionRef.current.scope !== sessionScope) sessionRef.current = { scope: sessionScope };
  const session = sessionRef.current;
  const pickerOpenRef = useRef(open);
  pickerOpenRef.current = open;

  useEffect(() => {
    onSummaryChange?.(summarizeAIChatConnectorBindings(bindings));
  }, [bindings, onSummaryChange]);

  useEffect(() => {
    setSetup(undefined);
  }, [sessionScope]);

  const attachConnection = useCallback(
    async (
      connection: PersonalConnection,
      attachTarget: ConnectorTargetRef = target,
      replacedBinding?: ConnectorPickerBindingRef
    ) => {
      if (!attachTarget.id) throw new Error('connector conversation target is required');
      const currentBinding = bindings.find(
        binding =>
          binding.connector_id === connection.connector_id &&
          binding.target.type === attachTarget.type &&
          binding.target.id === attachTarget.id &&
          binding.target.binding_instance_id === attachTarget.binding_instance_id
      );
      if (currentBinding && !replacedBinding) {
        throw new Error('connector switch confirmation is required');
      }
      if (
        replacedBinding &&
        (replacedBinding.connector_id !== connection.connector_id ||
          typeof replacedBinding.revision !== 'number')
      ) {
        throw new Error('connector binding revision is required');
      }
      try {
        await connectorService.createTargetBinding(
          replacedBinding
            ? {
                connection_id: connection.id,
                target: attachTarget,
                expected_revision: replacedBinding.revision,
                authorization_mode: 'ask_each_time',
              }
            : {
                connection_id: connection.id,
                target: attachTarget,
                authorization_mode: 'ask_each_time',
              },
          newIdempotencyKey()
        );
      } finally {
        // A revision conflict also refreshes the displayed slot; retries must
        // be explicitly chosen against the newly observed binding revision.
        if (attachTarget.type === target.type && attachTarget.id === target.id) {
          await bindingsQuery.refetch();
        } else {
          await queryClient.invalidateQueries({
            queryKey: CONNECTOR_KEYS.targetBindings(attachTarget),
          });
        }
      }
    },
    [bindings, bindingsQuery, queryClient, target]
  );

  if (!conversationId || bindingsQuery.isLoading || bindingsQuery.isError) {
    return (
      <AIChatConnectorBindingStateDialog
        open={open}
        anchorElement={anchorElement}
        targetName={targetName || t('picker.targetTypes.chat')}
        missingTarget={!conversationId}
        loadFailed={bindingsQuery.isError}
        onOpenChange={onOpenChange}
        onRetry={() => void bindingsQuery.refetch()}
      />
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
            if (!pickerOpenRef.current && anchorElement?.isConnected) anchorElement.focus();
          }}
        >
          <ConnectorCreateContent
            onBack={() => setSetup(undefined)}
            description={t('picker.subtitle', { target: setup.targetName })}
            connector={setupConnector}
            target={setup.replacedBinding ? undefined : setup.target}
            submitLabel={t(
              setup.replacedBinding
                ? 'picker.actions.addConnection'
                : 'picker.actions.connectAndAdd'
            )}
            onReady={(connection, result) => {
              if (sessionRef.current !== session || !pickerOpenRef.current) return;
              if (
                result.attachment?.connection_id === connection.id &&
                result.attachment.target.type === setup.target.type &&
                result.attachment.target.id === setup.target.id
              ) {
                void bindingsQuery.refetch().finally(() => {
                  if (sessionRef.current !== session || !pickerOpenRef.current) return;
                  setSetup(undefined);
                });
                return;
              }
              // A saved Key never replaces an occupied (or concurrently
              // claimed) target slot. Return to the picker so the user can
              // choose the new connection and confirm a CAS-protected switch.
              void bindingsQuery.refetch().finally(() => {
                if (sessionRef.current !== session || !pickerOpenRef.current) return;
                setSetup(undefined);
                toast.success(t('picker.connectionReadyNotAdded'));
              });
            }}
          />
        </DialogContent>
      </Dialog>
    );
  }

  return (
      <ConnectorPickerDialog
        groupByConnector
        open={open}
        anchorElement={anchorElement}
        target={{ type: 'chat', id: conversationId }}
        targetName={targetName}
        bindings={nativeBindings}
        onOpenChange={onOpenChange}
        onRequestAdd={request => attachConnection(request.connection)}
        onRequestSwitch={request =>
          attachConnection(request.connection, target, request.replacedBinding)
        }
        onRequestConnectAndAdd={request =>
          setSetup({
            connector: request.connector,
            target,
            targetName,
            replacedBinding: request.replacedBinding,
          })
        }
        onRequestUpdateKey={request => {
          onOpenChange(false);
          router.push(connectorDetailURL(request));
        }}
        onRequestViewReason={request => {
          onOpenChange(false);
          router.push(connectorDetailURL(request));
        }}
        onRequestDetach={async request => {
          if (!request.binding.revision) throw new Error('connector binding revision is required');
          await connectorService.detachTargetBinding(request.binding.id, {
            revision: request.binding.revision,
          });
          await bindingsQuery.refetch();
        }}
      />
  );
}

export function AIChatConnectedAppsDialog(props: AIChatConnectedAppsDialogProps) {
  if (!props.connectorsEnabled) return null;
  if (props.draft && props.onDraftBindingsChange) {
    return (
      <AIChatConnectorDraftDialog
        key={props.draft.id}
        draft={props.draft}
        open={props.open}
        anchorElement={props.anchorElement}
        onOpenChange={props.onOpenChange}
        onChange={props.onDraftBindingsChange}
      />
    );
  }
  return <AIChatConnectorDialog {...props} />;
}
