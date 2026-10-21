'use client';

import { useState } from 'react';
import { useRouter } from 'next/navigation';
import { AlertTriangle, Bell, CheckCheck, FileText, Settings } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
import { Skeleton } from '@/components/ui/skeleton';
import { useInboxList, useInboxMutations, useInboxUnreadCount } from '@/hooks/use-inbox';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { InboxNotification } from '@/services/inbox.service';

// The in-app inbox (ADR 0008): reports and failures of the reader's tasks.
export function InboxBell() {
  const t = useT('common');
  const { locale } = useLocale();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const unread = useInboxUnreadCount();
  const list = useInboxList({}, open);
  const { markRead, markAllRead } = useInboxMutations();
  const count = unread.data?.data.unread_count ?? 0;
  const items = list.data?.data.items ?? [];

  const openNotification = (notification: InboxNotification) => {
    if (!notification.read_at) markRead.mutate({ ids: [notification.id] });
    setOpen(false);
    if (notification.link.startsWith('/')) router.push(notification.link);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant="ghost" size="sm" className="relative size-9 px-0" aria-label={t('inbox.title')}>
          <Bell className="size-5" />
          {count > 0 ? (
            <span className="absolute -right-0.5 -top-0.5 flex min-w-4 items-center justify-center rounded-full bg-destructive px-1 text-[10px] font-semibold leading-4 text-white">
              {count > 99 ? '99+' : count}
            </span>
          ) : null}
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-[22rem] p-0">
        <div className="flex items-center justify-between border-b px-4 py-3">
          <span className="text-sm font-semibold">{t('inbox.title')}</span>
          <Button variant="ghost" size="sm" disabled={count === 0 || markAllRead.isPending} onClick={() => markAllRead.mutate()}>
            <CheckCheck className="size-4" />
            {t('inbox.markAllRead')}
          </Button>
        </div>
        <div className="max-h-[26rem] overflow-y-auto">
          {list.isLoading ? (
            <div className="space-y-2 p-4">
              <Skeleton className="h-12 w-full" />
              <Skeleton className="h-12 w-full" />
            </div>
          ) : items.length === 0 ? (
            <div className="px-4 py-10 text-center">
              <Bell className="mx-auto size-6 text-muted-foreground/50" />
              <p className="mt-2 text-sm text-muted-foreground">{t('inbox.empty')}</p>
            </div>
          ) : (
            <ul className="divide-y">
              {items.map(notification => {
                const failed = notification.event === 'digital_human.task_failed';
                return (
                  <li key={notification.id}>
                    <button
                      type="button"
                      className={cn('flex w-full gap-3 px-4 py-3 text-left hover:bg-muted/50', !notification.read_at && 'bg-primary/[0.03]')}
                      onClick={() => openNotification(notification)}
                    >
                      {failed ? (
                        <AlertTriangle className="mt-0.5 size-4 shrink-0 text-destructive" />
                      ) : (
                        <FileText className="mt-0.5 size-4 shrink-0 text-primary" />
                      )}
                      <span className="min-w-0 flex-1">
                        <span className="flex items-center gap-2 text-xs text-muted-foreground">
                          <span>{t(failed ? 'inbox.event.taskFailed' : 'inbox.event.taskReport')}</span>
                          <span className="ml-auto shrink-0">{new Date(notification.created_at).toLocaleString(locale)}</span>
                          {!notification.read_at ? <span className="size-1.5 shrink-0 rounded-full bg-primary" /> : null}
                        </span>
                        <span className="mt-0.5 block truncate text-sm font-medium">{notification.title}</span>
                        {notification.summary ? (
                          <span className="mt-0.5 line-clamp-2 block text-xs text-muted-foreground">{notification.summary}</span>
                        ) : null}
                      </span>
                    </button>
                  </li>
                );
              })}
            </ul>
          )}
        </div>
        <div className="border-t px-4 py-2 text-right">
          <Button variant="ghost" size="sm" onClick={() => { setOpen(false); router.push('/profile/notifications'); }}>
            <Settings className="size-4" />
            {t('notifications.settings')}
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}
