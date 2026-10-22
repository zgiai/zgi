'use client';

import { ArrowLeft, Bell } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { ProtectedRoute } from '@/components/auth/protected-route';
import { NotificationChannelManager } from '@/components/notifications/notification-channel-manager';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';

export default function ProfileNotificationsPage() {
  const t = useT('common');
  const router = useRouter();
  return (
    <ProtectedRoute>
      <div className="min-h-screen bg-muted/30">
        <div className="mx-auto max-w-3xl px-4 py-8 sm:px-6 lg:px-8">
          <Button variant="ghost" size="sm" className="mb-3 -ml-2" onClick={() => router.back()}>
            <ArrowLeft className="mr-1 size-4" />
            {t('notifications.cancel')}
          </Button>
          <div className="mb-6 flex items-start gap-3">
            <Bell className="mt-1 size-5 text-primary" />
            <div>
              <h1 className="text-2xl font-semibold tracking-tight">{t('notifications.title')}</h1>
              <p className="mt-1 text-sm text-muted-foreground">{t('notifications.description')}</p>
            </div>
          </div>
          <section className="rounded-xl border bg-background p-5">
            <h2 className="mb-3 text-sm font-semibold">{t('notifications.channelsTitle')}</h2>
            <NotificationChannelManager basePath="/notification-channels" types={['email', 'webhook']} />
          </section>
        </div>
      </div>
    </ProtectedRoute>
  );
}
