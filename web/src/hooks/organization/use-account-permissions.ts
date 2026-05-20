'use client';

import { useEffect, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { workspaceService } from '@/services/workspace.service';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { useWorkspaceStore } from '@/store/workspace-store';
import type { PermissionState } from '@/store/workspace-store';
import { WORKSPACE_KEYS } from '@/hooks/query-keys';
import { ALL_PERMISSION_CODES, type PermissionCode } from '@/constants/permissions';
import { useAuthStore } from '@/store';

interface UseAccountPermissionsOptions {
  /** Organization ID, defaults to 'current' */
  organizationId?: string;
  /** Workspace ID, defaults to 'current' */
  workspaceId?: string;
  /** Account ID, defaults to 'current' */
  accountId?: string;
  /** If true, automatically sync to workspace store */
  syncToStore?: boolean;
  /** If true, skip fetching when in org mode */
  skipInOrgMode?: boolean;
}

/**
 * Hook for fetching account permissions in the current workspace context
 * Uses /console/api/organizations/{organization_id}/workspaces/{workspace_id}/accounts/{account_id}/permissions
 */
export function useAccountPermissions(options: UseAccountPermissionsOptions = {}) {
  const {
    organizationId = 'current',
    workspaceId = 'current',
    accountId = 'current',
    syncToStore = true,
    skipInOrgMode = true,
  } = options;

  const t = useT('navigation');
  const setPermissions = useWorkspaceStore.use.setPermissions();
  const clearPermissions = useWorkspaceStore.use.clearPermissions();
  const isOrganizationMode = useWorkspaceStore.use.isOrganizationMode();
  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const user = useAuthStore.use.user();

  // Determining role based on profile (user object) instead of currentOrganization
  const organizationRoleFromProfile = user?.organization_role || null;
  const isOrgAdmin =
    organizationRoleFromProfile === 'owner' || organizationRoleFromProfile === 'admin';
  const organizationViewPermissions = useMemo(
    () =>
      isOrgAdmin
        ? [...ALL_PERMISSION_CODES]
        : ALL_PERMISSION_CODES.filter(permission => permission.endsWith('.view')),
    [isOrgAdmin]
  );

  // Determine effective workspace ID
  const effectiveWorkspaceId =
    workspaceId === 'current' && currentWorkspace ? currentWorkspace.id : workspaceId;

  // Skip query when in org mode (no workspace context)
  const shouldSkip = skipInOrgMode && isOrganizationMode;

  const {
    data: permissionsData,
    isLoading,
    isFetching,
    error,
    refetch,
  } = useQuery({
    queryKey: WORKSPACE_KEYS.permissions(organizationId, effectiveWorkspaceId, accountId),
    queryFn: async () => {
      return await workspaceService.getAccountPermissions(
        organizationId,
        effectiveWorkspaceId,
        accountId
      );
    },
    enabled: !shouldSkip && effectiveWorkspaceId !== 'current',
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: false,
  });

  // Sync permissions to store
  useEffect(() => {
    if (!syncToStore) return;

    if (shouldSkip) {
      if (isOrganizationMode) {
        // Sync organization role and derived permissions in organization view.
        setPermissions({
          organizationRole: organizationRoleFromProfile as PermissionState['organizationRole'],
          workspaceRole: null,
          workspaceRoleName: null,
          permissions: organizationViewPermissions,
        });
      } else {
        clearPermissions();
      }
      return;
    }

    if (permissionsData) {
      const permissionState: PermissionState = {
        organizationRole: permissionsData.organization_role ?? null,
        workspaceRole: permissionsData.workspace_role ?? null,
        workspaceRoleName: permissionsData.workspace_role_name ?? null,
        permissions: permissionsData.permissions || [],
      };
      setPermissions(permissionState);
    }
  }, [
    permissionsData,
    syncToStore,
    shouldSkip,
    setPermissions,
    clearPermissions,
    isOrganizationMode,
    organizationRoleFromProfile,
    organizationViewPermissions,
  ]);

  // Show error toast if query fails
  useEffect(() => {
    if (!error) return;
    toast.error(getErrorMessage(error) || t('switchWorkspace'));
  }, [error, t]);

  return {
    permissions: isOrganizationMode
      ? organizationViewPermissions
      : (permissionsData?.permissions ?? []),
    organizationRole: isOrganizationMode
      ? organizationRoleFromProfile
      : (permissionsData?.organization_role ?? null),
    workspaceRole: permissionsData?.workspace_role ?? null,
    workspaceRoleName: permissionsData?.workspace_role_name ?? null,
    isLoading,
    isFetching,
    error: error ? getErrorMessage(error) : null,
    refetch,
    // Helper functions with type-safe permission codes
    hasPermission: (permission: PermissionCode) => {
      if (isOrganizationMode) {
        if (isOrgAdmin) {
          return true;
        }
        return permission.endsWith('.view');
      }
      return permissionsData?.permissions.includes(permission) ?? false;
    },
    hasAnyPermission: (permissions: PermissionCode[]) => {
      if (isOrganizationMode) {
        if (isOrgAdmin) {
          return permissions.length > 0;
        }
        return permissions.some(p => p.endsWith('.view'));
      }
      return permissions.some(p => permissionsData?.permissions.includes(p) ?? false);
    },
    hasAllPermissions: (permissions: PermissionCode[]) => {
      if (isOrganizationMode) {
        if (isOrgAdmin) {
          return permissions.every(p => ALL_PERMISSION_CODES.includes(p));
        }
        return permissions.every(p => p.endsWith('.view'));
      }
      return permissions.every(p => permissionsData?.permissions.includes(p) ?? false);
    },
    isAdmin: () => {
      const gRole = isOrganizationMode
        ? organizationRoleFromProfile
        : (permissionsData?.organization_role ?? null);
      return gRole === 'owner' || gRole === 'admin';
    },
  };
}
