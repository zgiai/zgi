import React from 'react';
import { useT } from '@/i18n';
import type { WorkflowNodeData } from '../../store';
import { normalizeNotificationSMSNodeData } from './config';

type NotificationSMSData = Extract<WorkflowNodeData, { type: 'notification-sms' }>;

interface NotificationSMSContentProps {
  data: NotificationSMSData;
}

const NotificationSMSContent: React.FC<NotificationSMSContentProps> = ({ data }) => {
  const t = useT('nodes');
  const normalized = normalizeNotificationSMSNodeData(data);
  const phone = normalized.phone.trim() || t('notificationSms.preview.notConfigured');
  const title =
    normalized.notification_title.trim() || t('notificationSms.preview.notConfigured');

  return (
    <div className="space-y-2 text-xs text-muted-foreground">
      <div className="flex items-center gap-2">
        <span className="rounded-full border border-border bg-muted px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.08em] text-foreground">
          {t('notificationSms.preview.phone')}
        </span>
        <span className="truncate" title={phone}>
          {phone}
        </span>
      </div>
      <div className="flex items-center gap-2">
        <span className="rounded-full border border-border bg-muted px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.08em] text-foreground">
          {t('notificationSms.preview.title')}
        </span>
        <span className="truncate" title={title}>
          {title}
        </span>
      </div>
    </div>
  );
};

export default NotificationSMSContent;
