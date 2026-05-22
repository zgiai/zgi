'use client';

import { MessageSquareText } from 'lucide-react';
import { useT } from '@/i18n';
import {
  getNotificationSMSParamDisplayKey,
  type NotificationSMSTemplate,
  type NotificationSMSTemplateParam,
} from '@/lib/features/notification-sms';

interface NotificationSMSPreviewProps {
  template?: NotificationSMSTemplate;
  templateParams: Record<string, string>;
}

export function NotificationSMSPreview({ template, templateParams }: NotificationSMSPreviewProps) {
  const t = useT('common');
  const previewBody = renderPreviewTemplate(
    t,
    template?.preview_template,
    template,
    templateParams
  );

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
            {previewBody ?? t('notificationSms.previewUnavailable' as never)}
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
  t: ReturnType<typeof useT<'common'>>,
  previewTemplate: string | undefined,
  template: NotificationSMSTemplate | undefined,
  templateParams: Record<string, string>
): string | undefined {
  const text = previewTemplate?.trim();
  if (!text) {
    return undefined;
  }

  return text.replace(/\{\{\s*([A-Za-z0-9_]+)\s*\}\}/g, (_, key: string) => {
    const value = templateParams[key]?.trim();
    if (value) {
      return value;
    }
    const param = template?.params?.find(item => item.key === key);
    return param ? getParamLabel(t, param) : key;
  });
}

function getParamLabel(
  t: ReturnType<typeof useT<'common'>>,
  param: NotificationSMSTemplateParam
): string {
  const displayKey = getNotificationSMSParamDisplayKey(param);
  if (displayKey === 'notificationTitle') {
    return t('notificationSms.params.notificationTitle' as never);
  }
  if (displayKey === 'linkCode') {
    return t('notificationSms.params.linkCode' as never);
  }
  if (displayKey === 'remark') {
    return t('notificationSms.params.remark' as never);
  }
  if (displayKey === 'summary') {
    return t('notificationSms.params.summary' as never);
  }
  return param.label?.trim() || param.key;
}
