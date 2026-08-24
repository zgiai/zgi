'use client';

import { useEffect, useRef, useState, type ReactNode } from 'react';
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
  DigitalHumanChannelInstallation,
} from '@/services/types/digital-human';
import type { IntegrationConnection } from '@/services/types/integration';
import { getErrorMessage } from '@/utils/error-notifications';

interface EditingChannelCredential {
  provider: DigitalHumanChannelProviderDefinition;
  installation: DigitalHumanChannelInstallation;
}

const EMPTY_CHANNEL_CREDENTIALS: IntegrationConnection[] = [];

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
            installations={installations}
            loading={queries.installations.isLoading}
            canManage={canManage}
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
  installations,
  loading,
  canManage,
  onToggle,
  onDelete,
  onEditCredential,
  deletingInstallationId,
}: {
  installations: DigitalHumanChannelInstallation[];
  loading: boolean;
  canManage: boolean;
  onToggle: (installation: DigitalHumanChannelInstallation) => void;
  onDelete: (installationId: string) => void;
  onEditCredential: (
    provider: DigitalHumanChannelProviderDefinition,
    installation: DigitalHumanChannelInstallation
  ) => void;
  deletingInstallationId?: string;
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
                      event={eventsByID.get(delivery.inbound_event_id)}
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
            <details className="mt-1 text-muted-foreground">
              <summary className="cursor-pointer outline-none focus-visible:ring-2 focus-visible:ring-ring">
                {t('common.technicalDetails')}
              </summary>
              <code className="mt-1 block break-all">{delivery.error_code}</code>
            </details>
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
      <DialogContent size="xl" className="p-0">
        <DialogHeader>
          <DialogTitle>{t('channels.addDialog.title')}</DialogTitle>
          <DialogDescription>{t('channels.addDialog.description')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="pb-5">
          <div className="grid gap-5 md:grid-cols-[12rem_minmax(0,1fr)]">
            <ChannelProviderSelector
              providers={ENABLED_DIGITAL_HUMAN_CHANNEL_PROVIDERS}
              selectedID={provider?.id ?? ''}
              onSelect={selectProvider}
            />
            <div className="min-w-0 space-y-5 md:border-l md:pl-5">
              {provider ? (
                <>
                  <div>
                    <h3 className="font-medium">{t(provider.nameKey)}</h3>
                    <p className="mt-1 text-xs text-muted-foreground">
                      {t(provider.descriptionKey)}
                    </p>
                  </div>
                  <div className="space-y-4">
                    <Tabs
                      value={credentialMode}
                      onValueChange={value => setCredentialMode(value as 'embedded' | 'connection')}
                    >
                      <TabsList>
                        <TabsTrigger value="embedded">
                          {t('channels.installations.credential.directConfig')}
                        </TabsTrigger>
                        <TabsTrigger value="connection">
                          {t('channels.installations.credential.connectionCenter')}
                        </TabsTrigger>
                      </TabsList>
                      <TabsContent value="embedded" className="mt-4 space-y-4">
                        {CredentialFields ? (
                          <CredentialFields value={credential} onChange={setCredential} />
                        ) : null}
                      </TabsContent>
                      <TabsContent value="connection" className="mt-4 space-y-3">
                        {providerConnections.length > 0 ? (
                          <Field label={t('channels.installations.credential.selectConnection')}>
                            <select
                              value={selectedConnectionID}
                              onChange={event => setSelectedConnectionID(event.target.value)}
                              className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                            >
                              <option value="">
                                {t('channels.installations.credential.selectConnectionPlaceholder')}
                              </option>
                              {providerConnections.map(item => (
                                <option key={item.id} value={item.id}>
                                  {item.name}
                                </option>
                              ))}
                            </select>
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
                    <Field label={t('channels.installations.name')}>
                      <Input value={name} onChange={event => setName(event.target.value)} />
                    </Field>
                  </div>
                  {credentialError ? (
                    <p className="text-sm text-destructive">{credentialError}</p>
                  ) : null}
                </>
              ) : null}
            </div>
          </div>
        </DialogBody>
        <DialogFooter className="border-t bg-muted/30">
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
      <div className="grid gap-2 sm:grid-cols-2 md:grid-cols-1">
        {providers.map(provider => {
          const selected = provider.id === selectedID;
          return (
            <button
              key={provider.id}
              type="button"
              aria-pressed={selected}
              onClick={() => onSelect(provider)}
              className={cn(
                'flex items-center gap-3 rounded-xl border px-3 py-3 text-left outline-none transition-colors hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring',
                selected && 'border-primary bg-primary/5'
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
