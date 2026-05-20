import { queryClient } from '@/lib/query-client';
import { clearAuthClientCaches } from '@/utils/client-cache';
import { useOrganizationStore } from '@/store/organization-store';
import { useWorkspaceStore } from '@/store/workspace-store';

export function resetTenantContextState(): void {
  if (typeof window !== 'undefined') {
    try {
      window.localStorage.removeItem('workspace-storage');
      window.localStorage.removeItem('organization-storage');
    } catch {
      // Ignore storage cleanup errors.
    }
  }

  const workspaceStore = useWorkspaceStore.getState();
  workspaceStore.setWorkspaces([]);
  workspaceStore.enterOrganizationMode();

  const organizationStore = useOrganizationStore.getState();
  organizationStore.setOrganizations([]);
  organizationStore.setCurrentOrganization(null);
}

export async function clearSessionBoundClientState(): Promise<void> {
  queryClient.clear();
  clearAuthClientCaches();
  resetTenantContextState();
}
