'use client';

import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import Link from 'next/link';
import {
  AtSign,
  Bot,
  Cable,
  CheckCircle2,
  Clock3,
  Copy,
  ExternalLink,
  Loader2,
  MessageSquare,
  Plus,
  RefreshCw,
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
  useDigitalHumanChannelMutations,
  useDigitalHumanChannels,
} from '@/hooks/digital-human/use-digital-human-channels';
import {
  integrationConnectionItems,
  useAllIntegrationConnections,
} from '@/hooks/integrations/use-integrations';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  DigitalHumanChannelBindingView,
  DigitalHumanChannelDelivery,
  DigitalHumanChannelDeliveryStatus,
  DigitalHumanChannelEventStatus,
  DigitalHumanChannelInboundEvent,
  DigitalHumanChannelInstallation,
} from '@/services/types/digital-human';

const FEISHU_TENANT_APP_AUTH_METHOD = 'feishu_tenant_app';

export function DigitalHumanChannels({ id }: { id: string }) {
  const t = useT('digitalHumans');
  const permissions = useAccountPermissions();
  const canManage = permissions.hasPermission('digital_human.update');
  const canViewActivity = permissions.hasPermission('digital_human.logs.view');
  const canRetry = permissions.hasPermission('digital_human.run');
  const queries = useDigitalHumanChannels(id, canViewActivity, false);
  const mutations = useDigitalHumanChannelMutations(id);
  const [installationDialogOpen, setInstallationDialogOpen] = useState(false);
  const installations = queries.installations.data?.data ?? [];
  const bindings = queries.bindings.data?.data ?? [];
  const events = queries.events.data?.data?.items ?? [];
  const deliveries = queries.deliveries.data?.data?.items ?? [];
  const activeInstallations = installations.filter(item => item.status === 'active');
  const pendingDeliveries = deliveries.filter(item =>
    ['pending', 'processing'].includes(item.status)
  ).length;
  const configurationError =
    queries.installations.isError ||
    queries.bindings.isError;
  const activityError = queries.events.isError || queries.deliveries.isError;

  const refresh = () => {
    void Promise.all([
      queries.installations.refetch(),
      queries.bindings.refetch(),
      ...(canViewActivity ? [queries.events.refetch(), queries.deliveries.refetch()] : []),
    ]);
  };

  return (
    <main className="mx-auto w-full max-w-7xl space-y-6 px-4 py-6 md:px-8 md:py-8">
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
              {t('channels.installations.add')}
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
          label={t('channels.summary.conversations')}
          value={String(bindings.length)}
        />
        <MetricCard
          icon={Send}
          label={t('channels.summary.pending')}
          value={canViewActivity ? String(pendingDeliveries) : '—'}
        />
      </div>

      <section className="grid gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(22rem,0.9fr)]">
        {configurationError ? (
          <LoadFailure className="xl:col-span-2" onRetry={refresh} />
        ) : (
          <>
            <InstallationPanel
              id={id}
              installations={installations}
              bindings={bindings}
              loading={queries.installations.isLoading || queries.bindings.isLoading}
              canManage={canManage}
              onAdd={() => setInstallationDialogOpen(true)}
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
                  ? mutations.deleteInstallation.variables?.installationId
                  : undefined
              }
              onDelete={(installationId, removeBindings) =>
                mutations.deleteInstallation.mutate({ installationId, removeBindings })
              }
            />
            <BindingPanel
              bindings={bindings}
              installations={installations}
              loading={queries.bindings.isLoading}
            />
          </>
        )}
      </section>

      <ActivityPanel
        id={id}
        events={events}
        deliveries={deliveries}
        enabled={canViewActivity}
        loading={queries.events.isLoading || queries.deliveries.isLoading}
        error={activityError}
        canRetry={canRetry}
        retryingId={mutations.retryDelivery.variables}
        onRetry={deliveryId => mutations.retryDelivery.mutate(deliveryId)}
        onRefresh={refresh}
      />

      <CreateInstallationDialog
        open={installationDialogOpen}
        onOpenChange={setInstallationDialogOpen}
        id={id}
      />
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
  bindings,
  loading,
  canManage,
  onAdd,
  onToggle,
  onDelete,
  deletingInstallationId,
}: {
  id: string;
  installations: DigitalHumanChannelInstallation[];
  bindings: DigitalHumanChannelBindingView[];
  loading: boolean;
  canManage: boolean;
  onAdd: () => void;
  onToggle: (installation: DigitalHumanChannelInstallation) => void;
  onDelete: (installationId: string, removeBindings: boolean) => void;
  deletingInstallationId?: string;
}) {
  const t = useT('digitalHumans');
  const [origin, setOrigin] = useState('');
  useEffect(() => setOrigin(window.location.origin), []);
  if (loading) return <PanelSkeleton />;
  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between gap-4 space-y-0">
        <div className="space-y-1.5">
          <CardTitle className="text-lg">{t('channels.installations.title')}</CardTitle>
          <CardDescription>{t('channels.installations.description')}</CardDescription>
        </div>
        {canManage ? (
          <Button variant="outline" size="sm" onClick={onAdd}>
            <Plus className="size-4" />
            {t('channels.installations.addShort')}
          </Button>
        ) : null}
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
            const currentDigitalHumanBindingCount = bindings.filter(
              item => item.binding.installation_id === installation.id
            ).length;
            const bindingCount = installation.binding_count ?? currentDigitalHumanBindingCount;
            const usedBy = installation.used_by ?? [];
            const usageNames = Array.from(
              new Set(usedBy.map(usage => usage.digital_human_name.trim()).filter(Boolean))
            );
            const usageLabel =
              usageNames.length > 0
                ? usageNames.join('、')
                : t('channels.installations.unknownOwner');
            const webhookURL = origin
              ? `${origin}/console/api/webhooks/channels/feishu/${installation.id}`
              : '';
            return (
              <div key={installation.id} className="rounded-xl border bg-muted/10 p-4">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                  <div className="flex min-w-0 gap-3">
                    <span className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-blue-500/10 text-blue-600">
                      <Bot className="size-5" />
                    </span>
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <p className="truncate font-medium">{installation.name}</p>
                        <Badge variant={installation.status === 'active' ? 'success' : 'subtle'}>
                          {t(`channels.installations.status.${installation.status}`)}
                        </Badge>
                        {installation.default_subject_id === id ? (
                          <Badge variant="info">
                            {t('channels.installations.defaultForCurrent')}
                          </Badge>
                        ) : null}
                      </div>
                      <p className="mt-1 text-xs text-muted-foreground">
                        {t('channels.installations.boundCount', { count: bindingCount })}
                      </p>
                      {usedBy.length > 0 ? (
                        <p className="mt-1 text-xs text-amber-700 dark:text-amber-300">
                          {t('channels.installations.usedBy', { names: usageLabel })}
                        </p>
                      ) : null}
                      {installation.default_subject_id === id ? (
                        <p className="mt-1 text-xs text-blue-700 dark:text-blue-300">
                          {t('channels.installations.directAutoRoute')}
                        </p>
                      ) : null}
                    </div>
                  </div>
                  {canManage ? (
                    <div className="flex shrink-0 gap-1">
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
                        description={
                          bindingCount > 0
                            ? t('channels.installations.deleteInUse', {
                                names: usageLabel,
                                count: bindingCount,
                              })
                            : t('channels.installations.deleteDescription')
                        }
                        confirmText={
                          bindingCount > 0
                            ? t('channels.installations.removeBindingsAndDelete')
                            : t('channels.installations.delete')
                        }
                        cancelText={t('common.cancel')}
                        variant="danger"
                        loading={deletingInstallationId === installation.id}
                        onConfirm={() => onDelete(installation.id, bindingCount > 0)}
                      />
                    </div>
                  ) : null}
                </div>
                <div className="mt-3 grid gap-2 text-xs text-muted-foreground sm:grid-cols-2">
                  <span>
                    {t('channels.installations.tenant')}: {installation.external_tenant_id || '—'}
                  </span>
                  <span>
                    {t('channels.installations.bot')}: {installation.external_bot_id || '—'}
                  </span>
                </div>
                <div className="mt-3 rounded-lg border bg-background p-3">
                  <div className="flex items-center justify-between gap-3">
                    <span className="text-xs font-medium">
                      {t('channels.installations.webhook')}
                    </span>
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={!webhookURL}
                      onClick={() =>
                        void copyValue(webhookURL, t('channels.messages.webhookCopied'))
                      }
                    >
                      <Copy className="size-3.5" />
                      {t('channels.installations.copy')}
                    </Button>
                  </div>
                  <code className="mt-1 block break-all text-xs text-muted-foreground">
                    {webhookURL || t('channels.installations.webhookLoading')}
                  </code>
                </div>
              </div>
            );
          })
        )}
      </CardContent>
    </Card>
  );
}

function BindingPanel({
  bindings,
  installations,
  loading,
}: {
  bindings: DigitalHumanChannelBindingView[];
  installations: DigitalHumanChannelInstallation[];
  loading: boolean;
}) {
  const t = useT('digitalHumans');
  const installationsById = useMemo(
    () => new Map(installations.map(item => [item.id, item])),
    [installations]
  );
  if (loading) return <PanelSkeleton />;
  return (
    <Card>
      <CardHeader>
        <div className="space-y-1.5">
          <CardTitle className="text-lg">{t('channels.bindings.title')}</CardTitle>
          <CardDescription>{t('channels.bindings.description')}</CardDescription>
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {bindings.length === 0 ? (
          <EmptyState
            icon={AtSign}
            title={t('channels.bindings.empty')}
            description={t('channels.bindings.emptyDescription')}
          />
        ) : (
          bindings.map(item => {
            const installation = installationsById.get(item.binding.installation_id);
            return (
              <div
                key={item.binding.id}
                className="flex items-start justify-between gap-3 rounded-xl border p-4"
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="outline">
                      {t(`channels.bindings.type.${item.mapping.conversation_type}`)}
                    </Badge>
                    <p className="truncate text-sm font-medium">{t('channels.bindings.autoAssigned')}</p>
                  </div>
                  <p className="mt-2 text-xs text-muted-foreground">
                    {installation?.name ?? t('channels.bindings.unknownInstallation')} ·{' '}
                    {t(`channels.bindings.wake.${item.binding.wake_policy}`)}
                  </p>
                  {item.mapping.local_conversation_id ? (
                    <p className="mt-1 text-xs text-muted-foreground">
                      {t('channels.bindings.conversationReady')}
                    </p>
                  ) : null}
                </div>
              </div>
            );
          })
        )}
        <div className="flex gap-2 rounded-xl border border-blue-500/20 bg-blue-500/5 p-3 text-xs text-muted-foreground">
          <ShieldCheck className="mt-0.5 size-4 shrink-0 text-blue-600" />
          <p>{t('channels.bindings.securityHint')}</p>
        </div>
      </CardContent>
    </Card>
  );
}

function ActivityPanel({
  id,
  events,
  deliveries,
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
  enabled: boolean;
  loading: boolean;
  error: boolean;
  canRetry: boolean;
  retryingId?: string;
  onRetry: (deliveryId: string) => void;
  onRefresh: () => void;
}) {
  const t = useT('digitalHumans');
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
                    <EventRow key={event.id} event={event} id={id} />
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

function EventRow({ event, id }: { event: DigitalHumanChannelInboundEvent; id: string }) {
  const t = useT('digitalHumans');
  return (
    <div className="grid gap-3 p-4 md:grid-cols-[minmax(0,1fr)_11rem_10rem] md:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <EventStatus status={event.status} />
          <span className="truncate text-sm font-medium">{event.external_conversation_id}</span>
        </div>
        <p className="mt-1 truncate text-xs text-muted-foreground">
          {event.provider_event_id} · {event.provider_message_id}
        </p>
        {event.reason_code ? (
          <p className="mt-1 text-xs text-muted-foreground">{event.reason_code}</p>
        ) : null}
      </div>
      <div className="text-xs text-muted-foreground">{formatChannelDate(event.received_at)}</div>
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
  id,
  canRetry,
  retrying,
  onRetry,
}: {
  delivery: DigitalHumanChannelDelivery;
  id: string;
  canRetry: boolean;
  retrying: boolean;
  onRetry: () => void;
}) {
  const t = useT('digitalHumans');
  const canSafelyRetry = delivery.status === 'failed' && canRetry;
  const localizedError =
    delivery.error_code === 'channel.delivery.outcome_unknown'
      ? t('channels.deliveries.failure.outcomeUnknown')
      : delivery.error_code === 'channel.delivery.interrupted'
        ? t('channels.deliveries.failure.interrupted')
        : delivery.error_code === 'channel.delivery.payload_invalid'
          ? t('channels.deliveries.failure.payloadInvalid')
          : t('channels.deliveries.failure.generic');
  return (
    <div className="grid gap-3 p-4 md:grid-cols-[minmax(0,1fr)_12rem_13rem] md:items-center">
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <DeliveryStatus status={delivery.status} />
          <span className="text-xs text-muted-foreground">
            {t('channels.deliveries.attempts', { count: delivery.attempt_count })}
          </span>
        </div>
        <p className="mt-2 line-clamp-2 text-sm">
          {delivery.payload?.text || t('channels.deliveries.noText')}
        </p>
        {delivery.error_code ? (
          <p className="mt-1 text-xs text-destructive">
            {delivery.error_code} · {localizedError}
          </p>
        ) : null}
        {delivery.status === 'unknown' ? (
          <p className="mt-1 text-xs text-amber-700 dark:text-amber-300">
            {t('channels.deliveries.unknownHint')}
          </p>
        ) : null}
      </div>
      <div className="text-xs text-muted-foreground">{formatChannelDate(delivery.created_at)}</div>
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

function CreateInstallationDialog({
  open,
  onOpenChange,
  id,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  id: string;
}) {
  const t = useT('digitalHumans');
  const connectionsQuery = useAllIntegrationConnections(
    { integration_id: 'feishu', status: 'active' },
    open
  );
  const mutations = useDigitalHumanChannelMutations(id);
  const connections = integrationConnectionItems(connectionsQuery.data?.data).filter(
    item =>
      item.integration_id === 'feishu' &&
      item.status === 'active' &&
      item.credential_source === 'organization' &&
      item.auth_method_id === FEISHU_TENANT_APP_AUTH_METHOD
  );
  const [connectionId, setConnectionId] = useState('');
  const [name, setName] = useState('');
  const wasOpen = useRef(false);
  useEffect(() => {
    if (open && !wasOpen.current) {
      setConnectionId('');
      setName(t('channels.installations.defaultName'));
    }
    wasOpen.current = open;
  }, [open, t]);
  const submit = async () => {
    try {
      await mutations.createInstallation.mutateAsync({
        provider: 'feishu',
        connection_id: connectionId,
        name: name.trim(),
      });
      onOpenChange(false);
    } catch {
      // The mutation owns localized error feedback and keeps the form open.
    }
  };
  const ready = Boolean(connectionId && name.trim());
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{t('channels.installations.dialogTitle')}</DialogTitle>
          <DialogDescription>{t('channels.installations.dialogDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-5">
          <div className="rounded-xl border border-blue-500/20 bg-blue-500/5 p-4 text-sm">
            <p className="font-medium">{t('channels.installations.connectionPrerequisite')}</p>
            <p className="mt-1 text-muted-foreground">
              {t('channels.installations.connectionHint')}
            </p>
            <Button variant="link" className="mt-2 h-auto p-0" asChild>
              <Link href="/console/integrations">
                {t('channels.installations.openIntegrations')}
                <ExternalLink className="size-3.5" />
              </Link>
            </Button>
          </div>
          <Field label={t('channels.installations.connection')}>
            <Select
              value={connectionId}
              onValueChange={setConnectionId}
              disabled={
                connectionsQuery.isLoading || connectionsQuery.isError || connections.length === 0
              }
            >
              <SelectTrigger isLoading={connectionsQuery.isLoading}>
                <SelectValue placeholder={t('channels.installations.selectConnection')} />
              </SelectTrigger>
              <SelectContent className="z-[100]">
                {connections.map(connection => (
                  <SelectItem key={connection.id} value={connection.id}>
                    {connection.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {connectionsQuery.isLoading ? (
              <p className="text-xs text-muted-foreground">
                {t('channels.installations.loadingConnections')}
              </p>
            ) : null}
            {connectionsQuery.isError ? (
              <p className="text-xs text-destructive">
                {t('channels.installations.connectionLoadError')}
              </p>
            ) : null}
            {!connectionsQuery.isLoading &&
            !connectionsQuery.isError &&
            connections.length === 0 ? (
              <p className="text-xs text-destructive">
                {t('channels.installations.noConnections')}
              </p>
            ) : null}
          </Field>
          <Field label={t('channels.installations.name')}>
            <Input value={name} onChange={event => setName(event.target.value)} />
          </Field>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common.cancel')}
          </Button>
          <Button
            onClick={() => void submit()}
            disabled={!ready || mutations.createInstallation.isPending}
          >
            {mutations.createInstallation.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : null}
            {t('channels.installations.create')}
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

function formatChannelDate(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleString();
}

async function copyValue(value: string, successMessage: string) {
  if (!value) return;
  await navigator.clipboard.writeText(value);
  toast.success(successMessage);
}
