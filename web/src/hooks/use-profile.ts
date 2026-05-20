'use client';

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { accountService } from '@/services/account.service';
import type { UpdateProfileRequest, User } from '@/services/types/auth';
import { toast } from 'sonner';
import { useAuthStore } from '@/store/auth-store';
import { useT } from '@/i18n';

import { PROFILE_KEYS } from '@/hooks/query-keys';
import { sessionManager } from '@/lib/auth/session-manager';
import { clearProfileClientCache } from '@/utils/client-cache';

const PROFILE_QUERY_KEY = PROFILE_KEYS.current();

// Options for customizing profile query behavior
interface UseProfileOptions {
  staleTime?: number;
  refetchOnWindowFocus?: boolean | 'always';
  refetchOnReconnect?: boolean | 'always';
  enabled?: boolean;
}

// Fetch current user profile with caching
export function useProfile(options?: UseProfileOptions) {
  return useQuery<User>({
    queryKey: PROFILE_QUERY_KEY,
    queryFn: async () => {
      const data = await accountService.getProfile();
      return data;
    },
    staleTime: options?.staleTime ?? 60_000,
    refetchOnWindowFocus: options?.refetchOnWindowFocus ?? false,
    refetchOnReconnect: options?.refetchOnReconnect ?? false,
    enabled: options?.enabled ?? true,
  });
}

interface UseAutoProfileOptions {
  staleTime?: number;
  gcTime?: number;
  enabled?: boolean;
  refetchOnWindowFocus?: boolean | 'always';
  refetchOnReconnect?: boolean | 'always';
}

// Auto-refresh auth profile to keep auth state updated; default cache 30 minutes
export function useAutoProfile(options?: UseAutoProfileOptions) {
  const isAuthenticated = useAuthStore.use.isAuthenticated();
  const isLoggingOut = useAuthStore.use.isLoggingOut();

  return useQuery<User | null>({
    queryKey: PROFILE_QUERY_KEY,
    queryFn: async () => {
      try {
        const requestToken = sessionManager.getAccessToken();
        if (!requestToken) {
          return null;
        }

        const data = await accountService.getProfile();
        const authState = useAuthStore.getState();
        const currentToken = sessionManager.getAccessToken();
        if (authState.isLoggingOut || !currentToken || currentToken !== requestToken) {
          return authState.user;
        }

        useAuthStore.getState().setUser(data);
        return data;
      } catch (error) {
        const status = (error as { response?: { status?: number } })?.response?.status;
        if (status === 401) {
          return useAuthStore.getState().user;
        }
        throw error;
      }
    },
    staleTime: options?.staleTime ?? 1_800_000,
    gcTime: options?.gcTime ?? 1_800_000,
    enabled: (options?.enabled ?? true) && isAuthenticated && !isLoggingOut,
    refetchOnWindowFocus: options?.refetchOnWindowFocus ?? false,
    refetchOnReconnect: options?.refetchOnReconnect ?? false,
  });
}

// Sanitize payload: drop empty strings and undefined
function sanitizePayload(input: UpdateProfileRequest): UpdateProfileRequest {
  const out: UpdateProfileRequest = {};
  if (input.name !== undefined) {
    const v = input.name;
    if (v === null) {
      out.name = null;
    } else if (typeof v === 'string') {
      const t = v.trim();
      if (t.length > 0) out.name = t;
    }
  }
  if (input.avatar !== undefined) {
    // Allow null to clear; base64 string must be non-empty
    if (input.avatar === null) {
      out.avatar = null;
    } else if (typeof input.avatar === 'string') {
      out.avatar = input.avatar;
    }
  }
  if (input.language !== undefined) {
    const v = input.language;
    if (v === null) {
      out.language = null;
    } else if (typeof v === 'string' && v.trim().length > 0) {
      out.language = v;
    }
  }
  if (input.timezone !== undefined) {
    const v = input.timezone;
    if (v === null) {
      out.timezone = null;
    } else if (typeof v === 'string' && v.trim().length > 0) {
      out.timezone = v;
    }
  }
  if (input.mobile !== undefined) {
    const v = input.mobile;
    if (v === null) {
      out.mobile = null;
    } else if (typeof v === 'string' && v.trim().length > 0) {
      out.mobile = v;
    }
  }
  return out;
}

// Update profile mutation with optimistic update and toasts
export function useUpdateProfile() {
  const queryClient = useQueryClient();
  const t = useT('profile');

  return useMutation({
    mutationFn: async (payload: UpdateProfileRequest) => {
      const data = sanitizePayload(payload);
      return accountService.updateProfile(data);
    },
    onMutate: async (payload: UpdateProfileRequest) => {
      await queryClient.cancelQueries({ queryKey: PROFILE_QUERY_KEY });

      const previous = queryClient.getQueryData<User>(PROFILE_QUERY_KEY);

      // Optimistically update store and cache
      const storePrev = useAuthStore.getState().user;
      const optimistic: User | null = storePrev
        ? {
            ...storePrev,
            ...(payload.name !== undefined
              ? { name: payload.name === null ? '' : String(payload.name) }
              : {}),
            ...(payload.timezone !== undefined
              ? { timezone: payload.timezone === null ? '' : String(payload.timezone) }
              : {}),
            ...(payload.avatar !== undefined
              ? {
                  avatar: payload.avatar === null ? null : String(payload.avatar),
                  avatar_url: null,
                }
              : {}),
          }
        : previous || null;

      if (optimistic) {
        queryClient.setQueryData(PROFILE_QUERY_KEY, optimistic);
        useAuthStore.getState().setUser(optimistic);
      }

      return { previous, storePrev } as const;
    },
    onSuccess: async () => {
      clearProfileClientCache();
      // Refresh store and query cache with server truth
      await useAuthStore.getState().refreshProfile();
      await queryClient.invalidateQueries({ queryKey: PROFILE_QUERY_KEY });
      sessionManager.broadcastProfileUpdated();

      toast.success(t('profileUpdated'));
    },
    onError: (_error, _payload, context) => {
      if (context?.previous) {
        queryClient.setQueryData(PROFILE_QUERY_KEY, context.previous);
      }
      if (context?.storePrev) {
        useAuthStore.getState().setUser(context.storePrev);
      }
      toast.error(t('profileUpdateFailed'));
    },
  });
}
