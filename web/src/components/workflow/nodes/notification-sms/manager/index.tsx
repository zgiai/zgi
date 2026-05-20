'use client';

import React from 'react';
import { useLocalNodeData } from '../../../hooks';
import { NotificationSMSEditor } from '@/components/notification-sms/notification-sms-editor';
import type { NotificationSMSDraft } from '@/components/notification-sms/types';

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
  const { localData: notificationTitle, setLocalData: setNotificationTitle } =
    useLocalNodeData<string>(id, {
      path: 'notification_title',
      delay: 300,
    });
  const { localData: linkCode, setLocalData: setLinkCode } = useLocalNodeData<string>(id, {
    path: 'link_code',
    delay: 300,
  });

  const value = React.useMemo<NotificationSMSDraft>(
    () => ({
      recipients: [phone || ''],
      notificationTitle: notificationTitle || '',
      linkCode: linkCode || '',
    }),
    [linkCode, notificationTitle, phone]
  );

  const handleChange = React.useCallback(
    (next: NotificationSMSDraft) => {
      setPhone(next.recipients[0] ?? '');
      setNotificationTitle(next.notificationTitle);
      setLinkCode(next.linkCode);
    },
    [setLinkCode, setNotificationTitle, setPhone]
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
