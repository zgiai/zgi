'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  applyConnectionReconnectResult,
  beginConnectionReconnect,
  canSubmitConnectionReconnect,
  confirmConnectionReconnectIdentity,
  createConnectionReconnectState,
  failConnectionReconnect,
  requireConnectionReconnectConfirmation,
  resetConnectionReconnect,
  updateConnectionReconnectValue,
} from '@/components/connectors/connections/connection-reconnect-state';
import { CONNECTOR_KEYS } from '@/hooks/query-keys';
import { connectorService } from '@/services/connector.service';
import { connectorRequestErrorDetails } from './connector-request-error';
import { invalidatePersonalConnectionQueries } from './connection-detail-query';
import type { ConnectorKeyField, KeySetupResult } from '@/services/types/connector';

function destroyValues(values: Record<string, string>): void {
  for (const name of Object.keys(values)) {
    values[name] = '';
    delete values[name];
  }
}

export function useConnectorReconnect(
  connectionId: string,
  fields: readonly ConnectorKeyField[],
  enabled = true
) {
  const queryClient = useQueryClient();
  const mountedRef = useRef(true);
  const [state, setState] = useState(createConnectionReconnectState);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const finish = useCallback(
    (result: KeySetupResult) => {
      if (!mountedRef.current) return;
      setState(current => applyConnectionReconnectResult(current, connectionId, result));
      if (result.state === 'ready') {
        invalidatePersonalConnectionQueries(queryClient);
      }
    },
    [connectionId, queryClient]
  );

  const setupQuery = useQuery({
    queryKey: CONNECTOR_KEYS.setup(state.attemptId ?? ''),
    queryFn: () => connectorService.getKeySetup(state.attemptId ?? ''),
    enabled: enabled && state.status === 'verifying' && Boolean(state.attemptId),
    retry: false,
    refetchInterval: query =>
      query.state.status !== 'error' && query.state.data?.data.state === 'verifying'
        ? 1_000
        : false,
  });
  const { refetch: refetchStatus, isError: statusError, isFetching: statusRefreshing } = setupQuery;

  const retryStatus = useCallback(() => {
    if (!enabled || state.status !== 'verifying' || !state.attemptId || statusRefreshing) return;
    void refetchStatus();
  }, [enabled, refetchStatus, state.attemptId, state.status, statusRefreshing]);

  const refreshConnections = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: CONNECTOR_KEYS.catalog() });
    void queryClient.invalidateQueries({ queryKey: CONNECTOR_KEYS.connections() });
    void queryClient.invalidateQueries({ queryKey: CONNECTOR_KEYS.connection(connectionId) });
  }, [connectionId, queryClient]);

  useEffect(() => {
    if (setupQuery.data?.data) finish(setupQuery.data.data);
  }, [finish, setupQuery.data]);

  const updateValue = useCallback((name: string, value: string) => {
    setState(current => updateConnectionReconnectValue(current, name, value));
  }, []);

  const submit = useCallback(async () => {
    if (!enabled || submitting || !canSubmitConnectionReconnect(state, fields)) return null;
    const requestValues = { ...state.values };
    setSubmitting(true);
    setState(current => beginConnectionReconnect(current));
    try {
      const response = await connectorService.reconnectConnection(
        connectionId,
        {
          values: requestValues,
          ...(state.confirmUnstableIdentity ? { confirm_unstable_identity: true } : {}),
        },
        globalThis.crypto.randomUUID()
      );
      finish(response.data);
      return response.data;
    } catch (error) {
      const details = connectorRequestErrorDetails(error);
      if (mountedRef.current) {
        if (
          details.status === 409 &&
          details.code === 'connector_conflict' &&
          !state.confirmUnstableIdentity
        ) {
          setState(requireConnectionReconnectConfirmation());
        } else {
          setState(current =>
            failConnectionReconnect(
              current,
              details.code,
              details.retryable,
              details.outcomeUnknown
            )
          );
        }
      }
      return null;
    } finally {
      destroyValues(requestValues);
      if (mountedRef.current) setSubmitting(false);
    }
  }, [connectionId, enabled, fields, finish, state, submitting]);

  const reset = useCallback(() => setState(resetConnectionReconnect()), []);
  const confirmIdentityReplacement = useCallback(
    () => setState(confirmConnectionReconnectIdentity()),
    []
  );
  const cancel = useCallback(async () => {
    const attemptId = state.attemptId;
    reset();
    if (!attemptId) return;
    try {
      await connectorService.cancelKeySetup(attemptId);
    } catch {
      // The provider may already have completed; local write-only values are cleared regardless.
    }
  }, [reset, state.attemptId]);

  return useMemo(
    () => ({
      state,
      updateValue,
      submit,
      reset,
      retryStatus,
      statusError,
      statusRefreshing,
      refreshConnections,
      confirmIdentityReplacement,
      cancel,
      canSubmit: enabled && !submitting && canSubmitConnectionReconnect(state, fields),
    }),
    [
      cancel,
      confirmIdentityReplacement,
      enabled,
      fields,
      refreshConnections,
      reset,
      retryStatus,
      state,
      statusError,
      statusRefreshing,
      submit,
      submitting,
      updateValue,
    ]
  );
}
