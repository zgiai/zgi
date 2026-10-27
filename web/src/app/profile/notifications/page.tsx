'use client';

import { AccountPageHeader } from '@/components/account/account-shell';
import { NotificationChannelManager } from '@/components/notifications/notification-channel-manager';
import { QuietHoursCard } from '@/components/notifications/quiet-hours-card';
import { useT } from '@/i18n';

export default function ProfileNotificationsPage() {
  const t = useT('common');
  return (
    <div className="max-w-3xl">
      <AccountPageHeader
        title={t('notifications.title')}
        description={t('notifications.description')}
      />
      <section className="rounded-xl border bg-background p-5">
        <h2 className="mb-3 text-sm font-semibold">{t('notifications.channelsTitle')}</h2>
        <NotificationChannelManager
          basePath="/notification-channels"
          types={['email', 'im', 'webhook']}
        />
      </section>
      <section className="mt-4 rounded-xl border bg-background p-5">
        <h2 className="mb-3 text-sm font-semibold">{t('notifications.quietTitle')}</h2>
        <QuietHoursCard />
      </section>
    </div>
  );
}
