'use client';

import type { ReactNode } from 'react';
import { ProtectedRoute } from '@/components/auth/protected-route';
import { Providers } from '@/providers';

export default function OnboardingLayout({ children }: { children: ReactNode }) {
  return (
    <Providers>
      <ProtectedRoute>{children}</ProtectedRoute>
    </Providers>
  );
}
