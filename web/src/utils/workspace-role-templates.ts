import type { Role } from '@/services/types/organization';

export const WORKSPACE_BUILTIN_ROLE_OWNER_ID = '00000000-0000-0000-0000-000000000001';
export const WORKSPACE_BUILTIN_ROLE_ADMIN_ID = '00000000-0000-0000-0000-000000000002';

const FIXED_GOVERNANCE_ROLE_IDS = new Set([
  WORKSPACE_BUILTIN_ROLE_OWNER_ID,
  WORKSPACE_BUILTIN_ROLE_ADMIN_ID,
]);

function normalizedRoleName(role: Role) {
  return role.name.toLowerCase();
}

export function isWorkspaceOwnerRole(role: Role) {
  return (
    role.id === WORKSPACE_BUILTIN_ROLE_OWNER_ID ||
    Boolean(role.fixed_governance && normalizedRoleName(role) === 'owner')
  );
}

export function isWorkspaceAdminRole(role: Role) {
  return (
    role.id === WORKSPACE_BUILTIN_ROLE_ADMIN_ID ||
    Boolean(role.fixed_governance && normalizedRoleName(role) === 'admin')
  );
}

export function isWorkspaceGovernanceRole(role: Role) {
  return (
    Boolean(role.fixed_governance) ||
    role.role_kind === 'governance' ||
    FIXED_GOVERNANCE_ROLE_IDS.has(role.id) ||
    ['owner', 'admin'].includes(normalizedRoleName(role))
  );
}

export function isLegacyBuiltinWorkspaceRole(role: Role) {
  return role.role_kind === 'legacy_builtin' || (role.builtin && !isWorkspaceGovernanceRole(role));
}

export function isSelectableWorkspacePermissionTemplate(role: Role) {
  return (
    role.status === 'active' &&
    role.applicable !== false &&
    !isWorkspaceGovernanceRole(role) &&
    !isLegacyBuiltinWorkspaceRole(role)
  );
}

export function isAssignableWorkspaceAdminRole(role: Role) {
  return role.status === 'active' && isWorkspaceAdminRole(role);
}

export function isAssignableWorkspaceMemberRole(role: Role) {
  return (
    role.status === 'active' &&
    !isWorkspaceOwnerRole(role) &&
    (isWorkspaceAdminRole(role) || isSelectableWorkspacePermissionTemplate(role))
  );
}

export function workspaceMemberRoleForAssignableRole(role: Role): 'admin' | 'normal' {
  return isWorkspaceAdminRole(role) ? 'admin' : 'normal';
}
