import { useMemo } from 'react';
import { useWorkspaceStore } from '@/store/workspace-store';

/**
 * Hook to detect workspace mismatch between current workspace and target resource's workspace.
 * Used to conditionally disable sidebar navigation when accessing resources from different workspaces.
 *
 * @param targetWorkspaceId - The workspace ID that the resource belongs to
 * @returns Object containing isMismatch boolean
 */
export function useWorkspaceMismatch(targetWorkspaceId: string) {
  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const isOrganizationMode = useWorkspaceStore.use.isOrganizationMode();

  const isMismatch = useMemo(() => {
    // No target workspace ID means we can't determine mismatch yet (loading state)
    if (!targetWorkspaceId) return false;

    // In organization view, always show sidebar - no mismatch
    // The WorkspaceMismatchGuard handles content access control separately
    if (isOrganizationMode) {
      return false;
    }

    // In workspace mode, check if current workspace matches target
    if (currentWorkspace?.id === targetWorkspaceId) return false;

    // Workspace doesn't match
    return true;
  }, [targetWorkspaceId, isOrganizationMode, currentWorkspace?.id]);

  return { isMismatch };
}
