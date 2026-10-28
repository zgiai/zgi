import type { ReactNode } from 'react';
import { AccountShell } from '@/components/account/account-shell';
import { ProtectedRoute } from '@/components/auth/protected-route';
import { Providers } from '@/providers';

export default function ProfileLayout({ children }: { children: ReactNode }) {
  return (
    <Providers>
      <ProtectedRoute>
        <AccountShell>{children}</AccountShell>
      </ProtectedRoute>
    </Providers>
  );
}
