'use client';

import { MessageSquareText } from 'lucide-react';
import { useT } from '@/i18n';

interface NotificationSMSPreviewProps {
  notificationTitle: string;
  linkCode: string;
  previewTemplate?: string;
}

export function NotificationSMSPreview({
  notificationTitle,
  linkCode,
  previewTemplate,
}: NotificationSMSPreviewProps) {
  const t = useT('common');
  const title = notificationTitle.trim() || t('notificationSms.previewTitlePlaceholder' as never);
  const code = linkCode.trim() || t('notificationSms.previewCodePlaceholder' as never);
  const previewBody = renderPreviewTemplate(previewTemplate, title, code);

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
            {previewBody ?? t('notificationSms.previewBody' as never, { title, code } as never)}
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
  linkCode: string
): string | undefined {
  const template = previewTemplate?.trim();
  if (!template) {
    return undefined;
  }

  return template
    .replace(/\{\{\s*notification_title\s*\}\}/g, notificationTitle)
    .replace(/\{\{\s*link_code\s*\}\}/g, linkCode);
}
