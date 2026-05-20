'use client';

import Link from 'next/link';
import { ShieldAlert, ArrowLeft } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';

/**
 * @component DashboardAccessDenied
 * @category Feature
 * @status Stable
 * @description Dashboard-specific unauthorized fallback with i18n copy and a return action.
 * @usage Use as the `ProtectedRoute` fallback for dashboard pages that require admin access.
 * @example
 * <DashboardAccessDenied />
 */
export function DashboardAccessDenied() {
  const t = useT();
  const tDashboard = useT('dashboard');

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-6">
      <div className="w-full max-w-lg rounded-2xl border bg-card p-8 text-center shadow-sm">
        <div className="mx-auto mb-5 flex size-14 items-center justify-center rounded-full bg-muted">
          <ShieldAlert className="size-7 text-muted-foreground" />
        </div>
        <h1 className="text-2xl font-semibold text-foreground">{t('common.accessDenied')}</h1>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">
          {t('common.unauthorizedDescription')}
        </p>
        <div className="mt-6 flex justify-center">
          <Button asChild>
            <Link href="/console" className="inline-flex items-center gap-2">
              <ArrowLeft className="size-4" />
              {tDashboard('backToConsole')}
            </Link>
          </Button>
        </div>
      </div>
    </div>
  );
}
