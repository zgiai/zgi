'use client';

import { ProtectedRoute } from '@/components/auth/protected-route';
import type { ReactNode } from 'react';
import { customerAdapter } from '@/customer';

/**
 * Console layout with authentication protection
 * Automatically redirects to login page if user is not authenticated
 */
export default function ConsoleLayout({ children }: { children: ReactNode }) {
  const ConsoleShell = customerAdapter.ConsoleShell;

  return (
    <ProtectedRoute>
      <ConsoleShell>{children}</ConsoleShell>
    </ProtectedRoute>
  );
}
