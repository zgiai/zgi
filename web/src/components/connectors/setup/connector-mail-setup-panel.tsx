'use client';

import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible';
import { ChevronRight } from 'lucide-react';

import { useEffect, useRef } from 'react';
import Link from 'next/link';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { useT } from '@/i18n';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { useConnectorMailSetup } from '@/hooks/connectors/use-connector-mail-setup';
import type { PersonalConnection } from '@/services/types/connector';
import type { ConnectorKeySetupPanelProps } from './connector-key-setup-panel';
import { ConnectorSetupStatus } from './connector-setup-status';
import { ConnectorMailInformation } from '../detail/connector-mail-information';
import { useCreationDefault } from './use-creation-default';
import {
  mailFailureMessage,
  mailSetupEnabled,
  normalizeMailSecret,
  normalizeMailProfileEmail,
  connectionMailUsage,
} from './mail-setup-state';

type Props = ConnectorKeySetupPanelProps & {
  updateConnection?: PersonalConnection;
  onBusyChange?: (busy: boolean) => void;
};

export function ConnectorMailSetupPanel(props: Props) {
  const actor = useAuthStore.use.user()?.id;
  const org = useOrganizationStore.use.currentOrganization()?.id;
  return (
    <MailSetup
      key={`${org}:${actor}:${props.connector.id}:${props.updateConnection?.id ?? 'new'}:${JSON.stringify(props.target)}`}
      {...props}
    />
  );
}

function MailSetup({ connector, updateConnection, onReady, submitLabel, onBusyChange }: Props) {
  const t = useT('connectors');
  const setup = useConnectorMailSetup(connector, updateConnection);
  const creationDefault = useCreationDefault(!updateConnection);
  const notified = useRef('');
  useEffect(() => {
    const ready = setup.state.connection;
    if (setup.state.status !== 'ready' || !ready || notified.current === ready.id) return;
    notified.current = ready.id;
    void creationDefault.complete(ready).then(current => {
      if (current) onReady?.(ready, {});
    });
  }, [setup.state, creationDefault, onReady]);
  const state = setup.state;
  useEffect(() => {
    onBusyChange?.(state.status !== 'idle');
  }, [state.status, onBusyChange]);
  const upgrading = Boolean(
    updateConnection && connectionMailUsage(updateConnection) !== 'read_send'
  );
  const gmail = connector.id === 'gmail';
  return (
    <div className="space-y-4">
      {!mailSetupEnabled(connector) && (
        <Alert>
          <AlertDescription>{t('messaging.unavailable')}</AlertDescription>
        </Alert>
      )}
      {state.status === 'idle' && (
        <form
          className="space-y-4"
          autoComplete="off"
          onSubmit={event => {
            event.preventDefault();
            if (setup.canSubmit) {
              creationDefault.begin();
              void setup.submit();
            }
          }}
        >
          {!upgrading && <p className="text-sm text-muted-foreground">{t('mail.fixedUsage')}</p>}
          {upgrading && (
            <p className="text-sm text-muted-foreground">{t('mail.upgradeDescription')}</p>
          )}
          <div className="space-y-2">
            <Label htmlFor="gmail-email">{t('mail.email')}</Label>
            <Input
              id="gmail-email"
              name="email"
              type="email"
              autoComplete="off"
              autoCapitalize="none"
              spellCheck={false}
              required
              readOnly={Boolean(updateConnection)}
              value={setup.email}
              onChange={event => setup.setEmail(event.target.value)}
            />
            {updateConnection && (
              <p className="text-xs text-muted-foreground">{t('mail.lockedEmail')}</p>
            )}
            {setup.email && !normalizeMailProfileEmail(connector.id, setup.email) && (
              <p className="text-xs text-destructive">
                {t(
                  connector.id === 'qqmail'
                    ? 'mail.qqEmailHint'
                    : connector.id === 'netease163'
                      ? 'mail.neteaseEmailHint'
                      : 'mail.invalidEmail'
                )}
              </p>
            )}
          </div>
          <div className="space-y-2">
            <Label htmlFor="gmail-app-password">
              {t(gmail ? 'mail.appPassword' : 'mail.authorizationCode')}
            </Label>
            <Input
              id="gmail-app-password"
              name={gmail ? 'app_password' : 'authorization_code'}
              type="password"
              autoComplete="new-password"
              autoCapitalize="none"
              spellCheck={false}
              required
              maxLength={128}
              value={setup.password}
              onChange={event => setup.setPassword(event.target.value)}
            />
            <ConnectorMailInformation provider={connector.id} />
            {setup.password && !normalizeMailSecret(connector.id, setup.password) && (
              <p className="text-xs text-destructive">
                {t(gmail ? 'mail.invalidPassword' : 'mail.invalidAuthorizationCode')}
              </p>
            )}
          </div>
          <Button type="submit" className="w-full" disabled={!setup.canSubmit}>
            {upgrading
              ? t('mail.upgradeReconnect')
              : (submitLabel ?? t(updateConnection ? 'mail.reconnect' : 'mail.connect'))}
          </Button>
        </form>
      )}
      {(state.status === 'failed' || state.status === 'unknown') && (
        <div className="space-y-3">
          <Alert variant="destructive">
            <AlertDescription>
              {t(
                state.status === 'unknown'
                  ? 'setup.errors.resultUnknown'
                  : mailFailureMessage(state.errorCode)
              )}
            </AlertDescription>
          </Alert>
          {setup.verification && (
            <Collapsible className="space-y-1 rounded-lg border p-3 text-sm">
              <CollapsibleTrigger asChild>
                <Button
                  variant="ghost"
                  className="group h-auto max-w-full justify-start gap-2 whitespace-normal px-0 py-1 text-left hover:bg-transparent text-sm"
                >
                  <ChevronRight
                    aria-hidden="true"
                    className="size-4 shrink-0 transition-transform group-data-[state=open]:rotate-90"
                  />
                  <span className="min-w-0">
                    {t(
                      setup.verification.imap_status === 'failed' &&
                        setup.verification.smtp_status === 'failed'
                        ? 'mail.partialFailure'
                        : setup.verification.imap_status === 'failed'
                          ? 'mail.readFailed'
                          : 'mail.sendFailed'
                    )}
                  </span>
                </Button>
              </CollapsibleTrigger>
              <CollapsibleContent className="space-y-3">
                <p>IMAP: {t(`mail.protocol.${setup.verification.imap_status}`)}</p>
                <p>SMTP: {t(`mail.protocol.${setup.verification.smtp_status}`)}</p>
              </CollapsibleContent>
            </Collapsible>
          )}
          {state.status === 'failed' && <ConnectorMailInformation provider={connector.id} />}
          {state.status === 'unknown' ? (
            <Button asChild variant="outline">
              <Link href="/console/connectors/connections">{t('setup.checkConnections')}</Link>
            </Button>
          ) : (
            <Button type="button" variant="outline" onClick={setup.reset}>
              {t('mail.retry')}
            </Button>
          )}
        </div>
      )}
      <ConnectorSetupStatus
        state={state}
        showDefaultAction={!onReady && !updateConnection}
        statusError={setup.statusError}
        statusRefreshing={setup.statusRefreshing}
        onRetryStatus={setup.retryStatus}
      />
    </div>
  );
}
