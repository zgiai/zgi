'use client';

import { ProtectedRoute } from '@/components/auth/protected-route';
import { ZgiLoadingScreen } from '@/components/brand/zgi-loading-screen';
import { useAuthStore } from '@/store/auth-store';
import { useRouter } from 'next/navigation';
import { useEffect, type ReactNode } from 'react';
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
        <OrganizationContextRequired>
          <ConsoleShell>{children}</ConsoleShell>
        </OrganizationContextRequired>
      </ProtectedRoute>
    </Providers>
  );
}

function OrganizationContextRequired({ children }: { children: ReactNode }) {
  const router = useRouter();
  const user = useAuthStore.use.user();
  const hasExplicitlyEmptyOrganization = user?.current_organization_id === null;

  useEffect(() => {
    if (hasExplicitlyEmptyOrganization) {
      router.replace('/onboarding/organization');
    }
  }, [hasExplicitlyEmptyOrganization, router]);

  if (hasExplicitlyEmptyOrganization) {
    return <ZgiLoadingScreen phase="auth" />;
  }

  return <>{children}</>;
}
