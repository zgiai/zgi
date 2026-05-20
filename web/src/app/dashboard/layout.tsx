'use client';

import type { ReactNode } from 'react';
import { ProtectedRoute } from '@/components/auth/protected-route';
import { DashboardAccessDenied } from '@/components/dashboard/dashboard-access-denied';
import { customerAdapter } from '@/customer';

export default function DashboardLayout({ children }: { children: ReactNode }) {
  const DashboardShell = customerAdapter.DashboardShell;

  return (
    <ProtectedRoute requireAdmin fallback={<DashboardAccessDenied />}>
      <DashboardShell>{children}</DashboardShell>
    </ProtectedRoute>
  );
}
