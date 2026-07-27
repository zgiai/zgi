'use client';

import { useCallback, useEffect, useReducer, useRef } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { AICHAT_KEYS, INTEGRATION_KEYS } from '@/hooks/query-keys';
import { integrationService } from '@/services/integration.service';
import type {
  IntegrationOAuthFlow,
  IntegrationOAuthFlowStatus,
  IntegrationOAuthFlowStartResponse,
  StartIntegrationOAuthFlowRequest,
} from '@/services/types/integration';
import {
  initialIntegrationOAuthUIState,
  integrationOAuthUIReducer,
  isIntegrationOAuthFlowTerminal,
  normalizeOAuthPollInterval,
  oauthFlowExpiryDelay,
} from '@/components/integrations/oauth-flow-state';

const POPUP_WIDTH = 600;
const POPUP_HEIGHT = 700;
const POPUP_CHECK_INTERVAL = 500;

function popupFeatures() {
  const left = Math.max(0, window.screenX + (window.outerWidth - POPUP_WIDTH) / 2);
  const top = Math.max(0, window.screenY + (window.outerHeight - POPUP_HEIGHT) / 2);
  return [
    'popup=yes',
    `width=${POPUP_WIDTH}`,
    `height=${POPUP_HEIGHT}`,
    `left=${Math.round(left)}`,
    `top=${Math.round(top)}`,
    'resizable=yes',
    'scrollbars=yes',
  ].join(',');
}

function safeAuthorizationURL(value: string): string | null {
  try {
    const url = new URL(value);
    if (url.protocol === 'https:') return url.toString();
    if (
      url.protocol === 'http:' &&
      (url.hostname === 'localhost' || url.hostname === '127.0.0.1')
    ) {
      return url.toString();
    }
    return null;
  } catch {
    return null;
  }
}

function oauthFlowReference(flow: IntegrationOAuthFlow): string {
  const reference = flow.flow_ref?.trim() || flow.flow_id?.trim() || '';
  if (!reference || reference.length > 256) {
    throw new Error('invalid OAuth flow reference');
  }
  return reference;
}

function detachPopupOpener(popup: Window | null): void {
  if (!popup) return;
  // Provider pages do not need access to the application window. Completion
  // is reported through the per-flow BroadcastChannel.
  try {
    popup.opener = null;
  } catch {
    // Some browsers expose a read-only opener; strict origin checks still
    // protect the optional postMessage fallback.
  }
}

async function invalidateOAuthCompletionCaches(
  queryClient: ReturnType<typeof useQueryClient>
): Promise<void> {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.catalog() }),
    queryClient.invalidateQueries({ queryKey: INTEGRATION_KEYS.connections() }),
    queryClient.invalidateQueries({
      queryKey: [...INTEGRATION_KEYS.all, 'my-connections'],
    }),
    queryClient.invalidateQueries({
      queryKey: [...INTEGRATION_KEYS.all, 'available-connections'],
    }),
    queryClient.invalidateQueries({
      queryKey: [...AICHAT_KEYS.all, 'integration-preferences'],
    }),
  ]);
}

async function cancelOAuthFlowSilently(flowId: string): Promise<void> {
  if (!flowId) return;
  try {
    await integrationService.cancelOAuthFlow(flowId);
  } catch {
    // Cancellation is best effort when a start, callback, or another browser
    // action has already moved the flow to a terminal state.
  }
}

export function useIntegrationOAuthFlow() {
  const queryClient = useQueryClient();
  const [state, dispatch] = useReducer(integrationOAuthUIReducer, initialIntegrationOAuthUIState);
  const popupRef = useRef<Window | null>(null);
  const flowIdRef = useRef('');
  const authorizationURLRef = useRef('');
  const pollTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const popupTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);
  const expiryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const channelRef = useRef<BroadcastChannel | null>(null);
  const pollingRef = useRef(false);
  const mountedRef = useRef(true);
  const completionHandledRef = useRef(false);
  const operationRef = useRef(0);

  const clearTimers = useCallback(() => {
    if (pollTimerRef.current) clearTimeout(pollTimerRef.current);
    if (popupTimerRef.current) clearInterval(popupTimerRef.current);
    if (expiryTimerRef.current) clearTimeout(expiryTimerRef.current);
    pollTimerRef.current = null;
    popupTimerRef.current = null;
    expiryTimerRef.current = null;
  }, []);

  const clearChannel = useCallback(() => {
    channelRef.current?.close();
    channelRef.current = null;
  }, []);

  const closePopup = useCallback(() => {
    if (popupRef.current && !popupRef.current.closed) popupRef.current.close();
    popupRef.current = null;
  }, []);

  const finish = useCallback(
    async (flow: IntegrationOAuthFlow) => {
      clearTimers();
      clearChannel();
      if (flow.status === 'succeeded') {
        closePopup();
        if (!completionHandledRef.current) {
          completionHandledRef.current = true;
          await invalidateOAuthCompletionCaches(queryClient);
        }
      }
    },
    [clearChannel, clearTimers, closePopup, queryClient]
  );

  const poll = useCallback(async () => {
    const flowId = flowIdRef.current;
    if (!flowId || pollingRef.current || !mountedRef.current) return;
    pollingRef.current = true;
    try {
      const response = await integrationService.getOAuthFlow(flowId);
      if (!mountedRef.current || flowIdRef.current !== flowId) return;
      const flow = response.data;
      dispatch({ type: 'flow_updated', flow });
      if (isIntegrationOAuthFlowTerminal(flow.status)) {
        await finish(flow);
        return;
      }
      pollTimerRef.current = setTimeout(
        () => void poll(),
        normalizeOAuthPollInterval(flow.next_poll_after_ms)
      );
    } catch {
      if (!mountedRef.current || flowIdRef.current !== flowId) return;
      pollTimerRef.current = setTimeout(() => void poll(), 2_500);
    } finally {
      pollingRef.current = false;
    }
  }, [finish]);

  const monitorPopup = useCallback(() => {
    if (popupTimerRef.current) clearInterval(popupTimerRef.current);
    popupTimerRef.current = setInterval(() => {
      if (popupRef.current?.closed) {
        clearInterval(popupTimerRef.current ?? undefined);
        popupTimerRef.current = null;
        popupRef.current = null;
        dispatch({ type: 'popup_closed' });
        void poll();
      }
    }, POPUP_CHECK_INTERVAL);
  }, [poll]);

  const attachFlowSignals = useCallback(
    (flow: IntegrationOAuthFlow) => {
      clearTimers();
      clearChannel();

      if (typeof BroadcastChannel !== 'undefined') {
        const channel = new BroadcastChannel(`zgi:integration-oauth:${oauthFlowReference(flow)}`);
        channel.onmessage = event => {
          const status = (event.data as { type?: unknown; status?: unknown } | null)?.status;
          if (
            (event.data as { type?: unknown } | null)?.type === 'zgi:integration-oauth-status' &&
            typeof status === 'string'
          ) {
            void poll();
          }
        };
        channelRef.current = channel;
      }

      monitorPopup();

      expiryTimerRef.current = setTimeout(() => {
        dispatch({ type: 'timeout' });
        clearTimers();
        clearChannel();
      }, oauthFlowExpiryDelay(flow.expires_at));

      pollTimerRef.current = setTimeout(
        () => void poll(),
        normalizeOAuthPollInterval(flow.next_poll_after_ms)
      );
    },
    [clearChannel, clearTimers, monitorPopup, poll]
  );

  useEffect(() => {
    const receivePopupMessage = (event: MessageEvent) => {
      if (event.origin !== window.location.origin) return;
      const message = event.data as { type?: unknown; status?: unknown } | null;
      if (message?.type !== 'zgi:integration-oauth-status' || typeof message.status !== 'string') {
        return;
      }
      void poll();
    };
    window.addEventListener('message', receivePopupMessage);
    return () => window.removeEventListener('message', receivePopupMessage);
  }, [poll]);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      operationRef.current += 1;
      clearTimers();
      clearChannel();
      closePopup();
    };
  }, [clearChannel, clearTimers, closePopup]);

  const begin = useCallback(
    async (request: StartIntegrationOAuthFlowRequest) => {
      const operation = operationRef.current + 1;
      operationRef.current = operation;
      const previousFlowId = flowIdRef.current;
      clearTimers();
      clearChannel();
      closePopup();
      flowIdRef.current = '';
      authorizationURLRef.current = '';
      completionHandledRef.current = false;
      dispatch({ type: 'start' });
      void cancelOAuthFlowSilently(previousFlowId);

      let popup: Window | null = null;
      let createdFlowId = '';
      try {
        popup = window.open('', `zgi_oauth_${Date.now()}`, popupFeatures());
        detachPopupOpener(popup);
        popupRef.current = popup;
        const response = await integrationService.startOAuthFlow(request);
        const flow: IntegrationOAuthFlowStartResponse = {
          ...response.data,
          integration_id: request.integration_id,
          auth_method_id: request.auth_method_id,
          credential_source: request.credential_source,
          intent: request.intent,
          connection_name: request.connection_name,
        };
        createdFlowId = oauthFlowReference(flow);
        if (!mountedRef.current || operationRef.current !== operation) {
          if (popup && !popup.closed) popup.close();
          await cancelOAuthFlowSilently(createdFlowId);
          return;
        }
        const authorizationURL = safeAuthorizationURL(flow.authorization_url);
        if (!authorizationURL) throw new Error('unsafe OAuth authorization URL');

        flowIdRef.current = createdFlowId;
        authorizationURLRef.current = authorizationURL;
        dispatch({ type: 'flow_created', flow, popupBlocked: !popup });
        attachFlowSignals(flow);

        if (popup && !popup.closed) {
          popup.location.replace(authorizationURL);
          popup.focus();
        }
      } catch {
        if (popup && !popup.closed) popup.close();
        await cancelOAuthFlowSilently(createdFlowId);
        if (!mountedRef.current || operationRef.current !== operation) return;
        popupRef.current = null;
        dispatch({ type: 'request_failed' });
      }
    },
    [attachFlowSignals, clearChannel, clearTimers, closePopup]
  );

  const openFullPage = useCallback(() => {
    if (authorizationURLRef.current) window.location.assign(authorizationURLRef.current);
  }, []);

  const reopenPopup = useCallback(() => {
    const authorizationURL = authorizationURLRef.current;
    if (!authorizationURL) return false;
    const popup = window.open(authorizationURL, `zgi_oauth_${Date.now()}`, popupFeatures());
    popupRef.current = popup;
    if (!popup) return false;
    detachPopupOpener(popup);
    popup.focus();
    monitorPopup();
    dispatch({ type: 'popup_reopened' });
    return true;
  }, [monitorPopup]);

  const cancel = useCallback(async () => {
    const operation = operationRef.current + 1;
    operationRef.current = operation;
    const flowId = flowIdRef.current;
    flowIdRef.current = '';
    authorizationURLRef.current = '';
    clearTimers();
    clearChannel();
    closePopup();
    if (flowId) {
      try {
        await integrationService.cancelOAuthFlow(flowId);
        if (!mountedRef.current || operationRef.current !== operation) return;
        dispatch({ type: 'cancelled' });
        return;
      } catch {
        // The local flow still closes safely if cancellation raced completion.
      }
    }
    if (!mountedRef.current || operationRef.current !== operation) return;
    dispatch({ type: 'reset' });
  }, [clearChannel, clearTimers, closePopup]);

  const reset = useCallback(() => {
    operationRef.current += 1;
    const flowId = flowIdRef.current;
    clearTimers();
    clearChannel();
    closePopup();
    flowIdRef.current = '';
    authorizationURLRef.current = '';
    void cancelOAuthFlowSilently(flowId);
    dispatch({ type: 'reset' });
  }, [clearChannel, clearTimers, closePopup]);

  return {
    state,
    begin,
    cancel,
    reset,
    openFullPage,
    reopenPopup,
    refresh: poll,
  };
}

export interface IntegrationOAuthPopupStatusMessage {
  type: 'zgi:integration-oauth-status';
  status: IntegrationOAuthFlowStatus;
}
