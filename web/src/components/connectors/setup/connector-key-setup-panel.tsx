'use client';

import { useEffect, useRef } from 'react';
import Link from 'next/link';
import { AlertCircle, LockKeyhole, UserRound } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { useConnectorSetup } from '@/hooks/connectors/use-connector-setup';
import { useT, type ScopedTranslations } from '@/i18n';
import type {
  ConnectorCatalogItem,
  ConnectorTargetBinding,
  ConnectorTargetRef,
  PersonalConnection,
} from '@/services/types/connector';
import { ConnectorKeyForm } from './connector-key-form';
import { ConnectorSetupStatus } from './connector-setup-status';
import { ConnectorAppSetupPanel } from './connector-app-setup-panel';
import { useCreationDefault } from './use-creation-default';
import { supportsAppSetup } from './app-setup-state';
import { supportsMessagingContract } from './messaging-setup-state';
import { ConnectorMessagingSetupPanel } from './connector-messaging-setup-panel';

function errorText(t: ScopedTranslations<'connectors'>, code: string | undefined): string {
  if (
    code === 'connector_key_invalid' ||
    code === 'invalid_key' ||
    code === 'key_invalid' ||
    code === 'key_revoked'
  ) {
    return t('setup.errors.invalidKey');
  }
  if (
    code === 'connector_key_insufficient' ||
    code === 'insufficient_key' ||
    code === 'insufficient_access'
  ) {
    return t('setup.errors.insufficientKey');
  }
  if (code === 'connector_provider_unavailable' || code === 'provider_unavailable') {
    return t('setup.errors.providerUnavailable');
  }
  return t('setup.errors.requestFailed');
}

export interface ConnectorKeySetupPanelProps {
  connector: ConnectorCatalogItem;
  target?: ConnectorTargetRef;
  title?: string;
  description?: string;
  submitLabel?: string;
  embedded?: boolean;
  /** Connector creation completed; the host remains responsible for its draft/save transaction. */
  onReady?: (
    connection: PersonalConnection,
    result: { attachment?: ConnectorTargetBinding; bindingWarning?: string }
  ) => void;
}

export function ConnectorKeySetupPanel(props: ConnectorKeySetupPanelProps) {
  if (supportsMessagingContract(props.connector))
    {return <ConnectorMessagingSetupPanel {...props} />;}
  if (supportsAppSetup(props.connector)) {
    return (
      <ConnectorAppSetupPanel
        key={`${props.connector.id}:${JSON.stringify(props.target ?? null)}`}
        {...props}
      />
    );
  }
  return <LegacyConnectorKeySetupPanel {...props} />;
}

function LegacyConnectorKeySetupPanel({
  connector,
  target,
  title,
  description,
  submitLabel,
  embedded = false,
  onReady,
}: ConnectorKeySetupPanelProps) {
  const t = useT('connectors');
  const creationDefault = useCreationDefault();
  const notifiedConnectionIDRef = useRef('');
  const setup = useConnectorSetup(connector.id, connector.key_fields, {
    enabled: connector.key_fields.length > 0 && connector.capabilities.length > 0,
    target,
  });

  useEffect(() => {
    const connection = setup.state.connection;
    if (setup.state.status !== 'ready' || !connection) return;
    if (notifiedConnectionIDRef.current === connection.id) return;
    notifiedConnectionIDRef.current = connection.id;
    void creationDefault.complete(connection).then(
      current =>
        current &&
        onReady?.(connection, {
          attachment: setup.state.attachment,
          bindingWarning: setup.state.bindingWarning,
        })
    );
  }, [
    creationDefault,
    onReady,
    setup.state.attachment,
    setup.state.bindingWarning,
    setup.state.connection,
    setup.state.status,
  ]);

  return (
    <Card className={embedded ? 'border-0 shadow-none' : undefined}>
      {!embedded ? (
        <CardHeader>
          <CardTitle>{title ?? t('setup.title')}</CardTitle>
          <p className="text-sm leading-5 text-muted-foreground">
            {description ?? t('setup.description')}
          </p>
        </CardHeader>
      ) : null}
      <CardContent className={embedded ? 'space-y-5 p-0' : 'space-y-5'}>
        {setup.state.status === 'failed' || setup.state.status === 'unknown' ? (
          <div className="space-y-3">
            <Alert variant="destructive">
              <AlertCircle aria-hidden="true" className="size-4" />
              <AlertTitle>
                {t(setup.state.status === 'unknown' ? 'setup.unknownTitle' : 'setup.failedTitle')}
              </AlertTitle>
              <AlertDescription>
                {setup.state.status === 'unknown'
                  ? t('setup.errors.resultUnknown')
                  : errorText(t, setup.state.errorCode)}
              </AlertDescription>
            </Alert>
            {setup.state.status === 'unknown' ? (
              <Button asChild variant="outline" className="w-full">
                <Link href="/console/connectors/connections" onClick={setup.refreshConnections}>
                  {t('setup.checkConnections')}
                </Link>
              </Button>
            ) : null}
            <Button type="button" variant="outline" className="w-full" onClick={setup.retry}>
              {t('setup.retry')}
            </Button>
          </div>
        ) : null}

        {setup.state.status === 'idle' ? (
          <div className="space-y-3">
            {creationDefault.option(false)}
            <ConnectorKeyForm
              fields={connector.key_fields}
              values={setup.state.values}
              canSubmit={setup.canSubmit}
              onValueChange={setup.updateValue}
              onSubmit={() => {
                creationDefault.begin();
                void setup.submit();
              }}
              submitLabel={submitLabel}
            />
            <p className="text-xs leading-5 text-muted-foreground">{t('setup.nameAfterReady')}</p>
          </div>
        ) : null}

        <ConnectorSetupStatus
          state={setup.state}
          showDefaultAction={!onReady}
          statusError={setup.statusError}
          statusRefreshing={setup.statusRefreshing}
          onRetryStatus={setup.retryStatus}
        />

        {setup.state.status !== 'ready' ? (
          <div className="space-y-2 border-t pt-4 text-xs leading-5 text-muted-foreground">
            <p className="flex items-start gap-2">
              <LockKeyhole aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
              <span>{t('setup.securityNotice')}</span>
            </p>
            <p className="flex items-start gap-2">
              <UserRound aria-hidden="true" className="mt-0.5 size-3.5 shrink-0" />
              <span>{t('setup.ownerOnly')}</span>
            </p>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}
