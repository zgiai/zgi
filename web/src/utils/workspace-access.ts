import type { User } from '@/services/types/auth';

export function canManageOrganizationWorkspaces(user?: User | null): boolean {
  const accountRoleType = user?.account_role?.role_type;

  return Boolean(
    user?.is_super_admin ||
      user?.organization_role === 'owner' ||
      user?.organization_role === 'admin' ||
      accountRoleType === 'super_admin' ||
      accountRoleType === 'system_admin'
  );
}
