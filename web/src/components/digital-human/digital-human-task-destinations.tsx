'use client';

import { useQuery } from '@tanstack/react-query';
import { Bell, Plus, Webhook, X } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { useT } from '@/i18n';
import {
  notificationChannelService,
  TEAM_NOTIFICATION_CHANNELS_PATH,
} from '@/services/notification-channel.service';
import type { DigitalHumanTask } from '@/services/types/digital-human';

/**
 * Shows where a shared task's reports go: always the requester's inbox and
 * channels, plus any team channels. The requester adds and removes team
 * channels; a task manager may only remove them. Elsewhere only shared tasks
 * show this, unless requesterView asks to show the requester their own
 * private task's destinations too, such as before they approve it.
 */
export function TaskReportDestinations({
  task,
  onChange,
  requesterView = false,
}: {
  task: DigitalHumanTask;
  onChange?: (channelIds: string[]) => Promise<void> | void;
  requesterView?: boolean;
}) {
  const t = useT('digitalHumans');
  const selected = task.notify_channel_ids ?? [];
  const access = onChange ? task.notify_channel_access : undefined;
  const shared = task.visibility === 'workspace';
  const teamVisible = shared && (selected.length > 0 || Boolean(access));
  const choices = useQuery({
    queryKey: ['notification-channels', TEAM_NOTIFICATION_CHANNELS_PATH, 'options'],
    queryFn: () => notificationChannelService.teamChoices(),
    enabled: teamVisible,
  });
  // A private task shows its destinations only to a requester who could
  // share it; external end users have neither a team nor team channels.
  const canShare = (task.visibility_options ?? []).includes('workspace');
  if (!teamVisible && !(requesterView && !shared && canShare)) return null;

  const options = choices.data?.data.items ?? [];
  const nameOf = (id: string) => options.find(option => option.id === id)?.name;
  const addable = access === 'manage' ? options.filter(option => !selected.includes(option.id)) : [];

  return (
    <div className="space-y-2 rounded-md border px-3 py-2">
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-medium text-muted-foreground">{t('tasks.destinations.label')}</span>
        {access === 'manage' && addable.length > 0 ? (
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button size="sm" variant="ghost" className="h-7 px-2 text-xs">
                <Plus className="mr-1 size-3.5" />
                {t('tasks.destinations.add')}
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {addable.map(option => (
                <DropdownMenuItem key={option.id} onSelect={() => void onChange?.([...selected, option.id])}>
                  <Webhook className="mr-2 size-4 text-muted-foreground" />
                  {option.name}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        ) : null}
      </div>
      <ul className="space-y-1 text-xs">
        <li className="flex items-center gap-2 text-muted-foreground">
          <Bell className="size-3.5 shrink-0" />
          {requesterView ? t('tasks.destinations.yours') : t('tasks.destinations.requester')}
        </li>
        {selected.map(id => {
          const name = nameOf(id) ?? t('tasks.destinations.unavailable');
          return (
            <li key={id} className="flex items-center gap-2">
              <Webhook className="size-3.5 shrink-0 text-muted-foreground" />
              <span className="min-w-0 flex-1 truncate">{name}</span>
              {access ? (
                <button
                  type="button"
                  className="rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground"
                  aria-label={t('tasks.destinations.remove', { name })}
                  onClick={() => void onChange?.(selected.filter(item => item !== id))}
                >
                  <X className="size-3.5" />
                </button>
              ) : null}
            </li>
          );
        })}
      </ul>
      {!shared ? (
        <p className="text-xs text-muted-foreground">{t('tasks.destinations.shareForTeam')}</p>
      ) : null}
      {access === 'manage' && choices.isSuccess && options.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('tasks.destinations.none')}</p>
      ) : null}
    </div>
  );
}
