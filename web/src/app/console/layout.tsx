'use client';

import { ProtectedRoute } from '@/components/auth/protected-route';
import type { ReactNode } from 'react';
import { customerAdapter } from '@/customer';
import { Providers } from '@/providers';

/**
 * Console layout with authentication protection
 * Automatically redirects to login page if user is not authenticated
 */
export default function ConsoleLayout({ children }: { children: ReactNode }) {
  const ConsoleShell = customerAdapter.ConsoleShell;

  return (
    <Providers>
      <ProtectedRoute>
        <ConsoleShell>{children}</ConsoleShell>
      </ProtectedRoute>
    </Providers>
  );
}
