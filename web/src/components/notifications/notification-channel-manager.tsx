'use client';

import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Copy, KeyRound, Mail, Plus, Send, Trash2, Webhook } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
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
import { Switch } from '@/components/ui/switch';
import { useT } from '@/i18n';
import {
  notificationChannelService,
  type NotificationChannel,
  type NotificationChannelEvent,
  type NotificationChannelType,
} from '@/services/notification-channel.service';
import { getErrorMessage } from '@/utils/error-notifications';

// Approvals wait only for API end users, so only a DigitalHuman's webhook offers them.
const defaultEvents: NotificationChannelEvent[] = ['task_report', 'task_failed'];

/**
 * Manages notification channels at basePath: personal email and webhooks,
 * or a DigitalHuman's webhook for its API integrations.
 */
export function NotificationChannelManager({
  basePath,
  types,
  canManage = true,
  events: allEvents = defaultEvents,
}: {
  basePath: string;
  types: NotificationChannelType[];
  canManage?: boolean;
  // The events these channels may subscribe to.
  events?: NotificationChannelEvent[];
}) {
  const t = useT('common');
  const client = useQueryClient();
  const queryKey = ['notification-channels', basePath];
  const query = useQuery({ queryKey, queryFn: () => notificationChannelService.list(basePath) });
  const refresh = () => client.invalidateQueries({ queryKey });
  const onError = (error: unknown) => toast.error(getErrorMessage(error) || t('notifications.errors.generic'));
  const [adding, setAdding] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const channels = query.data?.data.items ?? [];

  const update = useMutation({
    mutationFn: ({ id, body }: { id: string; body: Parameters<typeof notificationChannelService.update>[2] }) =>
      notificationChannelService.update(basePath, id, body),
    onSuccess: refresh,
    onError,
  });
  const remove = useMutation({
    mutationFn: (id: string) => notificationChannelService.remove(basePath, id),
    onSuccess: () => {
      toast.success(t('notifications.deleted'));
      void refresh();
    },
    onError,
  });
  const test = useMutation({
    mutationFn: (id: string) => notificationChannelService.test(basePath, id),
    onSuccess: response =>
      response.data.sent
        ? toast.success(t('notifications.testSent'))
        : toast.error(t('notifications.testFailed', { error: response.data.error ?? '' })),
    onError,
  });
  const rotate = useMutation({
    mutationFn: (id: string) => notificationChannelService.rotateSecret(basePath, id),
    onSuccess: response => setSecret(response.data.secret),
    onError,
  });

  return (
    <div className="space-y-3">
      {canManage ? (
        <div className="flex justify-end">
          <Button size="sm" onClick={() => setAdding(true)}>
            <Plus className="size-4" />
            {t('notifications.add')}
          </Button>
        </div>
      ) : null}
      {query.isLoading ? (
        <Skeleton className="h-24 w-full" />
      ) : channels.length === 0 ? (
        <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
          {t('notifications.empty')}
        </div>
      ) : (
        <ul className="space-y-3">
          {channels.map(channel => (
            <ChannelRow
              key={channel.id}
              basePath={basePath}
              channel={channel}
              canManage={canManage}
              allEvents={allEvents}
              onEvents={events => update.mutate({ id: channel.id, body: { events } })}
              onEnabled={enabled => update.mutate({ id: channel.id, body: { enabled } })}
              onTest={() => test.mutate(channel.id)}
              onRotate={() => rotate.mutate(channel.id)}
              onDelete={() => remove.mutate(channel.id)}
              onVerified={() => void refresh()}
              busy={update.isPending || test.isPending}
            />
          ))}
        </ul>
      )}
      <AddChannelDialog
        open={adding}
        types={types}
        allEvents={allEvents}
        basePath={basePath}
        onOpenChange={setAdding}
        onCreated={created => {
          setAdding(false);
          if (created.secret) setSecret(created.secret);
          else if (created.type === 'email') toast.success(t('notifications.codeSent', { address: created.target }));
          void refresh();
        }}
      />
      <Dialog open={Boolean(secret)} onOpenChange={open => !open && setSecret(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('notifications.secretTitle')}</DialogTitle>
            <DialogDescription>{t('notifications.secretDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody>
            <div className="flex gap-2">
              <Input readOnly value={secret ?? ''} className="font-mono text-xs" />
              <Button
                variant="outline"
                onClick={() => {
                  void navigator.clipboard.writeText(secret ?? '');
                  toast.success(t('notifications.copied'));
                }}
              >
                <Copy className="size-4" />
              </Button>
            </div>
          </DialogBody>
          <DialogFooter>
            <Button onClick={() => setSecret(null)}>{t('notifications.secretDone')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function ChannelRow({
  basePath,
  channel,
  canManage,
  allEvents,
  busy,
  onEvents,
  onEnabled,
  onTest,
  onRotate,
  onDelete,
  onVerified,
}: {
  basePath: string;
  channel: NotificationChannel;
  canManage: boolean;
  allEvents: NotificationChannelEvent[];
  busy: boolean;
  onEvents: (events: NotificationChannelEvent[]) => void;
  onEnabled: (enabled: boolean) => void;
  onTest: () => void;
  onRotate: () => void;
  onDelete: () => void;
  onVerified: () => void;
}) {
  const t = useT('common');
  const [code, setCode] = useState('');
  const verify = useMutation({
    mutationFn: () => notificationChannelService.verify(basePath, channel.id, code.trim()),
    onSuccess: () => {
      toast.success(t('notifications.verified'));
      setCode('');
      onVerified();
    },
    onError: (error: unknown) => toast.error(getErrorMessage(error) || t('notifications.errors.generic')),
  });
  const resend = useMutation({
    mutationFn: () => notificationChannelService.resendCode(basePath, channel.id),
    onSuccess: () => toast.success(t('notifications.codeSent', { address: channel.target })),
    onError: (error: unknown) => toast.error(getErrorMessage(error) || t('notifications.errors.generic')),
  });
  const Icon = channel.type === 'email' ? Mail : Webhook;
  const statusVariant = channel.status === 'active' ? 'success' : channel.status === 'pending' ? 'warning' : 'subtle';
  return (
    <li className="rounded-lg border p-4">
      <div className="flex flex-wrap items-start gap-3">
        <Icon className="mt-0.5 size-5 shrink-0 text-muted-foreground" />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="break-all text-sm font-medium">{channel.name || channel.target}</span>
            {/* The switch already shows whether a manageable channel is on. */}
            {!canManage || channel.status === 'pending' ? (
              <Badge variant={statusVariant}>{t(`notifications.status.${channel.status}`)}</Badge>
            ) : null}
          </div>
          {channel.name && channel.name !== channel.target ? (
            <p className="mt-0.5 break-all text-xs text-muted-foreground">{channel.target}</p>
          ) : null}
        </div>
        {canManage && channel.status !== 'pending' ? (
          <div className="flex items-center gap-2">
            <Label htmlFor={`channel-${channel.id}`} className="text-xs text-muted-foreground">
              {t('notifications.enabled')}
            </Label>
            <Switch id={`channel-${channel.id}`} checked={channel.status === 'active'} disabled={busy} onCheckedChange={onEnabled} />
          </div>
        ) : null}
      </div>
      {channel.status === 'pending' && canManage ? (
        <div className="mt-3 flex flex-wrap items-center gap-2 rounded-md bg-warning/5 p-3">
          <span className="text-xs text-muted-foreground">{t('notifications.enterCode', { address: channel.target })}</span>
          <Input
            value={code}
            inputMode="numeric"
            maxLength={6}
            placeholder="000000"
            onChange={event => setCode(event.target.value.replace(/\D/g, ''))}
            className="h-8 w-28 font-mono"
          />
          <Button size="sm" disabled={code.length !== 6 || verify.isPending} onClick={() => verify.mutate()}>
            {t('notifications.verify')}
          </Button>
          <Button size="sm" variant="ghost" disabled={resend.isPending} onClick={() => resend.mutate()}>
            {t('notifications.resend')}
          </Button>
        </div>
      ) : null}
      <div className="mt-3 flex flex-wrap items-center gap-4">
        <span className="text-xs text-muted-foreground">{t('notifications.eventsLabel')}</span>
        {allEvents.map(event => (
          <label key={event} className="flex items-center gap-1.5 text-sm">
            <Checkbox
              checked={channel.events.includes(event)}
              disabled={!canManage || busy}
              onCheckedChange={checked => {
                const next = checked ? [...channel.events, event] : channel.events.filter(item => item !== event);
                if (next.length === 0) {
                  toast.error(t('notifications.atLeastOneEvent'));
                  return;
                }
                onEvents(next);
              }}
            />
            {t(`notifications.event.${event}`)}
          </label>
        ))}
        {canManage ? (
          <div className="ml-auto flex flex-wrap gap-1">
            <Button size="sm" variant="ghost" disabled={channel.status !== 'active' || busy} onClick={onTest}>
              <Send className="size-4" />
              {t('notifications.test')}
            </Button>
            {channel.type === 'webhook' ? (
              <Button size="sm" variant="ghost" onClick={onRotate}>
                <KeyRound className="size-4" />
                {t('notifications.rotate')}
              </Button>
            ) : null}
            <ConfirmDialog
              variant="danger"
              title={t('notifications.deleteTitle')}
              description={t('notifications.deleteDescription')}
              confirmText={t('notifications.delete')}
              onConfirm={onDelete}
              trigger={
                <Button size="sm" variant="ghost">
                  <Trash2 className="size-4" />
                  {t('notifications.delete')}
                </Button>
              }
            />
          </div>
        ) : null}
      </div>
    </li>
  );
}

function AddChannelDialog({
  open,
  types,
  allEvents,
  basePath,
  onOpenChange,
  onCreated,
}: {
  open: boolean;
  types: NotificationChannelType[];
  allEvents: NotificationChannelEvent[];
  basePath: string;
  onOpenChange: (open: boolean) => void;
  onCreated: (channel: NotificationChannel) => void;
}) {
  const t = useT('common');
  const [type, setType] = useState<NotificationChannelType>(types[0]);
  const [target, setTarget] = useState('');
  const [name, setName] = useState('');
  const [events, setEvents] = useState<NotificationChannelEvent[]>(['task_failed']);
  const create = useMutation({
    mutationFn: () => notificationChannelService.create(basePath, { type, target: target.trim(), name: name.trim() || undefined, events }),
    onSuccess: response => {
      setTarget('');
      setName('');
      onCreated(response.data);
    },
    onError: (error: unknown) => toast.error(getErrorMessage(error) || t('notifications.errors.generic')),
  });
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('notifications.addTitle')}</DialogTitle>
          <DialogDescription>{t('notifications.addDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          {types.length > 1 ? (
            <div className="flex gap-2">
              {types.map(item => (
                <Button key={item} size="sm" variant={type === item ? 'default' : 'outline'} onClick={() => setType(item)}>
                  {item === 'email' ? <Mail className="size-4" /> : <Webhook className="size-4" />}
                  {t(`notifications.type.${item}`)}
                </Button>
              ))}
            </div>
          ) : null}
          <div className="space-y-1.5">
            <Label>{type === 'email' ? t('notifications.emailLabel') : t('notifications.urlLabel')}</Label>
            <Input
              value={target}
              onChange={event => setTarget(event.target.value)}
              placeholder={type === 'email' ? 'name@example.com' : 'https://example.com/zgi/webhook'}
            />
            {type === 'webhook' ? <p className="text-xs text-muted-foreground">{t('notifications.webhookHint')}</p> : null}
          </div>
          <div className="space-y-1.5">
            <Label>{t('notifications.nameLabel')}</Label>
            <Input value={name} onChange={event => setName(event.target.value)} placeholder={t('notifications.namePlaceholder')} />
          </div>
          <div className="space-y-1.5">
            <Label>{t('notifications.eventsLabel')}</Label>
            <div className="flex gap-4">
              {allEvents.map(event => (
                <label key={event} className="flex items-center gap-1.5 text-sm">
                  <Checkbox
                    checked={events.includes(event)}
                    onCheckedChange={checked =>
                      setEvents(current => (checked ? [...current, event] : current.filter(item => item !== event)))
                    }
                  />
                  {t(`notifications.event.${event}`)}
                </label>
              ))}
            </div>
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t('notifications.cancel')}
          </Button>
          <Button disabled={!target.trim() || events.length === 0 || create.isPending} onClick={() => create.mutate()}>
            {t('notifications.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
