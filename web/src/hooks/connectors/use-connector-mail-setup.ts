'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { CONNECTOR_KEYS } from '@/hooks/query-keys';
import { connectorService } from '@/services/connector.service';
import type {
  ConnectorCatalogItem,
  ConnectorMailVerification,
  PersonalConnection,
} from '@/services/types/connector';
import {
  applyKeySetupResult,
  beginKeySetup,
  createKeySetupState,
  failKeySetup,
} from '@/components/connectors/setup/key-setup-state';
import {
  clearMailValues,
  mailSetupEnabled,
  mailFailureVerification,
  normalizeMailSecret,
  normalizeMailProfileEmail,
} from '@/components/connectors/setup/mail-setup-state';
import { connectorRequestErrorDetails } from './connector-request-error';
import { invalidatePersonalConnectionQueries } from './connection-detail-query';

/** Credential requests bypass mutation caches; only sanitized result snapshots are queried. */
export function useConnectorMailSetup(
  connector: ConnectorCatalogItem,
  connection?: PersonalConnection
) {
  const [email, setEmail] = useState(connection?.external_account_id ?? '');
  const [password, setPassword] = useState('');
  const [state, setState] = useState(createKeySetupState);
  const [verification, setVerification] = useState<ConnectorMailVerification>();
  const alive = useRef(true);
  const requestValues = useRef<Record<string, string>>();
  const submitting = useRef(false);
  const queryClient = useQueryClient();
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      if (requestValues.current) clearMailValues(requestValues.current);
    };
  }, []);
  const finish = useCallback(
    (result: Parameters<typeof applyKeySetupResult>[1]) => {
      if (!alive.current) return;
      setState(current => applyKeySetupResult(current, result));
      if (result.state === 'ready') invalidatePersonalConnectionQueries(queryClient);
    },
    [queryClient]
  );
  const query = useQuery({
    queryKey: CONNECTOR_KEYS.setup(state.attemptId ?? ''),
    queryFn: () => connectorService.getKeySetup(state.attemptId ?? ''),
    enabled: state.status === 'verifying' && Boolean(state.attemptId),
    retry: false,
    refetchInterval: current =>
      current.state.status !== 'error' && current.state.data?.data.state === 'verifying'
        ? 1000
        : false,
  });
  useEffect(() => {
    if (query.data?.data) finish(query.data.data);
  }, [query.data, finish]);
  const canSubmit =
    state.status === 'idle' &&
    !submitting.current &&
    mailSetupEnabled(connector) &&
    Boolean(
      normalizeMailProfileEmail(connector.id, email) && normalizeMailSecret(connector.id, password)
    ) &&
    (!connection || email === connection.external_account_id);
  async function submit() {
    if (!canSubmit || submitting.current) return;
    const normalizedEmail = normalizeMailProfileEmail(connector.id, email);
    const normalizedPassword = normalizeMailSecret(connector.id, password);
    if (!normalizedEmail || !normalizedPassword) return;
    const values = {
      email: normalizedEmail,
      [connector.id === 'gmail' ? 'app_password' : 'authorization_code']: normalizedPassword,
    };
    requestValues.current = values;
    submitting.current = true;
    setPassword('');
    setVerification(undefined);
    setState(current => beginKeySetup(current));
    try {
      const result = connection
        ? await connectorService.reconnectConnection(
            connection.id,
            { values, mail_usage: 'read_send' },
            crypto.randomUUID()
          )
        : await connectorService.startKeySetup(
            connector.id,
            { values, mail_usage: 'read_send' },
            crypto.randomUUID()
          );
      finish(result.data);
    } catch (error) {
      const details = connectorRequestErrorDetails(error);
      if (alive.current) {
        setVerification(mailFailureVerification(error));
        setState(current =>
          failKeySetup(current, details.code, details.retryable, details.outcomeUnknown)
        );
      }
    } finally {
      clearMailValues(values);
      requestValues.current = undefined;
      submitting.current = false;
    }
  }
  return {
    state,
    verification,
    email,
    password,
    canSubmit,
    submit,
    setEmail: (value: string) => {
      if (!connection) setEmail(value);
    },
    setPassword,
    reset: () => {
      setPassword('');
      setVerification(undefined);
      setState(createKeySetupState());
    },
    statusError: query.isError,
    statusRefreshing: query.isFetching,
    retryStatus: () => {
      void query.refetch();
    },
  };
}
