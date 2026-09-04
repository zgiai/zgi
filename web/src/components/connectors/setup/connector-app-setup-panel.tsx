'use client';

import { useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import QRCode from 'react-qr-code';
import { useQueryClient } from '@tanstack/react-query';
import { CheckCircle2, Loader2, LockKeyhole } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { ConnectorPermissionTemplateDetails } from '@/components/connectors/detail/connector-app-permissions';
import { useT } from '@/i18n';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import { connectorService } from '@/services/connector.service';
import { useConnectorOnboarding } from '@/hooks/connectors/use-connector-onboarding';
import { connectorRequestErrorDetails } from '@/hooks/connectors/connector-request-error';
import { invalidatePersonalConnectionQueries } from '@/hooks/connectors/connection-detail-query';
import { pendingSetupKeys, usePendingSetup } from '@/hooks/connectors/use-pending-setups';
import type { KeySetupResult, PersonalConnection } from '@/services/types/connector';
import type { ConnectorKeySetupPanelProps } from './connector-key-setup-panel';
import { useCreationDefault } from './use-creation-default';
import { activeOnboarding, appCredentialErrorKey, safeVerificationURL } from './app-setup-state';

type AppSetupProps = ConnectorKeySetupPanelProps & {
  updateConnection?: PersonalConnection;
  initialMode?: 'qr' | 'manual';
  initialIdentity?: { app_id: string; region: 'feishu' | 'lark' };
};
export function ConnectorAppSetupPanel(props: AppSetupProps) {
  const actor = useAuthStore.use.user()?.id;
  const organization = useOrganizationStore.use.currentOrganization()?.id;
  return (
    <ScopedConnectorAppSetupPanel
      key={`${organization}:${actor}:${props.connector.id}:${props.pendingSetupID ?? props.updateConnection?.id ?? 'create'}`}
      {...props}
    />
  );
}
function ScopedConnectorAppSetupPanel({
  connector,
  target,
  embedded = false,
  onReady,
  updateConnection,
  initialMode = 'qr',
  pendingSetupID,
  initialIdentity,
}: AppSetupProps) {
  const t = useT('connectors');
  const creationDefault = useCreationDefault(!updateConnection);
  const [mode, setMode] = useState(pendingSetupID ? 'manual' : initialMode);
  const pendingQuery = usePendingSetup(pendingSetupID);
  const pending = pendingQuery.data?.data;
  const [freshManual, setFreshManual] = useState(false);
  const [region, setRegion] = useState<'feishu' | 'lark'>(
    initialIdentity?.region ??
      updateConnection?.provider_evidence?.region ??
      (updateConnection?.external_account_id?.startsWith('lark:') ? 'lark' : 'feishu')
  );
  const [appID, setAppID] = useState(
    initialIdentity?.app_id ??
      updateConnection?.provider_evidence?.app_id ??
      updateConnection?.external_account_id?.split(':')[1] ??
      ''
  );
  const [secret, setSecret] = useState('');
  const [manualResult, setManualResult] = useState<KeySetupResult>();
  const [manualError, setManualError] = useState<string>();
  const [submitting, setSubmitting] = useState(false);
  const [expiryTick, setExpiryTick] = useState(0);
  const generation = useRef(0);
  const notified = useRef('');
  const queryClient = useQueryClient();
  const onboarding = useConnectorOnboarding(connector, updateConnection, pendingSetupID);
  const busy =
    submitting ||
    onboarding.busy ||
    activeOnboarding(onboarding.flow) ||
    manualResult?.state === 'verifying';
  const ready = manualResult?.state === 'ready' ? manualResult.connection : onboarding.connection;
  const isRestored = manualResult?.state !== 'ready' && onboarding.restored;
  const expiry = Date.parse(onboarding.flow?.expires_at ?? '');
  const expired = Number.isFinite(expiry) && expiry <= Date.now();
  const url = !expired ? safeVerificationURL(onboarding.flow?.verification_url) : undefined;
  const actor = useAuthStore.use.user()?.id;
  const organization = useOrganizationStore.use.currentOrganization()?.id;
  const current = (version: number) =>
    generation.current === version &&
    useAuthStore.getState().user?.id === actor &&
    useOrganizationStore.getState().currentOrganization?.id === organization;

  useEffect(() => {
    if (pending) {
      setRegion(pending.region);
      setAppID(pending.app_id);
    }
  }, [pending]);
  useEffect(() => {
    if (onboarding.flow?.pending_setup_id || onboarding.flow?.state === 'ready') {
      void queryClient.invalidateQueries({ queryKey: pendingSetupKeys });
    }
  }, [onboarding.flow?.state, onboarding.flow?.pending_setup_id, queryClient]);

  useEffect(() => {
    if (activeOnboarding(onboarding.flow)) setMode('qr');
    else if (pendingSetupID && onboarding.flow?.state === 'pending_completion') setMode('manual');
  }, [onboarding.flow, pendingSetupID]);
  useEffect(() => {
    if (!Number.isFinite(expiry) || expired) return;
    const timer = setTimeout(
      () => setExpiryTick(value => value + 1),
      Math.min(2147483647, Math.max(1, expiry - Date.now()))
    );
    return () => clearTimeout(timer);
  }, [expiry, expired, expiryTick]);
  useEffect(() => {
    const version = ++generation.current;
    return () => {
      generation.current = version + 1;
    };
  }, [actor, organization]);
  useEffect(() => {
    if (!ready || isRestored || notified.current === ready.id) return;
    notified.current = ready.id;
    const version = generation.current;
    void creationDefault.complete(ready).then(
      current =>
        current &&
        generation.current === version &&
        onReady?.(
          ready,
          manualResult?.state === 'ready'
            ? {
                attachment: manualResult.attachment,
                bindingWarning: manualResult.binding_warning,
              }
            : {}
        )
    );
  }, [ready, onReady, isRestored, manualResult, creationDefault]);
  useEffect(() => {
    if (manualResult?.state !== 'verifying') return;
    let alive = true;
    const timer = setTimeout(() => {
      void connectorService
        .getKeySetup(manualResult.attempt_id)
        .then(response => {
          if (
            !alive ||
            useAuthStore.getState().user?.id !== actor ||
            useOrganizationStore.getState().currentOrganization?.id !== organization
          ) {
            return;
          }
          setManualResult(response.data);
          if (response.data.state === 'ready') invalidatePersonalConnectionQueries(queryClient);
        })
        .catch(error => {
          if (alive) setManualError(connectorRequestErrorDetails(error).code);
        });
    }, 1000);
    return () => {
      alive = false;
      clearTimeout(timer);
    };
  }, [manualResult, queryClient, actor, organization]);

  const submit = async () => {
    if (busy || !actor || !organization || !appID.trim() || !secret.trim()) return;
    creationDefault.begin();
    const version = generation.current;
    const values: Record<string, string> = { region, app_id: appID.trim(), app_secret: secret };
    setSecret('');
    setSubmitting(true);
    setManualError(undefined);
    try {
      if (pendingSetupID) {
        const latest = await pendingQuery.refetch();
        if (!current(version)) return;
        const record = latest.data?.data;
        if (!record?.allowed_actions.includes('resume_manual')) {
          setManualError('connector_conflict');
          return;
        }
        const resumed = await connectorService.resumePendingManual(
          record.id,
          record.revision,
          values,
          crypto.randomUUID()
        );
        if (!current(version)) return;
        if (resumed.data.state === 'ready' && resumed.data.result_connection_id) {
          const saved = await connectorService.getConnection(resumed.data.result_connection_id);
          if (!current(version)) return;
          setManualResult({ state: 'ready', attempt_id: resumed.data.id, connection: saved.data });
          invalidatePersonalConnectionQueries(queryClient);
        } else if (
          resumed.data.error_code &&
          resumed.data.error_code !== 'connector_application_not_ready'
        ) {
          setManualError(resumed.data.error_code);
        }
        await queryClient.invalidateQueries({ queryKey: pendingSetupKeys });
        return;
      }
      const response = updateConnection
        ? await connectorService.reconnectConnection(
            updateConnection.id,
            { values },
            crypto.randomUUID()
          )
        : await connectorService.startAppCredentialSetup(
            connector.id,
            { values, ...(target ? { target } : {}) },
            crypto.randomUUID()
          );
      if (!current(version)) return;
      setManualResult(response.data);
      if (response.data.state === 'ready') invalidatePersonalConnectionQueries(queryClient);
      if (response.data.state === 'failed') setManualError(response.data.error_code);
    } catch (error) {
      if (current(version)) setManualError(connectorRequestErrorDetails(error).code);
    } finally {
      for (const name of Object.keys(values)) {
        values[name] = '';
        delete values[name];
      }
      if (current(version)) setSubmitting(false);
    }
  };

  // Swap the content of the original setup surface, not a second dialog. Old
  // default/host intents are discarded when verification becomes pending.
  if (
    !pendingSetupID &&
    onboarding.flow?.state === 'pending_completion' &&
    onboarding.flow.pending_setup_id
  ) {
    return (
      <ConnectorAppSetupPanel
        connector={connector}
        pendingSetupID={onboarding.flow.pending_setup_id}
        embedded={embedded}
        onReady={onReady}
      />
    );
  }
  if (freshManual && pending) {
    return (
      <ConnectorAppSetupPanel
        connector={connector}
        initialMode="manual"
        initialIdentity={pending}
        embedded={embedded}
        onReady={onReady}
      />
    );
  }
  if (pendingSetupID && !pending) {
    return (
      <Alert>
        <AlertDescription>
          {t(pendingQuery.isError ? 'pending.loadFailed' : 'connections.loading')}
        </AlertDescription>
        <Button variant="ghost" onClick={() => void pendingQuery.refetch()}>
          {t('actions.retry')}
        </Button>
      </Alert>
    );
  }
  if (
    pending &&
    !ready &&
    (pending.state === 'completed' || pending.state === 'expired' || pending.state === 'cancelled')
  ) {
    return (
      <div className="space-y-3">
        <Alert>
          <AlertTitle>{t(`pending.${pending.state}`)}</AlertTitle>
          <AlertDescription>
            {pending.app_id}
            <br />
            {t('pending.terminalHint')}
          </AlertDescription>
        </Alert>
        {pending.state === 'completed' ? (
          pending.result_connection_id ? (
            <Button asChild className="w-full">
              <Link
                href={`/console/connectors/connections/${encodeURIComponent(pending.result_connection_id)}`}
              >
                {t('app.manage')}
              </Link>
            </Button>
          ) : null
        ) : (
          <Button variant="outline" onClick={() => setFreshManual(true)}>
            {t('pending.newManual')}
          </Button>
        )}
      </div>
    );
  }

  const errorCode =
    mode === 'manual' ? manualError : (onboarding.errorCode ?? onboarding.flow?.error_code);
  const state = onboarding.flow?.state;
  return (
    <Card className={embedded ? 'border-0 shadow-none' : undefined}>
      {!embedded && (
        <CardHeader>
          <CardTitle>{t('app.title')}</CardTitle>
        </CardHeader>
      )}
      <CardContent className={embedded ? 'space-y-4 p-0' : 'space-y-4'}>
        {pending && !ready && (
          <Alert className="border-warning/30 bg-warning/5">
            <AlertTitle>{t('pending.title')}</AlertTitle>
            <AlertDescription>
              <p>
                {t(
                  pending.state === 'awaiting_application'
                    ? 'pending.awaitingHint'
                    : 'pending.verifyHint'
                )}
              </p>
              <p className="mt-2 font-mono text-xs break-all">
                {pending.app_id} · {t(pending.region === 'feishu' ? 'app.feishu' : 'app.lark')}
              </p>
              {pending.last_verification_at && (
                <p className="mt-1 text-xs">
                  {t('pending.lastChecked', {
                    date: new Date(pending.last_verification_at).toLocaleString(),
                  })}
                </p>
              )}
              <p className="mt-2">
                {t('pending.retention', {
                  date: new Date(pending.resume_expires_at).toLocaleDateString(),
                })}
              </p>
              {Date.parse(pending.resume_expires_at) - Date.now() < 3 * 86400000 && (
                <p>{t('pending.expiringSoon')}</p>
              )}
              {pending.resume_qr_unavailable_reason && (
                <p className="mt-2">{t('pending.qrUnavailable')}</p>
              )}
              {pending.reason_code && pending.reason_code !== 'connector_application_not_ready' && (
                <p className="mt-2">{t(appCredentialErrorKey(pending.reason_code))}</p>
              )}
              <p className="mt-2 text-xs">{t('pending.secretHint')}</p>
              <Button variant="link" asChild className="h-auto px-0">
                <a
                  href={
                    pending.region === 'feishu'
                      ? 'https://open.feishu.cn/app'
                      : 'https://open.larksuite.com/app'
                  }
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  {t('pending.openConsole')}
                </a>
              </Button>
              <Button
                variant="link"
                className="ml-3 h-auto px-0"
                disabled={pendingQuery.isFetching || submitting}
                onClick={async () => {
                  await pendingQuery.refetch();
                  // Refresh only an existing receipt; no implicit registration.
                  if (onboarding.flow?.id) await onboarding.retryStatus();
                }}
              >
                {t('pending.refresh')}
              </Button>
            </AlertDescription>
          </Alert>
        )}
        <p className="text-sm leading-6 text-muted-foreground">
          {t(pending ? 'pending.closeHint' : updateConnection ? 'app.updateHint' : 'app.notice')}
        </p>
        {ready ? (
          <div className="space-y-3">
            <Alert className="border-success/25 bg-success/5">
              <CheckCircle2 className="size-4 text-success" aria-hidden="true" />
              <AlertTitle>{t('app.ready')}</AlertTitle>
              <AlertDescription>
                {ready.name}
                <br />
                {t(
                  updateConnection && ready.id !== updateConnection.id
                    ? 'app.newConnection'
                    : 'app.readyHint'
                )}
              </AlertDescription>
            </Alert>
            {isRestored && onReady && (
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
            <Button variant="outline" asChild className="w-full">
              <Link href={`/console/connectors/connections/${encodeURIComponent(ready.id)}`}>
                {t('app.manage')}
              </Link>
            </Button>
          </div>
        ) : (
          <>
            {creationDefault.option(busy)}
            <div className="grid grid-cols-2 gap-2" role="group" aria-label={t('app.title')}>
              <Button
                variant={mode === 'qr' ? 'default' : 'outline'}
                disabled={
                  busy || Boolean(pending && !pending.allowed_actions.includes('resume_qr'))
                }
                onClick={() => {
                  setSecret('');
                  setMode('qr');
                }}
              >
                {t(pending ? 'pending.resumeQR' : updateConnection ? 'app.repair' : 'app.qr')}
              </Button>
              <Button
                variant={mode === 'manual' ? 'default' : 'outline'}
                disabled={busy}
                onClick={() => setMode('manual')}
              >
                {t('app.manual')}
              </Button>
            </div>
            {errorCode && (
              <Alert variant="destructive">
                <AlertTitle>{t('setup.failedTitle')}</AlertTitle>
                <AlertDescription>{t(appCredentialErrorKey(errorCode))}</AlertDescription>
              </Alert>
            )}
            {mode === 'manual' ? (
              <form
                className="space-y-4"
                onSubmit={event => {
                  event.preventDefault();
                  void submit();
                }}
              >
                <div className="space-y-2">
                  <Label htmlFor="connector-app-region">{t('app.region')}</Label>
                  <select
                    id="connector-app-region"
                    className="h-9 w-full rounded-md border bg-background px-3 text-sm"
                    disabled={busy || Boolean(updateConnection || pending)}
                    value={region}
                    onChange={event => setRegion(event.target.value as 'feishu' | 'lark')}
                  >
                    <option value="feishu">{t('app.feishu')}</option>
                    <option value="lark">{t('app.lark')}</option>
                  </select>
                </div>
                {connector.credential_fields
                  ?.filter(field => field.name !== 'region')
                  .map(field => (
                    <div key={field.name} className="space-y-2">
                      <Label htmlFor={`connector-app-${field.name}`}>{field.label}</Label>
                      <Input
                        id={`connector-app-${field.name}`}
                        type={field.secret ? 'password' : 'text'}
                        autoComplete="off"
                        spellCheck={false}
                        maxLength={field.max_length}
                        required={field.required}
                        disabled={busy || (!field.secret && Boolean(updateConnection || pending))}
                        value={field.secret ? secret : appID}
                        onChange={event =>
                          field.secret
                            ? setSecret(event.target.value)
                            : setAppID(event.target.value)
                        }
                      />
                    </div>
                  ))}
                <Button
                  type="submit"
                  className="w-full"
                  loading={submitting}
                  disabled={
                    busy ||
                    !appID.trim() ||
                    !secret.trim() ||
                    Boolean(pending && !pending.allowed_actions.includes('resume_manual'))
                  }
                >
                  {t(submitting ? 'app.verifying' : pending ? 'pending.verify' : 'setup.submit')}
                </Button>
                {manualError && (
                  <Button type="button" variant="outline" asChild className="w-full">
                    <Link href="/console/connectors/connections">
                      {t('app.refreshConnections')}
                    </Link>
                  </Button>
                )}
                {manualResult?.state === 'verifying' && manualError && (
                  <Button
                    type="button"
                    variant="outline"
                    className="w-full"
                    onClick={() => {
                      setManualError(undefined);
                      setManualResult({ ...manualResult });
                    }}
                  >
                    {t('app.restore')}
                  </Button>
                )}
              </form>
            ) : (
              <div className="space-y-4">
                <p className="text-sm text-muted-foreground">
                  {t(
                    pending
                      ? 'pending.resumeQRHint'
                      : updateConnection
                        ? 'app.repairHint'
                        : 'app.scanHint'
                  )}
                </p>
                {!pending && (
                  <ConnectorPermissionTemplateDetails template={connector.permission_template} />
                )}
                {updateConnection ? (
                  <p className="rounded-lg border bg-muted/20 p-3 text-sm">
                    {updateConnection.provider_evidence?.app_name ?? updateConnection.name}
                    <br />
                    <span className="font-mono text-xs">
                      {updateConnection.provider_evidence?.app_id ??
                        updateConnection.external_account_id}
                    </span>
                  </p>
                ) : pending ? null : (
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      variant={region === 'feishu' ? 'default' : 'outline'}
                      disabled={busy || onboarding.hasRecovery}
                      onClick={() => setRegion('feishu')}
                    >
                      {t('app.feishu')}
                    </Button>
                    <Button
                      size="sm"
                      variant={region === 'lark' ? 'default' : 'outline'}
                      disabled={busy || onboarding.hasRecovery}
                      onClick={() => setRegion('lark')}
                    >
                      {t('app.lark')}
                    </Button>
                  </div>
                )}
                {state === 'awaiting_authorization' && url ? (
                  <div className="space-y-3 rounded-lg border p-5 text-center">
                    <p className="text-sm font-medium">{t('app.scan')}</p>
                    <div className="mx-auto w-fit rounded-lg bg-white p-3">
                      <QRCode value={url} size={200} />
                    </div>
                    <p className="text-xs text-muted-foreground">{onboarding.flow?.user_code}</p>
                    <a
                      className="text-sm text-primary underline"
                      href={url}
                      target="_blank"
                      rel="noopener noreferrer"
                      referrerPolicy="no-referrer"
                    >
                      {t('app.open')}
                    </a>
                  </div>
                ) : null}
                {state === 'awaiting_authorization' && expired && (
                  <p role="status" className="text-sm text-muted-foreground">
                    {t('app.expired')}
                  </p>
                )}
                {(state === 'starting' || state === 'verifying' || onboarding.busy) && (
                  <div
                    className="flex items-center gap-2 rounded-lg border p-5 text-sm"
                    role="status"
                  >
                    <Loader2 aria-hidden="true" className="size-4 animate-spin" />
                    {t(state === 'verifying' ? 'app.verifying' : 'app.starting')}
                  </div>
                )}
                {state &&
                  !activeOnboarding(onboarding.flow) &&
                  state !== 'ready' &&
                  state !== 'pending_completion' && (
                    <Alert>
                      <AlertDescription>
                        {t(
                          state === 'recovery_required'
                            ? 'app.recovery'
                            : state === 'expired'
                              ? 'app.expired'
                              : state === 'cancelled'
                                ? 'app.cancelled'
                                : 'app.failed'
                        )}
                      </AlertDescription>
                    </Alert>
                  )}
                {activeOnboarding(onboarding.flow) && (
                  <Button
                    variant="outline"
                    className="w-full"
                    disabled={onboarding.busy}
                    onClick={() => void onboarding.cancel()}
                  >
                    {t('app.cancel')}
                  </Button>
                )}
                {onboarding.errorCode && onboarding.hasRecovery && (
                  <Button
                    variant="outline"
                    className="w-full"
                    disabled={onboarding.busy}
                    onClick={() => void onboarding.retryStatus()}
                  >
                    {t('app.restore')}
                  </Button>
                )}
                {!activeOnboarding(onboarding.flow) &&
                  state !== 'recovery_required' &&
                  !(onboarding.hasRecovery && !state) && (
                    <Button
                      className="w-full"
                      disabled={
                        busy || Boolean(pending && !pending.allowed_actions.includes('resume_qr'))
                      }
                      onClick={() => {
                        creationDefault.begin();
                        void onboarding.start(region, false, pending?.revision);
                      }}
                    >
                      {t(
                        pending ? 'pending.resumeQR' : updateConnection ? 'app.repair' : 'app.start'
                      )}
                    </Button>
                  )}
              </div>
            )}
          </>
        )}
        <p className="flex items-start gap-2 border-t pt-4 text-xs leading-5 text-muted-foreground">
          <LockKeyhole className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
          {t('app.secretNotice')}
        </p>
      </CardContent>
    </Card>
  );
}
