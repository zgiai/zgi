'use client';

import { useEffect } from 'react';
import { queryClient } from '@/lib/query-client';
import { useAuthStore } from '@/store/auth-store';
import { clearSessionBoundClientState } from '@/lib/auth/client-state';
import { sessionManager, type AuthSyncEvent } from '@/lib/auth/session-manager';
import { PROFILE_KEYS } from '@/hooks/query-keys';
import { clearProfileClientCache } from '@/utils/client-cache';

interface AuthProviderProps {
  children: React.ReactNode;
}

async function handleCrossTabEvent(event: AuthSyncEvent): Promise<void> {
  switch (event.type) {
    case 'SIGNED_IN': {
      await clearSessionBoundClientState();
      await useAuthStore.getState().initializeAuth({ force: true });
      return;
    }
    case 'SIGNED_OUT': {
      if (event.sourceTabId === sessionManager.getCurrentTabId()) {
        return;
      }
      await clearSessionBoundClientState();
      useAuthStore.getState().reset({ clearSession: false });
      await useAuthStore.getState().initializeAuth({ force: true });
      return;
    }
    case 'TOKEN_REFRESHED': {
      useAuthStore.getState().clearError();
      useAuthStore.getState().setNetworkError(false);
      return;
    }
    case 'PROFILE_UPDATED': {
      clearProfileClientCache();
      await queryClient.invalidateQueries({ queryKey: PROFILE_KEYS.current() });
      if (useAuthStore.getState().isAuthenticated) {
        try {
          await useAuthStore.getState().refreshProfile({ refresh: true });
        } catch {
          // Ignore refresh failures here and let the global auth flow decide next steps.
        }
      }
      return;
    }
    case 'CONTEXT_CHANGED': {
      clearProfileClientCache();
      queryClient.clear();
      await useAuthStore.getState().initializeAuth({ force: true });
      return;
    }
    default:
      return;
  }
}

/**
 * Authentication provider that initializes auth state on app startup
 * and keeps session state synchronized across browser tabs.
 */
export function AuthProvider({ children }: AuthProviderProps) {
  const initializeAuth = useAuthStore.use.initializeAuth();

  useEffect(() => {
    void initializeAuth();
  }, [initializeAuth]);

  useEffect(() => {
    if (typeof window === 'undefined') {
      return;
    }

    return sessionManager.subscribeToCrossTabEvents(event => {
      void handleCrossTabEvent(event);
    });
  }, []);

  return <>{children}</>;
}
