'use client';

import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import {
  AlertTriangle,
  CheckCircle2,
  Clock3,
  Copy,
  ExternalLink,
  Info,
  KeyRound,
  Loader2,
  MessageSquare,
  Pencil,
  RadioTower,
  RotateCcw,
  Send,
  ShieldCheck,
} from 'lucide-react';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import type {
  ChannelCredentialDialogProps,
  ChannelCredentialDraft,
  ChannelCredentialFieldsProps,
  ChannelInstallationSetupProps,
  ChannelInstallationVerificationProps,
} from '@/components/digital-human/channel-providers/types';
import { useDigitalHumanChannelMutations } from '@/hooks/digital-human/use-digital-human-channels';
import { useT } from '@/i18n';
import { digitalHumanService } from '@/services/digital-human.service';
import type { DigitalHumanChannelConnectionTest } from '@/services/types/digital-human';
import { getErrorMessage } from '@/utils/error-notifications';

const FEISHU_CREDENTIAL_KEYS = [
  'app_id',
  'app_secret',
  'verification_token',
  'encrypt_key',
  'connection_mode',
] as const;

const FEISHU_PERMISSION_BUNDLE = [
  'tenant:tenant:readonly',
  'im:message.p2p_msg:readonly',
  'im:message',
  'im:message:readonly',
  'im:message.group_msg',
  'im:chat.members:read',
  'im:message:send_as_bot',
].join('\n');

export function createEmptyFeishuCredentialDraft(): ChannelCredentialDraft {
  return {
    ...Object.fromEntries(FEISHU_CREDENTIAL_KEYS.map(key => [key, ''])),
    connection_mode: 'websocket',
  };
}

export function serializeFeishuCredential(value: ChannelCredentialDraft): Record<string, string> {
  return Object.fromEntries(
    FEISHU_CREDENTIAL_KEYS.map(key => [key, value[key]?.trim() ?? '']).filter(([, item]) =>
      Boolean(item)
    )
  );
}

export function isFeishuCredentialComplete(value: ChannelCredentialDraft): boolean {
  return Boolean(
    value.app_id?.trim() &&
      value.app_secret?.trim() &&
      (value.connection_mode === 'websocket' || value.verification_token?.trim())
  );
}

function CredentialPrivacyNotice() {
  const t = useT('digitalHumans');
  return (
    <div className="flex gap-3 rounded-lg border border-blue-500/20 bg-blue-500/5 px-3 py-2.5 text-sm">
      <KeyRound className="mt-0.5 size-4 shrink-0 text-blue-600" />
      <div className="min-w-0">
        <p className="font-medium">{t('channels.installations.credential.noticeTitle')}</p>
        <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
          {t('channels.installations.credential.noticeDescription')}
        </p>
      </div>
    </div>
  );
}

function CredentialLocationGuide({ websocket }: { websocket: boolean }) {
  const t = useT('digitalHumans');
  const copyPermissions = async () => {
    try {
      await navigator.clipboard.writeText(FEISHU_PERMISSION_BUNDLE);
      toast.success(t('channels.installations.credential.guide.permissionsCopied'));
    } catch {
      toast.error(t('channels.installations.credential.guide.permissionsCopyError'));
    }
  };
  return (
    <div className="rounded-xl border border-blue-500/20 bg-blue-500/[0.035] text-sm">
      <div className="flex items-start gap-3 border-b border-blue-500/15 px-4 py-3.5">
        <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-blue-500/10 text-blue-600">
          <ShieldCheck className="size-4" />
        </span>
        <div className="min-w-0">
          <p className="font-medium">{t('channels.installations.credential.guide.title')}</p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {t('channels.installations.credential.guide.description')}
          </p>
        </div>
      </div>
      <div className="space-y-4 px-4 py-4">
        <ol className="grid gap-2 sm:grid-cols-3">
          {(['bot', 'permissions', 'credentials'] as const).map((step, index) => (
            <li key={step} className="flex gap-2.5 rounded-lg border bg-background px-3 py-2.5">
              <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary/10 text-[11px] font-semibold text-primary">
                {index + 1}
              </span>
              <div className="min-w-0">
                <p className="text-xs font-medium">
                  {t(`channels.installations.credential.guide.steps.${step}.title`)}
                </p>
                <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
                  {websocket && step === 'credentials'
                    ? t('channels.installations.websocket.credentialsDescription')
                    : t(`channels.installations.credential.guide.steps.${step}.description`)}
                </p>
              </div>
            </li>
          ))}
        </ol>
        <p className="text-xs leading-5 text-muted-foreground">
          {t('channels.installations.credential.guide.afterSaveHint')}
        </p>
        <div className="rounded-lg border border-blue-500/20 bg-blue-500/5 p-3">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <p className="text-xs font-medium">
                {t('channels.installations.credential.guide.permissionBundleTitle')}
              </p>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {t('channels.installations.credential.guide.permissionBundleDescription')}
              </p>
            </div>
            <Button variant="outline" size="sm" onClick={() => void copyPermissions()}>
              <Copy className="size-3.5" />
              {t('channels.installations.credential.guide.copyPermissions')}
            </Button>
          </div>
          <pre className="mt-3 max-h-40 overflow-auto whitespace-pre-wrap rounded-md bg-background px-3 py-2 text-xs leading-5">
            {FEISHU_PERMISSION_BUNDLE}
          </pre>
        </div>
        <div className="grid gap-2 sm:grid-cols-2">
          <div className="flex gap-2 rounded-lg border bg-background px-3 py-2.5">
            <CheckCircle2 className="mt-0.5 size-3.5 shrink-0 text-emerald-600" />
            <div>
              <p className="text-xs font-medium">
                {t('channels.installations.credential.guide.appCredentials')}
              </p>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {t('channels.installations.credential.guide.appCredentialsPath')}
              </p>
            </div>
          </div>
          {!websocket ? (
            <div className="flex gap-2 rounded-lg border bg-background px-3 py-2.5">
              <CheckCircle2 className="mt-0.5 size-3.5 shrink-0 text-emerald-600" />
              <div>
                <p className="text-xs font-medium">
                  {t('channels.installations.credential.guide.eventSecurity')}
                </p>
                <p className="mt-1 text-xs leading-5 text-muted-foreground">
                  {t('channels.installations.credential.guide.eventSecurityPath')}
                </p>
              </div>
            </div>
          ) : null}
        </div>
        <Button variant="link" className="h-auto p-0 text-xs" asChild>
          <a href="https://open.feishu.cn/app" target="_blank" rel="noreferrer">
            {t('channels.installations.credential.openFeishuConsole')}
            <ExternalLink className="size-3.5" />
          </a>
        </Button>
      </div>
    </div>
  );
}

export function FeishuInstallationSetupGuide({ installation }: ChannelInstallationSetupProps) {
  const t = useT('digitalHumans');
  if (installation.receiver?.mode === 'unknown') {
    return (
      <div
        role="alert"
        className="mt-4 flex gap-3 rounded-xl border border-destructive/20 bg-destructive/5 p-4 text-sm text-destructive"
      >
        <AlertTriangle className="mt-0.5 size-4 shrink-0" />
        <p>{t('channels.installations.websocket.errors.credential_unavailable')}</p>
      </div>
    );
  }
  if (installation.receiver?.mode === 'websocket') {
    return <FeishuWebSocketSetup installation={installation} />;
  }
  const webhookReady = /^https:\/\//i.test(installation.webhook_url?.trim() ?? '');
  return (
    <div className="mt-4 rounded-xl border border-blue-500/20 bg-blue-500/[0.035] p-4">
      <div className="flex items-start gap-3">
        <Info className="mt-0.5 size-4 shrink-0 text-blue-600" />
        <div className="min-w-0 flex-1">
          <p className="text-sm font-medium">
            {t('channels.installations.credential.postSave.title')}
          </p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {t('channels.installations.credential.postSave.description')}
          </p>
        </div>
      </div>
      <ol className="mt-3 grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
        {(['callback', 'event', 'publish', 'test'] as const).map((step, index) => (
          <li key={step} className="flex gap-2 rounded-lg border bg-background px-3 py-2.5">
            <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary/10 text-[11px] font-semibold text-primary">
              {index + 1}
            </span>
            <div>
              <p className="text-xs font-medium">
                {t(`channels.installations.credential.postSave.steps.${step}.title`)}
              </p>
              <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
                {t(`channels.installations.credential.postSave.steps.${step}.description`)}
              </p>
            </div>
          </li>
        ))}
      </ol>
      {!webhookReady ? (
        <p className="mt-3 flex items-start gap-2 text-xs text-amber-700 dark:text-amber-300">
          <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
          {t('channels.installations.webhookUnavailableHint')}
        </p>
      ) : null}
    </div>
  );
}

function FeishuWebSocketSetup({ installation }: ChannelInstallationSetupProps) {
  const t = useT('digitalHumans');
  const state = installation.receiver?.state ?? 'pending';
  const active = installation.status === 'active';
  const transitioning = active && ['pending', 'connecting', 'reconnecting'].includes(state);
  const errorCode = installation.receiver?.error_code;
  const safeCode =
    (
      [
        'credentials_invalid',
        'permission_missing',
        'configuration_invalid',
        'connection_limit',
        'connection_in_use',
        'lease_unavailable',
        'credential_unavailable',
      ] as const
    ).find(code => code === errorCode) ?? 'connection_failed';
  return (
    <div className="mt-4 rounded-xl border border-blue-500/20 bg-blue-500/[0.035] p-4">
      <div className="flex items-start gap-3" role="status" aria-live="polite">
        {transitioning ? (
          <Loader2 className="mt-0.5 size-5 shrink-0 animate-spin motion-reduce:animate-none text-blue-600" />
        ) : state === 'error' && active ? (
          <AlertTriangle className="mt-0.5 size-5 shrink-0 text-destructive" />
        ) : (
          <RadioTower className="mt-0.5 size-5 shrink-0 text-blue-600" />
        )}
        <div className="min-w-0">
          <p className="text-sm font-medium">{t('channels.installations.websocket.title')}</p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {active
              ? t(`channels.installations.websocket.state.${state}`)
              : t('channels.installations.websocket.disabled')}
          </p>
          {state === 'error' && active ? (
            <p className="mt-2 text-xs leading-5 text-destructive">
              {t(`channels.installations.websocket.errors.${safeCode}`)}
            </p>
          ) : null}
        </div>
      </div>
      <ol className="mt-3 grid gap-2 sm:grid-cols-2 xl:grid-cols-4">
        {(['connect', 'subscribe', 'availability', 'test'] as const).map((step, index) => (
          <li key={step} className="flex gap-2 rounded-lg border bg-background px-3 py-2.5">
            <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary/10 text-[11px] font-semibold text-primary">
              {index + 1}
            </span>
            <div className="min-w-0">
              <p className="text-xs font-medium">
                {t(`channels.installations.websocket.steps.${step}.title`)}
              </p>
              <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
                {t(`channels.installations.websocket.steps.${step}.description`)}
              </p>
            </div>
          </li>
        ))}
      </ol>
      <p className="mt-3 text-xs leading-5 text-muted-foreground">
        {t('channels.installations.websocket.environmentHint')}
      </p>
    </div>
  );
}

export function FeishuInstallationVerificationStatus({
  digitalHumanID,
  installation,
  result,
  connectionTests,
  onConnectionTestChange,
  onStartConnectionTest,
  startingConnectionTest,
}: ChannelInstallationVerificationProps) {
  const t = useT('digitalHumans');
  const previouslyVerified = installation.identity_verified;
  const directTest = connectionTests?.direct;
  const groupTest = connectionTests?.group;
  let verificationNotice: ReactNode;
  if (!result && !previouslyVerified) {
    verificationNotice = (
      <VerificationNotice
        icon={Info}
        tone="pending"
        title={t('channels.installations.verification.savedTitle')}
        description={t('channels.installations.verification.savedDescription')}
      />
    );
  } else if (!result || result.status === 'verified') {
    verificationNotice = (
      <VerificationNotice
        icon={CheckCircle2}
        tone="success"
        title={t('channels.installations.verification.baseReady')}
        description={t('channels.installations.verification.runtimeHint')}
      />
    );
  } else {
    verificationNotice = <FeishuVerificationFailure result={result} />;
  }

  return (
    <>
      {directTest?.id ? (
        <FeishuConnectionTestController
          digitalHumanID={digitalHumanID}
          installationID={installation.id}
          websocket={installation.receiver?.mode === 'websocket'}
          test={directTest}
          onChange={onConnectionTestChange}
          onRestart={() => onStartConnectionTest?.('direct') ?? Promise.resolve()}
          restarting={startingConnectionTest}
        />
      ) : (
        verificationNotice
      )}
      {directTest?.status === 'succeeded' ? (
        groupTest?.id ? (
          <FeishuConnectionTestController
            digitalHumanID={digitalHumanID}
            installationID={installation.id}
            websocket={installation.receiver?.mode === 'websocket'}
            test={groupTest}
            onChange={onConnectionTestChange}
            onRestart={() => onStartConnectionTest?.('group') ?? Promise.resolve()}
            restarting={startingConnectionTest}
          />
        ) : (
          <div className="mt-3 flex flex-col gap-3 rounded-xl border border-blue-500/20 bg-blue-500/[0.025] p-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <p className="text-sm font-medium">
                {t('channels.installations.connectionTest.groupPrompt.title')}
              </p>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {t('channels.installations.connectionTest.groupPrompt.description')}
              </p>
            </div>
            <Button
              variant="outline"
              size="sm"
              className="shrink-0"
              disabled={startingConnectionTest}
              onClick={() => void onStartConnectionTest?.('group')}
            >
              <MessageSquare className="size-3.5" />
              {t('channels.installations.connectionTest.groupPrompt.action')}
            </Button>
          </div>
        )
      ) : null}
    </>
  );
}

function FeishuConnectionTestController({
  digitalHumanID,
  installationID,
  test,
  onChange,
  onRestart,
  restarting,
  websocket,
}: {
  digitalHumanID: string;
  installationID: string;
  test: DigitalHumanChannelConnectionTest;
  onChange?: (test: DigitalHumanChannelConnectionTest) => void;
  onRestart?: () => Promise<void>;
  restarting?: boolean;
  websocket: boolean;
}) {
  const sentConfirmationStorageKey = `digital-human-channel-connection-test-sent:${digitalHumanID}:${installationID}:${test.conversation_type}`;
  const [sentConfirmationTestID, setSentConfirmationTestID] = useState('');
  const onChangeRef = useRef(onChange);
  useEffect(() => {
    onChangeRef.current = onChange;
  }, [onChange]);
  useEffect(() => {
    if (!test.id) {
      setSentConfirmationTestID('');
      return;
    }
    try {
      const storedTestID = window.sessionStorage.getItem(sentConfirmationStorageKey);
      setSentConfirmationTestID(storedTestID === test.id ? test.id : '');
    } catch {
      setSentConfirmationTestID('');
    }
  }, [sentConfirmationStorageKey, test.id]);
  const userConfirmedSent = Boolean(test.id && sentConfirmationTestID === test.id);
  const connectionTestQuery = useQuery({
    queryKey: [
      'digital-human',
      digitalHumanID,
      'channel-connection-test',
      installationID,
      test.id,
      test.conversation_type,
      test.started_at,
    ],
    queryFn: () => {
      if (!test.id || !test.started_at) {
        throw new Error('Channel connection test identity is unavailable.');
      }
      return digitalHumanService.getChannelConnectionTest(
        digitalHumanID,
        installationID,
        test.id,
        test.conversation_type,
        test.started_at
      );
    },
    enabled: Boolean(test.id && test.started_at && userConfirmedSent),
    refetchInterval: query => {
      const status = query.state.data?.data.status ?? test.status;
      return connectionTestTerminal(status) ? false : 2_000;
    },
    refetchIntervalInBackground: true,
  });
  const polledTest = connectionTestQuery.data?.data;
  useEffect(() => {
    if (!polledTest?.id || polledTest.id !== test.id) return;
    onChangeRef.current?.(polledTest);
  }, [polledTest, test.id]);
  const currentTest = polledTest ?? test;
  const confirmSent = () => {
    if (!test.id) return;
    try {
      window.sessionStorage.setItem(sentConfirmationStorageKey, test.id);
    } catch {
      // The explicit confirmation remains usable even when session storage is blocked.
    }
    setSentConfirmationTestID(test.id);
  };
  return (
    <FeishuMessageRoundTripTest
      websocket={websocket}
      test={currentTest}
      userConfirmedSent={userConfirmedSent}
      refreshing={connectionTestQuery.isFetching}
      restarting={Boolean(restarting)}
      onConfirmSent={confirmSent}
      onRefresh={() => void connectionTestQuery.refetch()}
      onRestart={onRestart}
    />
  );
}

function FeishuVerificationFailure({
  result,
}: {
  result: NonNullable<ChannelInstallationVerificationProps['result']>;
}) {
  const t = useT('digitalHumans');
  const reason = result.failure?.reason_code;
  const failureCode = result.failure?.code ?? 'provider_rejected';
  const failureMessage = (() => {
    switch (reason) {
      case 'credentials_invalid':
        return t('channels.installations.verification.failure.credentials_invalid');
      case 'bot_capability_disabled':
        return t('channels.installations.verification.failure.bot_capability_disabled');
      case 'app_unavailable':
        return t('channels.installations.verification.failure.app_unavailable');
      case 'permission_missing':
        return t('channels.installations.verification.failure.permission_missing');
      case 'bot_not_in_chat':
        return t('channels.installations.verification.failure.bot_not_in_chat');
      case 'send_not_allowed':
        return t('channels.installations.verification.failure.send_not_allowed');
      case 'permission_or_resource_denied':
        return t('channels.installations.verification.failure.permission_or_resource_denied');
      default:
        switch (failureCode) {
          case 'credentials_invalid':
            return t('channels.installations.verification.failure.credentials_invalid');
          case 'access_denied':
            return t('channels.installations.verification.failure.access_denied');
          case 'configuration_invalid':
            return t('channels.installations.verification.failure.configuration_invalid');
          case 'rate_limited':
            return t('channels.installations.verification.failure.rate_limited');
          case 'temporarily_unavailable':
            return t('channels.installations.verification.failure.temporarily_unavailable');
          case 'response_invalid':
            return t('channels.installations.verification.failure.response_invalid');
          default:
            return t('channels.installations.verification.failure.provider_rejected');
        }
    }
  })();
  return (
    <div className="mt-4 rounded-lg border border-destructive/25 bg-destructive/5 px-3 py-3 text-sm">
      <div className="flex items-start gap-2.5">
        <AlertTriangle className="mt-0.5 size-4 shrink-0 text-destructive" />
        <div className="min-w-0">
          <p className="font-medium text-destructive">
            {t('channels.installations.verification.failedTitle')}
          </p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">{failureMessage}</p>
        </div>
      </div>
    </div>
  );
}

function FeishuMessageRoundTripTest({
  test,
  userConfirmedSent,
  refreshing,
  restarting,
  onConfirmSent,
  onRefresh,
  onRestart,
  websocket,
}: {
  test: DigitalHumanChannelConnectionTest;
  userConfirmedSent: boolean;
  refreshing: boolean;
  restarting: boolean;
  onConfirmSent: () => void;
  onRefresh: () => void;
  onRestart?: () => Promise<void>;
  websocket: boolean;
}) {
  const t = useT('digitalHumans');
  const groupTest = test.conversation_type === 'group';
  const succeeded = test.status === 'succeeded';
  const partiallyAvailable = succeeded && test.readiness === 'partial';
  const failed = test.status === 'failed';
  const expired = test.status === 'expired';
  const terminalFailure = failed || expired;
  const messageReceived = test.message_received || succeeded;
  const waitingForUser = !userConfirmedSent && !messageReceived && !succeeded;
  const callbackNotReceived = userConfirmedSent && !messageReceived && !expired;
  const copyMessage = async () => {
    if (!test.test_message) return;
    try {
      await navigator.clipboard.writeText(test.test_message);
      toast.success(t('channels.installations.connectionTest.messageCopied'));
    } catch {
      toast.error(t('channels.installations.connectionTest.messageCopyError'));
    }
  };
  const failureKey = connectionTestFailureKey(test.failure_code);

  return (
    <div
      className={`mt-4 rounded-xl border p-4 ${
        succeeded
          ? partiallyAvailable
            ? 'border-amber-500/30 bg-amber-500/5'
            : 'border-emerald-500/25 bg-emerald-500/5'
          : failed
            ? 'border-destructive/35 bg-destructive/[0.035]'
            : expired
              ? 'border-amber-500/30 bg-amber-500/5'
              : 'border-blue-500/25 bg-blue-500/[0.035]'
      }`}
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex min-w-0 gap-3">
          {succeeded ? (
            partiallyAvailable ? (
              <AlertTriangle className="mt-0.5 size-5 shrink-0 text-amber-600" />
            ) : (
              <CheckCircle2 className="mt-0.5 size-5 shrink-0 text-emerald-600" />
            )
          ) : failed ? (
            <AlertTriangle className="mt-0.5 size-5 shrink-0 text-destructive" />
          ) : expired ? (
            <AlertTriangle className="mt-0.5 size-5 shrink-0 text-amber-600" />
          ) : waitingForUser ? (
            <Clock3 className="mt-0.5 size-5 shrink-0 text-blue-600" />
          ) : (
            <Loader2 className="mt-0.5 size-5 shrink-0 animate-spin text-blue-600" />
          )}
          <div className="min-w-0">
            <p className="text-sm font-medium">
              {callbackNotReceived
                ? t(
                    groupTest
                      ? 'channels.installations.connectionTest.groupSentAwaitingCallbackTitle'
                      : 'channels.installations.connectionTest.sentAwaitingCallbackTitle'
                  )
                : succeeded && partiallyAvailable
                  ? t('channels.installations.connectionTest.status.partial')
                  : t(`channels.installations.connectionTest.status.${test.status}`)}
            </p>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">
              {callbackNotReceived
                ? t(
                    groupTest
                      ? 'channels.installations.connectionTest.groupSentAwaitingCallbackDescription'
                      : 'channels.installations.connectionTest.sentAwaitingCallbackDescription'
                  )
                : terminalFailure
                  ? t('channels.installations.connectionTest.failureDetectedDescription')
                  : succeeded
                    ? t(
                        partiallyAvailable
                          ? 'channels.installations.connectionTest.partialDescription'
                          : groupTest
                            ? 'channels.installations.connectionTest.groupSuccessDescription'
                            : 'channels.installations.connectionTest.successDescription'
                      )
                    : t(
                        groupTest
                          ? 'channels.installations.connectionTest.groupWaitingDescription'
                          : 'channels.installations.connectionTest.waitingDescription'
                      )}
            </p>
          </div>
        </div>
        {terminalFailure && onRestart ? (
          <Button
            variant="outline"
            size="sm"
            onClick={() => void onRestart()}
            disabled={restarting}
          >
            {restarting ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : (
              <RotateCcw className="size-3.5" />
            )}
            {t(
              restarting
                ? 'channels.installations.connectionTest.restartingTest'
                : 'channels.installations.connectionTest.restartTest'
            )}
          </Button>
        ) : userConfirmedSent ? (
          <Button variant="outline" size="sm" onClick={onRefresh} disabled={refreshing}>
            <Loader2 className={`size-3.5 ${refreshing ? 'animate-spin' : 'hidden'}`} />
            {t('common.refresh')}
          </Button>
        ) : null}
      </div>

      <div className="mt-4 grid gap-2 sm:grid-cols-2 xl:grid-cols-3">
        {(test.capabilities ?? []).map(capability => (
          <ConnectionTestStep
            key={capability.id}
            icon={connectionCapabilityIcon(capability.id)}
            status={capability.status}
            title={t(connectionCapabilityTitleKey(capability.id))}
            failure={
              capability.status === 'failed'
                ? t(
                    `channels.installations.connectionTest.capabilityFailure.${connectionCapabilityFailureKey(capability.failure_code)}`
                  )
                : undefined
            }
            requiredPermissions={
              capability.required_permissions?.length
                ? capability.required_permissions
                : feishuCapabilityPermissions(capability.id)
            }
          />
        ))}
      </div>

      {groupTest && (!succeeded || partiallyAvailable) ? <FeishuGroupPermissionGuide /> : null}

      {terminalFailure ? (
        <div
          role="alert"
          className={`mt-4 flex items-start gap-2.5 rounded-lg border bg-background px-3 py-3 text-sm ${
            failed ? 'border-destructive/30' : 'border-amber-500/30'
          }`}
        >
          <AlertTriangle
            className={`mt-0.5 size-4 shrink-0 ${failed ? 'text-destructive' : 'text-amber-600'}`}
          />
          <div className="min-w-0">
            <p className={`font-medium ${failed ? 'text-destructive' : 'text-foreground'}`}>
              {t('channels.installations.connectionTest.failureDetectedTitle')}
            </p>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">
              {websocket && failureKey === 'message_not_received'
                ? t('channels.installations.websocket.messageNotReceived')
                : t(
                    `channels.installations.connectionTest.failure.${
                      groupTest && failureKey === 'message_not_received'
                        ? 'group_message_not_received'
                        : failureKey
                    }`
                  )}
            </p>
          </div>
        </div>
      ) : null}

      {!succeeded && !terminalFailure ? (
        <div className="mt-4 rounded-lg border bg-background p-3">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
            <div className="min-w-0">
              <p className="text-xs font-medium">
                {t(
                  groupTest
                    ? 'channels.installations.connectionTest.groupSendTitle'
                    : 'channels.installations.connectionTest.sendTitle'
                )}
              </p>
              <p className="mt-1 text-xs leading-5 text-muted-foreground">
                {t(
                  groupTest
                    ? 'channels.installations.connectionTest.groupSendDescription'
                    : 'channels.installations.connectionTest.sendDescription'
                )}
              </p>
            </div>
            <div className="flex shrink-0 flex-wrap gap-2">
              <Button variant="outline" size="sm" onClick={() => void copyMessage()}>
                <Copy className="size-3.5" />
                {t('channels.installations.connectionTest.copyMessage')}
              </Button>
              <Button size="sm" onClick={onConfirmSent} disabled={userConfirmedSent}>
                {userConfirmedSent ? (
                  <CheckCircle2 className="size-3.5" />
                ) : (
                  <Send className="size-3.5" />
                )}
                {userConfirmedSent
                  ? t('channels.installations.connectionTest.sentConfirmed')
                  : t('channels.installations.connectionTest.confirmSent')}
              </Button>
            </div>
          </div>
          <code className="mt-3 block break-all rounded-md bg-muted/60 px-3 py-2 text-xs text-foreground">
            {test.test_message}
          </code>
          <p className="mt-2 flex items-center gap-1.5 text-xs text-muted-foreground">
            <Info className="size-3.5" />
            {t(
              groupTest
                ? 'channels.installations.connectionTest.groupConfirmSentHint'
                : 'channels.installations.connectionTest.confirmSentHint'
            )}
          </p>
        </div>
      ) : null}

      {callbackNotReceived ? (
        <div className="mt-3 rounded-lg border border-amber-500/25 bg-background px-3 py-2.5 text-xs leading-5 text-muted-foreground">
          <p className="font-medium text-foreground">
            {t('channels.installations.connectionTest.eventChecklistTitle')}
          </p>
          <ol className="mt-1 list-decimal space-y-1 pl-4">
            <li>
              {t(
                websocket
                  ? 'channels.installations.websocket.eventChecklist'
                  : 'channels.installations.connectionTest.eventChecklist.eventPage'
              )}
            </li>
            <li>
              {t(
                websocket
                  ? 'channels.installations.websocket.subscriptionChecklist'
                  : 'channels.installations.connectionTest.eventChecklist.subscription'
              )}
            </li>
            <li>{t('channels.installations.connectionTest.eventChecklist.publish')}</li>
            {groupTest ? (
              <li>{t('channels.installations.connectionTest.eventChecklist.groupAccess')}</li>
            ) : null}
          </ol>
        </div>
      ) : null}
    </div>
  );
}

function FeishuGroupPermissionGuide() {
  const t = useT('digitalHumans');
  const permissions = [
    {
      code: 'im:message.group_msg',
      title: t('channels.installations.connectionTest.permissionGuide.items.groupMessage.title'),
      effect: t('channels.installations.connectionTest.permissionGuide.items.groupMessage.effect'),
      level: t('channels.installations.connectionTest.permissionGuide.required'),
    },
    {
      code: 'im:message:send_as_bot',
      title: t('channels.installations.connectionTest.permissionGuide.items.sendAsBot.title'),
      effect: t('channels.installations.connectionTest.permissionGuide.items.sendAsBot.effect'),
      level: t('channels.installations.connectionTest.permissionGuide.required'),
    },
    {
      code: 'im:message:readonly',
      title: t('channels.installations.connectionTest.permissionGuide.items.messageHistory.title'),
      effect: t(
        'channels.installations.connectionTest.permissionGuide.items.messageHistory.effect'
      ),
      level: t('channels.installations.connectionTest.permissionGuide.enhanced'),
    },
    {
      code: 'im:chat.members:read',
      title: t('channels.installations.connectionTest.permissionGuide.items.memberNames.title'),
      effect: t('channels.installations.connectionTest.permissionGuide.items.memberNames.effect'),
      level: t('channels.installations.connectionTest.permissionGuide.enhanced'),
    },
  ];

  return (
    <div className="mt-3 rounded-lg border border-blue-500/20 bg-background p-3" role="note">
      <div className="flex items-start gap-2.5">
        <ShieldCheck className="mt-0.5 size-4 shrink-0 text-blue-600" />
        <div className="min-w-0">
          <p className="text-xs font-medium">
            {t('channels.installations.connectionTest.permissionGuide.title')}
          </p>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">
            {t('channels.installations.connectionTest.permissionGuide.description')}
          </p>
        </div>
      </div>
      <div className="mt-3 grid gap-2 lg:grid-cols-2">
        {permissions.map(permission => (
          <div key={permission.code} className="rounded-md border bg-muted/20 px-3 py-2.5">
            <div className="flex flex-wrap items-center gap-2">
              <code className="break-all text-[11px] font-medium text-foreground">
                {permission.code}
              </code>
              <span className="rounded-full bg-blue-500/10 px-2 py-0.5 text-[10px] text-blue-700 dark:text-blue-300">
                {permission.level}
              </span>
            </div>
            <p className="mt-1.5 text-xs font-medium">{permission.title}</p>
            <p className="mt-1 text-xs leading-5 text-muted-foreground">{permission.effect}</p>
          </div>
        ))}
      </div>
    </div>
  );
}

function ConnectionTestStep({
  icon: Icon,
  status,
  title,
  failure,
  requiredPermissions,
}: {
  icon: typeof ShieldCheck;
  status: 'pending' | 'passed' | 'failed';
  title: string;
  failure?: string;
  requiredPermissions?: string[];
}) {
  const passed = status === 'passed';
  const failed = status === 'failed';
  return (
    <div
      className={`flex items-start gap-2 rounded-lg border bg-background px-3 py-2.5 text-xs ${
        failed ? 'border-destructive/25' : ''
      }`}
    >
      {passed ? (
        <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-emerald-600" />
      ) : failed ? (
        <AlertTriangle className="mt-0.5 size-4 shrink-0 text-destructive" />
      ) : (
        <Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
      )}
      <div className="min-w-0">
        <span
          className={
            passed
              ? 'font-medium'
              : failed
                ? 'font-medium text-destructive'
                : 'text-muted-foreground'
          }
        >
          {title}
        </span>
        {failure ? <p className="mt-1 leading-4 text-muted-foreground">{failure}</p> : null}
        {failed && requiredPermissions?.length ? (
          <code className="mt-1 block break-all text-[11px] text-foreground">
            {requiredPermissions.join(' · ')}
          </code>
        ) : null}
      </div>
    </div>
  );
}

function connectionCapabilityIcon(capability: string) {
  switch (capability) {
    case 'reply_delivery':
      return Send;
    case 'direct_inbound':
    case 'group_mention':
    case 'message_history':
    case 'member_names':
      return MessageSquare;
    default:
      return ShieldCheck;
  }
}

function connectionCapabilityTitleKey(capability: string) {
  switch (capability) {
    case 'credentials':
      return 'channels.installations.connectionTest.capabilities.credentials' as const;
    case 'bot_identity':
      return 'channels.installations.connectionTest.capabilities.bot_identity' as const;
    case 'direct_inbound':
      return 'channels.installations.connectionTest.capabilities.direct_inbound' as const;
    case 'group_mention':
      return 'channels.installations.connectionTest.capabilities.group_mention' as const;
    case 'message_history':
      return 'channels.installations.connectionTest.capabilities.message_history' as const;
    case 'member_names':
      return 'channels.installations.connectionTest.capabilities.member_names' as const;
    default:
      return 'channels.installations.connectionTest.capabilities.reply_delivery' as const;
  }
}

function feishuCapabilityPermissions(capability: string): string[] {
  switch (capability) {
    case 'credentials':
      return ['tenant:tenant:readonly'];
    case 'direct_inbound':
      return ['im:message.p2p_msg:readonly'];
    case 'group_mention':
      return ['im:message.group_msg'];
    case 'message_history':
      return ['im:message:readonly'];
    case 'member_names':
      return ['im:chat.members:read'];
    case 'reply_delivery':
      return ['im:message:send_as_bot'];
    default:
      return [];
  }
}

function connectionCapabilityFailureKey(code?: string) {
  switch (code) {
    case 'permission_missing':
    case 'bot_not_in_chat':
    case 'app_unavailable':
    case 'credentials_invalid':
    case 'rate_limited':
    case 'temporarily_unavailable':
    case 'configuration_invalid':
      return code;
    default:
      return 'provider_rejected';
  }
}

function connectionTestTerminal(status: DigitalHumanChannelConnectionTest['status']) {
  return status === 'succeeded' || status === 'failed' || status === 'expired';
}

function connectionTestFailureKey(code?: string) {
  switch (code) {
    case 'send_permission_missing':
    case 'permission_or_resource_denied':
    case 'bot_capability_disabled':
    case 'app_unavailable':
    case 'bot_not_in_chat':
    case 'reply_outcome_unknown':
    case 'digital_human_unavailable':
    case 'runtime_unavailable':
    case 'callback_stale':
    case 'reply_not_created':
    case 'reply_failed':
    case 'message_rejected':
    case 'message_not_received':
      return code;
    default:
      return 'generic';
  }
}

function VerificationNotice({
  icon: Icon,
  tone,
  title,
  description,
}: {
  icon: typeof Info;
  tone: 'pending' | 'success';
  title: string;
  description: string;
}) {
  return (
    <div
      className={`mt-4 flex items-start gap-2 rounded-lg border px-3 py-2.5 text-xs ${
        tone === 'success'
          ? 'border-emerald-500/20 bg-emerald-500/5'
          : 'border-blue-500/20 bg-blue-500/5'
      }`}
    >
      <Icon
        className={`mt-0.5 size-3.5 shrink-0 ${
          tone === 'success' ? 'text-emerald-600' : 'text-blue-600'
        }`}
      />
      <div>
        <p className="font-medium">{title}</p>
        <p className="mt-0.5 leading-5 text-muted-foreground">{description}</p>
      </div>
    </div>
  );
}

export function FeishuCredentialFields({
  value,
  onChange,
  editing = false,
}: ChannelCredentialFieldsProps) {
  const t = useT('digitalHumans');
  const websocket = value.connection_mode === 'websocket';
  const update = (key: (typeof FEISHU_CREDENTIAL_KEYS)[number], nextValue: string) =>
    onChange({ ...value, [key]: nextValue });
  const keepExisting = editing
    ? t('channels.installations.credential.keepExistingPlaceholder')
    : undefined;
  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <Label>{t('channels.installations.websocket.modeLabel')}</Label>
        <Tabs
          value={websocket ? 'websocket' : 'webhook'}
          onValueChange={mode => update('connection_mode', mode)}
        >
          <TabsList className="grid h-auto w-full grid-cols-2">
            <TabsTrigger value="websocket" className="whitespace-normal py-2">
              {t('channels.installations.websocket.modeWebSocket')}
            </TabsTrigger>
            <TabsTrigger value="webhook" className="whitespace-normal py-2">
              {t('channels.installations.websocket.modeWebhook')}
            </TabsTrigger>
          </TabsList>
        </Tabs>
        <p className="text-xs leading-5 text-muted-foreground">
          {t(
            websocket
              ? 'channels.installations.websocket.modeDescription'
              : 'channels.installations.websocket.webhookDescription'
          )}
        </p>
        {editing ? (
          <p className="text-xs leading-5 text-amber-700 dark:text-amber-300">
            {t('channels.installations.websocket.switchHint')}
          </p>
        ) : null}
      </div>
      <CredentialLocationGuide websocket={websocket} />
      <div className="grid gap-4 sm:grid-cols-2">
        <Field label="App ID">
          <Input
            value={value.app_id ?? ''}
            placeholder={keepExisting ?? 'cli_…'}
            autoComplete="off"
            onChange={event => update('app_id', event.target.value)}
          />
        </Field>
        <Field label="App Secret">
          <Input
            type="password"
            value={value.app_secret ?? ''}
            placeholder={keepExisting}
            autoComplete="new-password"
            onChange={event => update('app_secret', event.target.value)}
          />
        </Field>
        {!websocket ? (
          <>
            <Field label="Verification Token">
              <Input
                type="password"
                value={value.verification_token ?? ''}
                placeholder={keepExisting}
                autoComplete="new-password"
                onChange={event => update('verification_token', event.target.value)}
              />
            </Field>
            <div className="sm:col-span-2">
              <Field label="Encrypt Key">
                <Input
                  type="password"
                  value={value.encrypt_key ?? ''}
                  placeholder={
                    keepExisting ?? t('channels.installations.credential.encryptKeyPlaceholder')
                  }
                  autoComplete="new-password"
                  onChange={event => update('encrypt_key', event.target.value)}
                />
              </Field>
            </div>
          </>
        ) : null}
      </div>
      <CredentialPrivacyNotice />
    </div>
  );
}

export function FeishuCredentialDialog({
  open,
  onOpenChange,
  digitalHumanID,
  installation,
}: ChannelCredentialDialogProps) {
  const t = useT('digitalHumans');
  const mutations = useDigitalHumanChannelMutations(digitalHumanID);
  const [value, setValue] = useState<ChannelCredentialDraft>(createEmptyFeishuCredentialDraft());
  const [error, setError] = useState('');
  useEffect(() => {
    if (!open) return;
    setValue({
      ...createEmptyFeishuCredentialDraft(),
      connection_mode: installation.receiver?.mode === 'websocket' ? 'websocket' : 'webhook',
    });
    setError('');
  }, [open, installation.id, installation.receiver?.mode]);
  const submit = async () => {
    const credentials = serializeFeishuCredential(value);
    if (!canSubmit) return;
    setError('');
    try {
      await mutations.updateCredential.mutateAsync({
        installationId: installation.id,
        expectedRevision: installation.revision,
        credentials,
      });
      toast.success(t('channels.messages.credentialUpdated'));
      onOpenChange(false);
    } catch (cause) {
      setError(getErrorMessage(cause) || t('channels.installations.credential.updateError'));
    }
  };
  const credentialFields = serializeFeishuCredential(value);
  const transportChanged =
    value.connection_mode !==
    (installation.receiver?.mode === 'websocket' ? 'websocket' : 'webhook');
  const hasCredentialChanges = Object.keys(credentialFields).some(key => key !== 'connection_mode');
  const canSubmit =
    (hasCredentialChanges || transportChanged) &&
    (value.connection_mode !== 'webhook' ||
      installation.receiver?.mode !== 'websocket' ||
      Boolean(value.verification_token?.trim())) &&
    (installation.credential_mode === 'embedded'
      ? Object.keys(credentialFields).length > 0
      : isFeishuCredentialComplete(value));
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="xl" className="w-[calc(100vw-2rem)] p-0">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Pencil className="size-4" />
            {t('channels.installations.credential.editTitle')}
          </DialogTitle>
          <DialogDescription>
            {t('channels.installations.credential.editDescription')}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4 pb-5">
          <FeishuCredentialFields
            value={value}
            onChange={setValue}
            editing={installation.credential_mode === 'embedded'}
          />
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
        </DialogBody>
        <DialogFooter className="border-t bg-muted/30">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button
            onClick={() => void submit()}
            disabled={!canSubmit || mutations.updateCredential.isPending}
          >
            {mutations.updateCredential.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : null}
            {t('common.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <Label>{label}</Label>
      {children}
    </div>
  );
}
