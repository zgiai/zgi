'use client';

import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import { useT } from '@/i18n';
import { notificationChannelService } from '@/services/notification-channel.service';
import { getErrorMessage } from '@/utils/error-notifications';

function toClock(minutes: number | null, fallback: string) {
  if (minutes === null) return fallback;
  return `${String(Math.floor(minutes / 60)).padStart(2, '0')}:${String(minutes % 60).padStart(2, '0')}`;
}

function toMinutes(clock: string) {
  const [hours, minutes] = clock.split(':').map(Number);
  return Number.isFinite(hours) && Number.isFinite(minutes) ? hours * 60 + minutes : null;
}

/**
 * @component QuietHoursCard
 * @category Feature
 * @status Beta
 * @description The member's quiet hours (ADR 0008 stage 4): notifications to
 * their own email and IM wait until the window ends.
 * @usage Rendered on the profile notification settings page.
 * @example
 * <QuietHoursCard />
 */
export function QuietHoursCard() {
  const t = useT('common');
  const client = useQueryClient();
  const queryKey = ['notification-preferences'];
  const query = useQuery({ queryKey, queryFn: () => notificationChannelService.preferences() });
  const preferences = query.data?.data;
  const [enabled, setEnabled] = useState(false);
  const [from, setFrom] = useState('22:00');
  const [to, setTo] = useState('08:00');
  useEffect(() => {
    if (!preferences) return;
    setEnabled(preferences.quiet_start !== null);
    setFrom(toClock(preferences.quiet_start, '22:00'));
    setTo(toClock(preferences.quiet_end, '08:00'));
  }, [preferences]);
  const save = useMutation({
    mutationFn: () =>
      enabled
        ? notificationChannelService.setQuietHours(toMinutes(from), toMinutes(to))
        : notificationChannelService.setQuietHours(null, null),
    onSuccess: () => {
      toast.success(t('notifications.quietSaved'));
      void client.invalidateQueries({ queryKey });
    },
    onError: (error: unknown) =>
      toast.error(getErrorMessage(error) || t('notifications.errors.generic')),
  });

  if (query.isLoading) return <Skeleton className="h-24 w-full" />;
  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">{t('notifications.quietDescription')}</p>
      <div className="flex items-center gap-2">
        <Switch id="quiet-hours" checked={enabled} onCheckedChange={setEnabled} />
        <Label htmlFor="quiet-hours" className="text-sm">
          {t('notifications.quietEnabled')}
        </Label>
      </div>
      {enabled ? (
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <span className="text-muted-foreground">{t('notifications.quietFrom')}</span>
          <Input
            type="time"
            value={from}
            onChange={event => setFrom(event.target.value)}
            className="h-9 w-32"
          />
          <span className="text-muted-foreground">{t('notifications.quietTo')}</span>
          <Input
            type="time"
            value={to}
            onChange={event => setTo(event.target.value)}
            className="h-9 w-32"
          />
        </div>
      ) : null}
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs text-muted-foreground">
          {t('notifications.quietTimezone', { timezone: preferences?.timezone ?? 'UTC' })}
        </span>
        <Button
          size="sm"
          disabled={save.isPending || (enabled && from === to)}
          onClick={() => save.mutate()}
        >
          {t('notifications.quietSave')}
        </Button>
      </div>
    </div>
  );
}
