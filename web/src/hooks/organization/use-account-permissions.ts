'use client';

import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { workspaceService } from '@/services/workspace.service';
import { toast } from 'sonner';
import { useT } from '@/i18n';
import { getErrorMessage } from '@/utils/error-notifications';
import { useWorkspaceStore } from '@/store/workspace-store';
import type { PermissionState } from '@/store/workspace-store';
import { useOrganizationStore } from '@/store/organization-store';
import { WORKSPACE_KEYS } from '@/hooks/query-keys';
import type { PermissionCode } from '@/constants/permissions';
import {
  observePermissionLoad,
  shouldRetryAccessLoadError,
  shouldShowAccessLoadToast,
} from '@/utils/access-load-error';
import { captureEvent } from '@/lib/observability';
import { useAuthStore } from '@/store/auth-store';

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
  const contextStatus = useWorkspaceStore.use.contextStatus();
  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const isSwitchingOrganization = useOrganizationStore.use.isSwitchingOrganization();

  // Determine effective workspace ID
  const effectiveWorkspaceId =
    workspaceId === 'current' && currentWorkspace ? currentWorkspace.id : workspaceId;

  const isWorkspaceRequired = contextStatus === 'workspace_required';
  const isWorkspaceLoading = contextStatus === 'loading';
  const isMissingCurrentWorkspace = workspaceId === 'current' && !currentWorkspace;
  // Skip query when no workspace context is usable.
  const shouldSkip =
    isSwitchingOrganization ||
    (skipInOrgMode && (isWorkspaceRequired || isWorkspaceLoading || isMissingCurrentWorkspace));

  const {
    data: permissionsData,
    isLoading,
    isFetching,
    error,
    refetch,
  } = useQuery({
    queryKey: WORKSPACE_KEYS.permissions(organizationId, effectiveWorkspaceId, accountId),
    queryFn: async () => {
      return await observePermissionLoad(
        () =>
          workspaceService.getAccountPermissions(organizationId, effectiveWorkspaceId, accountId),
        diagnostic => {
          const attributes = {
            ...diagnostic,
            session_status: useAuthStore.getState().sessionStatus,
            workspace_status: useWorkspaceStore.getState().contextStatus,
            switching_organization: useOrganizationStore.getState().isSwitchingOrganization,
          };
          // Keep a local diagnostic even when no external reporter is configured.
          // Never pass the raw error, credentials, response body or account IDs.
          console.warn('workspace.permissions.load_failed', attributes);
          captureEvent('workspace.permissions.load_failed', {
            level: 'warning',
            attributes,
          });
        }
      );
    },
    enabled: !shouldSkip && effectiveWorkspaceId !== 'current',
    staleTime: 2 * 60 * 1000,
    gcTime: 5 * 60 * 1000,
    refetchOnWindowFocus: false,
    retry: (failureCount, queryError) =>
      shouldRetryAccessLoadError(failureCount, queryError),
    retryDelay: attemptIndex => Math.min(250 * 2 ** attemptIndex, 1000),
  });

  const hasUsableWorkspaceContext =
    !shouldSkip && contextStatus === 'ready' && effectiveWorkspaceId !== 'current';

  // Sync permissions to store
  useEffect(() => {
    if (!syncToStore) return;

    if (shouldSkip) {
      clearPermissions();
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
  }, [permissionsData, syncToStore, shouldSkip, setPermissions, clearPermissions]);

  // Show error toast if query fails
  useEffect(() => {
    if (!error) return;
    if (isSwitchingOrganization || !hasUsableWorkspaceContext) return;
    if (!shouldShowAccessLoadToast(error)) return;
    toast.error(getErrorMessage(error) || t('switchWorkspace'), {
      id: 'workspace-permissions-load-error',
    });
  }, [error, t, isSwitchingOrganization, hasUsableWorkspaceContext]);

  return {
    permissions: hasUsableWorkspaceContext ? (permissionsData?.permissions ?? []) : [],
    organizationRole: hasUsableWorkspaceContext
      ? (permissionsData?.organization_role ?? null)
      : null,
    workspaceRole: hasUsableWorkspaceContext ? (permissionsData?.workspace_role ?? null) : null,
    workspaceRoleName: hasUsableWorkspaceContext
      ? (permissionsData?.workspace_role_name ?? null)
      : null,
    isLoading,
    isFetching,
    error: error ? getErrorMessage(error) : null,
    refetch,
    // Helper functions with type-safe permission codes
    hasPermission: (permission: PermissionCode) => {
      if (!hasUsableWorkspaceContext) {
        return false;
      }
      const gRole = permissionsData?.organization_role ?? null;
      if (gRole === 'owner' || gRole === 'admin') {
        return true;
      }
      return permissionsData?.permissions.includes(permission) ?? false;
    },
    hasAnyPermission: (permissions: readonly PermissionCode[]) => {
      if (!hasUsableWorkspaceContext) {
        return false;
      }
      const gRole = permissionsData?.organization_role ?? null;
      if (gRole === 'owner' || gRole === 'admin') {
        return permissions.length > 0;
      }
      return permissions.some(p => permissionsData?.permissions.includes(p) ?? false);
    },
    hasAllPermissions: (permissions: readonly PermissionCode[]) => {
      if (!hasUsableWorkspaceContext) {
        return false;
      }
      const gRole = permissionsData?.organization_role ?? null;
      if (gRole === 'owner' || gRole === 'admin') {
        return true;
      }
      return permissions.every(p => permissionsData?.permissions.includes(p) ?? false);
    },
    isAdmin: () => {
      const gRole = hasUsableWorkspaceContext ? (permissionsData?.organization_role ?? null) : null;
      return gRole === 'owner' || gRole === 'admin';
    },
    hasWorkspaceAccess: () => hasUsableWorkspaceContext,
    isWorkspaceManager: () => {
      const gRole = hasUsableWorkspaceContext ? (permissionsData?.organization_role ?? null) : null;
      if (gRole === 'owner' || gRole === 'admin') {
        return true;
      }
      const wRole = hasUsableWorkspaceContext ? (permissionsData?.workspace_role ?? null) : null;
      return wRole === 'owner' || wRole === 'admin';
    },
  };
}
