import type { OrganizationMemberRole } from '@/services/types/organization';

export type WorkspaceMemberRole = 'owner' | 'admin' | 'normal';

const ORGANIZATION_ROLE_VALUES = new Set<OrganizationMemberRole>(['owner', 'admin', 'normal']);
const WORKSPACE_ROLE_VALUES = new Set<WorkspaceMemberRole>(['owner', 'admin', 'normal']);

export function normalizeOrganizationRole(
  role?: string | null
): OrganizationMemberRole | null {
  if (!role) return null;
  return ORGANIZATION_ROLE_VALUES.has(role as OrganizationMemberRole)
    ? (role as OrganizationMemberRole)
    : null;
}

export function normalizeWorkspaceMemberRole(role?: string | null): WorkspaceMemberRole | null {
  if (!role) return null;
  return WORKSPACE_ROLE_VALUES.has(role as WorkspaceMemberRole)
    ? (role as WorkspaceMemberRole)
    : null;
}

export function isPrivilegedOrganizationRole(role?: string | null): boolean {
  const normalizedRole = normalizeOrganizationRole(role);
  return normalizedRole === 'owner' || normalizedRole === 'admin';
}
