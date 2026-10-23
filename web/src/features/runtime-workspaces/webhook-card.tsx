'use client';

import * as React from 'react';
import { Check, Copy, KeyRound, Loader2, RotateCcw, Send, Trash2, Webhook } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
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
import { Switch } from '@/components/ui/switch';
import { useRuntimeWebhook } from '@/hooks/runtime-workspace/use-runtime-webhook';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  RuntimeWebhookDelivery,
  RuntimeWebhookEvent,
} from '@/services/types/runtime-workspace';

const events: RuntimeWebhookEvent[] = [
  'quota.near',
  'quota.exceeded',
  'credits.low',
  'plan.expired',
];
type EventKey = 'quotaNear' | 'quotaExceeded' | 'creditsLow' | 'planExpired' | 'test';
const eventKeys: Record<RuntimeWebhookDelivery['event'], EventKey> = {
  'quota.near': 'quotaNear',
  'quota.exceeded': 'quotaExceeded',
  'credits.low': 'creditsLow',
  'plan.expired': 'planExpired',
  'webhook.test': 'test',
};

function formatTime(value?: string): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'short', timeStyle: 'medium' }).format(
    date
  );
}

/**
 * @component RuntimeWebhookCard
 * @category Feature
 * @status Beta
 * @description Sets up the webhook a Runtime Workspace sends its integrating
 * server (ADR 0011): the endpoint, the end-user events it receives, and the
 * credits threshold; shows the signing secret once, sends a test, rotates
 * the secret, and lists recent deliveries with retries.
 * @usage Rendered on the Runtime Workspace quota page.
 * @example
 * <RuntimeWebhookCard runtimeWorkspaceId={id} />
 */
export function RuntimeWebhookCard({ runtimeWorkspaceId }: { runtimeWorkspaceId: string }) {
  const t = useT('workspace');
  const tCommon = useT('common');
  const { webhook, deliveries, save, remove, rotate, test, retry } =
    useRuntimeWebhook(runtimeWorkspaceId);
  const current = webhook.data;
  const [url, setUrl] = React.useState('');
  const [selected, setSelected] = React.useState<RuntimeWebhookEvent[]>(events);
  const [threshold, setThreshold] = React.useState('0');
  const [enabled, setEnabled] = React.useState(true);
  const [secret, setSecret] = React.useState<string | null>(null);
  const [copied, setCopied] = React.useState(false);
  const [confirm, setConfirm] = React.useState<'rotate' | 'remove' | null>(null);

  const reset = React.useCallback(() => {
    setUrl(current?.url ?? '');
    setSelected(current?.configured ? current.events : events);
    setThreshold(String(current?.credits_low_threshold ?? 0));
    setEnabled(current?.enabled ?? true);
  }, [current]);
  React.useEffect(reset, [reset]);

  const thresholdValue = Number.parseFloat(threshold);
  const valid =
    /^https?:\/\/\S+$/i.test(url.trim()) &&
    selected.length > 0 &&
    Number.isFinite(thresholdValue) &&
    thresholdValue >= 0;
  const dirty =
    !current?.configured ||
    url.trim() !== (current.url ?? '') ||
    enabled !== current.enabled ||
    thresholdValue !== current.credits_low_threshold ||
    [...selected].sort().join() !== [...current.events].sort().join();

  const submit = async () => {
    const saved = await save.mutateAsync({
      url: url.trim(),
      events: selected,
      credits_low_threshold: Math.round(thresholdValue * 1000) / 1000,
      enabled,
    });
    if (saved.secret) setSecret(saved.secret);
    else toast.success(t('runtime.webhook.saved'));
  };

  const copySecret = async () => {
    if (!secret) return;
    await navigator.clipboard.writeText(secret);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };

  return (
    <Card className="border-border/80">
      <CardContent className="space-y-5 p-5">
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-start gap-3">
            <div className="flex size-9 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
              <Webhook className="size-4" />
            </div>
            <div className="min-w-0">
              <h3 className="text-base font-semibold">{t('runtime.webhook.title')}</h3>
              <p className="mt-1 text-sm leading-6 text-muted-foreground">
                {t('runtime.webhook.description')}
              </p>
            </div>
          </div>
          {current?.configured ? (
            <Badge variant={current.enabled ? 'secondary' : 'outline'} className="shrink-0">
              {current.enabled ? t('runtime.webhook.on') : t('runtime.webhook.off')}
            </Badge>
          ) : null}
        </div>

        <div className="grid gap-3 @3xl/console:grid-cols-[minmax(0,1fr)_12rem]">
          <label className="space-y-1">
            <span className="text-xs text-muted-foreground">{t('runtime.webhook.url')}</span>
            <Input
              value={url}
              placeholder="https://example.com/zgi/webhook"
              onChange={event => setUrl(event.target.value)}
              className="h-9 font-mono text-xs"
            />
          </label>
          <label className="space-y-1">
            <span className="text-xs text-muted-foreground">
              {t('runtime.webhook.creditsThreshold')}
            </span>
            <Input
              type="number"
              min={0}
              step="any"
              value={threshold}
              disabled={!selected.includes('credits.low')}
              onChange={event => setThreshold(event.target.value)}
              className="h-9"
            />
          </label>
        </div>

        <div className="grid gap-2 @2xl/console:grid-cols-2">
          {events.map(event => (
            <label
              key={event}
              className="flex cursor-pointer items-start gap-2.5 rounded-lg border p-3 hover:bg-muted/30"
            >
              <Checkbox
                className="mt-0.5"
                checked={selected.includes(event)}
                onCheckedChange={value =>
                  setSelected(
                    value === true ? [...selected, event] : selected.filter(item => item !== event)
                  )
                }
              />
              <span className="min-w-0">
                <span className="flex items-center gap-2 text-sm font-medium">
                  {t(`runtime.webhook.events.${eventKeys[event]}`)}
                  <code className="text-[11px] font-normal text-muted-foreground">{event}</code>
                </span>
                <span className="mt-0.5 block text-xs leading-5 text-muted-foreground">
                  {t(`runtime.webhook.eventDescriptions.${eventKeys[event]}`)}
                </span>
              </span>
            </label>
          ))}
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <label className="mr-auto flex items-center gap-2 text-sm">
            <Switch checked={enabled} onCheckedChange={setEnabled} />
            {t('runtime.webhook.enabled')}
          </label>
          {current?.configured ? (
            <>
              <Button
                type="button"
                size="sm"
                variant="outline"
                disabled={test.isPending}
                onClick={() =>
                  void test.mutateAsync().then(() => toast.success(t('runtime.webhook.testQueued')))
                }
              >
                {test.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : (
                  <Send className="size-4" />
                )}
                {t('runtime.webhook.test')}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setConfirm('rotate')}
              >
                <KeyRound className="size-4" />
                {t('runtime.webhook.rotate')}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="text-destructive hover:text-destructive"
                aria-label={t('runtime.webhook.remove')}
                onClick={() => setConfirm('remove')}
              >
                <Trash2 className="size-4" />
              </Button>
            </>
          ) : null}
          {current?.configured && dirty ? (
            <Button type="button" size="sm" variant="ghost" onClick={reset}>
              <RotateCcw className="size-4" />
              {t('runtime.quota.discard')}
            </Button>
          ) : null}
          <Button
            type="button"
            size="sm"
            disabled={!valid || !dirty || save.isPending}
            onClick={() => void submit()}
          >
            {save.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
            {current?.configured ? t('runtime.webhook.save') : t('runtime.webhook.create')}
          </Button>
        </div>

        <p className="rounded-md bg-muted/40 px-3 py-2 text-xs leading-5 text-muted-foreground">
          {t('runtime.webhook.signing')}
        </p>

        {current?.configured ? (
          <section className="space-y-2">
            <h4 className="text-sm font-medium">{t('runtime.webhook.deliveries')}</h4>
            {(deliveries.data ?? []).length === 0 ? (
              <p className="text-xs text-muted-foreground">{t('runtime.webhook.noDeliveries')}</p>
            ) : (
              <div className="divide-y rounded-lg border">
                {(deliveries.data ?? []).map(delivery => (
                  <div key={delivery.id} className="flex items-center gap-3 px-3 py-2 text-sm">
                    <span
                      className={cn(
                        'size-2 shrink-0 rounded-full',
                        delivery.status === 'delivered' && 'bg-success',
                        delivery.status === 'pending' && 'bg-warning',
                        delivery.status === 'failed' && 'bg-destructive'
                      )}
                    />
                    <div className="min-w-0 flex-1">
                      <div className="truncate">
                        {t(`runtime.webhook.events.${eventKeys[delivery.event]}`)}
                        <span className="text-muted-foreground">
                          {' · '}
                          {t(`runtime.webhook.status.${delivery.status}`)}
                          {delivery.attempts > 0
                            ? ` · ${t('runtime.webhook.attempts', { count: delivery.attempts })}`
                            : ''}
                        </span>
                      </div>
                      {delivery.last_error ? (
                        <div className="truncate text-xs text-destructive">
                          {delivery.last_error}
                        </div>
                      ) : null}
                    </div>
                    <span className="shrink-0 text-xs text-muted-foreground">
                      {formatTime(delivery.delivered_at ?? delivery.created_at)}
                    </span>
                    {delivery.status === 'failed' ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        disabled={retry.isPending}
                        onClick={() => void retry.mutateAsync(delivery.id)}
                      >
                        {t('runtime.webhook.retry')}
                      </Button>
                    ) : null}
                  </div>
                ))}
              </div>
            )}
          </section>
        ) : null}
      </CardContent>

      <Dialog open={Boolean(secret)} onOpenChange={open => (!open ? setSecret(null) : undefined)}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t('runtime.webhook.secretTitle')}</DialogTitle>
            <DialogDescription>{t('runtime.webhook.secretDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody>
            <div className="flex items-center gap-2 rounded-md border bg-muted/40 p-2">
              <code className="min-w-0 flex-1 break-all text-xs">{secret}</code>
              <Button type="button" size="sm" variant="ghost" onClick={() => void copySecret()}>
                {copied ? <Check className="size-4" /> : <Copy className="size-4" />}
              </Button>
            </div>
          </DialogBody>
          <DialogFooter>
            <Button type="button" onClick={() => setSecret(null)}>
              {t('runtime.webhook.secretDone')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={confirm !== null}
        onOpenChange={open => {
          if (!open) setConfirm(null);
        }}
        title={
          confirm === 'rotate' ? t('runtime.webhook.rotateTitle') : t('runtime.webhook.removeTitle')
        }
        description={
          confirm === 'rotate'
            ? t('runtime.webhook.rotateDescription')
            : t('runtime.webhook.removeDescription')
        }
        confirmText={
          confirm === 'rotate' ? t('runtime.webhook.rotate') : t('runtime.webhook.remove')
        }
        cancelText={tCommon('cancel')}
        variant={confirm === 'remove' ? 'danger' : 'warning'}
        loading={rotate.isPending || remove.isPending}
        closeOnConfirm={false}
        onConfirm={() => {
          if (confirm === 'rotate') {
            void rotate.mutateAsync().then(value => {
              setConfirm(null);
              setSecret(value);
            });
          } else if (confirm === 'remove') {
            void remove.mutateAsync().then(() => setConfirm(null));
          }
        }}
      />
    </Card>
  );
}
