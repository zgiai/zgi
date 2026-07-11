import type { Organization } from '@/services/types/organization';

export function getOrganizationDisplayName(
  organization?: Pick<Organization, 'name' | 'short_name'> | null
) {
  if (!organization) return '';
  return (organization.short_name || organization.name || '').trim();
}
