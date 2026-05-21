'use client';

import React from 'react';
import { Loader2 } from 'lucide-react';

import MarkdownViewer from '@/components/common/markdown-viewer';
import { Button } from '@/components/ui/button';
import { useAnnouncement } from '@/hooks/workflow/use-announcement';
import { useT } from '@/i18n';

interface AnnouncementPageClientProps {
  token: string;
}

export function AnnouncementPageClient({ token }: AnnouncementPageClientProps) {
  const t = useT('nodes');
  const announcementQuery = useAnnouncement(token);
  const announcement = announcementQuery.data;

  return (
    <main className="min-h-screen bg-background px-4 py-8 text-foreground">
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
        {announcementQuery.isLoading ? (
          <div className="flex min-h-[320px] items-center justify-center rounded-lg border">
            <Loader2 className="size-5 animate-spin text-muted-foreground" />
          </div>
        ) : announcementQuery.error || !announcement || announcement.expired ? (
          <div className="rounded-lg border p-6 text-center">
            <h1 className="text-lg font-semibold">{t('announcement.runtime.unavailable')}</h1>
            <p className="mt-2 text-sm text-muted-foreground">
              {announcementQuery.error instanceof Error
                ? announcementQuery.error.message
                : t('announcement.runtime.unavailableDescription')}
            </p>
            <Button className="mt-4" onClick={() => announcementQuery.refetch()}>
              {t('announcement.runtime.retry')}
            </Button>
          </div>
        ) : (
          <div className="rounded-lg border bg-card p-5 shadow-sm">
            <div className="space-y-1">
              <h1 className="text-lg font-semibold">{announcement.node_title}</h1>
              {announcement.expiration_at ? (
                <p className="text-xs text-muted-foreground">
                  {t('announcement.runtime.expiresAt', {
                    time: new Date(announcement.expiration_at * 1000).toLocaleString(),
                  })}
                </p>
              ) : null}
            </div>
            <div className="mt-5 rounded-lg border bg-background p-3">
              <MarkdownViewer
                content={announcement.content || ''}
                className="md-viewer break-words"
              />
            </div>
          </div>
        )}
      </div>
    </main>
  );
}
