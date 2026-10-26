'use client';

import { useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { CONNECTOR_KEYS } from '@/hooks/query-keys';
import { useConnectorOAuthFlow } from '@/hooks/connectors/use-connector-oauth';
import { useT } from '@/i18n';
import { connectorService } from '@/services/connector.service';
import type { ConnectorKeySetupPanelProps } from './connector-key-setup-panel';
import { ConnectorOAuthFlowStatus } from './connector-oauth-flow-status';
import { isOAuthFlowActive, safeOAuthAuthorizeURL } from './social-oauth-state';
import { useCreationDefault } from './use-creation-default';

interface Props extends ConnectorKeySetupPanelProps {
  reconnectId?: string;
  manualFallback: React.ReactNode;
  onManual?: () => void;
}

export function ConnectorSocialOAuthPanel({
  connector,
  title,
  embedded,
  onReady,
  reconnectId,
  manualFallback,
  onManual,
}: Props) {
  const t = useT('connectors');
  const [manual, setManual] = useState(false);
  const [starting, setStarting] = useState(false);
  const [error, setError] = useState<'blocked' | 'failed' | 'closed' | null>(null);
  const [completionAttempt, setCompletionAttempt] = useState(0);
  const [flowId, setFlowId] = useState<string>();
  const popup = useRef<Window | null>(null);
  const alive = useRef(true);
  const notified = useRef('');
  const completing = useRef(false);
  const creationDefault = useCreationDefault(!reconnectId);
  const latest = useRef({ creationDefault, onReady });
  latest.current = { creationDefault, onReady };
  const availability = useQuery({
    queryKey: [...CONNECTOR_KEYS.all, 'oauth-availability', connector.id],
    queryFn: () => connectorService.getOAuthAvailability(connector.id),
    retry: false,
  });
  const { flow, query } = useConnectorOAuthFlow(flowId);
  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      popup.current?.close();
    };
  }, []);
  useEffect(() => {
    if (!isOAuthFlowActive(flow) || !popup.current) return;
    const timer = window.setInterval(() => {
      if (popup.current?.closed && flow?.status === 'pending') setError('closed');
    }, 1500);
    return () => window.clearInterval(timer);
  }, [flow]);
  useEffect(() => {
    if (
      flow?.status !== 'ready' ||
      !flow.connection_id ||
      notified.current === flow.id ||
      completing.current
    ) {
      return;
    }
    setError(null);
    completing.current = true;
    const currentFlow = flow;
    void connectorService
      .getConnection(flow.connection_id)
      .then(async response => {
        if (!alive.current) return;
        const connection = response.data;
        if (
          !connection ||
          connection.id !== currentFlow.connection_id ||
          connection.connector_id !== connector.id
        ) {
          throw new Error('Incomplete connection');
        }
        const current = await latest.current.creationDefault.complete(connection);
        if (!alive.current || !current) return;
        notified.current = currentFlow.id;
        latest.current.onReady?.(connection, {});
      })
      .catch(() => {
        if (alive.current) setError('failed');
      })
      .finally(() => {
        completing.current = false;
      });
  }, [flow, connector.id, completionAttempt]);

  const start = async () => {
    if (starting) return;
    // Create the popup during the click, before awaiting the authenticated API.
    const opened = window.open('about:blank', '_blank', 'popup,width=640,height=760');
    if (!opened) {
      setError('blocked');
      return;
    }
    opened.opener = null;
    popup.current?.close();
    popup.current = opened;
    setError(null);
    setStarting(true);
    latest.current.creationDefault.begin();
    try {
      const response = await connectorService.startOAuth(connector.id, reconnectId);
      if (!alive.current) {
        opened.close();
        return;
      }
      const next = response.data;
      if (!next.id || next.connector_id !== connector.id) throw new Error('Invalid OAuth flow');
      const authorizeURL = safeOAuthAuthorizeURL(next.authorize_url);
      setFlowId(next.id);
      opened.location.replace(authorizeURL);
    } catch {
      opened.close();
      if (alive.current) setError('failed');
    } finally {
      if (alive.current) setStarting(false);
    }
  };

  if (manual) {
    return (
      <div className="space-y-3">
        <Button variant="ghost" onClick={() => setManual(false)}>
          {t('oauth.back')}
        </Button>
        {manualFallback}
      </div>
    );
  }
  const active =
    isOAuthFlowActive(flow) && !(flow?.status === 'select_page' && !flow.pages?.length);
  return (
    <Card className={embedded ? 'border-0 shadow-none' : undefined}>
      {!embedded && (
        <CardHeader>
          <CardTitle>{title ?? t('oauth.title')}</CardTitle>
        </CardHeader>
      )}
      <CardContent className={embedded ? 'space-y-4 p-0' : 'space-y-4'}>
        <p className="text-sm text-muted-foreground">{t('oauth.description')}</p>
        {!flowId && creationDefault.option(starting)}
        {availability.isLoading && (
          <p role="status" className="text-sm">
            {t('oauth.loading')}
          </p>
        )}
        {availability.data?.data.available === false && (
          <p role="status" className="text-sm">
            {t('oauth.unavailable')}
          </p>
        )}
        {availability.isError && (
          <div role="alert" className="space-y-2">
            <p className="text-sm">{t('oauth.statusFailed')}</p>
            <Button variant="outline" onClick={() => void availability.refetch()}>
              {t('actions.retry')}
            </Button>
          </div>
        )}
        {flowId && <ConnectorOAuthFlowStatus flowId={flowId} reconnectId={reconnectId} />}
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {t(
              error === 'blocked'
                ? 'oauth.blocked'
                : error === 'closed'
                  ? 'oauth.closed'
                  : 'oauth.failed'
            )}
          </p>
        )}
        {availability.data?.data.available && flow?.status !== 'ready' && (
          <Button
            type="button"
            className="w-full"
            disabled={starting || (active && !error)}
            onClick={() => void start()}
          >
            {t(starting ? 'oauth.loading' : reconnectId ? 'oauth.reconnect' : 'oauth.connect')}
          </Button>
        )}
        {flow?.status === 'ready' && error && (
          <Button
            variant="outline"
            onClick={() => {
              setError(null);
              setCompletionAttempt(value => value + 1);
              void query.refetch();
            }}
          >
            {t('actions.retry')}
          </Button>
        )}
        {!active && flow?.status !== 'ready' && (
          <Button
            type="button"
            variant="ghost"
            className="w-full"
            onClick={() => {
              if (onManual) onManual();
              else setManual(true);
            }}
          >
            {t('oauth.manual')}
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
