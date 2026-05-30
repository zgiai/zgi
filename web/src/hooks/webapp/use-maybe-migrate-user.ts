import { useEffect, useRef } from 'react';
import { useMutation } from '@tanstack/react-query';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { WebAppService } from '@/services/webapp.service';
import { getWebAppToken, WEBAPP_TOKEN_KEY } from '@/lib/http';
import { sessionManager } from '@/lib/auth/session-manager';
import { useAuthStore } from '@/store/auth-store';

export const WEBAPP_USER_MIGRATED_EVENT = 'zgi:webapp-user-migrated';

/**
 * Auto-migrate guest (webapp) conversations into the logged-in account.
 * Triggers once on client when both main-site tokens and local webapp token exist.
 * - Sends Authorization via main-site http client
 * - Sends X-User-Account-Id header with the local webapp token
 * - On success, removes local webapp token to prevent re-migration
 */
export function useMaybeMigrateUser(): void {
  const hasRunRef = useRef<boolean>(false);
  const t = useT('agents');
  const isAuthenticated = useAuthStore.use.isAuthenticated();
  const isAuthLoading = useAuthStore.use.isLoading();
  const isAuthInitialized = useAuthStore.use.isInitialized();

  const migrateMutation = useMutation({
    mutationFn: async (localToken: string) => {
      return WebAppService.migrateUser(localToken);
    },
    onSuccess: () => {
      try {
        if (typeof window !== 'undefined') {
          window.localStorage.removeItem(WEBAPP_TOKEN_KEY);
        }
      } catch {
        // ignore removal errors
      }
      window.dispatchEvent(new CustomEvent(WEBAPP_USER_MIGRATED_EVENT));
      toast.success(t('workflow.webappMigrateSuccess'));
    },
    onError: (err: unknown) => {
      const message = (err as { message?: string })?.message || t('workflow.webappMigrateFailed');
      toast.error(message);
    },
  });

  useEffect(() => {
    if (typeof window === 'undefined') return;
    if (hasRunRef.current) return;
    if (!isAuthInitialized || isAuthLoading || !isAuthenticated) return;

    const localToken = getWebAppToken();
    const shouldMigrate = Boolean(localToken && sessionManager.hasSession());
    if (!shouldMigrate) return;

    hasRunRef.current = true;
    migrateMutation.mutate(localToken as string);
  }, [isAuthInitialized, isAuthLoading, isAuthenticated, migrateMutation]);
}
