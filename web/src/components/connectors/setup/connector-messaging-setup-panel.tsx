'use client';

import { useEffect, useRef, useState } from 'react';
import { useLocale } from 'next-intl';
import Link from 'next/link';
import QRCode from 'react-qr-code';
import { useQueryClient } from '@tanstack/react-query';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Alert, AlertDescription } from '@/components/ui/alert';
import { useT } from '@/i18n';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { connectorService } from '@/services/connector.service';
import { useConnectorOnboarding } from '@/hooks/connectors/use-connector-onboarding';
import { invalidatePersonalConnectionQueries } from '@/hooks/connectors/connection-detail-query';
import { connectorRequestErrorDetails } from '@/hooks/connectors/connector-request-error';
import { resolveConnectorLocalizedText } from '../connector-metadata';
import type { KeySetupResult, PersonalConnection } from '@/services/types/connector';
import type { ConnectorKeySetupPanelProps } from './connector-key-setup-panel';
import { useCreationDefault } from './use-creation-default';
import { activeOnboarding } from './app-setup-state';
import { messagingMethod, messagingQR } from './messaging-setup-state';

type Props = ConnectorKeySetupPanelProps & {
  updateConnection?: PersonalConnection;
  initialMode?: 'manual' | 'qr';
};
export function ConnectorMessagingSetupPanel(props: Props) {
  const actor = useAuthStore.use.user()?.id;
  const org = useOrganizationStore.use.currentOrganization()?.id;
  return (
    <MessagingSetup
      key={`${org}:${actor}:${props.connector.id}:${props.updateConnection?.id ?? 'new'}:${JSON.stringify(props.target)}`}
      {...props}
    />
  );
}

function MessagingSetup({ connector, updateConnection, onReady, initialMode = 'qr' }: Props) {
  const t = useT('connectors');
  const locale = useLocale();
  const actor = useAuthStore.use.user()?.id;
  const org = useOrganizationStore.use.currentOrganization()?.id;
  const manual = messagingMethod(connector, 'manual');
  const qrEnabled = messagingMethod(connector, updateConnection ? 'qr_reauthorize' : 'qr_connect');
  const [mode, setMode] = useState(
    manual && (initialMode === 'manual' || !qrEnabled) ? 'manual' : 'qr'
  );
  const [values, setValues] = useState<Record<string, string>>({});
  const [code, setCode] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [manualResult, setManualResult] = useState<KeySetupResult>();
  const [manualError, setManualError] = useState<string>();
  const [clock, setClock] = useState(Date.now());
  const alive = useRef(true);
  const notified = useRef('');
  const manualKey = useRef<string>();
  const queryClient = useQueryClient();
  const creationDefault = useCreationDefault(!updateConnection);
  const onboarding = useConnectorOnboarding(connector, updateConnection);
  const current = () =>
    alive.current &&
    useAuthStore.getState().user?.id === actor &&
    useOrganizationStore.getState().currentOrganization?.id === org;
  const active = activeOnboarding(onboarding.flow);
  const busy = submitting || onboarding.busy || active || manualResult?.state === 'verifying';
  const ready = manualResult?.state === 'ready' ? manualResult.connection : onboarding.connection;
  const restored = manualResult?.state !== 'ready' && onboarding.restored;
  const payload = messagingQR(connector, onboarding.flow, clock);
  const localExpired = Boolean(
    onboarding.flow && Date.parse(onboarding.flow.expires_at) <= clock && active
  );
  const error =
    mode === 'manual' ? manualError : (onboarding.errorCode ?? onboarding.flow?.error_code);

  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
    };
  }, []);
  useEffect(() => {
    if (active) setMode('qr');
  }, [active]);
  useEffect(() => {
    if (!active) return;
    const timer = setInterval(() => setClock(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [active]);
  useEffect(() => {
    if (!ready || restored || notified.current === ready.id) return;
    notified.current = ready.id;
    void creationDefault.complete(ready).then(valid => {
      if (valid && alive.current) onReady?.(ready, {});
    });
  }, [ready, restored, onReady, creationDefault]);
  useEffect(() => {
    if (manualResult?.state !== 'verifying') return;
    let cancelled = false;
    const timer = setTimeout(() => {
      void connectorService
        .getKeySetup(manualResult.attempt_id)
        .then(result => {
          if (
            !cancelled &&
            alive.current &&
            useAuthStore.getState().user?.id === actor &&
            useOrganizationStore.getState().currentOrganization?.id === org
          ) {
            setManualResult(result.data);
            setManualError(result.data.state === 'failed' ? result.data.error_code : undefined);
            if (result.data.state === 'ready') invalidatePersonalConnectionQueries(queryClient);
          }
        })
        .catch(() => {
          if (!cancelled && alive.current) setManualError('transport_unconfirmed');
        });
    }, 1500);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [manualResult, actor, org, queryClient]);

  async function submitManual() {
    if (busy || !manual || !current()) return;
    const input = { ...values };
    if (connector.credential_fields?.some(field => field.required && !input[field.name]?.trim())) {
      return;
    }
    manualKey.current ??= crypto.randomUUID();
    setSubmitting(true);
    setManualError(undefined);
    setValues({});
    creationDefault.begin();
    try {
      // Connection creation is independent of target attachment. The host's
      // existing onReady/save flow owns the current draft binding.
      const response = updateConnection
        ? await connectorService.reconnectConnection(
            updateConnection.id,
            { values: input },
            manualKey.current
          )
        : await connectorService.startCredentialSetup(
            connector.id,
            { values: input },
            manualKey.current
          );
      if (!current()) return;
      setManualResult(response.data);
      if (response.data.state === 'ready') invalidatePersonalConnectionQueries(queryClient);
      if (response.data.state === 'failed') setManualError(response.data.error_code);
    } catch (failure) {
      if (current()) setManualError(connectorRequestErrorDetails(failure).code);
    } finally {
      for (const name of Object.keys(input)) {
        input[name] = '';
        delete input[name];
      }
      if (current()) setSubmitting(false);
    }
  }

  const stateText = localExpired
    ? 'messaging.localExpired'
    : onboarding.flow?.step === 'verify'
      ? 'messaging.verifying'
      : (
          {
            starting: 'messaging.starting',
            awaiting_authorization: 'messaging.waiting',
            verifying: 'messaging.verifying',
            ready: 'messaging.ready',
            cancelled: 'messaging.cancelled',
            expired: 'messaging.localExpired',
            failed: 'messaging.failed',
            recovery_required: 'messaging.recovery',
            // Not emitted by protocol 1; fail closed if a mixed-version server does.
            pending_completion: 'messaging.recovery',
          } as const
        )[onboarding.flow?.state ?? 'starting'];

  return (
    <div className="space-y-4">
      <p className="text-sm leading-6 text-muted-foreground">
        {t(updateConnection ? 'messaging.reauthorizeHint' : 'messaging.notice')}
      </p>
      <p className="text-sm leading-6">
        {t(
          connector.id === 'dingtalk'
            ? 'messaging.dingtalk'
            : connector.id === 'wecom'
              ? 'messaging.wecom'
              : 'messaging.weixin'
        )}
      </p>
      {ready ? (
        <div className="space-y-3">
          <Alert>
            <AlertDescription>
              {t('messaging.ready')} · {ready.name}
            </AlertDescription>
          </Alert>
          {restored && onReady && (
            <Button
              className="w-full"
              onClick={() => {
                if (notified.current !== ready.id) {
                  notified.current = ready.id;
                  onReady(ready, {});
                }
              }}
            >
              {t('app.use')}
            </Button>
          )}
          <Button asChild variant="outline" className="w-full">
            <Link href={`/console/connectors/connections/${encodeURIComponent(ready.id)}`}>
              {t('app.manage')}
            </Link>
          </Button>
        </div>
      ) : (
        <>
          {connector.setup_availability?.enabled === false && (
            <Alert>
              <AlertDescription>{t('messaging.unavailable')}</AlertDescription>
            </Alert>
          )}
          {creationDefault.option(busy)}
          <div className="flex gap-2">
            <Button
              disabled={busy || !qrEnabled}
              variant={mode === 'qr' ? 'default' : 'outline'}
              onClick={() => {
                setValues({});
                setMode('qr');
              }}
            >
              {t('messaging.qr')}
            </Button>
            {manual && (
              <Button
                disabled={busy}
                variant={mode === 'manual' ? 'default' : 'outline'}
                onClick={() => setMode('manual')}
              >
                {t('app.manual')}
              </Button>
            )}
          </div>
          {!qrEnabled && connector.setup_availability?.enabled && (
            <p className="text-sm text-muted-foreground">{t('messaging.qrUnavailable')}</p>
          )}
          {error && (
            <Alert variant="destructive">
              <AlertDescription>
                {t(
                  error === 'transport_unconfirmed' ||
                    error === 'connector_operation_outcome_unknown'
                    ? 'messaging.recovery'
                    : 'messaging.failed'
                )}
              </AlertDescription>
            </Alert>
          )}
          {mode === 'manual' && manual ? (
            <form
              className="space-y-4"
              onSubmit={event => {
                event.preventDefault();
                void submitManual();
              }}
            >
              {connector.credential_fields?.map(field => (
                <div className="space-y-2" key={field.name}>
                  <Label htmlFor={`messaging-${field.name}`}>
                    {resolveConnectorLocalizedText(field.label_i18n, locale, field.label)}
                  </Label>
                  <Input
                    id={`messaging-${field.name}`}
                    name={field.name}
                    type={field.secret ? 'password' : 'text'}
                    autoComplete="off"
                    spellCheck={false}
                    required={field.required}
                    maxLength={field.max_length}
                    disabled={busy}
                    value={values[field.name] ?? ''}
                    onChange={event =>
                      setValues(previous => ({ ...previous, [field.name]: event.target.value }))
                    }
                  />
                </div>
              ))}
              <Button type="submit" className="w-full" disabled={busy || Boolean(manualError)}>
                {t('messaging.verify')}
              </Button>
              {manualError &&
              (manualResult?.state === 'verifying' ||
                (manualError !== 'transport_unconfirmed' &&
                  manualError !== 'connector_operation_outcome_unknown')) ? (
                <Button
                  variant="outline"
                  type="button"
                  onClick={() => {
                    if (manualResult?.state === 'verifying') {
                      void connectorService
                        .getKeySetup(manualResult.attempt_id)
                        .then(result => {
                          if (!current()) return;
                          setManualResult(result.data);
                          setManualError(
                            result.data.state === 'failed' ? result.data.error_code : undefined
                          );
                          if (result.data.state === 'ready') {
                            invalidatePersonalConnectionQueries(queryClient);
                          }
                        })
                        .catch(() => {
                          if (current()) setManualError('transport_unconfirmed');
                        });
                    } else {
                      manualKey.current = undefined;
                      setManualError(undefined);
                      setManualResult(undefined);
                    }
                  }}
                >
                  {t(
                    manualResult?.state === 'verifying'
                      ? 'app.refreshConnections'
                      : 'messaging.startAgain'
                  )}
                </Button>
              ) : manualError ? (
                <Button asChild variant="outline">
                  <Link href="/console/connectors/connections">{t('app.refreshConnections')}</Link>
                </Button>
              ) : null}
            </form>
          ) : (
            <div className="space-y-3">
              {onboarding.flow && (
                <p role="status" className="text-sm">
                  {t(stateText)}
                </p>
              )}
              {payload && (
                <div className="mx-auto w-fit rounded-lg border bg-white p-4">
                  <QRCode value={payload} size={208} aria-label={t('messaging.qr')} />
                </div>
              )}
              {payload && (
                <p className="text-center text-xs text-muted-foreground">
                  {t('messaging.scanHint')}
                </p>
              )}
              {onboarding.flow?.step === 'awaiting_code' && !localExpired && (
                <form
                  className="space-y-2"
                  onSubmit={event => {
                    event.preventDefault();
                    const value = code;
                    setCode('');
                    void onboarding.submitCode(value);
                  }}
                >
                  <Label htmlFor="messaging-code">{t('messaging.code')}</Label>
                  <Input
                    id="messaging-code"
                    type="password"
                    inputMode="numeric"
                    autoComplete="off"
                    value={code}
                    onChange={event => setCode(event.target.value.replace(/\D/g, '').slice(0, 12))}
                  />
                  <Button disabled={onboarding.busy || !code || Boolean(onboarding.errorCode)}>
                    {t('messaging.submitCode')}
                  </Button>
                </form>
              )}
              {active && (
                <Button
                  variant="outline"
                  disabled={onboarding.busy}
                  onClick={() => {
                    setCode('');
                    void onboarding.cancel();
                  }}
                >
                  {t('messaging.cancel')}
                </Button>
              )}
              {onboarding.hasRecovery && (
                <Button
                  variant="outline"
                  disabled={onboarding.busy}
                  onClick={() => void onboarding.retryStatus()}
                >
                  {t('messaging.check')}
                </Button>
              )}
              {onboarding.flow?.state === 'recovery_required' && qrEnabled && (
                <Button
                  variant="outline"
                  disabled={onboarding.busy}
                  onClick={() => onboarding.clearTerminal()}
                >
                  {t('messaging.restart')}
                </Button>
              )}
              {!active &&
                onboarding.flow?.state !== 'recovery_required' &&
                !(onboarding.hasRecovery && !onboarding.flow) && (
                  <Button
                    className="w-full"
                    disabled={!qrEnabled || busy}
                    onClick={() => {
                      setClock(Date.now());
                      creationDefault.begin();
                      void onboarding.start();
                    }}
                  >
                    {t('messaging.start')}
                  </Button>
                )}
            </div>
          )}
        </>
      )}
      <p className="border-t pt-3 text-xs leading-5 text-muted-foreground">
        {t('messaging.secret')}
      </p>
    </div>
  );
}
