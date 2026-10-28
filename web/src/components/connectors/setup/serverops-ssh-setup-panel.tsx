'use client';

import { useCallback, useEffect, useRef, useState } from 'react';
import Link from 'next/link';
import { useQueryClient } from '@tanstack/react-query';
import { AlertCircle, AlertTriangle, CheckCircle2, Loader2, ShieldCheck } from 'lucide-react';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { invalidatePersonalConnectionQueries } from '@/hooks/connectors/connection-detail-query';
import { connectorRequestErrorDetails } from '@/hooks/connectors/connector-request-error';
import { useT } from '@/i18n';
import { connectorService } from '@/services/connector.service';
import { useAuthStore } from '@/store/auth-store';
import { useOrganizationStore } from '@/store/organization-store';
import type { KeySetupResult, PersonalConnection } from '@/services/types/connector';
import type { ConnectorKeySetupPanelProps } from './connector-key-setup-panel';
import { useCreationDefault } from './use-creation-default';
import {
  SERVEROPS_SSH_HOST_FIELD,
  SERVEROPS_SSH_HOST_KEY_FIELD,
  SERVEROPS_SSH_PASSWORD_FIELD,
  SERVEROPS_SSH_PASSPHRASE_FIELD,
  SERVEROPS_SSH_PORT_FIELD,
  SERVEROPS_SSH_PRIVATE_KEY_FIELD,
  SERVEROPS_SSH_USERNAME_FIELD,
  buildServerOpsSSHCredentialPayload,
  buildServerOpsSSHProbePayload,
  clearServerOpsSSHSecretValues,
  createServerOpsSSHFormValues,
  destroyServerOpsSSHCredentialPayload,
  destroyServerOpsSSHSecretValues,
  serverOpsSSHFieldError,
  serverOpsSSHObservationSignature,
  serverOpsSSHObservationTargetSignature,
  serverOpsSSHProbeFieldError,
  type ServerOpsSSHFieldError,
  type ServerOpsSSHAuthMethod,
  type ServerOpsSSHPrivilege,
  type ServerOpsSSHHostKeyObservation,
} from './serverops-ssh-setup-state';

type ServerOpsSSHMessageKey =
  | 'serveropsSsh.errors.host'
  | 'serveropsSsh.errors.port'
  | 'serveropsSsh.errors.username'
  | 'serveropsSsh.errors.password'
  | 'serveropsSsh.errors.privateKey'
  | 'serveropsSsh.errors.passphrase'
  | 'serveropsSsh.errors.fingerprint'
  | 'serveropsSsh.errors.rejected'
  | 'serveropsSsh.errors.unreachable'
  | 'serveropsSsh.errors.resultUnknown'
  | 'serveropsSsh.errors.staleObservation'
  | 'serveropsSsh.errors.fingerprintChanged'
  | 'serveropsSsh.errors.requestFailed';

const FIELD_MESSAGE_KEY: Record<ServerOpsSSHFieldError, ServerOpsSSHMessageKey> = {
  host: 'serveropsSsh.errors.host',
  port: 'serveropsSsh.errors.port',
  username: 'serveropsSsh.errors.username',
  password: 'serveropsSsh.errors.password',
  private_key: 'serveropsSsh.errors.privateKey',
  passphrase: 'serveropsSsh.errors.passphrase',
  host_key_sha256: 'serveropsSsh.errors.fingerprint',
};

/** Stable client-side codes for a confirmation whose form no longer matches. */
const OBSERVATION_STALE_CODE = 'connector_host_key_observation_stale';
const FINGERPRINT_CHANGED_CODE = 'connector_host_key_fingerprint_changed';

/** Only stable catalog codes are projected; anything else fails closed to a retry hint. */
function failureMessageKey(code: string | undefined): ServerOpsSSHMessageKey {
  switch (code) {
    case 'connector_app_credentials_invalid':
    case 'connector_key_invalid':
    case 'connector_invalid_input':
      return 'serveropsSsh.errors.rejected';
    case 'connector_provider_unavailable':
    case 'connector_unavailable':
      return 'serveropsSsh.errors.unreachable';
    case 'transport_unconfirmed':
    case 'connector_operation_outcome_unknown':
      return 'serveropsSsh.errors.resultUnknown';
    case OBSERVATION_STALE_CODE:
      return 'serveropsSsh.errors.staleObservation';
    case FINGERPRINT_CHANGED_CODE:
      return 'serveropsSsh.errors.fingerprintChanged';
    default:
      return 'serveropsSsh.errors.requestFailed';
  }
}

/** The probe owns the pre-save stages; the final strict save reuses the existing result flow. */
type ServerOpsSSHStage = 'form' | 'probing' | 'confirmation' | 'submitting';

/** Stored only while a confirmation is pending; contains no credential material. */
type ServerOpsSSHObservationState = ServerOpsSSHHostKeyObservation & {
  signature: string;
  targetSignature: string;
};

export type ServerOpsSSHSetupPanelProps = ConnectorKeySetupPanelProps & {
  /** Present only for reconnect; the existing connection keeps its alias and bindings. */
  updateConnection?: PersonalConnection;
};

/**
 * Dedicated Server Operations SSH setup surface shared by Connector Center and
 * embedded Agent/Digital Human creation flows.
 *
 * The flow is two-stage. A read-only probe reads the server host key first, the
 * user explicitly confirms the observed identity, and only then does the
 * existing strict credential setup or reconnect path persist the connection.
 * The alias is owned by Connection Center, so this panel never sends a display
 * name, and a reconnect whose verified identity differs is reported as a
 * separate connection instead of being adopted silently.
 */
export function ServerOpsSSHSetupPanel(props: ServerOpsSSHSetupPanelProps) {
  const actor = useAuthStore.use.user()?.id;
  const organization = useOrganizationStore.use.currentOrganization()?.id;
  return (
    <ScopedServerOpsSSHSetupPanel
      key={`${organization}:${actor}:${props.connector.id}:${props.updateConnection?.id ?? 'create'}`}
      {...props}
    />
  );
}

function ScopedServerOpsSSHSetupPanel({
  connector,
  embedded = false,
  title,
  description,
  onReady,
  updateConnection,
}: ServerOpsSSHSetupPanelProps) {
  const t = useT('connectors');
  const queryClient = useQueryClient();
  const creationDefault = useCreationDefault(!updateConnection);
  const actor = useAuthStore.use.user()?.id;
  const organization = useOrganizationStore.use.currentOrganization()?.id;
  const [values, setValues] = useState(createServerOpsSSHFormValues);
  const [authMethod, setAuthMethod] = useState<ServerOpsSSHAuthMethod>('password');
  const [privilege, setPrivilege] = useState<ServerOpsSSHPrivilege>('user');
  const [showVerificationSettings, setShowVerificationSettings] = useState(false);
  const [stage, setStage] = useState<ServerOpsSSHStage>('form');
  const [observation, setObservation] = useState<ServerOpsSSHObservationState>();
  const [checking, setChecking] = useState(false);
  const [result, setResult] = useState<KeySetupResult>();
  const [failure, setFailure] = useState<{ code: string; unknown: boolean }>();
  const [fieldError, setFieldError] = useState<ServerOpsSSHFieldError>();
  const alive = useRef(true);
  const notified = useRef('');
  const idempotencyKey = useRef<string>();
  const activeAttempt = useRef<string>();
  const latestValues = useRef(values);
  latestValues.current = values;
  const scope = `${organization}:${actor}:${connector.id}:${updateConnection?.id ?? 'create'}`;
  const scopeRef = useRef(scope);
  scopeRef.current = scope;

  // Every asynchronous continuation re-checks the live organization, account,
  // connector, and connection identity before it can touch this panel's state.
  const isCurrent = useCallback(
    () =>
      alive.current &&
      scopeRef.current === scope &&
      useAuthStore.getState().user?.id === actor &&
      useOrganizationStore.getState().currentOrganization?.id === organization,
    [actor, organization, scope]
  );

  useEffect(() => {
    alive.current = true;
    return () => {
      alive.current = false;
      destroyServerOpsSSHSecretValues(latestValues.current);
    };
  }, []);

  const refreshConnections = useCallback(() => {
    invalidatePersonalConnectionQueries(queryClient);
  }, [queryClient]);

  const applyResult = useCallback(
    (next: KeySetupResult, forAttempt?: string) => {
      if (!isCurrent()) return;
      if (forAttempt && activeAttempt.current !== forAttempt) return;
      if (next.state === 'ready' && next.connection.connector_id !== connector.id) {
        activeAttempt.current = undefined;
        setResult(undefined);
        setFailure({ code: 'connector_identity_mismatch', unknown: false });
        return;
      }
      setResult(next);
      setFailure(next.state === 'failed' ? { code: next.error_code, unknown: false } : undefined);
      if (next.state === 'ready') {
        activeAttempt.current = undefined;
        refreshConnections();
      }
    },
    [connector.id, isCurrent, refreshConnections]
  );

  useEffect(() => {
    if (result?.state !== 'verifying') return;
    const attemptId = result.attempt_id;
    let cancelled = false;
    const timer = setTimeout(() => {
      void connectorService
        .getKeySetup(attemptId)
        .then(response => {
          if (!cancelled) applyResult(response.data, attemptId);
        })
        .catch(() => {
          if (!cancelled && isCurrent()) {
            setFailure({ code: 'transport_unconfirmed', unknown: true });
          }
        });
    }, 1500);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [applyResult, isCurrent, result]);

  const ready = result?.state === 'ready' ? result.connection : undefined;
  const newConnection = Boolean(ready && updateConnection && ready.id !== updateConnection.id);
  const verifying = result?.state === 'verifying';
  const probing = stage === 'probing';
  const finalSubmitting = stage === 'submitting';
  const finalBusy = finalSubmitting || verifying;
  const busy = probing || finalBusy;
  const pendingObservation = stage === 'confirmation' ? observation : undefined;

  useEffect(() => {
    if (!ready || updateConnection || notified.current === ready.id) return;
    notified.current = ready.id;
    void creationDefault.complete(ready).then(valid => {
      // The host owns the draft/save transaction; a stale scope never reaches it.
      if (valid && isCurrent()) onReady?.(ready, {});
    });
  }, [creationDefault, isCurrent, onReady, ready, updateConnection]);

  const updateValue = (name: string, value: string) => {
    setValues(previous => ({ ...previous, [name]: value }));
    setFieldError(undefined);
    // Any edit makes an earlier observation stale, so the user must probe again
    // before the strict final save can consume it.
    if (observation || stage !== 'form') {
      setObservation(undefined);
      setStage('form');
    }
  };

  const updateAuthMethod = (next: ServerOpsSSHAuthMethod) => {
    if (next === authMethod || busy) return;
    setAuthMethod(next);
    setValues(previous => ({
      ...previous,
      [SERVEROPS_SSH_PASSWORD_FIELD]:
        next === 'password' ? previous[SERVEROPS_SSH_PASSWORD_FIELD] : '',
      [SERVEROPS_SSH_PRIVATE_KEY_FIELD]:
        next === 'private_key' ? previous[SERVEROPS_SSH_PRIVATE_KEY_FIELD] : '',
      [SERVEROPS_SSH_PASSPHRASE_FIELD]:
        next === 'private_key' ? previous[SERVEROPS_SSH_PASSPHRASE_FIELD] : '',
    }));
    setObservation(undefined);
    setStage('form');
    setFailure(undefined);
    setFieldError(undefined);
  };

  const rejectObservation = () => {
    if (busy) return;
    // Retain every field, including the write-only secrets, so the user can
    // adjust the server details and probe again.
    setObservation(undefined);
    setStage('form');
    setFieldError(undefined);
  };

  const startProbe = async () => {
    if (busy || !isCurrent()) return;
    const invalid = serverOpsSSHProbeFieldError(values, authMethod);
    setFieldError(invalid);
    if (invalid) return;
    const signature = serverOpsSSHObservationSignature(values, authMethod);
    const targetSignature = serverOpsSSHObservationTargetSignature(values);
    // The probe request map is a short-lived copy carrying write-only secrets.
    // It is destroyed in `finally`, even when the request throws, and the form
    // secret state is deliberately retained so a confirmation can follow.
    const probePayload = buildServerOpsSSHProbePayload(values, authMethod);
    setStage('probing');
    setFailure(undefined);
    setObservation(undefined);
    let observed = false;
    try {
      const response = await connectorService.testServerOpsSSHHostKey(connector.id, {
        values: probePayload,
      });
      if (!isCurrent()) return;
      if (serverOpsSSHObservationSignature(latestValues.current, authMethod) !== signature) {
        // The form changed while the probe was in flight; never present a stale
        // observation as if it belonged to the current server details.
        setFailure({ code: OBSERVATION_STALE_CODE, unknown: false });
        return;
      }
      setObservation({
        host_key_sha256: response.data.host_key_sha256 ?? '',
        hostname: response.data.hostname ?? '',
        request_id: response.data.request_id ?? '',
        signature,
        targetSignature,
      });
      setStage('confirmation');
      observed = true;
    } catch (error) {
      if (isCurrent()) {
        const details = connectorRequestErrorDetails(error);
        setFailure({ code: details.code, unknown: details.outcomeUnknown });
      }
    } finally {
      destroyServerOpsSSHCredentialPayload(probePayload);
      if (isCurrent() && !observed) setStage('form');
    }
  };

  const confirmObservation = async () => {
    if (busy || !isCurrent() || !observation) return;
    if (serverOpsSSHObservationSignature(values, authMethod) !== observation.signature) {
      const targetChanged =
        serverOpsSSHObservationTargetSignature(values) !== observation.targetSignature;
      setObservation(undefined);
      setStage('form');
      setFieldError(undefined);
      setFailure({
        code: targetChanged ? OBSERVATION_STALE_CODE : FINGERPRINT_CHANGED_CODE,
        unknown: false,
      });
      return;
    }
    // The strict final save always requires a canonical fingerprint. A captured
    // probe supplies it; a verified manual pin is used when the backend returns
    // no fingerprint because it strictly verified the supplied one.
    const fingerprint =
      observation.host_key_sha256 !== ''
        ? observation.host_key_sha256
        : (values[SERVEROPS_SSH_HOST_KEY_FIELD] ?? '');
    const invalid = serverOpsSSHFieldError(
      {
        ...values,
        [SERVEROPS_SSH_HOST_KEY_FIELD]: fingerprint,
      },
      authMethod
    );
    setFieldError(invalid);
    if (invalid) {
      setObservation(undefined);
      setStage('form');
      return;
    }
    // A fresh key per confirmed submission; the read-only probe never used one.
    const payload = buildServerOpsSSHCredentialPayload(
      {
        ...values,
        [SERVEROPS_SSH_HOST_KEY_FIELD]: fingerprint,
      },
      authMethod,
      privilege
    );
    idempotencyKey.current = globalThis.crypto.randomUUID();
    setStage('submitting');
    setFailure(undefined);
    setObservation(undefined);
    // Only now, after the final strict submission begins, may the form secret
    // state be cleared. The component unmount cleanup destroys it as well.
    setValues(previous => clearServerOpsSSHSecretValues(previous));
    creationDefault.begin();
    try {
      // Credential submissions carry write-only secrets, so both create and
      // reconnect opt out of shared error reporting and surface safe codes locally.
      const response = updateConnection
        ? await connectorService.reconnectConnection(
            updateConnection.id,
            { values: payload },
            idempotencyKey.current,
            true
          )
        : await connectorService.startCredentialSetup(
            connector.id,
            { values: payload },
            idempotencyKey.current,
            true
          );
      if (!isCurrent()) return;
      activeAttempt.current =
        response.data.state === 'verifying' ? response.data.attempt_id : undefined;
      applyResult(response.data);
    } catch (error) {
      if (isCurrent()) {
        const details = connectorRequestErrorDetails(error);
        setFailure({ code: details.code, unknown: details.outcomeUnknown });
      }
    } finally {
      destroyServerOpsSSHCredentialPayload(payload);
      if (isCurrent()) setStage('form');
    }
  };

  const retry = () => {
    idempotencyKey.current = undefined;
    activeAttempt.current = undefined;
    setResult(undefined);
    setFailure(undefined);
    setFieldError(undefined);
    setObservation(undefined);
    setStage('form');
    setValues(previous => clearServerOpsSSHSecretValues(previous));
  };

  const checkStatus = async () => {
    const attemptId = result?.state === 'verifying' ? result.attempt_id : undefined;
    if (!attemptId || checking || !isCurrent()) return;
    setChecking(true);
    try {
      const response = await connectorService.getKeySetup(attemptId);
      applyResult(response.data, attemptId);
    } catch {
      if (isCurrent()) setFailure({ code: 'transport_unconfirmed', unknown: true });
    } finally {
      if (isCurrent()) setChecking(false);
    }
  };

  const secretNotice = (
    <p className="text-xs leading-5 text-muted-foreground">{t('serveropsSsh.secretNotice')}</p>
  );

  return (
    <Card className={embedded ? 'border-0 shadow-none' : undefined}>
      {!embedded ? (
        <CardHeader>
          <CardTitle>
            {title ??
              t(updateConnection ? 'serveropsSsh.reconnectTitle' : 'serveropsSsh.createTitle')}
          </CardTitle>
          <p className="text-sm text-muted-foreground">
            {description ??
              t(
                updateConnection
                  ? 'serveropsSsh.reconnectDescription'
                  : 'serveropsSsh.createDescription'
              )}
          </p>
        </CardHeader>
      ) : null}
      <CardContent className={embedded ? 'space-y-5 p-0' : 'space-y-5'}>
        {ready ? (
          <div className="space-y-3">
            {newConnection ? (
              <Alert className="border-warning/30 bg-warning/10">
                <AlertTriangle aria-hidden="true" className="size-4 text-warning" />
                <AlertTitle>{t('serveropsSsh.newConnectionTitle')}</AlertTitle>
                <AlertDescription>{t('serveropsSsh.newConnectionDescription')}</AlertDescription>
              </Alert>
            ) : (
              <Alert className="border-success/25 bg-success/5 text-success">
                <CheckCircle2 aria-hidden="true" className="size-4" />
                <AlertTitle>{t('serveropsSsh.readyTitle')}</AlertTitle>
                <AlertDescription className="text-muted-foreground">
                  {t('serveropsSsh.readyDescription')}
                </AlertDescription>
              </Alert>
            )}
            <p className="rounded-lg border bg-muted/20 p-3 text-sm">
              {ready.name}
              {ready.external_account_id ? (
                <span className="mt-1 block font-mono text-xs break-all">
                  {ready.external_account_id}
                </span>
              ) : null}
            </p>
            <Button asChild variant="outline" className="w-full">
              <Link href={`/console/connectors/connections/${encodeURIComponent(ready.id)}`}>
                {t('serveropsSsh.manage')}
              </Link>
            </Button>
          </div>
        ) : finalBusy ? (
          <div className="space-y-3">
            <div className="rounded-xl border bg-muted/20 px-5 py-8 text-center" role="status">
              <Loader2 aria-hidden="true" className="mx-auto size-7 animate-spin text-primary" />
              <p className="mt-4 font-medium">{t('serveropsSsh.verifyingTitle')}</p>
              <p className="mt-1 text-sm text-muted-foreground">
                {t('serveropsSsh.verifyingDescription')}
              </p>
            </div>
            {failure ? (
              <Alert variant="destructive">
                <AlertCircle aria-hidden="true" className="size-4" />
                <AlertTitle>
                  {t(failure.unknown ? 'setup.unknownTitle' : 'serveropsSsh.failedTitle')}
                </AlertTitle>
                <AlertDescription>
                  {t(
                    failure.unknown
                      ? 'serveropsSsh.errors.resultUnknown'
                      : failureMessageKey(failure.code)
                  )}
                </AlertDescription>
              </Alert>
            ) : null}
            <Button
              type="button"
              variant="outline"
              className="w-full"
              disabled={checking || !verifying}
              onClick={() => void checkStatus()}
            >
              {t('serveropsSsh.checkStatus')}
            </Button>
            {failure?.unknown ? (
              <Button asChild variant="outline" className="w-full">
                <Link href="/console/connectors/connections" onClick={refreshConnections}>
                  {t('setup.checkConnections')}
                </Link>
              </Button>
            ) : null}
          </div>
        ) : pendingObservation ? (
          <div className="space-y-4">
            <div className="rounded-xl border border-primary/30 bg-primary/5 p-4">
              <div className="flex items-start gap-2">
                <ShieldCheck aria-hidden="true" className="mt-0.5 size-5 text-primary" />
                <div className="min-w-0">
                  <p className="font-medium">{t('serveropsSsh.confirmationTitle')}</p>
                  <p className="mt-1 text-sm text-muted-foreground">
                    {t('serveropsSsh.confirmationDescription')}
                  </p>
                </div>
              </div>
              <dl className="mt-4 space-y-3 text-sm">
                <div className="min-w-0">
                  <dt className="text-xs text-muted-foreground">
                    {t('serveropsSsh.confirmationTarget')}
                  </dt>
                  <dd className="mt-0.5 font-mono text-xs break-all">
                    {`${values[SERVEROPS_SSH_USERNAME_FIELD]}@${values[SERVEROPS_SSH_HOST_FIELD]}:${values[SERVEROPS_SSH_PORT_FIELD]}`}
                  </dd>
                </div>
                <div className="min-w-0">
                  <dt className="text-xs text-muted-foreground">{t('serveropsSsh.privilege')}</dt>
                  <dd className="mt-0.5 text-xs">
                    {t(
                      privilege === 'sudo'
                        ? 'serveropsSsh.privilegeSudo'
                        : 'serveropsSsh.privilegeUser'
                    )}
                  </dd>
                </div>
                <div className="min-w-0">
                  <dt className="text-xs text-muted-foreground">
                    {t('serveropsSsh.observedHostname')}
                  </dt>
                  <dd className="mt-0.5 font-mono text-xs break-all">
                    {pendingObservation.hostname}
                  </dd>
                </div>
                <div className="min-w-0">
                  <dt className="text-xs text-muted-foreground">
                    {t('serveropsSsh.observedFingerprint')}
                  </dt>
                  <dd className="mt-0.5 font-mono text-xs break-all">
                    {pendingObservation.host_key_sha256 ||
                      values[SERVEROPS_SSH_HOST_KEY_FIELD] ||
                      t('serveropsSsh.observedFingerprintUnavailable')}
                  </dd>
                </div>
              </dl>
            </div>

            <Alert className="border-warning/30 bg-warning/10">
              <AlertTriangle aria-hidden="true" className="size-4 text-warning" />
              <AlertTitle>{t('serveropsSsh.confirmationWarningTitle')}</AlertTitle>
              <AlertDescription>{t('serveropsSsh.confirmationWarning')}</AlertDescription>
            </Alert>

            <div className="flex flex-col gap-2 sm:flex-row">
              <Button
                type="button"
                className="w-full sm:flex-1"
                disabled={busy}
                onClick={() => void confirmObservation()}
              >
                {t('serveropsSsh.confirm')}
              </Button>
              <Button
                type="button"
                variant="outline"
                className="w-full sm:flex-1"
                disabled={busy}
                onClick={rejectObservation}
              >
                {t('serveropsSsh.reTest')}
              </Button>
            </div>

            <p className="text-xs leading-5 text-muted-foreground">
              {t('serveropsSsh.ownerNotice')}
            </p>
            {secretNotice}
          </div>
        ) : (
          <div className="space-y-4">
            {!updateConnection ? creationDefault.option(false) : null}

            {failure && !failure.unknown ? (
              <div className="space-y-3">
                <Alert variant="destructive">
                  <AlertCircle aria-hidden="true" className="size-4" />
                  <AlertTitle>{t('serveropsSsh.failedTitle')}</AlertTitle>
                  <AlertDescription>{t(failureMessageKey(failure.code))}</AlertDescription>
                </Alert>
                <Button type="button" variant="outline" className="w-full" onClick={retry}>
                  {t('serveropsSsh.retry')}
                </Button>
              </div>
            ) : null}

            {failure?.unknown ? (
              <div className="space-y-3">
                <Alert variant="destructive">
                  <AlertCircle aria-hidden="true" className="size-4" />
                  <AlertTitle>{t('setup.unknownTitle')}</AlertTitle>
                  <AlertDescription>{t('serveropsSsh.errors.resultUnknown')}</AlertDescription>
                </Alert>
                <Button asChild variant="outline" className="w-full">
                  <Link href="/console/connectors/connections" onClick={refreshConnections}>
                    {t('setup.checkConnections')}
                  </Link>
                </Button>
                <Button type="button" variant="outline" className="w-full" onClick={retry}>
                  {t('serveropsSsh.retry')}
                </Button>
              </div>
            ) : null}

            {probing ? (
              <div className="rounded-xl border bg-muted/20 px-5 py-6 text-center" role="status">
                <Loader2 aria-hidden="true" className="mx-auto size-6 animate-spin text-primary" />
                <p className="mt-3 font-medium">{t('serveropsSsh.probingTitle')}</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  {t('serveropsSsh.probingDescription')}
                </p>
              </div>
            ) : null}

            <form
              className="space-y-4"
              onSubmit={event => {
                event.preventDefault();
                void startProbe();
              }}
            >
              <div className="space-y-2">
                <Label htmlFor="serverops-ssh-host">{t('serveropsSsh.host')}</Label>
                <Input
                  id="serverops-ssh-host"
                  name={SERVEROPS_SSH_HOST_FIELD}
                  type="text"
                  required
                  maxLength={255}
                  autoComplete="off"
                  autoCapitalize="none"
                  autoCorrect="off"
                  spellCheck={false}
                  disabled={busy}
                  aria-invalid={fieldError === 'host'}
                  value={values[SERVEROPS_SSH_HOST_FIELD]}
                  onChange={event => updateValue(SERVEROPS_SSH_HOST_FIELD, event.target.value)}
                />
                <p className="text-xs leading-5 text-muted-foreground">
                  {t('serveropsSsh.hostHint')}
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="serverops-ssh-port">{t('serveropsSsh.port')}</Label>
                <Input
                  id="serverops-ssh-port"
                  name={SERVEROPS_SSH_PORT_FIELD}
                  type="text"
                  inputMode="numeric"
                  required
                  maxLength={5}
                  autoComplete="off"
                  disabled={busy}
                  aria-invalid={fieldError === 'port'}
                  value={values[SERVEROPS_SSH_PORT_FIELD]}
                  onChange={event => updateValue(SERVEROPS_SSH_PORT_FIELD, event.target.value)}
                />
                <p className="text-xs leading-5 text-muted-foreground">
                  {t('serveropsSsh.portHint')}
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="serverops-ssh-username">{t('serveropsSsh.username')}</Label>
                <Input
                  id="serverops-ssh-username"
                  name={SERVEROPS_SSH_USERNAME_FIELD}
                  type="text"
                  required
                  maxLength={128}
                  autoComplete="off"
                  autoCapitalize="none"
                  autoCorrect="off"
                  spellCheck={false}
                  disabled={busy}
                  aria-invalid={fieldError === 'username'}
                  value={values[SERVEROPS_SSH_USERNAME_FIELD]}
                  onChange={event => updateValue(SERVEROPS_SSH_USERNAME_FIELD, event.target.value)}
                />
              </div>

              <div className="space-y-3 rounded-xl border bg-muted/10 p-3">
                <div className="space-y-1">
                  <Label>{t('serveropsSsh.authMethod')}</Label>
                  <p className="text-xs leading-5 text-muted-foreground">
                    {t('serveropsSsh.authMethodHint')}
                  </p>
                </div>
                <div
                  className="grid grid-cols-2 gap-2"
                  role="tablist"
                  aria-label={t('serveropsSsh.authMethod')}
                >
                  {(['password', 'private_key'] as const).map(method => (
                    <Button
                      key={method}
                      type="button"
                      role="tab"
                      aria-selected={authMethod === method}
                      variant={authMethod === method ? 'default' : 'outline'}
                      className="w-full"
                      disabled={busy}
                      onClick={() => updateAuthMethod(method)}
                    >
                      {t(
                        method === 'password'
                          ? 'serveropsSsh.passwordMethod'
                          : 'serveropsSsh.privateKeyMethod'
                      )}
                    </Button>
                  ))}
                </div>

                {authMethod === 'password' ? (
                  <div className="space-y-2">
                    <Label htmlFor="serverops-ssh-password">{t('serveropsSsh.password')}</Label>
                    <Input
                      id="serverops-ssh-password"
                      name={SERVEROPS_SSH_PASSWORD_FIELD}
                      type="password"
                      required
                      maxLength={4096}
                      autoComplete="new-password"
                      autoCapitalize="none"
                      autoCorrect="off"
                      spellCheck={false}
                      disabled={busy}
                      aria-invalid={fieldError === 'password'}
                      value={values[SERVEROPS_SSH_PASSWORD_FIELD]}
                      onChange={event =>
                        updateValue(SERVEROPS_SSH_PASSWORD_FIELD, event.target.value)
                      }
                    />
                    <p className="text-xs leading-5 text-muted-foreground">
                      {t('serveropsSsh.passwordHint')}
                    </p>
                  </div>
                ) : (
                  <>
                    <div className="space-y-2">
                      <Label htmlFor="serverops-ssh-private-key">
                        {t('serveropsSsh.privateKey')}
                      </Label>
                      <Textarea
                        id="serverops-ssh-private-key"
                        name={SERVEROPS_SSH_PRIVATE_KEY_FIELD}
                        required
                        rows={6}
                        maxLength={16 * 1024}
                        autoComplete="off"
                        autoCapitalize="none"
                        autoCorrect="off"
                        spellCheck={false}
                        className="font-mono text-xs"
                        disabled={busy}
                        aria-invalid={fieldError === 'private_key'}
                        value={values[SERVEROPS_SSH_PRIVATE_KEY_FIELD]}
                        onChange={event =>
                          updateValue(SERVEROPS_SSH_PRIVATE_KEY_FIELD, event.target.value)
                        }
                      />
                      <p className="text-xs leading-5 text-muted-foreground">
                        {t('serveropsSsh.privateKeyHint')}
                      </p>
                    </div>

                    <div className="space-y-2">
                      <Label htmlFor="serverops-ssh-passphrase">
                        {t('serveropsSsh.passphrase')}
                      </Label>
                      <Input
                        id="serverops-ssh-passphrase"
                        name={SERVEROPS_SSH_PASSPHRASE_FIELD}
                        type="password"
                        maxLength={4096}
                        autoComplete="new-password"
                        autoCapitalize="none"
                        autoCorrect="off"
                        spellCheck={false}
                        disabled={busy}
                        aria-invalid={fieldError === 'passphrase'}
                        value={values[SERVEROPS_SSH_PASSPHRASE_FIELD]}
                        onChange={event =>
                          updateValue(SERVEROPS_SSH_PASSPHRASE_FIELD, event.target.value)
                        }
                      />
                      <p className="text-xs leading-5 text-muted-foreground">
                        {t('serveropsSsh.passphraseHint')}
                      </p>
                    </div>
                  </>
                )}
              </div>

              <div className="space-y-3 rounded-xl border bg-muted/10 p-3">
                <div className="space-y-1">
                  <Label id="serverops-ssh-privilege-label">{t('serveropsSsh.privilege')}</Label>
                  <p
                    id="serverops-ssh-privilege-hint"
                    className="text-xs leading-5 text-muted-foreground"
                  >
                    {t('serveropsSsh.privilegeHint')}
                  </p>
                </div>
                <div
                  className="grid grid-cols-1 gap-2 sm:grid-cols-2"
                  role="radiogroup"
                  aria-labelledby="serverops-ssh-privilege-label"
                  aria-describedby="serverops-ssh-privilege-hint"
                >
                  {(['user', 'sudo'] as const).map(option => (
                    <Button
                      key={option}
                      type="button"
                      role="radio"
                      aria-checked={privilege === option}
                      variant={privilege === option ? 'default' : 'outline'}
                      className="h-auto w-full whitespace-normal py-2"
                      disabled={busy}
                      onClick={() => setPrivilege(option)}
                    >
                      {t(
                        option === 'sudo'
                          ? 'serveropsSsh.privilegeSudo'
                          : 'serveropsSsh.privilegeUser'
                      )}
                    </Button>
                  ))}
                </div>
                {privilege === 'sudo' ? (
                  <p className="text-xs leading-5 text-muted-foreground">
                    {t('serveropsSsh.privilegeSudoHint')}
                  </p>
                ) : null}
              </div>

              <div className="space-y-3 rounded-xl border bg-muted/10 p-3">
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-start gap-2">
                    <ShieldCheck aria-hidden="true" className="mt-0.5 size-4 text-primary" />
                    <div>
                      <p className="text-sm font-medium">{t('serveropsSsh.hostVerification')}</p>
                      <p className="mt-1 text-xs leading-5 text-muted-foreground">
                        {t('serveropsSsh.hostVerificationHint')}
                      </p>
                    </div>
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    className="shrink-0"
                    aria-expanded={showVerificationSettings}
                    disabled={busy}
                    onClick={() => setShowVerificationSettings(value => !value)}
                  >
                    {t(
                      showVerificationSettings
                        ? 'serveropsSsh.hideVerificationSettings'
                        : 'serveropsSsh.showVerificationSettings'
                    )}
                  </Button>
                </div>
                {showVerificationSettings ? (
                  <div className="space-y-2">
                    <Label htmlFor="serverops-ssh-host-key">{t('serveropsSsh.fingerprint')}</Label>
                    <Input
                      id="serverops-ssh-host-key"
                      name={SERVEROPS_SSH_HOST_KEY_FIELD}
                      type="text"
                      maxLength={128}
                      autoComplete="off"
                      autoCapitalize="none"
                      autoCorrect="off"
                      spellCheck={false}
                      placeholder="SHA256:…"
                      className="font-mono text-xs"
                      disabled={busy}
                      aria-invalid={fieldError === 'host_key_sha256'}
                      value={values[SERVEROPS_SSH_HOST_KEY_FIELD]}
                      onChange={event =>
                        updateValue(SERVEROPS_SSH_HOST_KEY_FIELD, event.target.value)
                      }
                    />
                    <p className="text-xs leading-5 text-muted-foreground">
                      {t('serveropsSsh.fingerprintHelp')}
                    </p>
                  </div>
                ) : null}
              </div>

              {fieldError ? (
                <Alert variant="destructive">
                  <AlertCircle aria-hidden="true" className="size-4" />
                  <AlertDescription>{t(FIELD_MESSAGE_KEY[fieldError])}</AlertDescription>
                </Alert>
              ) : null}

              <Button
                type="submit"
                className="w-full"
                loading={probing}
                disabled={busy || Boolean(failure)}
              >
                {t('serveropsSsh.probe')}
              </Button>
            </form>

            <p className="text-xs leading-5 text-muted-foreground">
              {t('serveropsSsh.ownerNotice')}
            </p>
            {secretNotice}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
