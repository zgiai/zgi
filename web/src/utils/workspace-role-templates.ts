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

type WorkspaceRoleLocalizedText = NonNullable<Role['name_i18n']>;

function resolveWorkspaceRoleLocalizedText(
  fallback: string,
  localized: WorkspaceRoleLocalizedText | undefined,
  locale: string,
  customized: boolean
) {
  if (customized || !localized) return fallback;

  const localizedMap = localized as Record<string, string | undefined>;
  const localizedValues = Object.values(localizedMap).filter(
    (value): value is string => typeof value === 'string' && value.trim().length > 0
  );

  const localeKey = locale.replace('-', '_');
  return (
    localizedMap[localeKey] ||
    localized.en_US ||
    localized.zh_Hans ||
    localizedValues[0] ||
    fallback
  );
}

export function getWorkspaceRoleDisplayName(role: Role, locale: string) {
  return resolveWorkspaceRoleLocalizedText(
    role.name,
    role.name_i18n,
    locale,
    role.name_customized === true
  );
}

export function getWorkspaceRoleDisplayDescription(role: Role, locale: string) {
  return resolveWorkspaceRoleLocalizedText(
    role.description || '',
    role.description_i18n,
    locale,
    role.description_customized === true
  );
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
    (role.builtin && ['owner', 'admin'].includes(normalizedRoleName(role)))
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
