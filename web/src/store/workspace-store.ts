/**
 * Workspace state store
 * Manages global workspace state for the console
 */
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { createSelectors } from './utils/selectors';
import { ALL_PERMISSION_CODES, type PermissionCode } from '@/constants/permissions';

/**
 * Workspace entity representing a workspace in the frontend
 */
export interface Workspace {
  id: string;
  name: string;
}

/**
 * Permission state for current context
 */
export interface PermissionState {
  /** User's role in the organization (formerly enterprise group) */
  organizationRole: 'owner' | 'admin' | 'normal' | null;
  /** User's role in the current workspace */
  workspaceRole: string | null;
  /** Human-readable workspace role name */
  workspaceRoleName: string | null;
  /** List of permission strings */
  permissions: string[];
}

interface WorkspaceState {
  // Available workspaces list
  workspaces: Workspace[];
  // Currently selected workspace (null when in organization mode or no workspace selected)
  currentWorkspace: Workspace | null;
  // Organization view: true = no concrete workspace context selected
  // In this mode, non-admin users have no edit permissions
  isOrganizationMode: boolean;
  // Current permission state
  permissionState: PermissionState;
  // Actions
  setWorkspaces: (workspaces: Workspace[]) => void;
  setCurrentWorkspace: (workspace: Workspace | null) => void;
  setOrganizationMode: (isOrganizationMode: boolean) => void;
  setPermissions: (permissions: PermissionState) => void;
  clearPermissions: () => void;
  // Switch to organization mode (clears current workspace)
  enterOrganizationMode: () => void;
  // Switch to a specific workspace (exits organization mode)
  selectWorkspace: (workspace: Workspace) => void;
  // Permission check helpers
  hasPermission: (permission: PermissionCode) => boolean;
  hasAnyPermission: (permissions: PermissionCode[]) => boolean;
  hasAllPermissions: (permissions: PermissionCode[]) => boolean;
  isAdmin: () => boolean;
  // Hydration state
  _hasHydrated: boolean;
  setHasHydrated: (state: boolean) => void;
}

const defaultPermissionState: PermissionState = {
  organizationRole: null,
  workspaceRole: null,
  workspaceRoleName: null,
  permissions: [],
};

const useWorkspaceStoreBase = create<WorkspaceState>()(
  persist(
    (set, get) => ({
      workspaces: [],
      currentWorkspace: null,
      isOrganizationMode: true,
      permissionState: defaultPermissionState,
      _hasHydrated: false,
      setWorkspaces: workspaces => set({ workspaces }),
      setCurrentWorkspace: workspace => set({ currentWorkspace: workspace }),
      setOrganizationMode: isOrganizationMode => set({ isOrganizationMode }),
      setPermissions: permissions => set({ permissionState: permissions }),
      clearPermissions: () => set({ permissionState: defaultPermissionState }),
      setHasHydrated: state => set({ _hasHydrated: state }),
      enterOrganizationMode: () =>
        set({
          isOrganizationMode: true,
          currentWorkspace: null,
          permissionState: defaultPermissionState,
        }),
      selectWorkspace: workspace =>
        set({
          isOrganizationMode: false,
          currentWorkspace: workspace,
          permissionState: defaultPermissionState,
        }),
      // Permission helpers
      hasPermission: (permission: PermissionCode) => {
        const { isOrganizationMode, permissionState } = get();
        if (isOrganizationMode) {
          const { organizationRole } = permissionState;
          if (organizationRole === 'owner' || organizationRole === 'admin') {
            return true;
          }
          return permission.endsWith('.view');
        }
        return permissionState.permissions.includes(permission);
      },
      hasAnyPermission: (permissions: PermissionCode[]) => {
        const { isOrganizationMode, permissionState } = get();
        if (isOrganizationMode) {
          const { organizationRole } = permissionState;
          if (organizationRole === 'owner' || organizationRole === 'admin') {
            return permissions.length > 0;
          }
          return permissions.some(p => p.endsWith('.view'));
        }
        return permissions.some(p => permissionState.permissions.includes(p));
      },
      hasAllPermissions: (permissions: PermissionCode[]) => {
        const { isOrganizationMode, permissionState } = get();
        if (isOrganizationMode) {
          const { organizationRole } = permissionState;
          if (organizationRole === 'owner' || organizationRole === 'admin') {
            return permissions.every(p => ALL_PERMISSION_CODES.includes(p));
          }
          return permissions.every(p => p.endsWith('.view'));
        }
        return permissions.every(p => permissionState.permissions.includes(p));
      },
      isAdmin: () => {
        const { permissionState } = get();
        return (
          permissionState.organizationRole === 'owner' ||
          permissionState.organizationRole === 'admin'
        );
      },
    }),
    {
      name: 'workspace-storage',
      partialize: state => ({
        currentWorkspace: state.currentWorkspace,
        isOrganizationMode: state.isOrganizationMode,
        // Note: permissions are NOT persisted - they should be refetched
      }),
      onRehydrateStorage: () => state => {
        state?.setHasHydrated(true);
      },
    }
  )
);

export const useWorkspaceStore = createSelectors(useWorkspaceStoreBase);

// Convenience hooks
export const useCurrentWorkspace = () => useWorkspaceStore.use.currentWorkspace();
export const useWorkspaces = () => useWorkspaceStore.use.workspaces();
export const useIsOrganizationMode = () => useWorkspaceStore.use.isOrganizationMode();
export const usePermissions = () => useWorkspaceStore.use.permissionState();
export const useHasHydrated = () => useWorkspaceStore.use._hasHydrated();
