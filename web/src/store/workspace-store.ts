/**
 * Workspace state store
 * Manages global workspace state for the console
 */
import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { createSelectors } from './utils/selectors';
import type { PermissionCode } from '@/constants/permissions';

/**
 * Workspace entity representing a workspace in the frontend
 */
export interface Workspace {
  id: string;
  name: string;
}

export type WorkspaceContextStatus = 'loading' | 'ready' | 'workspace_required';

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
  // Currently selected workspace (null only while loading or when workspace is required)
  currentWorkspace: Workspace | null;
  // Explicit console workspace context state.
  contextStatus: WorkspaceContextStatus;
  // Legacy compatibility: true means no concrete workspace context is usable.
  isOrganizationMode: boolean;
  // Current permission state
  permissionState: PermissionState;
  // Actions
  setWorkspaces: (workspaces: Workspace[]) => void;
  setCurrentWorkspace: (workspace: Workspace | null) => void;
  setContextStatus: (status: WorkspaceContextStatus) => void;
  setOrganizationMode: (isOrganizationMode: boolean) => void;
  setPermissions: (permissions: PermissionState) => void;
  clearPermissions: () => void;
  // Mark console business features as requiring a workspace.
  markWorkspaceRequired: () => void;
  // Clear stale workspace context while a newly selected organization is loading.
  resetForOrganizationSwitch: () => void;
  // Legacy alias for markWorkspaceRequired.
  enterOrganizationMode: () => void;
  // Switch to a specific workspace (exits organization mode)
  selectWorkspace: (workspace: Workspace) => void;
  // Permission check helpers
  hasPermission: (permission: PermissionCode) => boolean;
  hasAnyPermission: (permissions: readonly PermissionCode[]) => boolean;
  hasAllPermissions: (permissions: readonly PermissionCode[]) => boolean;
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
      contextStatus: 'loading',
      isOrganizationMode: true,
      permissionState: defaultPermissionState,
      _hasHydrated: false,
      setWorkspaces: workspaces => set({ workspaces }),
      setCurrentWorkspace: workspace =>
        set({
          currentWorkspace: workspace,
          contextStatus: workspace ? 'ready' : 'workspace_required',
          isOrganizationMode: !workspace,
        }),
      setContextStatus: contextStatus =>
        set({
          contextStatus,
          isOrganizationMode: contextStatus !== 'ready',
        }),
      setOrganizationMode: isOrganizationMode =>
        set({
          isOrganizationMode,
          contextStatus: isOrganizationMode ? 'workspace_required' : 'ready',
        }),
      setPermissions: permissions => set({ permissionState: permissions }),
      clearPermissions: () => set({ permissionState: defaultPermissionState }),
      setHasHydrated: state => set({ _hasHydrated: state }),
      markWorkspaceRequired: () =>
        set({
          contextStatus: 'workspace_required',
          isOrganizationMode: true,
          currentWorkspace: null,
          permissionState: defaultPermissionState,
        }),
      resetForOrganizationSwitch: () =>
        set({
          workspaces: [],
          currentWorkspace: null,
          contextStatus: 'loading',
          isOrganizationMode: true,
          permissionState: defaultPermissionState,
        }),
      enterOrganizationMode: () => get().markWorkspaceRequired(),
      selectWorkspace: workspace =>
        set({
          contextStatus: 'ready',
          isOrganizationMode: false,
          currentWorkspace: workspace,
          permissionState: defaultPermissionState,
        }),
      // Permission helpers
      hasPermission: (permission: PermissionCode) => {
        const { contextStatus, permissionState } = get();
        if (contextStatus !== 'ready') {
          return false;
        }
        if (
          permissionState.organizationRole === 'owner' ||
          permissionState.organizationRole === 'admin'
        ) {
          return true;
        }
        return permissionState.permissions.includes(permission);
      },
      hasAnyPermission: (permissions: readonly PermissionCode[]) => {
        const { contextStatus, permissionState } = get();
        if (contextStatus !== 'ready') {
          return false;
        }
        if (
          permissionState.organizationRole === 'owner' ||
          permissionState.organizationRole === 'admin'
        ) {
          return permissions.length > 0;
        }
        return permissions.some(p => permissionState.permissions.includes(p));
      },
      hasAllPermissions: (permissions: readonly PermissionCode[]) => {
        const { contextStatus, permissionState } = get();
        if (contextStatus !== 'ready') {
          return false;
        }
        if (
          permissionState.organizationRole === 'owner' ||
          permissionState.organizationRole === 'admin'
        ) {
          return true;
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
        contextStatus: state.contextStatus,
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
export const useWorkspaceContextStatus = () => useWorkspaceStore.use.contextStatus();
export const useIsOrganizationMode = () => useWorkspaceStore.use.isOrganizationMode();
export const usePermissions = () => useWorkspaceStore.use.permissionState();
export const useHasHydrated = () => useWorkspaceStore.use._hasHydrated();
