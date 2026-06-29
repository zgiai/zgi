import type { Role } from '@/services/types/organization';

const FIXED_GOVERNANCE_ROLE_IDS = new Set([
  '00000000-0000-0000-0000-000000000001',
  '00000000-0000-0000-0000-000000000002',
]);

export function isWorkspaceGovernanceRole(role: Role) {
  return (
    Boolean(role.fixed_governance) ||
    role.role_kind === 'governance' ||
    FIXED_GOVERNANCE_ROLE_IDS.has(role.id) ||
    ['owner', 'admin'].includes(role.name.toLowerCase())
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
