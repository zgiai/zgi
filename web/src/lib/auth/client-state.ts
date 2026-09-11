import { queryClient } from '@/lib/query-client';
import { clearAuthClientCaches } from '@/utils/client-cache';
import { useOrganizationStore } from '@/store/organization-store';
import { useWorkspaceStore } from '@/store/workspace-store';
import { SYSTEM_KEYS } from '@/hooks/query-keys';

export function clearSessionBoundQueryState(): void {
  // This unauthenticated configuration is shared by login pages. Removing an
  // observed query on a late sign-out event can strand the login form's observer.
  // Preserve only this exact key, not the system settings or other private data.
  const publicFeatures = queryClient.getQueryCache().find({
    queryKey: SYSTEM_KEYS.features(),
    exact: true,
  });
  queryClient.removeQueries({ predicate: query => query !== publicFeatures });
  queryClient.getMutationCache().clear();
}

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
  organizationStore.setSwitchingOrganization(false);
  organizationStore.setOrganizations([]);
  organizationStore.setCurrentOrganization(null);
}

export async function clearSessionBoundClientState(): Promise<void> {
  clearSessionBoundQueryState();
  clearAuthClientCaches();
  resetTenantContextState();
}
