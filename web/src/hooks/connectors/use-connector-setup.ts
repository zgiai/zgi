'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import {
  applyKeySetupResult,
  beginKeySetup,
  canSubmitKeySetup,
  createKeySetupState,
  failKeySetup,
  resetKeySetup,
  retryKeySetup,
  updateKeySetupValue,
} from '@/components/connectors/setup/key-setup-state';
import { CONNECTOR_KEYS } from '@/hooks/query-keys';
import { connectorService } from '@/services/connector.service';
import { connectorRequestErrorDetails } from './connector-request-error';
import { invalidatePersonalConnectionQueries } from './connection-detail-query';
import type {
  ConnectorKeyField,
  ConnectorTargetRef,
  KeySetupResult,
} from '@/services/types/connector';

function newIdempotencyKey(): string {
  return globalThis.crypto.randomUUID();
}

function destroyValues(values: Record<string, string>): void {
  for (const key of Object.keys(values)) {
    values[key] = '';
    delete values[key];
  }
}

export function useConnectorSetup(
  connectorId: string,
  fields: readonly ConnectorKeyField[],
  options?: { target?: ConnectorTargetRef; enabled?: boolean }
) {
  const queryClient = useQueryClient();
  const mountedRef = useRef(true);
  const [state, setState] = useState(createKeySetupState);
  const [submitting, setSubmitting] = useState(false);
  const enabled = options?.enabled ?? true;
  const target = options?.target;

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

  const finish = useCallback(
    (result: KeySetupResult) => {
      if (!mountedRef.current) return;
      setState(current => applyKeySetupResult(current, result));
      if (result.state === 'ready') {
        invalidatePersonalConnectionQueries(queryClient);
        if (result.attachment) {
          void queryClient.invalidateQueries({
            queryKey: CONNECTOR_KEYS.targetBindings(result.attachment.target),
          });
        }
      }
    },
    [queryClient]
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
  }, [queryClient]);

  useEffect(() => {
    if (setupQuery.data?.data) finish(setupQuery.data.data);
  }, [finish, setupQuery.data]);

  const updateValue = useCallback((name: string, value: string) => {
    setState(current => updateKeySetupValue(current, name, value));
  }, []);

  const submit = useCallback(async () => {
    if (!enabled || submitting || !canSubmitKeySetup(state, fields)) return null;
    const requestValues = { ...state.values };
    setSubmitting(true);
    setState(current => beginKeySetup(current));
    try {
      const response = await connectorService.startKeySetup(
        connectorId,
        { values: requestValues, ...(target ? { target } : {}) },
        newIdempotencyKey()
      );
      finish(response.data);
      return response.data;
    } catch (error) {
      const details = connectorRequestErrorDetails(error);
      if (mountedRef.current) {
        setState(current =>
          failKeySetup(current, details.code, details.retryable, details.outcomeUnknown)
        );
      }
      return null;
    } finally {
      destroyValues(requestValues);
      if (mountedRef.current) setSubmitting(false);
    }
  }, [connectorId, enabled, fields, finish, state, submitting, target]);

  const retry = useCallback(() => setState(current => retryKeySetup(current)), []);
  const reset = useCallback(() => setState(resetKeySetup()), []);
  const cancel = useCallback(async () => {
    const attemptId = state.attemptId;
    reset();
    if (!attemptId) return;
    try {
      await connectorService.cancelKeySetup(attemptId);
    } catch {
      // The provider may already be verifying. Reset still clears all client Key state.
    }
  }, [reset, state.attemptId]);

  return useMemo(
    () => ({
      state,
      updateValue,
      submit,
      retry,
      retryStatus,
      statusError,
      statusRefreshing,
      refreshConnections,
      reset,
      cancel,
      canSubmit: enabled && !submitting && canSubmitKeySetup(state, fields),
    }),
    [
      cancel,
      enabled,
      fields,
      refreshConnections,
      reset,
      retry,
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
