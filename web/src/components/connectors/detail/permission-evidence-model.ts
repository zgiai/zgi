import type {
  ConnectorPermissionTemplate,
  ConnectorProviderEvidence,
} from '@/services/types/connector';

export function connectorPermissionRows(
  template?: ConnectorPermissionTemplate,
  evidence?: ConnectorProviderEvidence
) {
  const verified = evidence?.scope_status === 'verified' && Array.isArray(evidence.granted_scopes);
  const granted = new Set(verified ? evidence.granted_scopes : []);
  const tenant = new Set([...(template?.addons?.scopes?.tenant ?? []), ...granted]);
  const user = new Set(template?.addons?.scopes?.user ?? []);
  return [
    ...[...tenant]
      .sort()
      .map(scope => ({
        scope,
        identity: 'tenant' as const,
        status: verified
          ? granted.has(scope)
            ? ('verified' as const)
            : ('not_granted' as const)
          : ('unverified' as const),
      })),
    ...[...user]
      .sort()
      .map(scope => ({ scope, identity: 'user' as const, status: 'unverified' as const })),
  ];
}
