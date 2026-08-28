'use client';

import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react';
import Link from 'next/link';
import {
  AlertTriangle,
  Bot,
  Cable,
  CheckCircle2,
  Clock3,
  Copy,
  ExternalLink,
  Info,
  KeyRound,
  Loader2,
  MessageSquare,
  Plus,
  RefreshCw,
  RadioTower,
  RotateCcw,
  Send,
  ShieldCheck,
  Trash2,
} from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  ENABLED_DIGITAL_HUMAN_CHANNEL_PROVIDERS,
  digitalHumanChannelProvider,
  type DigitalHumanChannelProviderDefinition,
} from '@/components/digital-human/digital-human-channel-providers';
import type { ChannelCredentialDraft } from '@/components/digital-human/channel-providers/types';
import {
  useDigitalHumanChannelCredentials,
  useDigitalHumanChannelMutations,
  useDigitalHumanChannels,
} from '@/hooks/digital-human/use-digital-human-channels';
import { useLocale } from '@/hooks/use-locale';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  DigitalHumanChannelDelivery,
  DigitalHumanChannelDeliveryStatus,
  DigitalHumanChannelEventStatus,
  DigitalHumanChannelInboundEvent,
  DigitalHumanChannelConnectionTest,
  DigitalHumanChannelConnectionTestConversationType,
  DigitalHumanChannelInstallation,
  DigitalHumanChannelInstallationVerification,
} from '@/services/types/digital-human';
import type { IntegrationConnection } from '@/services/types/integration';
import { getErrorMessage } from '@/utils/error-notifications';

interface EditingChannelCredential {
  provider: DigitalHumanChannelProviderDefinition;
  installation: DigitalHumanChannelInstallation;
}

const EMPTY_CHANNEL_CREDENTIALS: IntegrationConnection[] = [];

type InstallationConnectionTests = Partial<
  Record<DigitalHumanChannelConnectionTestConversationType, DigitalHumanChannelConnectionTest>
>;

export function DigitalHumanChannels({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const permissions = useAccountPermissions();
  const canManage = permissions.hasPermission('digital_human.update');
  const canViewActivity = permissions.hasPermission('digital_human.logs.view');
  const canRetry = permissions.hasPermission('digital_human.run');
  const queries = useDigitalHumanChannels(id, canViewActivity);
  const mutations = useDigitalHumanChannelMutations(id);
  const credentialsQuery = useDigitalHumanChannelCredentials(
    ENABLED_DIGITAL_HUMAN_CHANNEL_PROVIDERS.map(provider => provider.integrationId),
    canManage
  );
  const [installationDialogOpen, setInstallationDialogOpen] = useState(false);
  const [editingCredential, setEditingCredential] = useState<EditingChannelCredential | null>(null);
  const [verificationResults, setVerificationResults] = useState<
    Record<string, DigitalHumanChannelInstallationVerification>
  >({});
  const [connectionTests, setConnectionTests] = useState<
    Record<string, InstallationConnectionTests>
  >({});
  useEffect(() => {
    try {
      const stored = window.sessionStorage.getItem(channelConnectionTestStorageKey(id));
      if (stored) {
        setConnectionTests(JSON.parse(stored) as Record<string, InstallationConnectionTests>);
      }
    } catch {
      setConnectionTests({});
    }
  }, [id]);
  useEffect(() => {
    try {
      window.sessionStorage.setItem(
        channelConnectionTestStorageKey(id),
        JSON.stringify(connectionTests)
      );
    } catch {
      // Connection testing remains usable even when session storage is blocked.
    }
  }, [connectionTests, id]);
  const installations = queries.installations.data?.data ?? [];
  const credentials = credentialsQuery.data ?? EMPTY_CHANNEL_CREDENTIALS;
  const events = queries.events.data?.data?.items ?? [];
  const deliveries = queries.deliveries.data?.data?.items ?? [];
  const activeInstallations = installations.filter(item => item.status === 'active');
  const pendingDeliveries = deliveries.filter(item =>
    ['pending', 'processing'].includes(item.status)
  ).length;
  const configurationError = queries.installations.isError;
  const activityError = queries.events.isError || queries.deliveries.isError;
  const CredentialDialog = editingCredential?.provider.credentialUI.Dialog;
  const updateConnectionTest = useCallback(
    (installationId: string, test: DigitalHumanChannelConnectionTest) => {
      setConnectionTests(current => {
        const existing = current[installationId]?.[test.conversation_type];
        if (existing === test) return current;
        return {
          ...current,
          [installationId]: {
            ...current[installationId],
            [test.conversation_type]: test,
          },
        };
      });
    },
    []
  );

  const refresh = () => {
    void Promise.all([
      queries.installations.refetch(),
      ...(canManage ? [credentialsQuery.refetch()] : []),
      ...(canViewActivity ? [queries.events.refetch(), queries.deliveries.refetch()] : []),
    ]);
  };

  return (
    <main className="mx-auto w-full max-w-[1600px] space-y-6 px-4 py-6 md:px-8 md:py-8">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-1">
          <h2 className="text-2xl font-semibold tracking-tight">{t('channels.title')}</h2>
          <p className="max-w-3xl text-sm text-muted-foreground">{t('channels.description')}</p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={refresh}>
            <RefreshCw className="size-4" />
            {t('common.refresh')}
          </Button>
          {canManage ? (
            <Button size="sm" onClick={() => setInstallationDialogOpen(true)}>
              <Plus className="size-4" />
              {t('channels.installations.addChannel')}
            </Button>
          ) : null}
        </div>
      </div>

      <div className="grid gap-3 sm:grid-cols-3">
        <MetricCard
          icon={Cable}
          label={t('channels.summary.installations')}
          value={String(activeInstallations.length)}
        />
        <MetricCard
          icon={MessageSquare}
          label={t('channels.summary.recentEvents')}
          value={canViewActivity ? String(events.length) : '—'}
        />
        <MetricCard
          icon={Send}
          label={t('channels.summary.pending')}
          value={canViewActivity ? String(pendingDeliveries) : '—'}
        />
      </div>

      <section>
        {configurationError ? (
          <LoadFailure onRetry={refresh} />
        ) : (
          <InstallationPanel
            id={id}
            installations={installations}
            loading={queries.installations.isLoading}
            canManage={canManage}
            verificationResults={verificationResults}
            connectionTests={connectionTests}
            onConnectionTestChange={updateConnectionTest}
            onVerify={async installation => {
              const provider = digitalHumanChannelProvider(installation.provider);
              if (provider?.connectionTestMode === 'message_round_trip') {
                const response = await mutations.startConnectionTest.mutateAsync({
                  installationId: installation.id,
                  conversationType: 'direct',
                });
                const verification = response.data.verification;
                if (verification) {
                  setVerificationResults(current => ({
                    ...current,
                    [installation.id]: verification,
                  }));
                }
                if (response.data.id) {
                  setConnectionTests(current => ({
                    ...current,
                    [installation.id]: {
                      ...current[installation.id],
                      [response.data.conversation_type]: response.data,
                    },
                  }));
                }
                return;
              }
              const response = await mutations.verifyInstallation.mutateAsync(installation.id);
              setVerificationResults(current => ({ ...current, [installation.id]: response.data }));
            }}
            onStartConnectionTest={async (installationId, conversationType) => {
              const response = await mutations.startConnectionTest.mutateAsync({
                installationId,
                conversationType,
              });
              const verification = response.data.verification;
              if (verification) {
                setVerificationResults(current => ({
                  ...current,
                  [installationId]: verification,
                }));
              }
              return response.data;
            }}
            verifyingInstallationId={
              mutations.startConnectionTest.isPending
                ? mutations.startConnectionTest.variables?.installationId
                : mutations.verifyInstallation.isPending
                  ? mutations.verifyInstallation.variables
                  : undefined
            }
            onToggle={installation =>
              mutations.updateInstallation.mutate({
                installationId: installation.id,
                request: {
                  expected_revision: installation.revision,
                  status: installation.status === 'active' ? 'disabled' : 'active',
                },
              })
            }
            deletingInstallationId={
              mutations.deleteInstallation.isPending
                ? mutations.deleteInstallation.variables
                : undefined
            }
            onDelete={installationId => mutations.deleteInstallation.mutate(installationId)}
            onEditCredential={(provider, installation) =>
              setEditingCredential({ provider, installation })
            }
          />
        )}
      </section>

      <ActivityPanel
        id={id}
        events={events}
        deliveries={deliveries}
        enabled={canViewActivity}
        installations={installations}
        loading={queries.events.isLoading || queries.deliveries.isLoading}
        error={activityError}
        canRetry={canRetry}
        retryingId={mutations.retryDelivery.variables}
        onRetry={deliveryId => mutations.retryDelivery.mutate(deliveryId)}
        onRefresh={refresh}
      />

      <CreateChannelDialog
        open={installationDialogOpen}
        onOpenChange={setInstallationDialogOpen}
        id={id}
        credentials={credentials}
      />
      {editingCredential && CredentialDialog ? (
        <CredentialDialog
          open
          onOpenChange={open => {
            if (!open) setEditingCredential(null);
          }}
          digitalHumanID={id}
          installation={editingCredential.installation}
        />
      ) : null}
    </main>
  );
}

function MetricCard({
  icon: Icon,
  label,
  value,
}: {
  icon: typeof Cable;
  label: string;
  value: string;
}) {
  return (
    <div className="flex items-center gap-3 rounded-xl border bg-card px-4 py-3 shadow-sm">
      <span className="flex size-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
        <Icon className="size-4" />
      </span>
      <div>
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className="text-lg font-semibold tabular-nums">{value}</p>
      </div>
    </div>
  );
}

function InstallationPanel({
  id,
  installations,
  loading,
  canManage,
  onToggle,
  onDelete,
  onEditCredential,
  onVerify,
  onStartConnectionTest,
  onConnectionTestChange,
  verificationResults,
  connectionTests,
  deletingInstallationId,
  verifyingInstallationId,
}: {
  id: string;
  installations: DigitalHumanChannelInstallation[];
  loading: boolean;
  canManage: boolean;
  onToggle: (installation: DigitalHumanChannelInstallation) => void;
  onDelete: (installationId: string) => void;
  onEditCredential: (
    provider: DigitalHumanChannelProviderDefinition,
    installation: DigitalHumanChannelInstallation
  ) => void;
  onVerify: (installation: DigitalHumanChannelInstallation) => Promise<void>;
  onStartConnectionTest: (
    installationId: string,
    conversationType: DigitalHumanChannelConnectionTestConversationType
  ) => Promise<DigitalHumanChannelConnectionTest>;
  onConnectionTestChange: (installationId: string, test: DigitalHumanChannelConnectionTest) => void;
  verificationResults: Record<string, DigitalHumanChannelInstallationVerification>;
  connectionTests: Record<string, InstallationConnectionTests>;
  deletingInstallationId?: string;
  verifyingInstallationId?: string;
}) {
  const t = useT('digitalHumans');
  if (loading) return <PanelSkeleton />;
  return (
    <Card>
      <CardHeader>
        <div className="space-y-1.5">
          <CardTitle className="text-lg">{t('channels.installations.title')}</CardTitle>
          <CardDescription>{t('channels.installations.description')}</CardDescription>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {installations.length === 0 ? (
          <EmptyState
            icon={Bot}
            title={t('channels.installations.empty')}
            description={t('channels.installations.emptyDescription')}
          />
        ) : (
          installations.map(installation => {
            const provider = digitalHumanChannelProvider(installation.provider);
            const SetupGuide = provider?.installationUI?.SetupGuide;
            const VerificationStatus = provider?.installationUI?.VerificationStatus;
            const storedVerificationResult = verificationResults[installation.id];
            const verificationResult = isCurrentVerificationResult(
              installation,
              storedVerificationResult
            )
              ? storedVerificationResult
              : undefined;
            const storedConnectionTests = connectionTests[installation.id];
            const currentConnectionTests = Object.fromEntries(
              Object.entries(storedConnectionTests ?? {}).filter(([, test]) =>
                isCurrentConnectionTest(installation, test)
              )
            ) as InstallationConnectionTests;
            const webhookURL = installation.webhook_url?.trim() ?? '';
            const webhookReady = /^https:\/\//i.test(webhookURL);
            const usesStream = installation.provider === 'dingtalk';
            const receiverState = installation.receiver?.state ?? 'pending';
            const receiverStateLabel = {
              not_applicable: 'channels.installations.stream.state.not_applicable',
              pending: 'channels.installations.stream.state.pending',
              connecting: 'channels.installations.stream.state.connecting',
              connected: 'channels.installations.stream.state.connected',
              disconnected: 'channels.installations.stream.state.disconnected',
              error: 'channels.installations.stream.state.error',
            }[receiverState] as
              | 'channels.installations.stream.state.not_applicable'
              | 'channels.installations.stream.state.pending'
              | 'channels.installations.stream.state.connecting'
              | 'channels.installations.stream.state.connected'
              | 'channels.installations.stream.state.disconnected'
              | 'channels.installations.stream.state.error';
            return (
              <div key={installation.id} className="rounded-xl border bg-muted/10 p-4">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div className="flex min-w-0 gap-3">
                    <span
                      className={cn(
                        'flex size-10 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground',
                        provider?.accentClassName
                      )}
                    >
                      <Bot className="size-5" />
                    </span>
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <p className="truncate font-medium">{installation.name}</p>
                        <Badge variant={installation.status === 'active' ? 'success' : 'subtle'}>
                          {t(`channels.installations.status.${installation.status}`)}
                        </Badge>
                      </div>
                      <div className="mt-1">
                        <ProviderBadge providerID={installation.provider} />
                      </div>
                    </div>
                  </div>
                  {canManage ? (
                    <div className="flex shrink-0 gap-1">
                      {provider ? (
                        <Button
                          variant="ghost"
                          size="sm"
                          onClick={() => onEditCredential(provider, installation)}
                        >
                          <KeyRound className="size-3.5" />
                          {t('channels.installations.credential.edit')}
                        </Button>
                      ) : null}
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={verifyingInstallationId === installation.id}
                        onClick={() => void onVerify(installation)}
                      >
                        <RefreshCw
                          className={cn(
                            'size-3.5',
                            verifyingInstallationId === installation.id && 'animate-spin'
                          )}
                        />
                        {t('channels.installations.verification.action')}
                      </Button>
                      <Button variant="ghost" size="sm" onClick={() => onToggle(installation)}>
                        {installation.status === 'active'
                          ? t('channels.installations.disable')
                          : t('channels.installations.enable')}
                      </Button>
                      <ConfirmDialog
                        trigger={
                          <Button
                            variant="ghost"
                            size="sm"
                            isIcon
                            aria-label={t('channels.installations.delete')}
                          >
                            <Trash2 className="size-4" />
                          </Button>
                        }
                        title={t('channels.installations.deleteTitle')}
                        description={t('channels.installations.deleteDescription')}
                        confirmText={t('channels.installations.delete')}
                        cancelText={t('common.cancel')}
                        variant="danger"
                        loading={deletingInstallationId === installation.id}
                        onConfirm={() => onDelete(installation.id)}
                      />
                    </div>
                  ) : null}
                </div>
                {VerificationStatus ? (
                  <VerificationStatus
                    digitalHumanID={id}
                    installation={installation}
                    result={verificationResult}
                    connectionTests={currentConnectionTests}
                    onConnectionTestChange={test => onConnectionTestChange(installation.id, test)}
                    onStartConnectionTest={async conversationType => {
                      const response = await onStartConnectionTest(installation.id, conversationType);
                      onConnectionTestChange(installation.id, response);
                    }}
                    startingConnectionTest={verifyingInstallationId === installation.id}
                  />
                ) : (
                  <GenericInstallationVerificationStatus
                    installation={installation}
                    result={verificationResult}
                  />
                )}
                {SetupGuide ? <SetupGuide installation={installation} /> : null}
                {usesStream ? (
                  <div className="mt-4 rounded-lg border bg-background p-3">
                    <div className="flex items-start gap-3">
                      <RadioTower className="mt-0.5 size-4 shrink-0 text-primary" />
                      <div className="min-w-0">
                        <p className="text-xs font-medium">
                          {t('channels.installations.stream.title')}
                        </p>
                        <p className="mt-0.5 text-xs text-muted-foreground">
                          {t(receiverStateLabel)}
                        </p>
                        {installation.receiver?.state === 'error' ? (
                          <p className="mt-2 text-xs text-destructive">
                            {t('channels.installations.stream.errorHint')}
                          </p>
                        ) : null}
                      </div>
                    </div>
                  </div>
                ) : (
                  <div className="mt-4">
                    <div className="rounded-lg border bg-background p-3">
                      <div className="flex items-start justify-between gap-3">
                        <div>
                          <p className="text-xs font-medium">
                            {t('channels.installations.webhook')}
                          </p>
                          <p className="mt-0.5 text-xs text-muted-foreground">
                            {t('channels.installations.webhookDescription')}
                          </p>
                        </div>
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={!webhookReady}
                          onClick={() =>
                            void copyValue(webhookURL, t('channels.messages.webhookCopied'))
                          }
                        >
                          <Copy className="size-3.5" />
                          {t('channels.installations.copy')}
                        </Button>
                      </div>
                      <code className="mt-3 block break-all rounded-md bg-muted/60 px-3 py-2 text-xs text-foreground">
                        {webhookURL || t('channels.installations.webhookUnavailable')}
                      </code>
                      {!webhookReady ? (
                        <p className="mt-2 flex gap-2 text-xs text-amber-700 dark:text-amber-300">
                          <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
                          {t('channels.installations.webhookUnavailableHint')}
                        </p>
                      ) : null}
                    </div>
                  </div>
                )}
                <ChannelContextStatus readiness={installation.channel_context} />
              </div>
            );
          })
        )}
      </CardContent>
    </Card>
  );
}

function isCurrentVerificationResult(
  installation: DigitalHumanChannelInstallation,
  result?: DigitalHumanChannelInstallationVerification
) {
  if (!result) return false;
  const checkedAt = Date.parse(result.checked_at);
  const installationUpdatedAt = Date.parse(installation.updated_at);
  if (!Number.isFinite(checkedAt) || !Number.isFinite(installationUpdatedAt)) return true;
  return checkedAt >= installationUpdatedAt;
}

function isCurrentConnectionTest(
  installation: DigitalHumanChannelInstallation,
  result?: DigitalHumanChannelConnectionTest
) {
  if (!result?.started_at) return false;
  const startedAt = Date.parse(result.started_at);
  const installationUpdatedAt = Date.parse(installation.updated_at);
  if (!Number.isFinite(startedAt) || !Number.isFinite(installationUpdatedAt)) return true;
  return startedAt >= installationUpdatedAt;
}

function channelConnectionTestStorageKey(digitalHumanID: string) {
  return `digital-human:${digitalHumanID}:channel-connection-tests`;
}

function GenericInstallationVerificationStatus({
  installation,
  result,
}: {
  installation: DigitalHumanChannelInstallation;
  result?: DigitalHumanChannelInstallationVerification;
}) {
  const t = useT('digitalHumans');
  const verified = result?.status === 'verified' || installation.identity_verified;
  const failed = result?.status === 'failed';
  return (
    <div
      className={cn(
        'mt-4 flex items-start gap-2 rounded-lg border px-3 py-2.5 text-xs',
        verified && 'border-emerald-500/20 bg-emerald-500/5',
        failed && 'border-destructive/25 bg-destructive/5',
        !verified && !failed && 'border-blue-500/20 bg-blue-500/5'
      )}
    >
      {verified ? (
        <CheckCircle2 className="mt-0.5 size-3.5 shrink-0 text-emerald-600" />
      ) : failed ? (
        <AlertTriangle className="mt-0.5 size-3.5 shrink-0 text-destructive" />
      ) : (
        <Info className="mt-0.5 size-3.5 shrink-0 text-blue-600" />
      )}
      <div>
        <p className={cn('font-medium', failed && 'text-destructive')}>
          {verified
            ? t('channels.installations.verification.baseReady')
            : failed
              ? t('channels.installations.verification.failedTitle')
              : t('channels.installations.verification.savedTitle')}
        </p>
        <p className="mt-0.5 leading-5 text-muted-foreground">
          {verified
            ? t('channels.installations.verification.runtimeHint')
            : failed
              ? t('channels.installations.verification.failure.provider_rejected')
              : t('channels.installations.verification.savedDescription')}
        </p>
      </div>
    </div>
  );
}

function ChannelContextStatus({
  readiness,
}: {
  readiness?: DigitalHumanChannelInstallation['channel_context'];
}) {
  const t = useT('digitalHumans');
  if (!readiness) return null;
  const status = readiness.status;
  const warning =
    status === 'missing_message_history' ||
    status === 'missing_member_names' ||
    status === 'temporarily_unavailable';
  const Icon =
    status === 'ready'
      ? CheckCircle2
      : status === 'runtime_verification'
        ? ShieldCheck
        : status === 'not_supported'
          ? Info
          : AlertTriangle;
  const hint = t(`channels.installations.context.hint.${status}`);

  return (
    <div
      className={cn(
        'mt-3 flex items-start gap-2 rounded-lg border px-3 py-2 text-xs',
        warning
          ? 'border-amber-200/80 bg-amber-50/70 text-amber-800 dark:border-amber-500/25 dark:bg-amber-500/10 dark:text-amber-200'
          : 'bg-muted/20 text-muted-foreground'
      )}
    >
      <Icon className="mt-0.5 size-3.5 shrink-0" />
      <div className="min-w-0">
        <p className="font-medium text-foreground">
          {t(`channels.installations.context.status.${status}`)}
        </p>
        {hint ? <p className="mt-0.5 leading-5">{hint}</p> : null}
      </div>
    </div>
  );
}

function ProviderBadge({ providerID }: { providerID: string }) {
  const t = useT('digitalHumans');
  const provider = digitalHumanChannelProvider(providerID);
  return <Badge variant="subtle">{provider ? t(provider.nameKey) : providerID}</Badge>;
}

function ActivityPanel({
  id,
  events,
  deliveries,
  installations,
  enabled,
  loading,
  error,
  canRetry,
  retryingId,
  onRetry,
  onRefresh,
}: {
  id: string;
  events: DigitalHumanChannelInboundEvent[];
  deliveries: DigitalHumanChannelDelivery[];
  installations: DigitalHumanChannelInstallation[];
  enabled: boolean;
  loading: boolean;
  error: boolean;
  canRetry: boolean;
  retryingId?: string;
  onRetry: (deliveryId: string) => void;
  onRefresh: () => void;
}) {
  const t = useT('digitalHumans');
  const eventsByID = new Map(events.map(event => [event.id, event]));
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">{t('channels.activity.title')}</CardTitle>
        <CardDescription>{t('channels.activity.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        {!enabled ? (
          <EmptyState
            icon={ShieldCheck}
            title={t('channels.activity.permissionTitle')}
            description={t('channels.activity.permissionDescription')}
          />
        ) : loading ? (
          <div className="space-y-3">
            <Skeleton className="h-10 w-full" />
            <Skeleton className="h-20 w-full" />
            <Skeleton className="h-20 w-full" />
          </div>
        ) : error ? (
          <LoadFailure embedded onRetry={onRefresh} />
        ) : (
          <Tabs defaultValue="events">
            <TabsList>
              <TabsTrigger value="events">
                {t('channels.events.title', { count: events.length })}
              </TabsTrigger>
              <TabsTrigger value="deliveries">
                {t('channels.deliveries.title', { count: deliveries.length })}
              </TabsTrigger>
            </TabsList>
            <TabsContent value="events" className="mt-4">
              {events.length === 0 ? (
                <EmptyState
                  icon={MessageSquare}
                  title={t('channels.events.empty')}
                  description={t('channels.events.emptyDescription')}
                />
              ) : (
                <div className="divide-y rounded-xl border">
                  {events.map(event => (
                    <EventRow
                      key={event.id}
                      event={event}
                      id={id}
                      provider={
                        event.provider ||
                        installations.find(item => item.id === event.installation_id)?.provider
                      }
                    />
                  ))}
                </div>
              )}
            </TabsContent>
            <TabsContent value="deliveries" className="mt-4">
              {deliveries.length === 0 ? (
                <EmptyState
                  icon={Send}
                  title={t('channels.deliveries.empty')}
                  description={t('channels.deliveries.emptyDescription')}
                />
              ) : (
                <div className="divide-y rounded-xl border">
                  {deliveries.map(delivery => (
                    <DeliveryRow
                      key={delivery.id}
                      delivery={delivery}
                      event={
                        delivery.inbound_event_id
                          ? eventsByID.get(delivery.inbound_event_id)
                          : undefined
                      }
                      id={id}
                      canRetry={canRetry}
                      retrying={retryingId === delivery.id}
                      onRetry={() => onRetry(delivery.id)}
                    />
                  ))}
                </div>
              )}
            </TabsContent>
          </Tabs>
        )}
      </CardContent>
    </Card>
  );
}

function EventRow({
  event,
  id,
  provider,
}: {
  event: DigitalHumanChannelInboundEvent;
  id: string;
  provider?: string;
}) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const time = formatChannelTime(event.received_at, locale);
  const preview = event.content_preview?.trim()
    ? event.content_preview.trim()
    : event.mention_only
      ? t('channels.events.mentionOnly')
      : t('channels.events.previewUnavailable');
  const source =
    event.conversation_type === 'group'
      ? t('channels.events.source.group')
      : event.conversation_type === 'direct'
        ? t('channels.events.source.direct')
        : t('channels.events.source.unknown');
  const reason = event.reason_code ? t(eventReasonKey(event.reason_code)) : '';
  return (
    <div className="grid gap-3 p-4 md:grid-cols-[minmax(0,1fr)_10rem] md:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <EventStatus status={event.status} />
          {provider ? <ProviderBadge providerID={provider} /> : null}
          <span className="text-xs text-muted-foreground">{source}</span>
          <time
            dateTime={event.received_at}
            title={time.exact}
            className="text-xs text-muted-foreground"
          >
            {t('channels.events.receivedAt', { time: time.relative })}
          </time>
        </div>
        <p className="mt-2 line-clamp-2 text-sm leading-6 text-foreground">“{preview}”</p>
        {reason ? <p className="mt-1 text-xs text-muted-foreground">{reason}</p> : null}
      </div>
      <div className="flex justify-start md:justify-end">
        {event.invocation_run_id ? (
          <Button variant="ghost" size="sm" asChild>
            <Link href={`/console/digital-humans/${id}/runs?run=${event.invocation_run_id}`}>
              {t('channels.activity.viewRun')}
              <ExternalLink className="size-3.5" />
            </Link>
          </Button>
        ) : (
          <span className="text-xs text-muted-foreground">{t('channels.events.noRun')}</span>
        )}
      </div>
    </div>
  );
}

function DeliveryRow({
  delivery,
  event,
  id,
  canRetry,
  retrying,
  onRetry,
}: {
  delivery: DigitalHumanChannelDelivery;
  event?: DigitalHumanChannelInboundEvent;
  id: string;
  canRetry: boolean;
  retrying: boolean;
  onRetry: () => void;
}) {
  const t = useT('digitalHumans');
  const { locale } = useLocale();
  const canSafelyRetry = delivery.status === 'failed' && canRetry;
  const displayTime = delivery.delivered_at || delivery.updated_at || delivery.created_at;
  const time = formatChannelTime(displayTime, locale);
  const duration = channelDuration(event?.received_at, delivery.delivered_at);
  const localizedError =
    delivery.error_code === 'channel.delivery.outcome_unknown'
      ? t('channels.deliveries.failure.outcomeUnknown')
      : delivery.error_code === 'channel.delivery.interrupted'
        ? t('channels.deliveries.failure.interrupted')
        : delivery.error_code === 'channel.delivery.payload_invalid'
          ? t('channels.deliveries.failure.payloadInvalid')
          : delivery.error_code === 'channel.provider.credentials_invalid'
            ? t('channels.deliveries.failure.credentialsInvalid')
            : delivery.error_code === 'channel.provider.bot_not_in_chat'
              ? t('channels.deliveries.failure.botNotInChat')
              : delivery.error_code === 'channel.provider.bot_capability_disabled'
                ? t('channels.deliveries.failure.botCapabilityDisabled')
                : delivery.error_code === 'channel.provider.app_unavailable'
                  ? t('channels.deliveries.failure.appUnavailable')
                  : delivery.error_code === 'channel.provider.permission_missing' ||
                      delivery.error_code === 'channel.provider.send_not_allowed'
                    ? t('channels.deliveries.failure.sendPermissionMissing')
                    : delivery.error_code === 'channel.provider.permission_or_resource_denied'
                      ? t('channels.deliveries.failure.permissionOrResourceDenied')
                      : t('channels.deliveries.failure.generic');
  return (
    <div className="grid gap-3 p-4 md:grid-cols-[minmax(0,1fr)_13rem] md:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <DeliveryStatus status={delivery.status} />
          <ProviderBadge providerID={delivery.provider} />
          <time dateTime={displayTime} title={time.exact} className="text-xs text-muted-foreground">
            {t(`channels.deliveries.time.${delivery.status}`, { time: time.relative })}
          </time>
          {duration !== null ? (
            <span className="text-xs text-muted-foreground">
              {t('channels.deliveries.duration', { seconds: duration })}
            </span>
          ) : null}
          {delivery.attempt_count > 1 || delivery.status === 'failed' ? (
            <span className="text-xs text-muted-foreground">
              {t('channels.deliveries.attempts', { count: delivery.attempt_count })}
            </span>
          ) : null}
        </div>
        <p className="mt-2 line-clamp-3 text-sm leading-6">
          {delivery.payload?.text || t('channels.deliveries.noText')}
        </p>
        {delivery.error_code ? (
          <div className="mt-1 text-xs text-destructive">
            <p>{localizedError}</p>
          </div>
        ) : null}
        {delivery.status === 'unknown' ? (
          <p className="mt-1 text-xs text-amber-700 dark:text-amber-300">
            {t('channels.deliveries.unknownHint')}
          </p>
        ) : null}
      </div>
      <div className="flex flex-wrap justify-start gap-1 md:justify-end">
        <Button variant="ghost" size="sm" asChild>
          <Link href={`/console/digital-humans/${id}/runs?run=${delivery.run_id}`}>
            {t('channels.activity.viewRun')}
          </Link>
        </Button>
        {canSafelyRetry ? (
          <Button variant="outline" size="sm" onClick={onRetry} disabled={retrying}>
            {retrying ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : (
              <RotateCcw className="size-3.5" />
            )}
            {t('channels.deliveries.retry')}
          </Button>
        ) : null}
      </div>
    </div>
  );
}

function CreateChannelDialog({
  open,
  onOpenChange,
  id,
  credentials,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  id: string;
  credentials: IntegrationConnection[];
}) {
  const t = useT('digitalHumans');
  const mutations = useDigitalHumanChannelMutations(id);
  const [credentialMode, setCredentialMode] = useState<'embedded' | 'connection'>('embedded');
  const [selectedConnectionID, setSelectedConnectionID] = useState('');
  const [providerID, setProviderID] = useState(
    ENABLED_DIGITAL_HUMAN_CHANNEL_PROVIDERS[0]?.id ?? ''
  );
  const provider =
    ENABLED_DIGITAL_HUMAN_CHANNEL_PROVIDERS.find(item => item.id === providerID) ??
    ENABLED_DIGITAL_HUMAN_CHANNEL_PROVIDERS[0];
  const [name, setName] = useState('');
  const [credential, setCredential] = useState<ChannelCredentialDraft>(
    () => provider?.credentialUI.createEmptyDraft() ?? {}
  );
  const [credentialError, setCredentialError] = useState('');
  const wasOpen = useRef(false);
  useEffect(() => {
    if (open && !wasOpen.current) {
      const initialProvider = ENABLED_DIGITAL_HUMAN_CHANNEL_PROVIDERS[0];
      setProviderID(initialProvider?.id ?? '');
      setName(
        initialProvider
          ? t(initialProvider.defaultBotNameKey)
          : t('channels.installations.defaultName')
      );
      setCredential(initialProvider?.credentialUI.createEmptyDraft() ?? {});
      setCredentialError('');
      setCredentialMode('embedded');
      setSelectedConnectionID('');
    }
    wasOpen.current = open;
  }, [open, t]);
  const selectProvider = (nextProvider: DigitalHumanChannelProviderDefinition) => {
    setProviderID(nextProvider.id);
    setName(t(nextProvider.defaultBotNameKey));
    setCredential(nextProvider.credentialUI.createEmptyDraft());
    setCredentialError('');
    setCredentialMode('embedded');
    setSelectedConnectionID('');
  };
  const submit = async () => {
    if (!provider) return;
    setCredentialError('');
    try {
      await mutations.createInstallation.mutateAsync({
        provider: provider.id,
        name: name.trim(),
        ...(credentialMode === 'connection'
          ? { connection_id: selectedConnectionID }
          : { credentials: provider.credentialUI.serialize(credential) }),
      });
      onOpenChange(false);
    } catch (error) {
      setCredentialError(
        getErrorMessage(error) || t('channels.installations.credential.createError')
      );
    }
  };
  const credentialReady = provider?.credentialUI.isComplete(credential) ?? false;
  const submitting = mutations.createInstallation.isPending;
  const providerConnections = provider?.connectionFilter
    ? credentials.filter(provider.connectionFilter)
    : [];
  const CredentialFields = provider?.credentialUI.Fields;
  const ready = Boolean(
    provider &&
      name.trim() &&
      (credentialMode === 'embedded' ? credentialReady : selectedConnectionID)
  );
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        size="xl"
        className="h-[min(820px,calc(100dvh-2rem))] max-w-5xl overflow-hidden p-0"
      >
        <DialogHeader className="shrink-0 border-b px-6 py-5 pr-12">
          <DialogTitle>{t('channels.addDialog.title')}</DialogTitle>
          <DialogDescription>{t('channels.addDialog.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="min-h-0 overflow-hidden p-0">
          <div className="grid h-full min-h-0 grid-rows-[auto_minmax(0,1fr)] md:grid-cols-[13.5rem_minmax(0,1fr)] md:grid-rows-1">
            <aside className="border-b bg-muted/20 px-4 py-4 md:overflow-y-auto md:border-b-0 md:border-r md:px-5 md:py-5">
              <ChannelProviderSelector
                providers={ENABLED_DIGITAL_HUMAN_CHANNEL_PROVIDERS}
                selectedID={provider?.id ?? ''}
                onSelect={selectProvider}
              />
            </aside>
            <div className="min-h-0 min-w-0 overflow-y-auto px-5 py-5 sm:px-6">
              {provider ? (
                <>
                  <div className="flex items-start gap-3 border-b pb-4">
                    <span
                      className={cn(
                        'flex size-9 shrink-0 items-center justify-center rounded-lg',
                        provider.accentClassName
                      )}
                    >
                      <Bot className="size-4" />
                    </span>
                    <div className="min-w-0">
                      <h3 className="font-medium">{t(provider.nameKey)}</h3>
                      <p className="mt-0.5 text-xs leading-5 text-muted-foreground">
                        {t(provider.descriptionKey)}
                      </p>
                    </div>
                  </div>
                  <div className="mt-5 space-y-5">
                    <Field label={t('channels.installations.name')}>
                      <Input value={name} onChange={event => setName(event.target.value)} />
                    </Field>
                    <Tabs
                      value={credentialMode}
                      onValueChange={value => setCredentialMode(value as 'embedded' | 'connection')}
                    >
                      <TabsList className="grid h-10 w-full grid-cols-2 sm:w-fit">
                        <TabsTrigger value="embedded" className="min-w-28">
                          {t('channels.installations.credential.directConfig')}
                        </TabsTrigger>
                        <TabsTrigger value="connection" className="min-w-32">
                          {t('channels.installations.credential.connectionCenter')}
                        </TabsTrigger>
                      </TabsList>
                      <TabsContent value="embedded" className="mt-4">
                        {CredentialFields ? (
                          <CredentialFields value={credential} onChange={setCredential} />
                        ) : null}
                      </TabsContent>
                      <TabsContent value="connection" className="mt-4 space-y-3">
                        {providerConnections.length > 0 ? (
                          <Field label={t('channels.installations.credential.selectConnection')}>
                            <Select
                              value={selectedConnectionID || undefined}
                              onValueChange={setSelectedConnectionID}
                            >
                              <SelectTrigger className="h-10 w-full">
                                <SelectValue
                                  placeholder={t(
                                    'channels.installations.credential.selectConnectionPlaceholder'
                                  )}
                                />
                              </SelectTrigger>
                              <SelectContent>
                                {providerConnections.map(item => (
                                  <SelectItem key={item.id} value={item.id}>
                                    {item.name}
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          </Field>
                        ) : (
                          <div className="rounded-xl border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
                            {t('channels.installations.credential.noConnections')}
                          </div>
                        )}
                        <Button variant="link" className="h-auto p-0" asChild>
                          <Link href="/console/integrations">
                            {t('channels.installations.credential.openConnectionCenter')}
                            <ExternalLink className="size-3.5" />
                          </Link>
                        </Button>
                      </TabsContent>
                    </Tabs>
                  </div>
                  {credentialError ? (
                    <p className="mt-4 text-sm text-destructive">{credentialError}</p>
                  ) : null}
                </>
              ) : null}
            </div>
          </div>
        </DialogBody>
        <DialogFooter className="shrink-0 border-t bg-background px-6 py-4">
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button onClick={() => void submit()} disabled={!ready || submitting}>
            {submitting ? <Loader2 className="size-4 animate-spin" /> : null}
            {t('channels.installations.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ChannelProviderSelector({
  providers,
  selectedID,
  onSelect,
}: {
  providers: readonly DigitalHumanChannelProviderDefinition[];
  selectedID: string;
  onSelect: (provider: DigitalHumanChannelProviderDefinition) => void;
}) {
  const t = useT('digitalHumans');
  return (
    <div>
      <p className="mb-2 text-xs font-medium text-muted-foreground">
        {t('channels.addDialog.providerLabel')}
      </p>
      <div className="grid grid-cols-2 gap-2 md:grid-cols-1">
        {providers.map(provider => {
          const selected = provider.id === selectedID;
          return (
            <button
              key={provider.id}
              type="button"
              aria-pressed={selected}
              onClick={() => onSelect(provider)}
              className={cn(
                'flex items-center gap-3 rounded-lg border bg-background px-3 py-2.5 text-left outline-none transition-colors hover:border-primary/40 hover:bg-background focus-visible:ring-2 focus-visible:ring-ring',
                selected && 'border-primary bg-primary/5 shadow-sm'
              )}
            >
              <span
                className={cn(
                  'flex size-8 shrink-0 items-center justify-center rounded-lg',
                  provider.accentClassName
                )}
              >
                <Bot className="size-4" />
              </span>
              <span className="min-w-0 truncate text-sm font-medium">{t(provider.nameKey)}</span>
            </button>
          );
        })}
      </div>
    </div>
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

function EmptyState({
  icon: Icon,
  title,
  description,
}: {
  icon: typeof Bot;
  title: string;
  description: string;
}) {
  return (
    <div className="flex flex-col items-center rounded-xl border border-dashed px-6 py-10 text-center">
      <span className="flex size-10 items-center justify-center rounded-xl bg-muted text-muted-foreground">
        <Icon className="size-5" />
      </span>
      <p className="mt-3 text-sm font-medium">{title}</p>
      <p className="mt-1 max-w-md text-xs text-muted-foreground">{description}</p>
    </div>
  );
}

function PanelSkeleton() {
  return (
    <Card>
      <CardHeader>
        <Skeleton className="h-5 w-36" />
        <Skeleton className="h-4 w-64" />
      </CardHeader>
      <CardContent className="space-y-3">
        <Skeleton className="h-32 w-full" />
        <Skeleton className="h-24 w-full" />
      </CardContent>
    </Card>
  );
}

function LoadFailure({
  onRetry,
  className,
  embedded = false,
}: {
  onRetry: () => void;
  className?: string;
  embedded?: boolean;
}) {
  const t = useT('digitalHumans');
  const content = (
    <div className="flex flex-col items-center px-6 py-10 text-center">
      <span className="flex size-10 items-center justify-center rounded-xl bg-destructive/10 text-destructive">
        <RefreshCw className="size-5" />
      </span>
      <p className="mt-3 text-sm font-medium">{t('channels.loadErrorTitle')}</p>
      <p className="mt-1 max-w-md text-xs text-muted-foreground">
        {t('channels.loadErrorDescription')}
      </p>
      <Button variant="outline" size="sm" className="mt-4" onClick={onRetry}>
        <RefreshCw className="size-3.5" />
        {t('common.refresh')}
      </Button>
    </div>
  );
  if (embedded) return content;
  return <Card className={className}>{content}</Card>;
}

function EventStatus({ status }: { status: DigitalHumanChannelEventStatus }) {
  const t = useT('digitalHumans');
  const variant =
    status === 'accepted'
      ? 'success'
      : status === 'failed'
        ? 'destructive'
        : status === 'ignored'
          ? 'subtle'
          : 'warning';
  return <Badge variant={variant}>{t(`channels.events.status.${status}`)}</Badge>;
}

function DeliveryStatus({ status }: { status: DigitalHumanChannelDeliveryStatus }) {
  const t = useT('digitalHumans');
  const variant =
    status === 'succeeded'
      ? 'success'
      : status === 'failed'
        ? 'destructive'
        : status === 'unknown'
          ? 'warning'
          : 'info';
  const Icon =
    status === 'succeeded'
      ? CheckCircle2
      : status === 'pending' || status === 'processing'
        ? Clock3
        : Send;
  return (
    <Badge variant={variant}>
      <Icon className={cn(status === 'processing' && 'animate-pulse')} />
      {t(`channels.deliveries.status.${status}`)}
    </Badge>
  );
}

function formatChannelTime(value: string, locale: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return { relative: '—', exact: '—' };
  const deltaSeconds = Math.round((date.getTime() - Date.now()) / 1000);
  const absoluteSeconds = Math.abs(deltaSeconds);
  const [amount, unit]: [number, Intl.RelativeTimeFormatUnit] =
    absoluteSeconds < 60
      ? [deltaSeconds, 'second']
      : absoluteSeconds < 3600
        ? [Math.round(deltaSeconds / 60), 'minute']
        : absoluteSeconds < 86400
          ? [Math.round(deltaSeconds / 3600), 'hour']
          : [Math.round(deltaSeconds / 86400), 'day'];
  return {
    relative: new Intl.RelativeTimeFormat(locale, { numeric: 'auto' }).format(amount, unit),
    exact: new Intl.DateTimeFormat(locale, {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    }).format(date),
  };
}

function channelDuration(receivedAt?: string, deliveredAt?: string | null) {
  if (!receivedAt || !deliveredAt) return null;
  const start = new Date(receivedAt).getTime();
  const end = new Date(deliveredAt).getTime();
  if (!Number.isFinite(start) || !Number.isFinite(end) || end < start) return null;
  return Math.max(0, Math.round((end - start) / 1000));
}

function eventReasonKey(reasonCode: string) {
  switch (reasonCode) {
    case 'channel.wake_policy.not_matched':
      return 'channels.events.reason.notMentioned' as const;
    case 'channel.callback.stale':
      return 'channels.events.reason.stale' as const;
    case 'channel.binding.not_found':
      return 'channels.events.reason.channelUnavailable' as const;
    case 'channel.conversation.reserve_failed':
      return 'channels.events.reason.conversationFailed' as const;
    case 'channel.invocation.submit_failed':
      return 'channels.events.reason.runFailed' as const;
    default:
      return 'channels.events.reason.generic' as const;
  }
}

async function copyValue(value: string, successMessage: string) {
  if (!value) return;
  await navigator.clipboard.writeText(value);
  toast.success(successMessage);
}
