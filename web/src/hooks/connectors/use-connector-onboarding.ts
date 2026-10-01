'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { connectorService } from '@/services/connector.service';
import {
  activeOnboarding,
  messagingMethod,
  supportsMessagingContract,
} from '@/components/connectors/setup/app-setup-state';
import { connectorRequestErrorDetails } from './connector-request-error';
import { invalidatePersonalConnectionQueries } from './connection-detail-query';
import type {
  ConnectorCatalogItem,
  ConnectorOnboardingFlow,
  ConnectorOnboardingRequest,
  PersonalConnection,
} from '@/services/types/connector';

interface RecoveryTicket {
  key: string;
  request: ConnectorOnboardingRequest;
  flowId?: string;
}
// Only a scoped request/flow reference survives closing the dialog. QR URLs,
// user codes, app credentials and provider responses never enter storage/cache.
function persist(key: string, ticket?: RecoveryTicket) {
  try {
    if (ticket) sessionStorage.setItem(key, JSON.stringify(ticket));
    else sessionStorage.removeItem(key);
  } catch {
    /* Storage may be disabled. */
  }
}
function recover(key: string): RecoveryTicket | undefined {
  try {
    const value = JSON.parse(sessionStorage.getItem(key) ?? 'null') as RecoveryTicket | null;
    if (!value || !/^[a-f0-9-]{36}$/.test(value.key) || !value.request) {
      return undefined;
    }
    const incoming = value.request;
    if (incoming.intent === 'resume_app') {
      if (
        !/^[a-f0-9-]{36}$/.test(incoming.pending_setup_id) ||
        !Number.isSafeInteger(incoming.expected_revision) ||
        incoming.expected_revision < 1 ||
        (value.flowId && !/^[a-f0-9-]{36}$/.test(value.flowId))
      ) {
        return;
      }
      return {
        key: value.key,
        flowId: value.flowId,
        request: {
          intent: 'resume_app',
          pending_setup_id: incoming.pending_setup_id,
          expected_revision: incoming.expected_revision,
        },
      };
    }
    if ('protocol_version' in incoming) {
      if (
        incoming.protocol_version !== 1 ||
        (incoming.intent !== 'register_bot' &&
          incoming.intent !== 'login_session' &&
          incoming.intent !== 'reauthorize')
      ) {
        return;
      }
      if (
        incoming.intent === 'reauthorize' &&
        (!Number.isSafeInteger(incoming.expected_revision) || incoming.expected_revision < 1)
      ) {
        return;
      }
      if (value.flowId && !/^[a-f0-9-]{36}$/.test(value.flowId)) return;
      return {
        key: value.key,
        flowId: value.flowId,
        request:
          incoming.intent === 'reauthorize'
            ? {
                intent: incoming.intent,
                protocol_version: 1,
                expected_revision: incoming.expected_revision,
              }
            : { intent: incoming.intent, protocol_version: 1 },
      };
    }
    if (value.request.intent !== 'create_app' && value.request.intent !== 'update_app') {
      return undefined;
    }
    const request = value.request;
    if (typeof request.template_id !== 'string') return;
    if (
      request.intent === 'create_app' &&
      request.region !== 'feishu' &&
      request.region !== 'lark'
    ) {
      return undefined;
    }
    if (
      request.intent === 'update_app' &&
      (!Number.isSafeInteger(request.expected_revision) || request.expected_revision < 1)
    ) {
      return undefined;
    }
    if (value.flowId && !/^[a-f0-9-]{36}$/.test(value.flowId)) return undefined;
    // Whitelist persisted fields, even if another page has modified storage.
    return {
      key: value.key,
      flowId: value.flowId,
      request:
        request.intent === 'create_app'
          ? { intent: request.intent, region: request.region, template_id: request.template_id }
          : {
              intent: request.intent,
              expected_revision: request.expected_revision,
              template_id: request.template_id,
            },
    };
  } catch {
    return undefined;
  }
}

export function useConnectorOnboarding(
  connector: ConnectorCatalogItem,
  update?: PersonalConnection,
  pendingSetupID?: string
) {
  const accountID = useAuthStore.use.user()?.id;
  const organizationID = useOrganizationStore.use.currentOrganization()?.id;
  const scope = `connector-onboarding:${organizationID}:${accountID}:${connector.id}:${pendingSetupID ?? update?.id ?? 'create'}`;
  const [flow, setFlow] = useState<ConnectorOnboardingFlow>();
  const [connection, setConnection] = useState<PersonalConnection>();
  const [errorCode, setErrorCode] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [restored, setRestored] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const generation = useRef(0);
  const ticket = useRef<RecoveryTicket | undefined>(undefined);
  const verificationCommand = useRef<{ flowID: string; revision: number; key: string }>();
  const verificationInFlight = useRef(false);
  const activeScope = useRef(scope);
  const queryClient = useQueryClient();
  const current = useCallback(
    (version: number) =>
      generation.current === version &&
      activeScope.current === scope &&
      Boolean(accountID && organizationID) &&
      useAuthStore.getState().user?.id === accountID &&
      useOrganizationStore.getState().currentOrganization?.id === organizationID,
    [accountID, organizationID, scope]
  );

  useEffect(() => {
    const version = ++generation.current;
    activeScope.current = scope;
    setFlow(undefined);
    setConnection(undefined);
    setErrorCode(undefined);
    setBusy(false);
    verificationCommand.current = undefined;
    verificationInFlight.current = false;
    ticket.current = recover(scope);
    setRestored(Boolean(ticket.current));
    const controller = new AbortController();
    if (ticket.current?.flowId) {
      void connectorService
        .getOnboarding(ticket.current.flowId, controller.signal)
        .then(result => {
          if (current(version)) setFlow(result.data);
        })
        .catch(error => {
          if (current(version)) setErrorCode(connectorRequestErrorDetails(error).code);
        });
    } else if (ticket.current) setErrorCode('transport_unconfirmed');
    else if (pendingSetupID) {
      setRestored(true);
      void connectorService
        .getPendingSetup(pendingSetupID)
        .then(async result => {
          if (!current(version) || !result.data.active_flow_id) return;
          const active = await connectorService.getOnboarding(
            result.data.active_flow_id,
            controller.signal
          );
          if (current(version)) setFlow(active.data);
        })
        .catch(error => {
          if (current(version)) setErrorCode(connectorRequestErrorDetails(error).code);
        });
    }
    return () => {
      generation.current = version + 1;
      controller.abort();
      ticket.current = undefined;
    };
  }, [scope, current, pendingSetupID]);

  useEffect(() => {
    if (!flow || !activeOnboarding(flow) || errorCode) return;
    const controller = new AbortController();
    const version = generation.current;
    const timer = setTimeout(
      () => {
        void connectorService
          .getOnboarding(flow.id, controller.signal)
          .then(result => {
            if (current(version)) setFlow(result.data);
          })
          .catch(error => {
            if (current(version)) setErrorCode(connectorRequestErrorDetails(error).code);
          });
      },
      Math.max(2, flow.retry_after) * 1000
    );
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [flow, current, errorCode, refresh]);

  useEffect(() => {
    if (flow?.state !== 'ready' || !flow.result_connection_id || connection) return;
    const version = generation.current;
    void connectorService
      .getConnection(flow.result_connection_id)
      .then(result => {
        if (!current(version)) return;
        setConnection(result.data);
        persist(scope);
        invalidatePersonalConnectionQueries(queryClient);
      })
      .catch(error => {
        if (current(version)) setErrorCode(connectorRequestErrorDetails(error).code);
      });
  }, [flow, connection, current, queryClient, scope, refresh]);

  useEffect(() => {
    if (flow?.state !== 'pending_completion') return;
    // The server-side pending record is now the recovery source. A later Add
    // connection must not replay this finished create attempt from storage.
    persist(scope);
    ticket.current = undefined;
  }, [flow?.state, scope]);

  const start = useCallback(
    async (region: 'feishu' | 'lark' = 'feishu', resume = false, pendingRevision?: number) => {
      const templateID = connector.setup?.template_id;
      const messaging = supportsMessagingContract(connector);
      if (
        busy ||
        !accountID ||
        !organizationID ||
        (!messaging && !templateID) ||
        activeOnboarding(flow)
      ) {
        return;
      }
      if (
        messaging &&
        !resume &&
        !messagingMethod(connector, update ? 'qr_reauthorize' : 'qr_connect')
      ) {
        return;
      }
      if (!resume && ticket.current && !flow) return;
      const version = generation.current;
      if (pendingSetupID && !resume && !pendingRevision) return;
      const request: ConnectorOnboardingRequest = pendingSetupID
        ? {
            intent: 'resume_app',
            pending_setup_id: pendingSetupID,
            expected_revision: pendingRevision ?? 0,
          }
        : messaging
          ? update
            ? { intent: 'reauthorize', protocol_version: 1, expected_revision: update.revision }
            : {
                intent: connector.id === 'weixin' ? 'login_session' : 'register_bot',
                protocol_version: 1,
              }
          : update
            ? {
                intent: 'update_app',
                expected_revision: update.revision,
                template_id: templateID ?? '',
              }
            : { intent: 'create_app', region, template_id: templateID ?? '' };
      if (!resume) {
        ticket.current = { key: crypto.randomUUID(), request };
        setRestored(false);
      }
      const active = ticket.current;
      if (!active) return;
      persist(scope, active);
      setBusy(true);
      setErrorCode(undefined);
      try {
        const result =
          active.request.intent === 'resume_app'
            ? await connectorService.resumePendingSetup(
                active.request.pending_setup_id,
                active.request.expected_revision,
                active.key
              )
            : await connectorService.startOnboarding(
                connector.id,
                active.request,
                active.key,
                update?.id
              );
        if (!current(version)) return;
        ticket.current = { ...active, flowId: result.data.id };
        persist(scope, ticket.current);
        setFlow(result.data);
      } catch (error) {
        if (!current(version)) return;
        const details = connectorRequestErrorDetails(error);
        setErrorCode(details.code);
        if (!details.outcomeUnknown && details.status && details.status < 500) {
          persist(scope);
          ticket.current = undefined;
        }
      } finally {
        if (current(version)) setBusy(false);
      }
    },
    [busy, accountID, organizationID, flow, update, connector, scope, current, pendingSetupID]
  );

  const retryStatus = useCallback(async () => {
    const version = generation.current;
    setErrorCode(undefined);
    const id = flow?.id ?? ticket.current?.flowId;
    if (!id) {
      await start('feishu', true);
      return;
    }
    try {
      const result = await connectorService.getOnboarding(id);
      if (current(version)) {
        setFlow(result.data);
        setRefresh(value => value + 1);
      }
    } catch (error) {
      if (current(version)) setErrorCode(connectorRequestErrorDetails(error).code);
    }
  }, [flow?.id, start, current]);

  const cancel = useCallback(async () => {
    if (!flow || busy) return;
    const version = generation.current;
    setBusy(true);
    try {
      // Refresh first: the durable worker may have advanced the public revision.
      const latest = await connectorService.getOnboarding(flow.id);
      if (!current(version)) return;
      const result = await connectorService.cancelOnboarding(flow.id, latest.data.revision);
      if (current(version)) {
        setFlow(result.data);
        setErrorCode(undefined);
        if (result.data.state === 'cancelled') {
          persist(scope);
          ticket.current = undefined;
        }
      }
    } catch (error) {
      if (current(version)) setErrorCode(connectorRequestErrorDetails(error).code);
    } finally {
      if (current(version)) setBusy(false);
    }
  }, [flow, busy, current, scope]);

  const retryVerification = useCallback(async () => {
    if (
      !flow ||
      busy ||
      verificationInFlight.current ||
      !flow.allowed_actions?.includes('retry_verification')
    ) {
      return;
    }
    const version = generation.current;
    verificationInFlight.current = true;
    setBusy(true);
    setErrorCode(undefined);
    try {
      const latest = await connectorService.getOnboarding(flow.id);
      if (!current(version)) return;
      setFlow(latest.data);
      if (!latest.data.allowed_actions?.includes('retry_verification')) return;
      if (
        verificationCommand.current?.flowID !== flow.id ||
        verificationCommand.current.revision !== latest.data.revision
      ) {
        verificationCommand.current = {
          flowID: flow.id,
          revision: latest.data.revision,
          key: crypto.randomUUID(),
        };
      }
      const result = await connectorService.retryOnboardingVerification(
        latest.data,
        verificationCommand.current.key
      );
      if (current(version)) setFlow(result.data);
    } catch (error) {
      if (current(version)) setErrorCode(connectorRequestErrorDetails(error).code);
    } finally {
      if (current(version)) {
        verificationInFlight.current = false;
        setBusy(false);
      }
    }
  }, [flow, busy, current]);

  const submitCode = useCallback(
    async (code: string) => {
      if (
        !flow ||
        busy ||
        flow.protocol_version !== 1 ||
        flow.step !== 'awaiting_code' ||
        !flow.allowed_actions?.includes('submit_code')
      ) {
        return;
      }
      const version = generation.current;
      setBusy(true);
      setErrorCode(undefined);
      try {
        const result = await connectorService.submitOnboardingCode(flow, code);
        if (current(version)) setFlow(result.data);
      } catch (error) {
        if (current(version)) setErrorCode(connectorRequestErrorDetails(error).code);
      } finally {
        if (current(version)) setBusy(false);
      }
    },
    [flow, busy, current]
  );

  return {
    clearTerminal: () => {
      if (
        !flow ||
        activeOnboarding(flow) ||
        flow.state === 'ready' ||
        flow.state === 'pending_completion' ||
        busy
      ) {
        return;
      }
      persist(scope);
      ticket.current = undefined;
      setFlow(undefined);
      setErrorCode(undefined);
      setRestored(false);
    },
    submitCode,
    retryVerification,
    flow,
    connection,
    errorCode,
    busy,
    restored,
    start,
    cancel,
    retryStatus,
    hasRecovery: Boolean(ticket.current),
  };
}
