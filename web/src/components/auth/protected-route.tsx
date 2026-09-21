'use client';

import { useEffect, useMemo } from 'react';
import { useRouter, usePathname } from 'next/navigation';
import { useAuthStore } from '@/store/auth-store';
import { AlertCircle, RefreshCw } from 'lucide-react';
import { ZgiLoadingScreen } from '@/components/brand/zgi-loading-screen';
import { useT } from '@/i18n';
import { consumePendingLogoutRedirect } from '@/utils/logout-redirect';
import { isAuthRedirectInProgress, isLogoutInProgress } from '@/lib/auth/logout-state';

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
  const networkError = useAuthStore.use.networkError();
  const initializeAuth = useAuthStore.use.initializeAuth();
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
      if (isAuthRedirectInProgress() || isLogoutInProgress()) return;

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

  const retryAuth = () => {
    void initializeAuth({ force: true });
  };

  // A transient auth/network failure must not leave a protected route on an
  // infinite splash screen. We still keep the route gated (no children are
  // rendered until the session is verified), but give the user a recoverable
  // state and a retry action.
  if (!isSystemReady && !isLoading && networkError) {
    return (
      <main className="flex min-h-svh items-center justify-center bg-background px-6">
        <div
          className="flex w-full max-w-md flex-col items-center gap-4 rounded-xl border bg-card p-8 text-center shadow-sm"
          role="alert"
        >
          <div className="flex size-11 items-center justify-center rounded-full bg-destructive/10 text-destructive">
            <AlertCircle className="size-5" aria-hidden="true" />
          </div>
          <div className="space-y-1">
            <h2 className="text-base font-semibold">{t('common.networkError')}</h2>
            <p className="text-sm text-muted-foreground">
              {t('common.requestErrors.timeout')}
            </p>
          </div>
          <button
            type="button"
            onClick={retryAuth}
            className="inline-flex h-9 items-center gap-2 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground shadow-sm transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50"
          >
            <RefreshCw className="size-4" aria-hidden="true" />
            {t('common.errorPages.error.retry')}
          </button>
        </div>
      </main>
    );
  }

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
