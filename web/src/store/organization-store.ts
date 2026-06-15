import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { createSelectors } from './utils/selectors';
import type { Organization } from '@/services/types/organization';

interface OrganizationState {
  organizations: Organization[];
  currentOrganization: Organization | null;
  isSwitchingOrganization: boolean;
  setOrganizations: (organizations: Organization[]) => void;
  setCurrentOrganization: (organization: Organization | null) => void;
  setSwitchingOrganization: (isSwitching: boolean) => void;
}

const useOrganizationStoreBase = create<OrganizationState>()(
  persist(
    (set, _get) => ({
      organizations: [],
      currentOrganization: null,
      isSwitchingOrganization: false,
      setOrganizations: organizations => set({ organizations }),
      setCurrentOrganization: organization => set({ currentOrganization: organization }),
      setSwitchingOrganization: isSwitching =>
        set({ isSwitchingOrganization: isSwitching }),
    }),
    {
      name: 'organization-storage',
      partialize: state => ({
        currentOrganization: state.currentOrganization,
      }),
    }
  )
);

export const useOrganizationStore = createSelectors(useOrganizationStoreBase);
