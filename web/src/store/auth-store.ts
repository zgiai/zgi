// Authentication state management for ZGI platform
// Optimized with Zustand for auth bootstrap and session synchronization

import { create } from 'zustand';
import { createSelectors } from './utils/selectors';
import { AuthenticationService } from '@/services/auth.service';
import type { SystemFeatures, SetupStatus, User } from '@/services/types/auth';
import { toast } from 'sonner';
import { deleteCookie } from '@/utils/cookie';
import { clearAuthClientCaches } from '@/utils/client-cache';
import { ENABLE_ROOT_COOKIE_TOKEN_SYNC, IS_CLOUD, ROOT_COOKIE_DOMAIN } from '@/lib/config';
import { sessionManager } from '@/lib/auth/session-manager';
import { getCurrentLocale } from '@/lib/i18n';
import { syncAccountContextStores } from '@/lib/auth/context-sync';

export type AuthSessionStatus = 'booting' | 'guest' | 'syncing' | 'authenticated';

interface AuthState {
  user: User | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  isInitialized: boolean;
  sessionStatus: AuthSessionStatus;
  isLoggingOut: boolean;

  systemFeatures: SystemFeatures | null;
  setupStatus: SetupStatus | null;
  isSystemReady: boolean;

  error: string | null;
  networkError: boolean;

  setUser: (user: User | null) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  setSystemFeatures: (features: SystemFeatures) => void;
  setSetupStatus: (status: SetupStatus) => void;
  setSessionStatus: (status: AuthSessionStatus) => void;
  setLoggingOut: (isLoggingOut: boolean) => void;

  refreshProfile: (options?: { refresh?: boolean }) => Promise<User | null>;
  initializeAuth: (options?: { force?: boolean }) => Promise<void>;
  clearError: () => void;
  setNetworkError: (hasError: boolean) => void;
  refreshSystemFeatures: (useCache?: boolean) => Promise<void>;

  reset: (options?: { clearSession?: boolean }) => void;
}

const defaultState = {
  user: null,
  isAuthenticated: false,
  isLoading: false,
  isInitialized: false,
  sessionStatus: 'booting' as AuthSessionStatus,
  systemFeatures: null,
  setupStatus: null,
  isSystemReady: false,
  isLoggingOut: false,
  error: null,
  networkError: false,
};

const authService = new AuthenticationService();

function isSuperAdminUser(user: User | null | undefined): boolean {
  if (IS_CLOUD) {
    return user?.account_role?.role_type === 'super_admin';
  }

  return Boolean(user?.is_super_admin || user?.account_role?.role_type === 'super_admin');
}

function isNetworkError(error: unknown): boolean {
  const networkErr = error as { code?: string; message?: string; response?: unknown };
  const message = networkErr?.message?.toLowerCase() || '';
  return (
    !networkErr?.response &&
    (networkErr?.code === 'NETWORK_ERROR' ||
      networkErr?.code === 'ERR_NETWORK' ||
      networkErr?.code === 'ECONNABORTED' ||
      networkErr?.code === 'ETIMEDOUT' ||
      message === 'network error' ||
      message.includes('timeout'))
  );
}

function isAuthError(error: unknown): boolean {
  return (
    (error as { response?: { status?: number } })?.response?.status === 401 ||
    (error as { businessError?: { code?: string } })?.businessError?.code === '201020'
  );
}

function getSessionExpiredMessage(): string {
  return getCurrentLocale() === 'zh-Hans'
    ? '登录态失效，请重新登录'
    : 'Your session expired. Please sign in again.';
}

const useAuthStoreBase = create<AuthState>()((set, get) => ({
  ...defaultState,

  setUser: user => {
    syncAccountContextStores(user);
    set({
      user,
      isAuthenticated: !!user,
      sessionStatus: user ? 'authenticated' : 'guest',
      error: null,
    });
  },

  setLoading: isLoading => set({ isLoading }),

  setError: error => set({ error }),

  setSystemFeatures: systemFeatures =>
    set({
      systemFeatures,
      isSystemReady: true,
    }),

  setSetupStatus: setupStatus => set({ setupStatus }),

  setSessionStatus: sessionStatus =>
    set(state => ({
      sessionStatus,
      isAuthenticated: sessionStatus === 'authenticated' && !!state.user,
    })),

  setLoggingOut: isLoggingOut => set({ isLoggingOut }),

  refreshProfile: async (options?: { refresh?: boolean }) => {
    const token = authService.getToken();
    if (!token) {
      set({
        user: null,
        isAuthenticated: false,
        sessionStatus: 'guest',
        error: null,
        networkError: false,
      });
      return null;
    }

    try {
      set({
        networkError: false,
        error: null,
        sessionStatus: 'syncing',
      });

      const user = await authService.getProfile(!(options?.refresh ?? false));
      const currentToken = authService.getToken();
      if (get().isLoggingOut || !currentToken || currentToken !== token) {
        return null;
      }

      syncAccountContextStores(user);
      set({
        user,
        isAuthenticated: true,
        sessionStatus: 'authenticated',
        isSystemReady: true,
        error: null,
        networkError: false,
      });
      return user;
    } catch (error) {
      console.error('Profile refresh failed:', error);

      if (isNetworkError(error)) {
        set(state => ({
          networkError: true,
          error: 'Network connection failed',
          sessionStatus: state.user ? 'authenticated' : 'syncing',
        }));
        throw error;
      }

      if (isAuthError(error)) {
        get().reset();
        toast.error(getSessionExpiredMessage());
        throw error;
      }

      set({
        error: (error as Error).message,
      });
      throw error;
    }
  },

  initializeAuth: async (options?: { force?: boolean }) => {
    if (get().isLoading && !options?.force) {
      return;
    }

    if (get().isInitialized && !options?.force) {
      return;
    }

    set({
      isLoading: true,
      isInitialized: true,
      error: null,
      networkError: false,
    });

    try {
      const token = authService.getToken();

      if (!token) {
        set({
          user: null,
          isAuthenticated: false,
          sessionStatus: 'guest',
          isLoading: false,
          isSystemReady: true,
        });

        await get().refreshSystemFeatures();
        return;
      }

      set({
        sessionStatus: 'syncing',
        isAuthenticated: false,
      });

      await get().refreshProfile({ refresh: true });
      await get().refreshSystemFeatures();
    } catch (error) {
      console.error('Auth initialization failed:', error);

      const networkError = isNetworkError(error);

      set(state => ({
        user: networkError ? state.user : null,
        isAuthenticated: networkError ? state.isAuthenticated : false,
        sessionStatus: networkError
          ? state.user
            ? 'authenticated'
            : 'syncing'
          : sessionManager.hasSession()
            ? state.sessionStatus
            : 'guest',
        isSystemReady: !networkError,
        error: networkError ? 'Network connection failed' : (error as Error).message,
        networkError,
      }));
    } finally {
      set({ isLoading: false });
    }
  },

  reset: (options?: { clearSession?: boolean }) => {
    if (options?.clearSession !== false) {
      sessionManager.clearSession({ type: 'SIGNED_OUT' });
    }

    if (typeof window !== 'undefined') {
      deleteCookie('idToken');
      if (ENABLE_ROOT_COOKIE_TOKEN_SYNC) {
        deleteCookie('idToken', { domain: ROOT_COOKIE_DOMAIN });
      }
      clearAuthClientCaches();
    }

    set({
      ...defaultState,
      isInitialized: true,
      isSystemReady: true,
      sessionStatus: 'guest',
    });
  },

  clearError: () => set({ error: null }),

  setNetworkError: (hasError: boolean) => set({ networkError: hasError }),

  refreshSystemFeatures: async (useCache: boolean = true) => {
    try {
      const response = await authService.getSystemFeatures(useCache);
      if (response && 'data' in response && response.data && 'features' in response.data) {
        get().setSystemFeatures(response.data.features as SystemFeatures);
      }
    } catch (error) {
      console.error('Fetching system features failed:', error);
    }
  },
}));

export const useAuthStore = createSelectors(useAuthStoreBase);

export const useCurrentUser = () => useAuthStore.use.user();
export const useIsAuthenticated = () => useAuthStore.use.isAuthenticated();
export const useAuthLoading = () => useAuthStore.use.isLoading();
export const useAuthError = () => useAuthStore.use.error();
export const useIsInitialized = () => useAuthStore.use.isInitialized();
export const useIsLoggingOut = () => useAuthStore.use.isLoggingOut();
export const useNetworkError = () => useAuthStore.use.networkError();
export const useSessionStatus = () => useAuthStore.use.sessionStatus();
export const useIsSuperAdmin = () => useAuthStore(state => isSuperAdminUser(state.user));

export const authSelectors = {
  isSuperAdmin: (state: AuthState) => isSuperAdminUser(state.user),

  isAdmin: (state: AuthState) =>
    isSuperAdminUser(state.user) ||
    state.user?.account_role?.role_type === 'super_admin' ||
    state.user?.account_role?.role_type === 'system_admin',

  displayName: (state: AuthState) => state.user?.name || state.user?.email || 'User',

  userEmail: (state: AuthState) => state.user?.email || '',

  hasPermission: (permission: string) => {
    return (state: AuthState) => {
      const permissions =
        (
          state.user as
            | (User & {
                current_role?: { permissions?: string[] };
              })
            | null
        )?.current_role?.permissions ?? [];
      return permissions.includes(permission);
    };
  },

  hasAnyPermission: (permissions: string[]) => {
    return (state: AuthState) => {
      const userPermissions =
        (
          state.user as
            | (User & {
                current_role?: { permissions?: string[] };
              })
            | null
        )?.current_role?.permissions ?? [];
      return permissions.some(permission => userPermissions.includes(permission));
    };
  },

  hasAllPermissions: (permissions: string[]) => {
    return (state: AuthState) => {
      const userPermissions =
        (
          state.user as
            | (User & {
                current_role?: { permissions?: string[] };
              })
            | null
        )?.current_role?.permissions ?? [];
      return permissions.every(permission => userPermissions.includes(permission));
    };
  },
};
