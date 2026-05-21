'use client';

import React from 'react';
import { useLocalNodeData } from '../../../hooks';
import { NotificationSMSEditor } from '@/components/notification-sms/notification-sms-editor';
import type { NotificationSMSDraft } from '@/components/notification-sms/types';
import { NOTIFICATION_SMS_TEMPLATE } from '@/lib/features/notification-sms';

interface NotificationSMSManagerProps {
  id: string;
  className?: string;
  readOnly?: boolean;
}

const NotificationSMSManager: React.FC<NotificationSMSManagerProps> = ({
  id,
  className,
  readOnly = false,
}) => {
  const { localData: phone, setLocalData: setPhone } = useLocalNodeData<string>(id, {
    path: 'phone',
    delay: 300,
  });
  const { localData: template, setLocalData: setTemplate } = useLocalNodeData<string>(id, {
    path: 'template',
    delay: 300,
  });
  const { localData: templateParams, setLocalData: setTemplateParams } = useLocalNodeData<
    Record<string, string>
  >(id, {
    path: 'template_params',
    delay: 300,
  });

  const value = React.useMemo<NotificationSMSDraft>(
    () => ({
      recipients: [phone || ''],
      template: template || NOTIFICATION_SMS_TEMPLATE,
      templateParams: templateParams ?? {},
    }),
    [phone, template, templateParams]
  );

  const handleChange = React.useCallback(
    (next: NotificationSMSDraft) => {
      setPhone(next.recipients[0] ?? '');
      setTemplate(next.template);
      setTemplateParams(next.templateParams);
    },
    [setPhone, setTemplate, setTemplateParams]
  );

  return (
    <NotificationSMSEditor
      nodeId={id}
      value={value}
      onChange={handleChange}
      readOnly={readOnly}
      recipientMode="single"
      className={className}
    />
  );
};

export default NotificationSMSManager;
