'use client';

import { useQuery } from '@tanstack/react-query';
import { PROFILE_KEYS } from '@/hooks/query-keys';
import { accountService } from '@/services/account.service';
import type { RuntimeResourceList } from '@/services/account.service';
import { useAuthStore } from '@/store/auth-store';

export function useAccountCapabilities() {
  const isAuthenticated = useAuthStore.use.isAuthenticated();
  const isLoggingOut = useAuthStore.use.isLoggingOut();

  const query = useQuery({
    queryKey: PROFILE_KEYS.capabilities(),
    queryFn: accountService.getCapabilities,
    enabled: isAuthenticated && !isLoggingOut,
    staleTime: 60 * 1000,
    gcTime: 5 * 60 * 1000,
    retry: false,
    refetchOnWindowFocus: false,
  });

  const capabilities = query.data ?? null;
  const isOrganizationAdmin = capabilities?.organization.is_admin ?? false;

  return {
    capabilities,
    isLoading: query.isLoading,
    isFetching: query.isFetching,
    error: query.error,
    refetch: query.refetch,
    canUseOrganizationScope: capabilities?.routes.organization_scope_allowed ?? false,
    canUseWorkspaceScope: capabilities?.routes.workspace_scope_allowed ?? false,
    isWorkspaceRequired: capabilities?.routes.workspace_required ?? true,
    canAccessOrganizationDashboard:
      capabilities?.organization.can_access_dashboard ?? isOrganizationAdmin,
    canManageModelConfig:
      capabilities?.organization.can_manage_model_config ?? isOrganizationAdmin,
    runtimeResourceLists: capabilities?.runtime_resource_lists ?? null,
    canUseRuntimeResourceList: (key: RuntimeResourceList) =>
      capabilities?.runtime_resource_lists[key]?.enabled ?? false,
  };
}
