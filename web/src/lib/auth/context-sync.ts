import { ORGANIZATION_KEYS, WORKSPACE_KEYS } from '@/hooks/query-keys';
import { queryClient } from '@/lib/query-client';
import { useOrganizationStore } from '@/store/organization-store';
import { useWorkspaceStore } from '@/store/workspace-store';

interface AccountContextProfile {
  current_organization_id?: string | null;
  current_workspace_id?: string | null;
}

function normalizeContextID(value: string | null | undefined): string | null {
  if (!value) return null;
  const trimmed = value.trim();
  return trimmed.length > 0 ? trimmed : null;
}

export function syncAccountContextStores(profile: AccountContextProfile | null | undefined): void {
  if (!profile) return;
  if (
    !Object.prototype.hasOwnProperty.call(profile, 'current_organization_id') &&
    !Object.prototype.hasOwnProperty.call(profile, 'current_workspace_id')
  ) {
    return;
  }

  const organizationStore = useOrganizationStore.getState();
  const workspaceStore = useWorkspaceStore.getState();

  const profileOrganizationID = normalizeContextID(profile.current_organization_id);
  const profileWorkspaceID = normalizeContextID(profile.current_workspace_id);
  const storeOrganizationID = normalizeContextID(organizationStore.currentOrganization?.id);
  const storeWorkspaceID = normalizeContextID(workspaceStore.currentWorkspace?.id);

  const organizationChanged =
    storeOrganizationID !== null && storeOrganizationID !== profileOrganizationID;
  const workspaceChanged =
    storeWorkspaceID !== null && storeWorkspaceID !== profileWorkspaceID;

  if (!organizationChanged && !workspaceChanged) {
    return;
  }

  organizationStore.setSwitchingOrganization(false);

  if (organizationChanged) {
    const matchedOrganization =
      profileOrganizationID === null
        ? null
        : (organizationStore.organizations.find(
            organization => organization.id === profileOrganizationID
          ) ?? null);
    organizationStore.setCurrentOrganization(matchedOrganization);
  }

  workspaceStore.resetForOrganizationSwitch();

  void queryClient.cancelQueries({ queryKey: WORKSPACE_KEYS.all });
  queryClient.removeQueries({ queryKey: WORKSPACE_KEYS.all });

  if (organizationChanged) {
    void queryClient.cancelQueries({ queryKey: ORGANIZATION_KEYS.current() });
    queryClient.removeQueries({ queryKey: ORGANIZATION_KEYS.current() });
  }
}
