import type { IntegrationCatalogItem, IntegrationConnection } from '@/services/types/integration';
import { actionSupportsAuthMethod } from './action-auth-compatibility';

function normalizeScope(value: string): string {
  return value.trim().toLowerCase();
}

/**
 * Maps a provider-reported scope gap back to explicit catalog actions. This
 * preserves least privilege: an upgrade never requests every provider action.
 *
 * `undefined` means this is a normal reconnect. `null` means an upgrade is
 * required but the current catalog cannot safely explain which action needs it.
 */
export function resolveOAuthScopeUpgradeActionIDs(
  connection: Pick<
    IntegrationConnection,
    'attention_code' | 'missing_required_scopes' | 'auth_method_id'
  >,
  provider: Pick<IntegrationCatalogItem, 'actions'> | null | undefined
): string[] | null | undefined {
  if (connection.attention_code !== 'scope_update_required') return undefined;

  const missingScopes = new Set(
    (connection.missing_required_scopes ?? []).map(normalizeScope).filter(Boolean)
  );
  if (missingScopes.size === 0) return null;

  const actionIDs = (provider?.actions ?? [])
    .filter(action => actionSupportsAuthMethod(action, connection.auth_method_id))
    .filter(action => {
      const relevantScopes = [
        ...(action.required_scopes ?? []),
        ...(action.required_any_scopes ?? []),
        ...(action.preferred_scopes ?? []),
      ];
      return relevantScopes.some(scope => missingScopes.has(normalizeScope(scope)));
    })
    .map(action => action.id.trim())
    .filter(Boolean);

  const uniqueActionIDs = [...new Set(actionIDs)];
  return uniqueActionIDs.length > 0 ? uniqueActionIDs : null;
}
