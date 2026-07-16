'use client';

import { useEffect, useMemo } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import { useAuthStore } from '@/store/auth-store';
import { AlertCircle } from 'lucide-react';
import { ZgiLoadingScreen } from '@/components/brand/zgi-loading-screen';
import { useT } from '@/i18n';
import { consumePendingLogoutRedirect } from '@/utils/logout-redirect';

interface ProtectedRouteProps {
  children: React.ReactNode;
  requireAdmin?: boolean;
  fallback?: React.ReactNode;
}

/**
 * Protected route component that handles authentication and authorization
 * Redirects to login if not authenticated or shows fallback if provided
 */
export function ProtectedRoute({ children, requireAdmin = false, fallback }: ProtectedRouteProps) {
  const router = useRouter();
  const pathname = usePathname();
  const t = useT();

  const isAuthenticated = useAuthStore.use.isAuthenticated();
  const isLoading = useAuthStore.use.isLoading();
  const user = useAuthStore.use.user();
  const isSystemReady = useAuthStore.use.isSystemReady();
  // Check admin requirements
  const isAdmin = useMemo(() => {
    if (!user) return false;
    // Check organization role
    const hasOrgAdminRole = ['owner', 'admin'].includes(user.organization_role ?? '');
    // Check global account role
    const hasGlobalAdminRole =
      user.account_role?.role_type === 'super_admin' ||
      user.account_role?.role_type === 'system_admin';

    return hasOrgAdminRole || hasGlobalAdminRole;
  }, [user]);

  useEffect(() => {
    // Wait for system to be ready before making decisions
    if (!isSystemReady || isLoading) {
      return;
    }

    // Redirect to login if not authenticated
    if (!isAuthenticated) {
      const pendingLogoutRedirect = consumePendingLogoutRedirect();
      if (pendingLogoutRedirect) {
        window.location.replace(pendingLogoutRedirect);
        return;
      }

      const currentSearch = window.location.search;
      const currentUrl = currentSearch ? `${pathname}${currentSearch}` : pathname;
      const loginUrl = `/login?redirect=${encodeURIComponent(currentUrl)}`;
      router.push(loginUrl);
      return;
    }

    // Check admin requirements (temporarily disable admin check due to user type change)
    // if (requireAdmin && !isAdmin) {
    //   router.push('/console'); // Redirect to dashboard if admin required but not admin
    //   return;
    // }
  }, [isAuthenticated, isLoading, isSystemReady, requireAdmin, isAdmin, router, pathname]);

  const loadingState = <ZgiLoadingScreen phase="auth" />;

  // Never show authorization fallback before auth bootstrap completes.
  if (!isSystemReady || isLoading) {
    return loadingState;
  }

  // Keep showing loading while redirecting unauthenticated users.
  if (!isAuthenticated) {
    return loadingState;
  }

  // Only use the permission fallback after auth has been fully resolved.
  if (requireAdmin && !isAdmin) {
    return (
      fallback || (
        <div className="flex min-h-screen items-center justify-center">
          <div className="flex flex-col items-center space-y-4 text-center">
            <AlertCircle className="h-12 w-12 text-destructive" />
            <div>
              <h2 className="text-lg font-semibold">{t('common.accessDenied')}</h2>
              <p className="text-sm text-muted-foreground">{t('common.unauthorizedDescription')}</p>
            </div>
          </div>
        </div>
      )
    );
  }

  // Render children if all checks pass
  return <>{children}</>;
}
