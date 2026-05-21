'use client';

import { MessageSquareText } from 'lucide-react';
import { useT } from '@/i18n';

interface NotificationSMSPreviewProps {
  notificationTitle: string;
  linkSuffix: string;
  previewTemplate?: string;
}

export function NotificationSMSPreview({
  notificationTitle,
  linkSuffix,
  previewTemplate,
}: NotificationSMSPreviewProps) {
  const t = useT('common');
  const title = notificationTitle.trim() || t('notificationSms.previewTitlePlaceholder' as never);
  const suffix = linkSuffix.trim() || t('notificationSms.previewCodePlaceholder' as never);
  const previewBody = renderPreviewTemplate(previewTemplate, title, suffix);

  return (
    <div className="rounded-xl border border-border/70 bg-background p-3">
      <div className="flex items-start gap-2">
        <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
          <MessageSquareText className="size-3.5" />
        </div>
        <div className="min-w-0 space-y-1">
          <p className="text-[11px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
            {t('notificationSms.preview' as never)}
          </p>
          <p className="break-words text-xs leading-5 text-foreground">
            {previewBody ??
              t('notificationSms.previewBody' as never, { title, code: suffix } as never)}
          </p>
          <p className="text-[10px] leading-4.5 text-muted-foreground">
            {t('notificationSms.previewHint' as never)}
          </p>
        </div>
      </div>
    </div>
  );
}

function renderPreviewTemplate(
  previewTemplate: string | undefined,
  notificationTitle: string,
  linkSuffix: string
): string | undefined {
  const template = previewTemplate?.trim();
  if (!template) {
    return undefined;
  }

  return template
    .replace(/\{\{\s*notification_title\s*\}\}/g, notificationTitle)
    .replace(/\{\{\s*link_suffix\s*\}\}/g, linkSuffix)
    .replace(/\{\{\s*link_code\s*\}\}/g, linkSuffix);
}
